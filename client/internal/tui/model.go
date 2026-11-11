// Package tui is the Bubble Tea terminal UI.
package tui

import (
	"context"
	"errors"
	"fmt"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/sound"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/api"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/kaomoji"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/links"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/update"
)

type pane int

const (
	paneInput pane = iota
	paneMain
	paneThread
	paneThreadInput // the textbox at the bottom of the thread panel
	paneSide        // the left sidebar: up/down pick a conversation, Right opens it
)

type overlay int

const (
	overlayNone overlay = iota
	overlayPicker
	overlayProfile
	overlayHelp
	overlayQuit // "quit?" confirmation after Ctrl+C
	overlayMembers
	overlayClose // "close this room?" confirmation
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
	buzzTickMsg    struct{}
	fatalMsg       struct{ text string } // reconnecting can never succeed: show why and stop
)

// link is the connection surface the UI needs (satisfied by *api.Conn; fakeable in tests).
type link interface {
	Send(v any) error
	Close()
	Events() <-chan proto.Event
	CloseCode() int
}

type pendingSend struct {
	fromThread         bool // which textbox it came from (failed text is restored there)
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

	w, h   int
	input  textarea.Model // main textbox: always posts to the main conversation
	tinput textarea.Model // thread textbox: replies to the open thread root

	pane               pane
	selMain, selThread int
	// Scroll state of the message list and thread panel (see window). Updated while rendering.
	topMain, topThr int
	pinMain, pinThr bool
	threadRoot      string

	overlay          overlay
	pickIdx          int
	mentionNames     map[string]string // "@Display\u00a0Name" -> username for mentions picked in the textbox
	helpTop          int               // /h modal: first visible line
	sideSel, sideTop int               // sidebar focus: selected entry and scroll offset (rows)
	sideSeen         int               // the line the sidebar last scrolled to, so wheel scrolling is not undone
	memIdx, memTop   int               // members modal: selected row and scroll offset
	profileUser      string

	drag         dragSel
	selAllBox    *textarea.Model // the textbox whose whole text is selected (select all), or nil
	screen       []string        // plain text of the last rendered frame, for mouse selection
	leakUntil    time.Time       // while set, stray mouse-report fragments are swallowed
	selectMode   bool            // mouse capture off so the terminal can select text
	sentUnread   int             // last unread total sent to the desktop app
	pickThread   bool            // the kaomoji picker was opened from the thread box
	fatal        string          // non-empty: connection is permanently gone (revoked / locked)
	status       string
	statusUntil  time.Time
	reconnecting bool
	lastTyping   time.Time
	buzz         buzzState
	nudge        nudgeState // gentle shake when someone mentions you
	dance        danceState
	fish         danceState
	petOff       bool // /e hides the sidebar pet
	refN         int
	roomRefs     map[string]bool // refs of room commands awaiting a reply
	hover        string          // id of the clickable zone under the mouse ("" = none)
	sideWidth    int             // sidebar width; 0 = default (dragged with the mouse)
	resizing     bool            // the sidebar edge is being dragged
	mentSel      int             // highlighted row of the @-mention popup
	mentOffAt    int             // 1 + rune index of an "@" whose popup Esc closed (0: none)
	pending      map[string]*pendingSend
	retry        []*pendingSend
	curOut       *cursorOut // real-cursor placement for input methods (nil in tests)
}

// Run starts the UI and blocks until the user quits.
func Run(ctx context.Context, conn *api.Conn, st *chat.State, kp e2e.KeyPair, reconnect func(context.Context) (*api.Conn, error)) error {
	zone.NewGlobal()
	defer zone.Close()
	m := NewModel(ctx, conn, st, kp, reconnect)
	m.curOut = &cursorOut{File: os.Stdout}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithContext(ctx), tea.WithOutput(m.curOut))
	_, err := p.Run()
	m.curOut.reset()
	m.conn.Close()
	return err
}

