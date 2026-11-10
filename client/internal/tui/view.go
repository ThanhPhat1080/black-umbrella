package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/avatar"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/kaomoji"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/links"
)

var (
	dim       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	accent    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	keptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	mentionSt = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	linkSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Underline(true)
	errSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	nameSt    = lipgloss.NewStyle().Bold(true)
)

// wrapText wraps s to at most width display cells per line without ever splitting a rune.
// Explicit newlines are kept; words longer than width are broken between runes.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		para = strings.TrimRight(para, "\r")
		if para == "" {
			out = append(out, "")
			continue
		}
		var line strings.Builder
		lw := 0
		flush := func() {
			out = append(out, strings.TrimRight(line.String(), " "))
			line.Reset()
			lw = 0
		}
		for _, tok := range splitTokens(para) {
			isSpace := tok[0] == ' '
			tw := runewidth.StringWidth(tok)
			switch {
			case isSpace:
				if lw == 0 {
					continue // no leading spaces after a wrap
				}
				if lw+tw > width {
					flush()
					continue
				}
				line.WriteString(tok)
				lw += tw
			case lw+tw <= width:
				line.WriteString(tok)
				lw += tw
			case tw <= width:
				flush()
				line.WriteString(tok)
				lw = tw
			default: // a word wider than a whole line: break between runes
				if lw > 0 {
					flush()
				}
				for _, r := range tok {
					rw := runewidth.RuneWidth(r)
					if lw+rw > width && lw > 0 {
						flush()
					}
					line.WriteRune(r)
					lw += rw
				}
			}
		}
		if lw > 0 || line.Len() > 0 {
			flush()
		}
	}
	return out
}

