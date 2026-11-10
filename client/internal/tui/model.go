// Package tui is the Bubble Tea terminal UI.
package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/api"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/clip"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/kaomoji"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/links"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

type pane int

const (
	paneInput pane = iota
	paneMain
	paneThread
)

type overlay int

const (
	overlayNone overlay = iota
	overlayPicker
	overlayProfile
)

const (
	typingEvery = 2 * time.Second
	pickerCols  = 5
)

// tea messages
type (
	eventMsg struct {
		conn link
		ev   proto.Event
	}
	connClosedMsg  struct{ conn link }
	reconnectedMsg struct{ conn link }
	tickMsg        time.Time
)

// link is the connection surface the UI needs (satisfied by *api.Conn; fakeable in tests).
type link interface {
	Send(v any) error
	Close()
	Events() <-chan proto.Event
}

type pendingSend struct {
	conv, parent, text string
	names              []string
	retried            bool
}

type Model struct {
	ctx       context.Context
	conn      link
	st        *chat.State
	kp        e2e.KeyPair
	reconnect func(context.Context) (*api.Conn, error)

	w, h  int
	input textarea.Model

	pane               pane
	selMain, selThread int
	threadRoot         string

	overlay     overlay
	pickIdx     int
	profileUser string

	status       string
	statusUntil  time.Time
	reconnecting bool
	lastTyping   time.Time
	refN         int
	pending      map[string]*pendingSend
	retry        []*pendingSend
}

// Run starts the UI and blocks until the user quits.
func Run(ctx context.Context, conn *api.Conn, st *chat.State, kp e2e.KeyPair, reconnect func(context.Context) (*api.Conn, error)) error {
	zone.NewGlobal()
	defer zone.Close()
	m := NewModel(ctx, conn, st, kp, reconnect)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	_, err := p.Run()
	m.conn.Close()
	return err
}

func NewModel(ctx context.Context, conn link, st *chat.State, kp e2e.KeyPair, reconnect func(context.Context) (*api.Conn, error)) *Model {
	ta := textarea.New()
	ta.Prompt = "› "
	ta.ShowLineNumbers = false
	ta.CharLimit = maxInputRune
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	ta.Placeholder = "Message… (/i kaomoji, /whois @user, /name <name>, /quit)"
	ta.Focus()
	return &Model{ctx: ctx, conn: conn, st: st, kp: kp, reconnect: reconnect, input: ta,
		selMain: -1, selThread: -1, pending: map[string]*pendingSend{}, w: 0, h: 0}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, waitEvent(m.conn), tick())
}

func tick() tea.Cmd { return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) }) }

func waitEvent(c link) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-c.Events()
		if !ok {
			return connClosedMsg{c}
		}
		return eventMsg{c, ev}
	}
}