func NewModel(ctx context.Context, conn link, st *chat.State, kp e2e.KeyPair, reconnect func(context.Context) (*api.Conn, error)) *Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle() // no own background: the box paints one
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.CharLimit = maxInputRune
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	editKeys(&ta.KeyMap)
	ta.Placeholder = ""                    // the hint lives in the line under the box: "[+] kaomoji  /h for shortcut"
	ta.Cursor.SetMode(cursor.CursorStatic) // never blink, so the cursor cell is always drawn
	ta.Focus()
	tb := textarea.New()
	tb.Prompt = ""
	tb.FocusedStyle.CursorLine = lipgloss.NewStyle() // no own background: the box paints one
	tb.BlurredStyle.CursorLine = lipgloss.NewStyle()
	tb.ShowLineNumbers = false
	tb.CharLimit = maxInputRune
	tb.SetHeight(3)
	tb.KeyMap.InsertNewline = ta.KeyMap.InsertNewline
	editKeys(&tb.KeyMap)
	tb.Placeholder = "Reply in thread…"
	tb.Cursor.SetMode(cursor.CursorStatic)
	tb.Blur()
	return &Model{tinput: tb, ctx: ctx, conn: conn, st: st, kp: kp, reconnect: reconnect, input: ta,
		selMain: -1, selThread: -1, pinMain: true, pinThr: true, pending: map[string]*pendingSend{}, roomRefs: map[string]bool{}, w: 0, h: 0}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, waitEvent(m.conn), tick())
}

func tick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

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
			var ae *api.APIError
			if errors.As(err, &ae) {
				switch ae.Code {
				case "locked":
					return fatalMsg{"Account locked until " + time.UnixMilli(ae.Until).Local().Format("15:04") + ". Press Ctrl+C to quit."}
				case "bad_credentials":
					return fatalMsg{"Login rejected: your account was reset or the PIN changed. Run `umbrella register --force`, then restart."}
				}
			}
			select {
			case <-ctx.Done():
				return tea.Quit()
			case <-time.After(backoff(attempt)):
			}
		}
	}
}

// Version is the running build's version, set by main; used by /update.
var Version = "dev"

// sudoUpdateMsg is the result of `sudo umbrella update`, run with the terminal handed over.
type sudoUpdateMsg struct{ err error }

type updateMsg struct {
	tag     string
	updated bool
	err     error
}

func doUpdate() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		tag, done, err := update.Run(ctx, Version)
		return updateMsg{tag, done, err}
	}
}

func (m *Model) setStatus(s string) {
	m.status = chat.SanitizeLine(s)
	m.statusUntil = time.Now().Add(6 * time.Second)
}

const (
	defaultSideW = 24
	minSideW     = 16
	maxSideW     = 50
)

// clampSideW keeps the sidebar between its limits and leaves the chat at least 40 cells.
func (m *Model) clampSideW(w int) int {
	return clampInt(w, minSideW, maxInt(minSideW, minInt(maxSideW, m.w-40)))
}

// resizeMouse lets the user drag the sidebar's right edge. It reports whether it used the event.
func (m *Model) resizeMouse(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft && !m.resizing {
		return false
	}
	side, _, _ := m.layout()
	switch msg.Action {
	case tea.MouseActionPress:
		// the border column and the one beside it are the handle
		if side > 0 && (msg.X == side-1 || msg.X == side) && msg.Y >= 0 && msg.Y < m.h-1 {
			m.resizing = true
			return true
		}
	case tea.MouseActionMotion:
		if m.resizing {
			m.sideWidth = m.clampSideW(msg.X + 1)
			m.resize()
			return true
		}
	case tea.MouseActionRelease:
		if m.resizing {
			m.resizing = false
			return true
		}
	}
	return false
}

