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
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

var (
	dim       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	artSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("223"))
	accent    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	keptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	mentionSt = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	linkSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Underline(true)
	errSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	nameSt    = lipgloss.NewStyle().Bold(true)
	italicSt  = lipgloss.NewStyle().Italic(true)
)

// wrapText wraps s to at most width display cells per line without ever splitting a rune.
// Explicit newlines are kept; words longer than width are broken between runes.
// nameStyle is bold in the user's own colour (the same hue as their identicon).
func nameStyle(username string) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(avatar.NameColor(username))
}

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

// mentionRe finds a mention: a chip (dot, no-break space, @, display name up to the next space) or a plain @name.
var mentionRe = regexp.MustCompile(`●\x{00a0}@\S*|@[A-Za-z0-9_]+`)

// chipDot precedes a mention of a known user so that line wrapping leaves room for the presence dot
// drawn there; chipRe finds the mentions that get one (@name at the start or after whitespace).
const chipDot = "●\u00a0" // dot + no-break space, so wrapping never splits the chip

var chipRe = regexp.MustCompile(`(^|\s)@([A-Za-z0-9_]+)`)

// chipLookup resolves a typed name to a known user: the canonical username, the display name shown
// in the chip, and whether they are online.
type chipLookup func(name string) (user, label string, online, ok bool)

// chipLabel is the chip text after the @: the display name with spaces made non-breaking so that
// wrapping keeps the chip in one piece.
func chipLabel(label string) string { return strings.ReplaceAll(label, " ", "\u00a0") }

// chipText puts chipDot in front of every mention of a known user.
func chipText(text string, look chipLookup) string {
	if look == nil {
		return text
	}
	return chipRe.ReplaceAllStringFunc(text, func(tok string) string {
		sub := chipRe.FindStringSubmatch(tok)
		_, label, _, ok := look(sub[2])
		if !ok {
			return tok
		}
		return sub[1] + chipDot + "@" + chipLabel(label)
	})
}

// chipUsers lists the users behind the chips of text, in order (the index is the click-zone number).
func chipUsers(text string, look chipLookup) []string {
	if look == nil {
		return nil
	}
	var out []string
	for _, sub := range chipRe.FindAllStringSubmatch(text, -1) {
		if u, _, _, ok := look(sub[2]); ok {
			out = append(out, u)
		}
	}
	return out
}

// chipCtx carries what drawing the chips of one message needs.
type chipCtx struct {
	id    string
	look  chipLookup
	hover string
	users []string // the users behind the chips of the message, in order
	n     int      // chips drawn so far in this message
}

// styleBody styles a (wrapped) body line: italic text, links underlined, mentions of me highlighted.
func styleBody(line, me string) string {
	var out strings.Builder
	rest := line
	for _, u := range links.Find(line) {
		i := strings.Index(rest, u)
		if i < 0 {
			continue
		}
		out.WriteString(styleMentions(rest[:i], me, nil))
		out.WriteString(linkSt.Italic(true).Render(u))
		rest = rest[i+len(u):]
	}
	out.WriteString(styleMentions(rest, me, nil))
	return out.String()
}

func styleMentions(s, me string, c *chipCtx) string {
	var out strings.Builder
	pos := 0
	for _, loc := range mentionRe.FindAllStringIndex(s, -1) {
		tok := s[loc[0]:loc[1]]
		isChip := strings.HasPrefix(tok, chipDot)
		name := strings.TrimPrefix(strings.TrimPrefix(tok, chipDot), "@")
		switch {
		case isChip && c != nil:
			if c.n >= len(c.users) {
				continue
			}
			user := c.users[c.n]
			_, label, online, ok := c.look(user)
			if !ok {
				continue
			}
			// the token runs to the next space; cut it to the display name (punctuation may follow)
			want := chipDot + "@" + chipLabel(label)
			if strings.HasPrefix(tok, want) {
				loc[1] = loc[0] + len(want)
			} else if !strings.HasPrefix(want, tok) {
				continue // not this chip's text: leave it as plain text
			}
			id := fmt.Sprintf("men:%s:%d", c.id, c.n)
			c.n++
			st := nameStyle(user)
			if user == me {
				st = mentionSt
			}
			if c.hover == id {
				st = st.Underline(true)
			}
			out.WriteString(italic(s[pos:loc[0]]))
			out.WriteString(zone.Mark(id, presenceDot(online)+" "+st.Italic(true).Render(s[loc[0]+len(chipDot):loc[1]])))
			pos = loc[1]
		case !isChip && me != "" && strings.EqualFold(name, me):
			out.WriteString(italic(s[pos:loc[0]]))
			out.WriteString(mentionSt.Italic(true).Render(tok))
			pos = loc[1]
		}
	}
	out.WriteString(italic(s[pos:]))
	return out.String()
}