// backoff returns the reconnect delay for the nth failed attempt: 1,2,4,…,30 s.
func backoff(attempt int) time.Duration {
	d := time.Second << uint(attempt)
	if attempt > 5 || d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

func (m *Model) doReconnect() tea.Cmd {
	ctx, rc := m.ctx, m.reconnect
	return func() tea.Msg {
		for attempt := 0; ; attempt++ {
			if ctx.Err() != nil || rc == nil {
				return tea.Quit()
			}
			c, err := rc(ctx)
			if err == nil {
				return reconnectedMsg{c}
			}
			select {
			case <-ctx.Done():
				return tea.Quit()
			case <-time.After(backoff(attempt)):
			}
		}
	}
}

func (m *Model) setStatus(s string) {
	m.status = s
	m.statusUntil = time.Now().Add(6 * time.Second)
}

func (m *Model) layout() (sideW, chatW, threadW int) {
	sideW = 24
	if m.w < 70 {
		sideW = 0
	}
	rest := m.w - sideW
	if m.threadRoot != "" {
		threadW = rest * 40 / 100
		if threadW < 30 {
			threadW = minInt(30, rest/2)
		}
	}
	chatW = rest - threadW
	return
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *Model) resize() {
	_, cw, _ := m.layout()
	m.input.SetWidth(maxInt(10, cw-4))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
		return m, nil
	case tickMsg:
		return m, tick()
	case eventMsg:
		if msg.conn != m.conn {
			return m, nil
		}
		return m, tea.Batch(m.onEvent(msg.ev), waitEvent(m.conn))
	case connClosedMsg:
		if msg.conn != m.conn {
			return m, nil
		}
		m.reconnecting = true
		return m, m.doReconnect()
	case reconnectedMsg:
		m.conn = msg.conn
		m.reconnecting = false
		m.pending = map[string]*pendingSend{}
		m.setStatus("Reconnected.")
		return m, waitEvent(m.conn)
	case tea.KeyMsg:
		return m.onKey(msg)
	case tea.MouseMsg:
		return m.onMouse(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func errorText(code string) string {
	switch code {
	case "bad_conv":
		return "That conversation isn't available."
	case "bad_parent":
		return "Can't reply: that thread's root has expired."
	case "too_large":
		return "Message is too large."
	case "not_owner":
		return "Only the sender can keep a message."
	case "not_found":
		return "That message no longer exists."
	case "invalid_display_name":
		return "Display name must be 1-32 characters."
	case "recipients_mismatch":
		return "Someone joined while you were sending. Please send again."
	default:
		return "Server error: " + code
	}
}

func (m *Model) onEvent(ev proto.Event) tea.Cmd {
	var cmds []tea.Cmd
	fx := m.st.Apply(ev)
	if fx.Bell {
		cmds = append(cmds, func() tea.Msg { _, _ = os.Stdout.WriteString("\a"); return nil })
	}
	switch ev.T {
	case "hello", "user":
		m.flushRetries()
	case "error":
		p := m.pending[ev.Ref]
		delete(m.pending, ev.Ref)
		if ev.Error == "recipients_mismatch" && p != nil && !p.retried {
			p.retried = true
			if !sameNames(p.names, m.recipientNames(p.conv)) {
				m.send(p) // our user list already changed: resend right away
			} else {
				// refresh users via a fresh hello snapshot, then retry once
				m.retry = append(m.retry, p)
				m.conn.Close()
			}
			break
		}
		m.setStatus(errorText(ev.Error))
	}
	m.clampSel()
	return tea.Batch(cmds...)
}

func (m *Model) recipientNames(conv string) []string {
	var n []string
	for k := range m.st.Recipients(conv) {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m *Model) flushRetries() {
	r := m.retry
	m.retry = nil
	for _, p := range r {
		m.send(p)
	}
}

// send seals and transmits p. It returns false (with a status) when nothing was sent.
func (m *Model) send(p *pendingSend) bool {
	rec := m.st.Recipients(p.conv)
	if strings.HasPrefix(p.conv, "dm:") && len(rec) < 2 {
		m.setStatus("Can't message that user yet: no public key.")
		return false
	}
	s, err := e2e.Seal(p.text, m.kp, rec)
	if err != nil {
		m.setStatus("Encryption failed.")
		return false
	}
	m.refN++
	ref := fmt.Sprintf("r%d", m.refN)
	if err := m.conn.Send(proto.NewSend(ref, p.conv, p.parent, s.Nonce, s.CT, s.Keys)); err != nil {
		m.setStatus("Not connected. Reconnecting…")
		return false
	}
	p.names = m.recipientNames(p.conv)
	m.pending[ref] = p
	return true
}

func (m *Model) convs() []string {
	var c []string
	for _, e := range m.st.Sidebar() {
		c = append(c, e.Conv)
	}
	return c
}

func (m *Model) switchConv(conv string) {
	m.st.SetActive(conv)
	m.threadRoot = ""
	m.selMain, m.selThread = -1, -1
	m.pane = paneInput
	m.input.Focus()
	m.resize()
}

func (m *Model) cycleConv(d int) {
	cs := m.convs()
	for i, c := range cs {
		if c == m.st.Active() {
			m.switchConv(cs[(i+d+len(cs))%len(cs)])
			return
		}
	}
}

func (m *Model) threadItems() (root *chat.Msg, items []chat.Msg) {
	if m.threadRoot == "" {
		return nil, nil
	}
	root, replies := m.st.Thread(m.threadRoot)
	if root.Text != chat.ExpiredText || root.Sender != "" {
		items = append(items, *root)
	}
	items = append(items, replies...)
	return root, items
}

func (m *Model) clampSel() {
	tl := m.st.Timeline(m.st.Active())
	if m.selMain >= len(tl) {
		m.selMain = len(tl) - 1
	}
	_, it := m.threadItems()
	if m.selThread >= len(it) {
		m.selThread = len(it) - 1
	}
}

func (m *Model) selectedMsg() (chat.Msg, bool) {
	switch m.pane {
	case paneMain:
		tl := m.st.Timeline(m.st.Active())
		if m.selMain >= 0 && m.selMain < len(tl) {
			return tl[m.selMain], true
		}
	case paneThread:
		_, it := m.threadItems()
		if m.selThread >= 0 && m.selThread < len(it) {
			return it[m.selThread], true
		}
	}
	return chat.Msg{}, false
}

func (m *Model) openThread(msg chat.Msg) {
	id := msg.ID
	if msg.ParentID != "" {
		id = msg.ParentID
	}
	m.threadRoot = id
	m.selThread = -1
	m.resize()
}

func (m *Model) toggleKeep(msg chat.Msg) {
	if msg.Sender != m.st.Me() {
		m.setStatus(errorText("not_owner"))
		return
	}
	if err := m.conn.Send(proto.NewKeep(msg.ID, !msg.Kept)); err != nil {
		m.setStatus("Not connected. Reconnecting…")
	}
}

func (m *Model) copyLink(msg chat.Msg) {
	ls := links.Find(msg.Text)
	if len(ls) == 0 {
		m.setStatus("No link in this message.")
		return
	}
	if err := clip.Copy(ls[0]); err != nil {
		m.setStatus("Couldn't copy: " + err.Error())
		return
	}
	m.setStatus("Link copied.")
}

func (m *Model) onKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == keyQuit {
		return m, tea.Quit
	}
	switch m.overlay {
	case overlayPicker:
		return m.pickerKey(s)
	case overlayProfile:
		m.overlay = overlayNone
		return m, nil
	}
	switch s {
	case keyNextConv:
		m.cycleConv(1)
		return m, nil
	case keyPrevConv:
		m.cycleConv(-1)
		return m, nil
	case keyFocus:
		m.cycleFocus()
		return m, nil
	case keyEsc:
		switch {
		case m.threadRoot != "":
			m.threadRoot = ""
			m.resize()
		}
		m.pane = paneInput
		m.input.Focus()
		return m, nil
	}
	if m.pane != paneInput {
		return m.listKey(s)
	}
	if s == keyEnter {
		return m, m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	if v := m.input.Value(); v != "" && !strings.HasPrefix(v, "/") && time.Since(m.lastTyping) >= typingEvery {
		m.lastTyping = time.Now()
		_ = m.conn.Send(proto.NewTyping(m.st.Active()))
	}
	return m, cmd
}

func (m *Model) cycleFocus() {
	next := map[pane]pane{paneInput: paneMain, paneMain: paneInput, paneThread: paneInput}
	if m.threadRoot != "" {
		next = map[pane]pane{paneInput: paneMain, paneMain: paneThread, paneThread: paneInput}
	}
	m.pane = next[m.pane]
	if m.pane == paneInput {
		m.input.Focus()
		return
	}
	m.input.Blur()
	if m.pane == paneMain {
		m.selMain = len(m.st.Timeline(m.st.Active())) - 1
	} else {
		_, it := m.threadItems()
		m.selThread = len(it) - 1
	}
}

func (m *Model) listKey(s string) (tea.Model, tea.Cmd) {
	sel := &m.selMain
	n := len(m.st.Timeline(m.st.Active()))
	if m.pane == paneThread {
		sel = &m.selThread
		_, it := m.threadItems()
		n = len(it)
	}
	switch s {
	case keyUp, "k":
		if *sel > 0 {
			*sel--
		}
	case keyDown, "j":
		if *sel < n-1 {
			*sel++
		}
	case keyCopyLink:
		if msg, ok := m.selectedMsg(); ok {
			m.copyLink(msg)
		}
	case keyKeep:
		if msg, ok := m.selectedMsg(); ok {
			m.toggleKeep(msg)
		}
	case keyOpenThrd:
		if msg, ok := m.selectedMsg(); ok && m.pane == paneMain {
			m.openThread(msg)
		}
	}
	return m, nil
}

func (m *Model) submit() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	if strings.HasPrefix(text, "/") {
		return m.command(text)
	}
	if utf8.RuneCountInString(text) > maxInputRune {
		m.setStatus("Message is longer than 4000 characters.")
		return nil
	}
	p := &pendingSend{conv: m.st.Active(), parent: m.threadRoot, text: text}
	if m.send(p) {
		m.input.Reset()
		m.lastTyping = time.Time{}
	}
	return nil
}

func (m *Model) command(text string) tea.Cmd {
	fields := strings.Fields(text)
	arg := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	switch fields[0] {
	case "/quit", "/q":
		return tea.Quit
	case "/i":
		m.input.Reset()
		m.overlay, m.pickIdx = overlayPicker, 0
	case "/whois":
		u := strings.TrimPrefix(arg, "@")
		if _, ok := m.st.Users()[u]; !ok {
			m.setStatus("No such user: " + arg)
			return nil
		}
		m.input.Reset()
		m.showProfile(u)
	case "/name":
		if n := utf8.RuneCountInString(arg); n < 1 || n > 32 {
			m.setStatus("Display name must be 1-32 characters.")
			return nil
		}
		if err := m.conn.Send(proto.NewName(arg)); err != nil {
			m.setStatus("Not connected. Reconnecting…")
			return nil
		}
		m.input.Reset()
		m.setStatus("Name updated.")
	default:
		m.setStatus("Unknown command: " + fields[0])
	}
	return nil
}

func (m *Model) showProfile(u string) {
	m.overlay, m.profileUser = overlayProfile, u
}

func (m *Model) pickerKey(s string) (tea.Model, tea.Cmd) {
	n := len(kaomoji.All)
	switch s {
	case keyEsc:
		m.overlay = overlayNone
	case keyLeft:
		m.pickIdx = (m.pickIdx + n - 1) % n
	case keyRight:
		m.pickIdx = (m.pickIdx + 1) % n
	case keyUp:
		if m.pickIdx >= pickerCols {
			m.pickIdx -= pickerCols
		}
	case keyDown:
		if m.pickIdx+pickerCols < n {
			m.pickIdx += pickerCols
		}
	case keyEnter:
		k := kaomoji.All[m.pickIdx]
		m.input.InsertString(k)
		_ = clip.Copy(k)
		m.overlay = overlayNone
		m.setStatus("Inserted and copied " + k)
	}
	return m, nil
}

func (m *Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			d := 1
			if msg.Button == tea.MouseButtonWheelUp {
				d = -1
			}
			if m.pane == paneInput {
				m.pane = paneMain
				m.input.Blur()
				m.selMain = len(m.st.Timeline(m.st.Active())) - 1
			}
			if m.pane == paneMain {
				m.selMain = clampInt(m.selMain+d, 0, len(m.st.Timeline(m.st.Active()))-1)
			}
			return m, nil
		}
	}
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.overlay != overlayNone {
		if m.overlay == overlayPicker {
			for i, k := range kaomoji.All {
				if zone.Get(fmt.Sprintf("kao:%d", i)).InBounds(msg) {
					m.pickIdx = i
					return m.pickerKey(keyEnter)
				}
				_ = k
			}
		}
		m.overlay = overlayNone
		return m, nil
	}
	if zone.Get("plus").InBounds(msg) {
		m.overlay, m.pickIdx = overlayPicker, 0
		return m, nil
	}
	for _, e := range m.st.Sidebar() {
		for i := 0; i < 3; i++ {
			if e.Username != "" && zone.Get(fmt.Sprintf("avs:%s:%d", e.Username, i)).InBounds(msg) {
				m.showProfile(e.Username)
				return m, nil
			}
			if zone.Get(fmt.Sprintf("side:%s:%d", e.Conv, i)).InBounds(msg) {
				m.switchConv(e.Conv)
				return m, nil
			}
		}
	}
	var visible []chat.Msg
	visible = append(visible, m.st.Timeline(m.st.Active())...)
	_, it := m.threadItems()
	visible = append(visible, it...)
	for _, v := range visible {
		switch {
		case zone.Get("av:" + v.ID).InBounds(msg):
			m.showProfile(v.Sender)
			return m, nil
		case zone.Get("copy:" + v.ID).InBounds(msg):
			m.copyLink(v)
			return m, nil
		case zone.Get("keep:" + v.ID).InBounds(msg):
			m.toggleKeep(v)
			return m, nil
		case zone.Get("thr:" + v.ID).InBounds(msg):
			m.openThread(v)
			return m, nil
		}
	}
	return m, nil
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