func (m *Model) layout() (sideW, chatW, threadW int) {
	sideW = m.clampSideW(m.sideWidth)
	if m.sideWidth == 0 {
		sideW = defaultSideW
	}
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
	_, cw, tw := m.layout()
	m.input.SetWidth(maxInt(10, cw-6)) // pane border + box border + 1 space padding each side
	m.tinput.SetWidth(maxInt(10, tw-6))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	mm, cmd := m.update(msg)
	m.hostUnread()
	return mm, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.resize()
		return m, nil
	case tickMsg:
		return m, tick()
	case buzzTickMsg:
		return m, m.animTick(time.Now())
	case eventMsg:
		if msg.conn != m.conn {
			return m, nil
		}
		return m, tea.Batch(m.onEvent(msg.ev), waitEvent(m.conn))
	case connClosedMsg:
		if msg.conn != m.conn {
			return m, nil
		}
		if msg.conn.CloseCode() == 4001 {
			m.fatal = "Session revoked by an admin. Run `umbrella register --force` with a new invite code, then restart."
			return m, nil
		}
		m.reconnecting = true
		return m, m.doReconnect()
	case fatalMsg:
		m.fatal, m.reconnecting = msg.text, false
		return m, nil
	case reconnectedMsg:
		m.conn = msg.conn
		m.reconnecting = false
		m.pending = map[string]*pendingSend{}
		m.setStatus("Reconnected.")
		return m, waitEvent(m.conn)
	case sudoUpdateMsg:
		if msg.err != nil {
			m.setStatus("Update failed. In a terminal run: sudo umbrella update")
		} else {
			m.setStatus("Updated. Quit (/q) and reopen to use it.")
		}
		return m, nil
	case updateMsg:
		switch {
		case msg.err != nil && update.NeedsRoot(msg.err) && runtime.GOOS != "windows":
			if exe, err := os.Executable(); err == nil {
				m.setStatus("Needs administrator rights: enter your password below…")
				return m, tea.ExecProcess(exec.Command("sudo", exe, "update"), func(err error) tea.Msg { return sudoUpdateMsg{err} })
			}
			m.setStatus("Update failed. In a terminal run: sudo umbrella update")
		case msg.err != nil:
			m.setStatus("Update failed: " + msg.err.Error() + "." + update.ManualHint(runtime.GOOS))
		case msg.updated:
			m.setStatus("Updated to " + msg.tag + ". Quit (/q) and reopen to use it.")
		default:
			m.setStatus("Already up to date (" + msg.tag + ").")
		}
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	case tea.MouseMsg:
		return m.onMouse(msg)
	}
	var cmd tea.Cmd
	if m.pane == paneThreadInput {
		m.tinput, cmd = m.tinput.Update(msg)
	} else {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

// box is the textbox that currently has (or last had) the keyboard.
func (m *Model) box() *textarea.Model {
	if m.pane == paneThreadInput {
		return &m.tinput
	}
	return &m.input
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
	case "keep_limit":
		return "You can keep at most 200 messages. Unkeep one first."
	case "bad_request":
		return "The server rejected that message as malformed."
	case "recipients_mismatch":
		return "Someone joined while you were sending. Please send again."
	default:
		return "Server error: " + chat.SanitizeLine(code)
	}
}

func (m *Model) onEvent(ev proto.Event) tea.Cmd {
	var cmds []tea.Cmd
	mainID, thrID := m.selIDs()
	defer m.reselect(mainID, thrID)
	prev := m.st.Active()
	fx := m.st.Apply(ev)
	if a := m.st.Active(); a != prev { // the open room vanished (kicked, closed, left)
		m.switchConv(a)
	}
	m.hostNotify(ev, fx)
	if fx.Bell {
		cmds = append(cmds, func() tea.Msg { sound.Mention(); return nil }) // falls back to BEL
	}
	if fx.Bell { // Bell is only ever set by a mention of me
		cmds = append(cmds, m.nudge.begin(time.Now()))
	}
	if fx.Buzz && ev.Message != nil {
		cmds = append(cmds, func() tea.Msg { sound.Buzz(); return nil })
	}
	if fx.Buzz && ev.Message != nil && ev.Message.Conv == m.st.Active() {
		cmds = append(cmds, m.buzz.begin(ev.Message.ID, time.Now()))
	}
	if fx.Fish && ev.Message != nil && ev.Message.Conv == m.st.Active() {
		cmds = append(cmds, m.fish.begin(ev.Message.ID, time.Now()))
	}
	if fx.Dance && ev.Message != nil && ev.Message.Conv == m.st.Active() {
		cmds = append(cmds, m.dance.begin(ev.Message.ID, time.Now()))
	}
	switch ev.T {
	case "hello", "user":
		m.flushRetries()
	case "room", "room_gone":
		m.onRoomEvent(ev, fx)
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
		if m.roomRefs[ev.Ref] {
			delete(m.roomRefs, ev.Ref)
			m.setStatus(roomErrorText(ev.Error, ev.Detail))
		} else {
			m.setStatus(errorText(ev.Error))
		}
		if p != nil {
			m.restore(p.text, p.fromThread) // never lose what the user wrote
		}
	}
	m.clampSel()
	return tea.Batch(cmds...)
}