func italic(s string) string {
	if s == "" {
		return s
	}
	return italicSt.Render(s)
}

// linkTracker finds which parts of the wrapped body lines belong to which URL, including URLs
// that wrapText broke across lines (every fragment stays clickable).
type linkTracker struct {
	urls      []string
	idx       int    // next URL to look for
	cur       int    // URL currently continuing onto this line (-1: none)
	remaining string // the rest of that URL
	frag      int
}

type linkSpan struct{ start, end, url, frag int }

func newLinkTracker(text string) *linkTracker { return &linkTracker{urls: links.Find(text), cur: -1} }

func (t *linkTracker) spans(line string) []linkSpan {
	var out []linkSpan
	pos := 0
	if t.cur >= 0 && t.remaining != "" {
		n := minInt(len(t.remaining), len(line))
		if strings.HasPrefix(t.remaining, line[:n]) && n > 0 {
			t.frag++
			out = append(out, linkSpan{0, n, t.cur, t.frag})
			t.remaining = t.remaining[n:]
			pos = n
			if t.remaining != "" {
				return out
			}
		}
		if t.remaining != "" {
			t.cur, t.remaining = -1, ""
		}
	}
	for t.idx < len(t.urls) {
		u := t.urls[t.idx]
		if i := strings.Index(line[pos:], u); i >= 0 {
			out = append(out, linkSpan{pos + i, pos + i + len(u), t.idx, 0})
			pos += i + len(u)
			t.idx++
			continue
		}
		// the line may end in the first part of the URL
		start := -1
		for j := pos; j < len(line); j++ {
			if line[j] == 'h' && strings.HasPrefix(u, line[j:]) {
				start = j
				break
			}
		}
		if start >= 0 {
			out = append(out, linkSpan{start, len(line), t.idx, 0})
			t.cur, t.remaining, t.frag = t.idx, u[len(line)-start:], 0
			t.idx++
		}
		break
	}
	return out
}

// styleBodyZ is styleBody with click zones on the link spans.
func styleBodyZ(line, me, msgID string, spans []linkSpan, c *chipCtx) string {
	var out strings.Builder
	pos := 0
	for _, sp := range spans {
		out.WriteString(styleMentions(line[pos:sp.start], me, c))
		out.WriteString(zone.Mark(fmt.Sprintf("lnk:%s:%d:%d", msgID, sp.url, sp.frag), linkSt.Italic(true).Render(line[sp.start:sp.end])))
		pos = sp.end
	}
	out.WriteString(styleMentions(line[pos:], me, c))
	return out.String()
}

type msgView struct {
	Name     string
	Me       string
	Width    int
	Selected bool
	InThread bool // replies in the thread panel get no 💬 button
	Online   bool // sender's presence, shown as the dot in the header
	Buzz     buzzAnim
	Dance    buzzAnim
	User     chipLookup // resolves @mentions to users (nil: mentions stay plain text)
	Hover    string     // id of the click zone under the mouse
}

func presenceDot(online bool) string {
	if online {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
	}
	return dim.Render("○")
}

