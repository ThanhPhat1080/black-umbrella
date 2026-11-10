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

func styleBody(line, me string) string {
	var out strings.Builder
	rest := line
	for _, u := range links.Find(line) {
		i := strings.Index(rest, u)
		if i < 0 {
			continue
		}
		out.WriteString(styleMentions(rest[:i], me))
		out.WriteString(linkSt.Render(u))
		rest = rest[i+len(u):]
	}
	out.WriteString(styleMentions(rest, me))
	return out.String()
}

func styleMentions(s, me string) string {
	if me == "" {
		return s
	}
	return mentionRe.ReplaceAllStringFunc(s, func(m string) string {
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
	switch {
	case v.Me != "" && m.Sender == v.Me: // only the sender can keep: a real button
		if m.Kept {
			btns = append(btns, zone.Mark("keep:"+m.ID, keptStyle.Render("[⚓]")))
		} else {
			btns = append(btns, zone.Mark("keep:"+m.ID, dim.Render("[⚓]")))
		}
	case m.Kept: // someone else's kept message: a marker, not a button
		btns = append(btns, keptStyle.Render("⚓"))
	}
	if !v.InThread {
		label := "[💬]"
		if m.Replies > 0 {
			label = fmt.Sprintf("[💬 %d]", m.Replies)
		}
		btns = append(btns, zone.Mark("thr:"+m.ID, label))
	}
	btnStr := strings.Join(btns, "")
	btnW := width(btnStr)

	last := ""
	if n := len(body); n > 0 {
		last = body[n-1]
	}
	for i, b := range body {
		if i == len(body)-1 && width(b)+2+btnW <= v.Width-3 {
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

// width is THE display-width function: runewidth on the visible text (ANSI sequences and
// bubblezone markers removed). Everything that measures, pads or cuts lines uses it, so the
// layout agrees with terminals even where lipgloss/uniseg would not (e.g. halfwidth kana).
func width(s string) int { return runewidth.StringWidth(ansi.Strip(s)) }

func truncate(s string, w int) string { return runewidth.Truncate(s, w, "…") }

var escSeqRe = regexp.MustCompile(`\x1b\[[0-9;:?<=>]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]`)

// cut limits s to w display cells, keeping escape sequences (so styles still close) but
// dropping text beyond the limit.
func cut(s string, w int) string {
	if width(s) <= w {
		return s
	}
	var out strings.Builder
	used := 0
	pos := 0
	emit := func(text string) {
		if text == "" || used >= w {
			return
		}
		t := runewidth.Truncate(text, w-used, "")
		out.WriteString(t)
		used += runewidth.StringWidth(t)
	}
	for _, loc := range escSeqRe.FindAllStringIndex(s, -1) {
		emit(s[pos:loc[0]])
		out.WriteString(s[loc[0]:loc[1]])
		pos = loc[1]
	}
	emit(s[pos:])
	return out.String()
}

// sliceCells returns the part of s covering display cells [from, to). Cuts happen on rune
// boundaries (a wide rune straddling an edge becomes a space); escape sequences are kept so
// styles still open and close correctly.
func sliceCells(s string, from, to int) string {
	var out strings.Builder
	col := 0
	emit := func(text string) {
		for _, r := range text {
			rw := runewidth.RuneWidth(r)
			switch {
			case rw == 0:
				if col > from && col <= to {
					out.WriteRune(r) // combining mark: stays with its base
				}
			case col >= from && col+rw <= to:
				out.WriteRune(r)
			case col+rw > from && col < to: // straddles an edge
				out.WriteString(strings.Repeat(" ", minInt(col+rw, to)-maxInt(col, from)))
			}
			col += rw
		}
	}
	pos := 0
	for _, loc := range escSeqRe.FindAllStringIndex(s, -1) {
		emit(s[pos:loc[0]])
		out.WriteString(s[loc[0]:loc[1]])
		pos = loc[1]
	}
	emit(s[pos:])
	return out.String()
}

func padTo(s string, w int) string {
	s = cut(s, w)
	return s + strings.Repeat(" ", maxInt(0, w-width(s)))
}

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
		out = append(out, padTo(l, w))
	}
	return strings.Join(out, "\n")
}

// ---- Model rendering ----

func (m *Model) nameOf(u string) string { return m.st.DisplayName(u) }

func (m *Model) convTitle(conv string) string {
	if conv == "hell" {
		return "# Hell room"
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
			label := nameSt.Render("# Hell room")
			if e.Conv == active {
				label = accent.Render("# Hell room")
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
	return paneBox(fit(lines[off:], inner, h-2), inner, false)
}

// paneBox draws a rounded border by hand (lipgloss would measure widths its own way).
// content is lines already fitted to exactly innerW cells.
func paneBox(content string, innerW int, focused bool) string {
	c := lipgloss.Color("240")
	if focused {
		c = lipgloss.Color("39")
	}
	st := lipgloss.NewStyle().Foreground(c)
	var out []string
	out = append(out, st.Render("╭"+strings.Repeat("─", innerW)+"╮"))
	for _, l := range strings.Split(content, "\n") {
		out = append(out, st.Render("│")+padTo(l, innerW)+st.Render("│"))
	}
	out = append(out, st.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	return strings.Join(out, "\n")
}

// joinCols places blocks side by side, row by row.
func joinCols(cols []string) string {
	split := make([][]string, len(cols))
	widths := make([]int, len(cols))
	rows := 0
	for i, c := range cols {
		split[i] = strings.Split(c, "\n")
		for _, l := range split[i] {
			widths[i] = maxInt(widths[i], width(l))
		}
		rows = maxInt(rows, len(split[i]))
	}
	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		var sb strings.Builder
		for i := range split {
			l := ""
			if r < len(split[i]) {
				l = split[i][r]
			}
			sb.WriteString(padTo(l, widths[i]))
		}
		out[r] = sb.String()
	}
	return strings.Join(out, "\n")
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
	var target string
	if m.threadRoot != "" {
		inH++
		if m.toMain {
			target = accent.Render("Posting to main chat") + dim.Render(" · Ctrl+T: reply in thread")
		} else {
			target = accent.Render("Replying in thread") + dim.Render(" · Esc to close · Ctrl+T: main chat")
		}
	}
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
	if target != "" {
		lines = append(lines, target)
	}
	lines = append(lines, strings.Split(m.input.View(), "\n")...)
	lines = append(lines, foot)
	return paneBox(fit(lines, inner, h-2), inner, m.pane != paneThread)
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
	return paneBox(fit(lines, inner, h-2), inner, m.pane == paneThread)
}

func (m *Model) viewStatus() string {
	left := accent.Render("Umbrella") + dim.Render(" @"+m.st.Me()+" · ")
	var right string
	switch {
	case m.fatal != "":
		right = errSt.Render(m.fatal)
	case m.reconnecting:
		right = errSt.Render("Reconnecting…")
	case time.Now().Before(m.statusUntil):
		right = m.status
	default:
		right = dim.Render(helpLine)
	}
	return fit([]string{left + right}, m.w, 1)
}

func (m *Model) overlayView() []string {
	var lines []string
	switch m.overlay {
	case overlayPicker:
		cell := minInt(17, maxInt(8, (m.w-8)/pickerCols))
		lines = append(lines, nameSt.Render("Kaomoji")+dim.Render("  arrows + Enter to insert (also copies), Esc to cancel"), "")
		for r := 0; r*pickerCols < len(kaomoji.All); r++ {
			var sb strings.Builder
			for c := 0; c < pickerCols; c++ {
				i := r*pickerCols + c
				if i >= len(kaomoji.All) {
					break
				}
				text := " " + padTo(truncate(kaomoji.All[i], cell-2), cell-2) + " "
				if i == m.pickIdx {
					text = lipgloss.NewStyle().Reverse(true).Render(text)
				}
				sb.WriteString(zone.Mark(fmt.Sprintf("kao:%d", i), text))
			}
			lines = append(lines, sb.String())
		}
	case overlayProfile:
		u, ok := m.st.Users()[m.profileUser]
		if !ok {
			return nil
		}
		col := lipgloss.NewStyle().Foreground(lipgloss.Color(avatar.Color(u.Username)))
		for _, l := range avatar.Render(u.Username) {
			lines = append(lines, col.Render(l))
		}
		seen := "online"
		if !u.Online {
			seen = "offline"
			if u.LastSeen != nil {
				seen = "last seen " + humanAgo(time.Since(time.UnixMilli(*u.LastSeen)))
			}
		}
		lines = append(lines, "", nameSt.Render(u.DisplayName), dim.Render("@"+u.Username),
			"joined "+time.UnixMilli(u.JoinedAt).Local().Format("2 Jan 2006"), seen, "", dim.Render("press any key"))
	}
	inner := 0
	for _, l := range lines {
		inner = maxInt(inner, width(l)+2)
	}
	inner = minInt(inner, maxInt(3, m.w-6))
	padded := make([]string, len(lines)+2)
	for i, l := range lines {
		padded[i+1] = " " + l
	}
	return strings.Split(paneBox(fit(padded, inner, len(padded)), inner, true), "\n")
}

// overlayOn draws box centred over the background lines, cell-accurately: whatever is outside
// the box's columns stays visible.
func overlayOn(bg, box []string, w, h int) []string {
	for len(bg) < h {
		bg = append(bg, "")
	}
	if len(box) > h {
		box = box[:h]
	}
	bw := 0
	for _, l := range box {
		bw = maxInt(bw, width(l))
	}
	left, top := maxInt(0, (w-bw)/2), maxInt(0, (h-len(box))/2)
	out := append([]string(nil), bg...)
	for i, l := range box {
		row := padTo(bg[top+i], w)
		out[top+i] = sliceCells(row, 0, left) + padTo(l, bw) + sliceCells(row, left+bw, w)
	}
	return out
}

func (m *Model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
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
	lines := strings.Split(joinCols(cols)+"\n"+m.viewStatus(), "\n")
	if m.overlay != overlayNone {
		if box := m.overlayView(); box != nil {
			lines = overlayOn(lines, box, m.w, m.h)
		}
	}
	lines = strings.Split(zone.Scan(strings.Join(lines, "\n")), "\n")
	// Safety net: exactly h lines of exactly w cells, cut and padded by display width.
	for len(lines) < m.h {
		lines = append(lines, "")
	}
	lines = lines[:m.h]
	for i, l := range lines {
		lines[i] = padTo(l, m.w)
	}
	return strings.Join(lines, "\n")
}