// restore puts a message that failed to send back into the input (ahead of any new draft).
func (m *Model) restore(text string, thread bool) {
	b := &m.input
	if thread {
		b = &m.tinput
	}
	if cur := b.Value(); cur != "" {
		text += "\n" + cur
	}
	b.SetValue(text)
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
	m.focusInput()
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

// isBuzz: a buzz is a one-off effect, so it takes no thread replies.
func isBuzz(msg chat.Msg) bool { return msg.Text == chat.BuzzText && !msg.Undecryptable }

func (m *Model) openThread(msg chat.Msg) {
	id := msg.ID
	if msg.ParentID != "" {
		id = msg.ParentID
	}
	if id != m.threadRoot {
		m.tinput.Reset() // a draft belongs to its thread
	}
	m.threadRoot = id
	m.selThread = -1
	m.resize()
}

// focusThreadInput focuses the thread panel's textbox.
func (m *Model) focusThreadInput() {
	m.pane = paneThreadInput
	m.tinput.Focus()
	m.input.Blur()
	m.pinThr = true
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
	if err := copyText(ls[0]); err != nil {
		m.setStatus("Couldn't copy: " + err.Error())
		return
	}
	m.setStatus("Link copied.")
}

func (m *Model) onKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mouseLeak(k) {
		return m, nil
	}
	s := k.String()
	if hostElectron && m.fatal == "" && (s == "ctrl+c" || s == "ctrl+x") {
		return m.copySel(s == "ctrl+x") // like a web app: Ctrl+C copies, never quits
	}
	if m.overlay == overlayQuit { // Ctrl+C again, y or Enter confirms; any other key stays
		if s == keyQuit || s == "y" || s == "Y" || s == keyEnter {
			return m, tea.Quit
		}
		m.overlay = overlayNone
		return m, nil
	}
	if s == keyQuit {
		if m.fatal != "" { // nothing left to lose: the connection is gone
			return m, tea.Quit
		}
		m.overlay = overlayQuit
		return m, nil
	}
	if m.overlay == overlayNone && m.pane == paneSide {
		if done, cmd := m.sideKey(k); done {
			return m, cmd
		}
	}
	switch m.overlay {
	case overlayPicker:
		return m.pickerKey(s)
	case overlayClose:
		return m.closeKey(s)
	case overlayMembers:
		return m.membersKey(s)
	case overlayHelp:
		switch s { // scrolling keys stay in the help; any other key closes it
		case keyUp, "k":
			m.helpMove(-1)
		case keyDown, "j":
			m.helpMove(1)
		case "pgup":
			m.helpMove(-8)
		case "pgdown", " ":
			m.helpMove(8)
		case "home":
			m.helpMove(-1 << 20)
		case "end":
			m.helpMove(1 << 20)
		default:
			m.overlay = overlayNone
		}
		return m, nil
	case overlayProfile:
		m.overlay = overlayNone
		return m, nil
	}
	if s == keyCopySel {
		return m.copySel(false)
	}
	m.drag.shown = false
	if handled, cmd := m.mentionKey(k); handled {
		return m, cmd
	}
	switch s {
	case keySelect:
		m.selectMode = !m.selectMode
		if m.selectMode {
			return m, tea.DisableMouse
		}
		return m, tea.EnableMouseAllMotion
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
		if m.threadRoot != "" {
			m.closeThread() // back to the main list, same message selected
			return m, nil
		}
		m.focusInput()
		return m, nil
	}
	if m.pane != paneInput && m.pane != paneThreadInput {
		return m.listKey(k)
	}
	thread := m.pane == paneThreadInput
	b := m.box()
	if s == keyEnter {
		return m, m.submit()
	}
	if m.selAllBox != nil && (m.selAllBox != b || b.Value() == "") {
		m.selAllBox = nil
	}
	if handled, cmd := m.selectAllKey(b, k); handled {
		return m, cmd
	}
	atStart := b.Value() == "" || (b.Line() == 0 && b.LineInfo().CharOffset == 0 && b.LineInfo().RowOffset == 0)
	switch {
	case s == keyUp && (b.Value() == "" || b.Line() == 0):
		if thread {
			// Up from the thread box moves to the replies list, selecting the newest reply.
			if _, it := m.threadItems(); len(it) > 0 {
				m.pane = paneThread
				m.tinput.Blur()
				m.selThread = len(it) - 1
				return m, nil
			}
		} else if n := len(m.st.Timeline(m.st.Active())); n > 0 {
			// Up from the input moves to the message list, selecting the newest message.
			m.pane = paneMain
			m.input.Blur()
			m.selMain = n - 1
			return m, nil
		}
	case s == keyLeft && thread && atStart:
		m.closeThread()
		return m, nil
	}
	var cmd tea.Cmd
	before := b.Value()
	*b, cmd = b.Update(k)
	m.mentionTyped(before)
	if v := b.Value(); v != "" && !strings.HasPrefix(v, "/") && time.Since(m.lastTyping) >= typingEvery {
		m.lastTyping = time.Now()
		_ = m.conn.Send(proto.NewTyping(m.st.Active()))
	}
	return m, cmd
}