// renderMessage returns the message as display lines, each at most v.Width cells wide:
//
//	● Name  hh:mm                       (others: left)
//	  text…                 [📋][⚓][💬 N]
//	                hh:mm  Name ●       (own messages: flush right)
func renderMessage(m chat.Msg, v msgView) (lines []string) {
	prefix := " "
	if v.Selected {
		prefix = accent.Render("▌")
		defer func() {
			for i := range lines {
				lines[i] = bgFill(lines[i], v.Width, selectBg)
			}
		}()
	}
	own := v.Me != "" && m.Sender == v.Me
	_, isArt := chat.Preformatted(m.Text)
	// ASCII art and /dance are drawn as a block in the middle of the pane, whoever sent them.
	special := !m.Undecryptable && (isArt || m.Text == chat.DanceText)
	flush := own && !special
	bodyN := 0 // number of body lines (set once the body is drawn)
	dot := zone.Mark("av:"+m.ID, presenceDot(v.Online))
	stamp := dim.Render(m.CreatedAt.Local().Format("15:04"))
	nst := nameStyle(m.Sender)
	if v.Hover == "nm:"+m.ID {
		nst = nst.Underline(true)
	}
	name := zone.Mark("nm:"+m.ID, nst.Render(v.Name))
	var head string
	if own {
		head = stamp + "  " + name + " " + dot
	} else {
		head = dot + " " + name + "  " + stamp
	}
	if m.ParentID != "" && !v.InThread {
		head += "  " + dim.Render("↳ reply to "+chat.ExpiredText)
	}
	lines = []string{head}
	defer func() {
		// Own messages hug the right edge (block right-aligned, text left-aligned inside).
		pad := 0
		if flush {
			mw := 0
			for _, l := range lines {
				mw = maxInt(mw, width(l))
			}
			pad = maxInt(0, v.Width-2-mw)
			for i := range lines { // header, text and buttons all flush right
				lines[i] = strings.Repeat(" ", maxInt(0, mw-width(lines[i]))) + lines[i]
			}
		}
		if special && own { // our header hugs the right edge, theirs stays left
			lines[0] = strings.Repeat(" ", maxInt(0, v.Width-2-width(lines[0]))) + lines[0]
		}
		if special && bodyN > 0 { // the art itself is centred
			mw := 0
			for _, l := range lines[1 : 1+bodyN] {
				mw = maxInt(mw, width(l))
			}
			mid := maxInt(0, (v.Width-1-mw)/2)
			for i := 1; i <= bodyN; i++ {
				lines[i] = strings.Repeat(" ", mid) + lines[i]
			}
		}
		for i := range lines {
			lines[i] = prefix + strings.Repeat(" ", pad) + lines[i]
		}
	}()

	textW := v.Width - 3
	if textW < 1 {
		textW = 1
	}
	if flush && v.Width > 40 {
		textW = minInt(textW, maxInt(20, v.Width*7/10)) // own messages wrap narrower so they sit visibly right
	}
	isBuzz := m.Text == chat.BuzzText && !m.Undecryptable
	text := m.Text
	if isBuzz {
		text = fmt.Sprintf(buzzFmt, v.Name)
	}
	var body []string
	if art, ok := chat.Preformatted(m.Text); ok && !m.Undecryptable { // ASCII art: keep every space
		for _, l := range strings.Split(art, "\n") {
			body = append(body, artSt.Render(cut(strings.TrimRight(l, "\r"), textW)))
		}
	} else if m.Text == chat.DanceText && !m.Undecryptable {
		body = danceBody(m.ID, v.Name, v.Dance)
	} else {
		body = wrapText(chipText(text, v.User), textW)
		lt := newLinkTracker(text)
		var chips *chipCtx
		if v.User != nil && !isBuzz && !m.Undecryptable {
			chips = &chipCtx{id: m.ID, look: v.User, hover: v.Hover, users: chipUsers(text, v.User)}
		}
		off := 0
		for i := range body {
			s := body[i]
			if isBuzz {
				s = styleBuzz(s, off, v.Buzz)
				off += utf8.RuneCountInString(body[i])
			} else if m.Undecryptable {
				s = dim.Render(s)
			} else {
				s = styleBodyZ(s, v.Me, m.ID, lt.spans(s), chips)
			}
			body[i] = s
		}
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
	if !v.InThread && !isBuzz { // a buzz takes no thread replies
		label := "[💬]"
		if m.Replies > 0 {
			label = fmt.Sprintf("[💬 %d]", m.Replies)
		}
		btns = append(btns, zone.Mark("thr:"+m.ID, label))
	}
	if special { // art and dances have no copy / keep / thread buttons
		btns = nil
	}
	btnStr := strings.Join(btns, "")

	for _, b := range body {
		lines = append(lines, "  "+b)
	}
	bodyN = len(body)
	if btnStr != "" {
		lines = append(lines, "  "+btnStr)
	}
	return lines
}

// renderThread renders the thread panel body: the root (a placeholder when nil or expired) then replies.
func renderThread(root *chat.Msg, replies []chat.Msg, name func(string) string, online func(string) bool, me string, width, sel int, opts ...func(*msgView)) [][]string {
	var blocks [][]string
	mk := func(v msgView) msgView {
		for _, o := range opts {
			o(&v)
		}
		return v
	}
	idx := 0
	if root == nil || root.Text == chat.ExpiredText && root.Sender == "" {
		blocks = append(blocks, []string{" " + dim.Render(chat.ExpiredText)})
	} else {
		blocks = append(blocks, renderMessage(*root, mk(msgView{Name: name(root.Sender), Online: online(root.Sender), Me: me, Width: width, Selected: sel == idx, InThread: true})))
		idx++
	}
	blocks = append(blocks, []string{" " + dim.Render(strings.Repeat("─", maxInt(1, width-2)))})
	for _, r := range replies {
		blocks = append(blocks, renderMessage(r, mk(msgView{Name: name(r.Sender), Online: online(r.Sender), Me: me, Width: width, Selected: sel == idx, InThread: true})))
		idx++
	}
	return spaced(blocks)
}

// spaced adds a blank line under every block but the last, so messages don't touch.
func spaced(blocks [][]string) [][]string {
	for i := 0; i < len(blocks)-1; i++ {
		blocks[i] = append(blocks[i][:len(blocks[i]):len(blocks[i])], "")
	}
	return blocks
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// window is a scroll window of h lines over all blocks. *top is the first visible line and
// *pinned means "stuck to the bottom" (the default). A selection never changes what is shown;
// it only scrolls the minimum needed to keep the selected block visible.
func window(blocks [][]string, sel, h int, top *int, pinned *bool) []string {
	var lines []string
	selStart, selEnd := -1, -1
	for i, b := range blocks {
		if i == sel {
			selStart = len(lines)
		}
		lines = append(lines, b...)
		if i == sel {
			selEnd = len(lines)
		}
	}
	maxTop := maxInt(0, len(lines)-h)
	t := *top
	if *pinned {
		t = maxTop
	}
	if selStart >= 0 {
		if selStart < t {
			t = selStart
		} else if selEnd > t+h {
			t = minInt(selEnd-h, selStart) // a block taller than the window shows its start
		}
	}
	t = clampInt(t, 0, maxTop)
	*top = t
	*pinned = t >= maxTop
	out := append([]string(nil), lines[t:minInt(len(lines), t+h)]...)
	for len(out) < h {
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

// isOnline snapshots presence once per render.
func (m *Model) isOnline() func(string) bool {
	us := m.st.Users()
	return func(u string) bool { return us[u].Online }
}

func (m *Model) nameOf(u string) string { return m.st.DisplayName(u) }

func (m *Model) convTitle(conv string) string {
	if conv == "hell" {
		return nameSt.Render("# Hell room")
	}
	if r, ok := m.st.RoomOf(conv); ok {
		return nameSt.Render("◈ "+r.Name) + dim.Render(fmt.Sprintf("  %d members", len(r.Members)))
	}
	for _, e := range m.st.Sidebar() {
		if e.Conv == conv {
			return nameStyle(e.Username).Render(e.Label) + " " + dim.Render("@"+e.Username)
		}
	}
	return conv
}

// Row backgrounds: a subtle accent, readable on dark and light terminals.
var (
	activeBg = lipgloss.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Dark: "#2a3b5c", Light: "#dbe6ff"})
	inputCol = lipgloss.AdaptiveColor{Dark: "#000000", Light: "#e2e2e8"}
	inputBg  = lipgloss.NewStyle().Background(inputCol)
	selectBg = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Dark: "#232f47", Light: "#e8eefc"})
)