func splitTokens(s string) []string {
	var toks []string
	var cur strings.Builder
	inSpace := false
	for i, r := range s {
		sp := r == ' ' || r == '\t'
		if i > 0 && sp != inSpace {
			toks = append(toks, cur.String())
			cur.Reset()
		}
		inSpace = sp
		if r == '\t' {
			r = ' '
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		toks = append(toks, cur.String())
	}
	return toks
}

var mentionRe = regexp.MustCompile(`@[A-Za-z0-9_]+`)
var urlRe = regexp.MustCompile(`https?://\S+`)

func styleBody(line, me string) string {
	line = urlRe.ReplaceAllStringFunc(line, func(u string) string { return linkSt.Render(u) })
	if me == "" {
		return line
	}
	return mentionRe.ReplaceAllStringFunc(line, func(m string) string {
		if strings.EqualFold(m[1:], me) {
			return mentionSt.Render(m)
		}
		return m
	})
}

type msgView struct {
	Name     string
	Me       string
	Width    int
	Selected bool
	InThread bool // replies in the thread panel get no 💬 button
}

// renderMessage returns the message as display lines, each at most v.Width cells wide:
//
//	■ Name  hh:mm
//	  text…                 [📋][⚓][💬 N]
func renderMessage(m chat.Msg, v msgView) []string {
	prefix := " "
	if v.Selected {
		prefix = accent.Render("▌")
	}
	head := zone.Mark("av:"+m.ID, avatar.Dot(m.Sender)) + " " + nameSt.Render(v.Name) + "  " + dim.Render(m.CreatedAt.Local().Format("15:04"))
	if m.ParentID != "" && !v.InThread {
		head += "  " + dim.Render("↳ reply to "+chat.ExpiredText)
	}
	lines := []string{prefix + head}

	textW := v.Width - 3
	if textW < 1 {
		textW = 1
	}
	body := wrapText(m.Text, textW)
	for i := range body {
		s := body[i]
		if m.Undecryptable {
			s = dim.Italic(true).Render(s)
		} else {
			s = styleBody(s, v.Me)
		}
		body[i] = s
	}

	var btns []string
	if len(links.Find(m.Text)) > 0 {
		btns = append(btns, zone.Mark("copy:"+m.ID, "[📋]"))
	}
	if m.Kept {
		btns = append(btns, zone.Mark("keep:"+m.ID, keptStyle.Render("[⚓]")))
	} else {
		btns = append(btns, zone.Mark("keep:"+m.ID, dim.Render("[⚓]")))
	}
	if !v.InThread {
		label := "[💬]"
		if m.Replies > 0 {
			label = fmt.Sprintf("[💬 %d]", m.Replies)
		}
		btns = append(btns, zone.Mark("thr:"+m.ID, label))
	}
	btnStr := strings.Join(btns, "")
	btnW := lipgloss.Width(btnStr)

	last := ""
	if n := len(body); n > 0 {
		last = body[n-1]
	}
	for i, b := range body {
		if i == len(body)-1 && lipgloss.Width(b)+2+btnW <= v.Width-3 {
			b += "  " + btnStr
			btnStr = ""
		}
		lines = append(lines, prefix+"  "+b)
	}
	_ = last
	if btnStr != "" {
		lines = append(lines, prefix+"  "+btnStr)
	}
	return lines
}

// renderThread renders the thread panel body: the root (a placeholder when nil or expired) then replies.
func renderThread(root *chat.Msg, replies []chat.Msg, name func(string) string, me string, width, sel int) [][]string {
	var blocks [][]string
	idx := 0
	if root == nil || root.Text == chat.ExpiredText && root.Sender == "" {
		blocks = append(blocks, []string{" " + dim.Italic(true).Render(chat.ExpiredText)})
	} else {
		blocks = append(blocks, renderMessage(*root, msgView{Name: name(root.Sender), Me: me, Width: width, Selected: sel == idx, InThread: true}))
		idx++
	}
	blocks = append(blocks, []string{" " + dim.Render(strings.Repeat("─", maxInt(1, width-2)))})
	for _, r := range replies {
		blocks = append(blocks, renderMessage(r, msgView{Name: name(r.Sender), Me: me, Width: width, Selected: sel == idx, InThread: true}))
		idx++
	}
	return blocks
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// window picks the last h lines of the blocks, ending at the selected block (or the end when sel < 0).
func window(blocks [][]string, sel, h int) []string {
	var lines []string
	selEnd := -1
	for i, b := range blocks {
		lines = append(lines, b...)
		if i == sel {
			selEnd = len(lines)
		}
	}
	end := len(lines)
	if selEnd >= 0 {
		end = selEnd
	}
	start := end - h
	if start < 0 {
		start = 0
	}
	out := lines[start:end]
	pad := h - len(out)
	for i := 0; i < pad; i++ {
		out = append(out, "")
	}
	return out
}

func truncate(s string, w int) string { return runewidth.Truncate(s, w, "…") }

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// fit pads or cuts a block of lines to exactly w x h cells.
func fit(lines []string, w, h int) string {
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		if lipgloss.Width(l) > w {
			l = ansi.Truncate(l, w, "")
		}
		out = append(out, l+strings.Repeat(" ", maxInt(0, w-lipgloss.Width(l))))
	}
	return strings.Join(out, "\n")
}

// ---- Model rendering ----

func (m *Model) nameOf(u string) string { return m.st.DisplayName(u) }

func (m *Model) convTitle(conv string) string {
	if conv == "hell" {
		return "# Hell"
	}
	for _, e := range m.st.Sidebar() {
		if e.Conv == conv {
			return e.Label + " " + dim.Render("@"+e.Username)
		}
	}
	return conv
}

func (m *Model) viewSidebar(w, h int) string {
	inner := w - 2
	active := m.st.Active()
	var blocks [][]string
	activeIdx := 0
	for i, e := range m.st.Sidebar() {
		if e.Conv == active {
			activeIdx = i
		}
		badge := ""
		if e.Unread > 0 {
			badge = accent.Render(fmt.Sprintf(" (%d)", e.Unread))
		}
		mark := func(i int, s string) string { return zone.Mark(fmt.Sprintf("side:%s:%d", e.Conv, i), s) }
		if e.Conv == "hell" {
			label := nameSt.Render("# Hell")
			if e.Conv == active {
				label = accent.Render("# Hell")
			}
			blocks = append(blocks, []string{mark(0, label+badge)})
			continue
		}
		av := avatar.Render(e.Username)
		col := lipgloss.NewStyle().Foreground(lipgloss.Color(avatar.Color(e.Username)))
		dot := dim.Render("○")
		if e.Online {
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
		}
		label := truncate(e.Label, inner-13)
		if e.Conv == active {
			label = accent.Render(label)
		}
		info := []string{dot + " " + label, dim.Render(truncate("@"+e.Username, inner-11)), strings.TrimSpace(badge)}
		var b []string
		for i := 0; i < 3; i++ {
			b = append(b, zone.Mark(fmt.Sprintf("avs:%s:%d", e.Username, i), col.Render(av[i]))+" "+mark(i, info[i]))
		}
		blocks = append(blocks, b)
	}
	var lines []string
	start := 0
	for i, b := range blocks {
		if i == activeIdx {
			start = len(lines)
		}
		lines = append(lines, b...)
	}
	off := 0
	if start+3 > h-2 {
		off = start + 3 - (h - 2)
	}
	if off > len(lines) {
		off = len(lines)
	}
	return paneBox(fit(lines[off:], inner, h-2), false)
}

func paneBox(content string, focused bool) string {
	c := lipgloss.Color("240")
	if focused {
		c = lipgloss.Color("39")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c).Render(content)
}

func (m *Model) viewChat(w, h int) string {
	inner := w - 2
	conv := m.st.Active()
	title := nameSt.Render(m.convTitle(conv))
	if ty := m.st.Typing(conv, time.Now()); len(ty) > 0 {
		var names []string
		for _, u := range ty {
			names = append(names, m.nameOf(u))
		}
		title += "  " + dim.Italic(true).Render(strings.Join(names, ", ")+" is typing…")
	}
	inH := m.input.Height() + 1
	bodyH := h - 2 - 1 - inH
	if bodyH < 1 {
		bodyH = 1
	}
	var blocks [][]string
	for i, msg := range m.st.Timeline(conv) {
		blocks = append(blocks, renderMessage(msg, msgView{Name: m.nameOf(msg.Sender), Me: m.st.Me(), Width: inner, Selected: m.pane == paneMain && i == m.selMain}))
	}
	sel := -1
	if m.pane == paneMain {
		sel = m.selMain
	}
	lines := []string{title}
	lines = append(lines, window(blocks, sel, bodyH)...)
	n := utf8.RuneCountInString(m.input.Value())
	counter := dim.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	if n > maxInputRune-200 {
		counter = errSt.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	}
	plus := zone.Mark("plus", accent.Render("[+]"))
	foot := plus + dim.Render(" kaomoji  ") + counter
	lines = append(lines, strings.Split(m.input.View(), "\n")...)
	lines = append(lines, foot)
	return paneBox(fit(lines, inner, h-2), m.pane != paneThread)
}

func (m *Model) viewThread(w, h int) string {
	inner := w - 2
	root, replies := m.st.Thread(m.threadRoot)
	if root.Text == chat.ExpiredText && root.Sender == "" {
		root = nil
	}
	sel := -1
	if m.pane == paneThread {
		sel = m.selThread
	}
	blocks := renderThread(root, replies, m.nameOf, m.st.Me(), inner, sel)
	// block index for selection: blocks[0]=root, blocks[1]=separator, then replies
	bsel := -1
	if sel >= 0 {
		bsel = sel
		if root != nil {
			if sel > 0 {
				bsel = sel + 1
			}
		} else {
			bsel = sel + 2
		}
	}
	lines := []string{nameSt.Render("Thread") + dim.Render("  (Esc to close)")}
	lines = append(lines, window(blocks, bsel, h-3)...)
	return paneBox(fit(lines, inner, h-2), m.pane == paneThread)
}

func (m *Model) viewStatus() string {
	left := accent.Render("Umbrella") + dim.Render(" @"+m.st.Me()+" · ")
	var right string
	switch {
	case m.reconnecting:
		right = errSt.Render("Reconnecting…")
	case time.Now().Before(m.statusUntil):
		right = m.status
	default:
		right = dim.Render(helpLine)
	}
	return fit([]string{left + right}, m.w, 1)
}

func (m *Model) overlayView() string {
	var box string
	switch m.overlay {
	case overlayPicker:
		const cell = 17
		var rows []string
		for r := 0; r*pickerCols < len(kaomoji.All); r++ {
			var cells []string
			for c := 0; c < pickerCols; c++ {
				i := r*pickerCols + c
				if i >= len(kaomoji.All) {
					break
				}
				k := truncate(kaomoji.All[i], cell-2)
				k += strings.Repeat(" ", maxInt(0, cell-2-runewidth.StringWidth(k)))
				st := lipgloss.NewStyle().Padding(0, 1)
				if i == m.pickIdx {
					st = st.Reverse(true)
				}
				cells = append(cells, zone.Mark(fmt.Sprintf("kao:%d", i), st.Render(k)))
			}
			rows = append(rows, strings.Join(cells, ""))
		}
		box = nameSt.Render("Kaomoji") + dim.Render("  arrows + Enter to insert (also copies), Esc to cancel") + "\n\n" + strings.Join(rows, "\n")
	case overlayProfile:
		u, ok := m.st.Users()[m.profileUser]
		if !ok {
			return ""
		}
		col := lipgloss.NewStyle().Foreground(lipgloss.Color(avatar.Color(u.Username)))
		var av []string
		for _, l := range avatar.Render(u.Username) {
			av = append(av, col.Render(l))
		}
		seen := "online"
		if !u.Online {
			seen = "offline"
			if u.LastSeen != nil {
				seen = "last seen " + humanAgo(time.Since(time.UnixMilli(*u.LastSeen)))
			}
		}
		info := []string{nameSt.Render(u.DisplayName), dim.Render("@" + u.Username),
			"joined " + time.UnixMilli(u.JoinedAt).Local().Format("2 Jan 2006"), seen}
		box = strings.Join(av, "\n") + "\n\n" + strings.Join(info, "\n") + "\n\n" + dim.Render("press any key")
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("39")).Padding(1, 2).Render(box))
}

func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.overlay != overlayNone {
		return zone.Scan(m.overlayView())
	}
	sideW, chatW, threadW := m.layout()
	ph := m.h - 1
	var cols []string
	if sideW > 0 {
		cols = append(cols, m.viewSidebar(sideW, ph))
	}
	cols = append(cols, m.viewChat(chatW, ph))
	if threadW > 0 {
		cols = append(cols, m.viewThread(threadW, ph))
	}
	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left, lipgloss.JoinHorizontal(lipgloss.Top, cols...), m.viewStatus()))
}