// focusInput returns focus to the textbox and re-pins both lists to the newest message.
func (m *Model) focusInput() {
	m.pane = paneInput
	m.input.Focus()
	m.tinput.Blur()
	m.pinMain, m.pinThr = true, true
}

// closeThread closes the thread panel and focuses the main list on the thread's root.
func (m *Model) closeThread() {
	root := m.threadRoot
	m.threadRoot = ""
	m.pinThr = true
	m.resize()
	tl := m.st.Timeline(m.st.Active())
	found := false
	for i, t := range tl {
		if t.ID == root {
			m.selMain, found = i, true
		}
	}
	if !found && (m.selMain < 0 || m.selMain >= len(tl)) {
		m.selMain = len(tl) - 1
	}
	m.pane = paneMain
	m.input.Blur()
	m.tinput.Blur()
}

// selIDs / reselect keep the selection on the same messages when the timeline changes underneath.
func (m *Model) selIDs() (mainID, thrID string) {
	if tl := m.st.Timeline(m.st.Active()); m.selMain >= 0 && m.selMain < len(tl) {
		mainID = tl[m.selMain].ID
	}
	if _, it := m.threadItems(); m.selThread >= 0 && m.selThread < len(it) {
		thrID = it[m.selThread].ID
	}
	return
}

func (m *Model) reselect(mainID, thrID string) {
	if mainID != "" {
		for i, t := range m.st.Timeline(m.st.Active()) {
			if t.ID == mainID {
				m.selMain = i
			}
		}
	}
	if thrID != "" {
		_, it := m.threadItems()
		for i, t := range it {
			if t.ID == thrID {
				m.selThread = i
			}
		}
	}
}

func (m *Model) cycleFocus() {
	switch {
	case m.pane == paneSide:
		m.focusInput()
		return
	case m.threadRoot != "": // input -> thread box -> sidebar -> input
		if m.pane == paneInput {
			m.focusThreadInput()
		} else {
			m.focusSide()
		}
		return
	case m.pane == paneMain, m.pane == paneThread:
		m.focusSide()
		return
	}
	m.pane = paneMain // from the textbox: the message list
	m.input.Blur()
	m.selMain = len(m.st.Timeline(m.st.Active())) - 1
}

// focusSide moves focus to the left sidebar, selecting the open conversation.
func (m *Model) focusSide() {
	m.pane = paneSide
	m.input.Blur()
	m.tinput.Blur()
	m.sideSel = 0
	for i, e := range m.st.Sidebar() {
		if e.Conv == m.st.Active() {
			m.sideSel = i
		}
	}
	m.sideSeen = -1
}