// bgFill pads line to w cells and paints st's background (and weight) across all of it. Inner
// styles end with a full reset, so the opening sequence is re-issued after every reset.
func bgFill(line string, w int, st lipgloss.Style) string {
	line = padTo(line, w)
	probe := st.Render("x")
	i := strings.Index(probe, "x")
	if i <= 0 { // terminal without colour: nothing to paint
		return line
	}
	pre := probe[:i]
	return pre + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+pre) + "\x1b[0m"
}

func (m *Model) viewSidebar(w, h int) string {
	inner := w - 2
	active := m.st.Active()
	var lines []string
	activeLine := 0
	section := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("244")).Faint(true)
	lines = append(lines, section.Render("Channel"))
	firstUser, firstRoom := true, true
	for _, e := range m.st.Sidebar() {
		if e.Room && firstRoom {
			lines = append(lines, dim.Render(" "+strings.Repeat("─", maxInt(1, inner-2))), section.Render("Rooms"))
			firstRoom = false
		}
		if e.Conv != "hell" && !e.Room {
			if firstUser {
				lines = append(lines, dim.Render(" "+strings.Repeat("─", maxInt(1, inner-2))), section.Render("User"), "")
				firstUser = false
			} else {
				lines = append(lines, "") // vertical gap between users
			}
		}
		badge, badgeW := "", 0
		if e.Unread > 0 {
			n := fmt.Sprintf("%d", e.Unread)
			badge, badgeW = accent.Render(n), len(n)
		}
		var row string
		if e.Conv == "hell" {
			row = padTo(nameSt.Render("# Hell room"), inner-badgeW-1) + " " + badge
			if badgeW == 0 {
				row = nameSt.Render("# Hell room")
			}
			row = zone.Mark("side:hell", padTo(row, inner))
		} else if e.Room {
			nameW := inner - 2
			if badgeW > 0 {
				nameW -= badgeW + 1
			}
			rest := accent.Render("◈") + " " + padTo(nameSt.Render(truncate(e.Label, maxInt(1, nameW))), maxInt(1, nameW))
			if badgeW > 0 {
				rest += " " + badge
			}
			row = zone.Mark("side:"+e.Conv, padTo(rest, inner))
		} else {
			dot := dim.Render("○")
			if e.Online {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
			}
			mini := lipgloss.NewStyle().Foreground(lipgloss.Color(avatar.Color(e.Username))).Render(avatar.Mini(e.Username))
			avail := inner - 6 // mini(3) + space + dot + space
			nameW := avail
			if badgeW > 0 {
				nameW = avail - badgeW - 1
			}
			label := nameStyle(e.Username).Render(truncate(e.Label, maxInt(1, nameW)))
			rest := dot + " " + padTo(label, maxInt(1, nameW))
			if badgeW > 0 {
				rest += " " + badge
			}
			row = zone.Mark("avs:"+e.Username, mini) + zone.Mark("side:"+e.Conv, " "+padTo(rest, inner-4))
		}
		if e.Conv == active {
			activeLine = len(lines)
			row = bgFill(row, inner, activeBg)
		} else if m.hover == "side:"+e.Conv || (e.Username != "" && m.hover == "avs:"+e.Username) {
			row = bgFill(row, inner, selectBg)
		}
		lines = append(lines, row)
	}
	pet := m.petLines(inner, h-2)
	listH := h - 2 - len(pet)
	off := 0
	if activeLine >= listH { // many users: the list scrolls, the pet stays put
		off = activeLine - listH + 1
	}
	list := fit(lines[minInt(off, len(lines)):], inner, listH)
	if len(pet) > 0 {
		list += "\n" + strings.Join(pet, "\n")
	}
	return paneBox(list, inner, false)
}

// petFrames is a little dancing cat, 11 cells wide.
var petFrames = [4][3]string{
	{`  /\_/\  ♪`, ` ( ^.^ )/  `, `  _/ \_    `},
	{`♪ /\_/\    `, `\( ^o^ )   `, `   _/ \_   `},
	{`  /\_/\ ♪  `, ` ( ^.^ )\  `, `  _/ \_/   `},
	{` ♪/\_/\    `, ` ( ^o^ )/  `, `   \_/ \_  `},
}

// petLines is the dancing cat at the bottom of the sidebar (nil when the sidebar is too small).
func (m *Model) petLines(inner, h int) []string {
	const petW = 11
	if m.petOff || inner < petW+2 || h < 16 {
		return nil
	}
	f := petFrames[int(time.Now().UnixMilli()/500)%len(petFrames)]
	pad := strings.Repeat(" ", (inner-petW)/2)
	out := []string{""}
	for _, l := range f {
		out = append(out, pad+accent.Render(l))
	}
	return out
}

// paneBox draws a rounded border by hand (lipgloss would measure widths its own way).
// content is lines already fitted to exactly innerW cells.
func paneBox(content string, innerW int, focused bool) string {
	return paneBoxBg(content, innerW, focused, nil)
}

// paneBoxBg is paneBox whose border cells also carry bg, so a filled box has no gaps: the
// border glyphs only draw a thin line and would otherwise leave the terminal colour around it.
func paneBoxBg(content string, innerW int, focused bool, bg lipgloss.TerminalColor) string {
	c := lipgloss.Color("240")
	if focused {
		c = lipgloss.Color("39")
	}
	st := lipgloss.NewStyle().Foreground(c)
	if bg != nil {
		st = st.Background(bg)
	}
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
	title := m.convTitle(conv)
	if ty := m.st.Typing(conv, time.Now()); len(ty) > 0 {
		var names []string
		for _, u := range ty {
			names = append(names, nameStyle(u).Render(m.nameOf(u)))
		}
		title += "  " + strings.Join(names, dim.Render(", ")) + dim.Italic(true).Render(" is typing…")
	}
	inH := m.input.Height() + 2 + 1 // text lines + box borders + the [+] line
	bodyH := h - 2 - 1 - inH - 1    // one blank row keeps the text off the textbox
	room, isRoom := m.activeRoom()
	if isRoom {
		bodyH -= 2 // member list + separator under the title
	}
	if bodyH < 1 {
		bodyH = 1
	}
	var blocks [][]string
	online := m.isOnline()
	for i, msg := range m.st.Timeline(conv) {
		blocks = append(blocks, renderMessage(msg, msgView{Name: m.nameOf(msg.Sender), Online: online(msg.Sender), Me: m.st.Me(), Width: inner, Selected: m.pane == paneMain && i == m.selMain, Buzz: m.buzz.anim(msg.ID, time.Now()), Dance: m.dance.anim(msg.ID, time.Now()), User: m.chipLookup(), Hover: m.hover}))
	}
	blocks = spaced(blocks)
	sel := -1
	if m.pane == paneMain {
		sel = m.selMain
	}
	lines := []string{title}
	if isRoom {
		lines = append(lines, m.roomMembersLine(room, inner), dim.Render(strings.Repeat("─", inner)))
	}
	lines = append(lines, window(blocks, sel, bodyH, &m.topMain, &m.pinMain)...)
	lines = append(lines, dim.Italic(true).Render(truncate(m.roomNotice(), maxInt(1, inner))))
	if pop := m.mentionPopup(inner, false); pop != nil {
		lines = overlayAbove(lines, pop, len(lines), 1, inner)
	}
	n := utf8.RuneCountInString(m.input.Value())
	counter := dim.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	if n > maxInputRune-200 {
		counter = errSt.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	}
	plus := zone.Mark("plus", accent.Render("[+]"))
	foot := plus + dim.Render(" kaomoji  ") + counter
	var boxed []string
	for _, l := range strings.Split(m.boxView(&m.input), "\n") {
		boxed = append(boxed, bgFill(" "+l, inner-2, inputBg))
	}
	// The whole box (border included) is one click zone that focuses the input.
	box := paneBoxBg(fit(boxed, inner-2, m.input.Height()), inner-2, m.pane == paneInput, inputCol)
	lines = append(lines, strings.Split(zone.Mark("input", box), "\n")...)
	lines = append(lines, foot)
	return paneBox(fit(lines, inner, h-2), inner, m.pane != paneThread && m.pane != paneThreadInput)
}