// sideKey drives the sidebar while it has focus; typing is ignored so nothing leaks into a box.
func (m *Model) sideKey(k tea.KeyMsg) (bool, tea.Cmd) {
	s := k.String()
	es := m.st.Sidebar()
	m.sideSel = clampInt(m.sideSel, 0, maxInt(0, len(es)-1))
	switch s {
	case keyUp, "k":
		m.sideSel = maxInt(0, m.sideSel-1)
	case keyDown, "j":
		m.sideSel = minInt(len(es)-1, m.sideSel+1)
	case "home":
		m.sideSel = 0
	case "end":
		m.sideSel = maxInt(0, len(es)-1)
	case keyRight, keyEnter, "l":
		if len(es) > 0 {
			m.switchConv(es[m.sideSel].Conv) // also hands focus to the textbox
		}
	case keyEsc:
		m.focusInput()
	case keyFocus, keyNextConv, keyPrevConv, keyQuit:
		return false, nil
	}
	return true, nil
}

func (m *Model) listKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := k.String()
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
		} else if n > 0 && m.pane == paneThread { // already on the last reply: Down drops into its textbox
			m.focusThreadInput()
		} else if n > 0 {
			m.focusInput()
		}
	case keyCopyLink:
		if msg, ok := m.selectedMsg(); ok {
			m.copyLink(msg)
		}
	case keyKeep:
		if msg, ok := m.selectedMsg(); ok {
			m.toggleKeep(msg)
		}
	case keyRight:
		if msg, ok := m.selectedMsg(); ok && m.pane == paneMain && !isBuzz(msg) {
			m.openThread(msg)
			m.focusThreadInput()
		}
	case keyLeft:
		if m.threadRoot != "" {
			m.closeThread()
		}
	case keyOpen:
		if msg, ok := m.selectedMsg(); ok {
			if ls := links.Find(msg.Text); len(ls) > 0 {
				m.openLink(ls[0])
			} else {
				m.setStatus("No link in this message.")
			}
		}
	case keyCopyMsg:
		if msg, ok := m.selectedMsg(); ok {
			if err := copyText(msg.Text); err != nil {
				m.setStatus("Couldn't copy: " + err.Error())
			} else {
				m.setStatus("Copied message")
			}
		}
	case keyOpenThrd:
		if msg, ok := m.selectedMsg(); ok && m.pane == paneMain && !isBuzz(msg) {
			m.openThread(msg)
		}
	default:
		// Anything else that types a character belongs to the input: refocus it and forward.
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			var cmd tea.Cmd
			if m.pane == paneThread {
				m.focusThreadInput()
				m.tinput, cmd = m.tinput.Update(k)
			} else {
				m.focusInput()
				m.input, cmd = m.input.Update(k)
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m *Model) submit() tea.Cmd {
	thread := m.pane == paneThreadInput
	b := m.box()
	text := strings.TrimSpace(b.Value())
	if text == "" {
		return nil
	}
	text = m.expandMentions(text)
	if strings.HasPrefix(text, "/") {
		return m.command(text)
	}
	if utf8.RuneCountInString(text) > maxInputRune {
		m.setStatus("Message is longer than 4000 characters.")
		return nil
	}
	parent := ""
	if thread {
		parent = m.threadRoot // the main box never posts into a thread
	}
	p := &pendingSend{fromThread: thread, conv: m.st.Active(), parent: parent, text: text}
	if m.send(p) {
		m.pinMain, m.pinThr = true, true
		b.Reset()
		m.lastTyping = time.Time{}
	}
	return nil
}

// sendHere sends text to the open conversation, into the thread when the thread box has focus.
func (m *Model) sendHere(text string) bool {
	thread := m.pane == paneThreadInput
	parent := ""
	if thread {
		parent = m.threadRoot
	}
	return m.send(&pendingSend{fromThread: thread, conv: m.st.Active(), parent: parent, text: text})
}