// roomMembersLine lists the room's people as "[ ● A, ○ B ]" (green dot = online); the owner is
// picked out in the accent colour and marked "(owner)".
func (m *Model) roomMembersLine(r proto.Room, w int) string {
	on := m.isOnline()
	var parts []string
	for _, u := range r.Members {
		dot := dim.Render("○")
		if on(u) {
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
		}
		st, label := nameStyle(u), m.nameOf(u)
		if u == r.Owner {
			st, label = accent.Bold(true), label+" (owner)"
		}
		if m.hover == "rm:"+u {
			st = st.Underline(true)
		}
		parts = append(parts, zone.Mark("rm:"+u, dot+" "+st.Render(label)))
	}
	return fit([]string{dim.Render("[ ") + strings.Join(parts, dim.Render(", ")) + dim.Render(" ]")}, w, 1)
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
	look, hov := m.chipLookup(), m.hover
	blocks := renderThread(root, replies, m.nameOf, m.isOnline(), m.st.Me(), inner, sel, func(v *msgView) { v.User, v.Hover = look, hov })
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
	boxH := m.tinput.Height() + 2 + 1 // text lines + borders + counter line
	bodyH := maxInt(1, h-2-1-boxH-1)
	lines = append(lines, window(blocks, bsel, bodyH, &m.topThr, &m.pinThr)...)
	lines = append(lines, "")
	if pop := m.mentionPopup(inner, true); pop != nil {
		lines = overlayAbove(lines, pop, len(lines), 1, inner)
	}
	var boxed []string
	for _, l := range strings.Split(m.boxView(&m.tinput), "\n") {
		boxed = append(boxed, bgFill(" "+l, inner-2, inputBg))
	}
	box := paneBoxBg(fit(boxed, inner-2, m.tinput.Height()), inner-2, m.pane == paneThreadInput, inputCol)
	lines = append(lines, strings.Split(zone.Mark("tinput", box), "\n")...)
	n := utf8.RuneCountInString(m.tinput.Value())
	counter := dim.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	if n > maxInputRune-200 {
		counter = errSt.Render(fmt.Sprintf("%d/%d", n, maxInputRune))
	}
	lines = append(lines, counter)
	return paneBox(fit(lines, inner, h-2), inner, m.pane == paneThread || m.pane == paneThreadInput)
}