func (m *Model) command(text string) tea.Cmd {
	fields := strings.Fields(text)
	arg := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	if ok, c := m.roomCommand(fields[0], arg); ok {
		return c
	}
	switch fields[0] {
	case "/quit", "/q":
		return tea.Quit
	case "/update":
		if hostElectron {
			m.box().Reset()
			m.setStatus("The desktop app checks for new versions when it starts.")
			return nil
		}
		m.box().Reset()
		m.setStatus("Checking for updates…")
		return doUpdate()
	case "/buzz":
		if wait := buzzCooldown - time.Since(m.buzz.sent); wait > 0 {
			m.setStatus(fmt.Sprintf("Easy! You can buzz again in %ds.", int(wait.Seconds())+1))
			return nil
		}
		thread := m.pane == paneThreadInput
		parent := ""
		if thread {
			parent = m.threadRoot
		}
		if m.send(&pendingSend{fromThread: thread, conv: m.st.Active(), parent: parent, text: chat.BuzzText}) {
			m.buzz.sent = time.Now()
			m.box().Reset()
		}
	case "/dance":
		if wait := danceCooldown - time.Since(m.dance.sent); wait > 0 {
			m.setStatus(fmt.Sprintf("Easy! You can dance again in %ds.", int(wait.Seconds())+1))
			return nil
		}
		if m.sendHere(chat.DanceText) {
			m.dance.sent = time.Now()
			m.box().Reset()
		}
	case "/cow":
		if m.sendHere(chat.Art(cowArt(arg))) {
			m.box().Reset()
		}
	case "/cat", "/rabbit", "/dog", "/duck":
		if art, ok := talkArt(fields[0][1:], arg); ok && m.sendHere(chat.Art(art)) {
			m.box().Reset()
		}
	case "/fish":
		if wait := danceCooldown - time.Since(m.fish.sent); wait > 0 {
			m.setStatus(fmt.Sprintf("Easy! You can send fish again in %ds.", int(wait.Seconds())+1))
			return nil
		}
		if m.sendHere(chat.FishText) {
			m.fish.sent = time.Now()
			m.box().Reset()
		}
	case "/e":
		m.petOff = !m.petOff
		m.box().Reset()
		if m.petOff {
			m.setStatus("Pet hidden. /e shows it again.")
		} else {
			m.setStatus("Pet is back!")
		}
	case "/i":
		m.box().Reset()
		m.overlay, m.pickIdx, m.pickThread = overlayPicker, 0, m.pane == paneThreadInput
	case "/h", "/help":
		m.box().Reset()
		m.overlay, m.helpTop = overlayHelp, 0
	case "/whois":
		u := strings.TrimPrefix(arg, "@")
		if _, ok := m.st.Users()[u]; !ok {
			m.setStatus("No such user: " + arg)
			return nil
		}
		m.box().Reset()
		m.showProfile(u)
	case "/iam":
		m.box().Reset()
		m.showProfile(m.st.Me())
	case "/name":
		if n := utf8.RuneCountInString(arg); n < 1 || n > 32 {
			m.setStatus("Display name must be 1-32 characters.")
			return nil
		}
		if err := m.conn.Send(proto.NewName(arg)); err != nil {
			m.setStatus("Not connected. Reconnecting…")
			return nil
		}
		m.box().Reset()
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
		if m.pickThread {
			m.tinput.InsertString(k)
		} else {
			m.input.InsertString(k)
		}
		_ = copyText(k)
		m.overlay = overlayNone
		m.setStatus("Inserted and copied " + k)
	}
	return m, nil
}

func (m *Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.selectMode {
		return m, nil
	}
	if msg.Action == tea.MouseActionMotion && msg.Button == tea.MouseButtonNone {
		m.hoverAt(msg) // plain pointer movement: only the hover feedback changes
		return m, nil
	}
	if m.overlay == overlayNone && m.resizeMouse(msg) {
		return m, nil
	}
	if m.overlay == overlayNone && m.dragMouse(msg) {
		return m, nil
	}
	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			d := 1
			if msg.Button == tea.MouseButtonWheelUp {
				d = -1
			}
			if m.overlay == overlayMembers {
				m.memMove(d)
				return m, nil
			}
			if m.overlay == overlayHelp {
				m.helpMove(3 * d)
				return m, nil
			}
			if sw, _, _ := m.layout(); m.overlay == overlayNone && msg.X < sw {
				m.sideTop += 3 * d // the list's own scroll, independent of the selection
				return m, nil
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
	if m.overlay == overlayMembers {
		return m.membersClick(msg)
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
	if zone.Get("input").InBounds(msg) {
		m.focusInput()
		return m, nil
	}
	if m.threadRoot != "" && zone.Get("tinput").InBounds(msg) {
		m.focusThreadInput()
		return m, nil
	}
	if zone.Get("plus").InBounds(msg) {
		m.overlay, m.pickIdx, m.pickThread = overlayPicker, 0, false
		return m, nil
	}
	if m.clickUser(msg) {
		return m, nil
	}
	var visible []chat.Msg
	visible = append(visible, m.st.Timeline(m.st.Active())...)
	_, it := m.threadItems()
	visible = append(visible, it...)
	for _, v := range visible {
		for ui, u := range links.Find(v.Text) {
			for f := 0; f < 64; f++ {
				if zone.Get(fmt.Sprintf("lnk:%s:%d:%d", v.ID, ui, f)).InBounds(msg) {
					m.openLink(u)
					return m, nil
				}
			}
		}
		switch {
		case zone.Get("copy:" + v.ID).InBounds(msg):
			m.copyLink(v)
			return m, nil
		case zone.Get("keep:" + v.ID).InBounds(msg):
			m.toggleKeep(v)
			return m, nil
		case !isBuzz(v) && zone.Get("thr:"+v.ID).InBounds(msg):
			m.openThread(v)
			m.focusThreadInput()
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

var mouseLeakRe = regexp.MustCompile(`^(?:\x1b?\[?<\d+;\d+;\d+[Mm])+$`)

// mouseLeak reports whether k is a fragment of an SGR mouse report (ESC [ < b ; x ; y M) that the
// input parser failed to recognise, which happens when wheel events arrive faster than they are
// read or are split across reads. Without this the fragment is typed into the textbox as "[<64;40;20M".
func (m *Model) mouseLeak(k tea.KeyMsg) bool {
	if k.Type != tea.KeyRunes || k.Paste {
		return false
	}
	now := time.Now()
	txt := string(k.Runes)
	switch {
	case k.Alt && txt == "[": // the ESC [ prefix arrived alone; the rest follows
		m.leakUntil = now.Add(150 * time.Millisecond)
		return true
	case mouseLeakRe.MatchString(txt):
		return true
	case now.Before(m.leakUntil) && strings.Trim(txt, "<0123456789;Mm") == "":
		m.leakUntil = now.Add(150 * time.Millisecond)
		return true
	}
	return false
}

// membersKey drives the members modal: arrows move, Enter opens the chat, Esc closes; every
// other key is ignored so a stray keypress cannot dismiss it.
func (m *Model) membersKey(s string) (tea.Model, tea.Cmd) {
	switch s {
	case keyEsc:
		m.overlay = overlayNone
	case keyUp:
		m.memMove(-1)
	case keyDown:
		m.memMove(1)
	case "pgup":
		m.memMove(-8)
	case "pgdown":
		m.memMove(8)
	case "home":
		m.memMove(-1 << 20)
	case "end":
		m.memMove(1 << 20)
	case keyEnter:
		if p := m.headerPeople(m.st.Active()); m.memIdx < len(p) {
			u := p[m.memIdx]
			m.overlay = overlayNone
			m.openDM(u)
		}
	}
	return m, nil
}

func (m *Model) membersClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if zone.Get("mem:x").InBounds(msg) {
		m.overlay = overlayNone
		return m, nil
	}
	for _, u := range m.headerPeople(m.st.Active()) {
		if zone.Get("memu:" + u).InBounds(msg) {
			m.overlay = overlayNone
			m.openDM(u)
			return m, nil
		}
	}
	return m, nil // clicks elsewhere leave the modal open
}