func (m *Model) viewStatus() string {
	left := accent.Render("Umbrella") + dim.Render(" @"+m.st.Me()+" · ")
	var right string
	switch {
	case m.selectMode:
		right = accent.Render("Select mode — drag to select, copy with your terminal · Ctrl+X to exit")
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
	case overlayQuit:
		lines = append(lines, nameSt.Render("Quit Umbrella?"), "",
			accent.Render("Ctrl+C")+dim.Render(" / ")+accent.Render("y")+dim.Render(" / ")+accent.Render("Enter")+"  quit",
			dim.Render("any other key  stay"))
	case overlayClose:
		r, ok := m.activeRoom()
		if !ok {
			return nil
		}
		lines = append(lines, nameSt.Render("Close "+truncate(r.Name, 32)+"?"), "",
			"This deletes the room and all its messages for everyone.", "",
			accent.Render("y")+dim.Render(" / ")+accent.Render("Enter")+"  close it",
			dim.Render("any other key  keep it"))
	case overlayMembers:
		r, ok := m.activeRoom()
		if !ok {
			return nil
		}
		lines = append(lines, nameSt.Render(truncate(r.Name, 32))+dim.Render(fmt.Sprintf("  %d members · press any key", len(r.Members))), "")
		on := m.isOnline()
		for _, u := range r.Members {
			dot := dim.Render("○")
			if on(u) {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
			}
			row := dot + " " + nameStyle(u).Render(m.nameOf(u)) + " " + dim.Render("@"+u)
			if u == r.Owner {
				row += " " + accent.Render("owner")
			}
			lines = append(lines, row)
		}
	case overlayHelp:
		lines = append(lines, nameSt.Render("Shortcuts")+dim.Render("  press any key to close"), "")
		for _, e := range helpEntries {
			if e[0] == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, accent.Render(padTo(e[0], 18))+e[1])
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
		lines = append(lines, "", nameStyle(u.Username).Render(u.DisplayName), dim.Render("@"+u.Username),
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
		m.curOut.set(0, 0, false)
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
	m.screen = m.screen[:0]
	dx := m.buzz.shake(time.Now())
	if dx == 0 { // a buzz wins over the gentler mention nudge
		dx = m.nudge.shake(time.Now())
	}
	if dx != 0 {
		lines = shiftLines(lines, dx, m.w)
	}
	for i, l := range lines {
		lines[i] = padTo(l, m.w)
		m.screen = append(m.screen, ansi.Strip(lines[i]))
	}
	m.placeCursor(lines)
	m.highlight(lines)
	return strings.Join(lines, "\n")
}
