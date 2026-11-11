package tui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Typing "@" opens a list of people to mention (like Slack): Up/Down choose, Tab or Enter insert
// "@username ", Esc closes it (the text stays), and deleting the "@" closes it too.

const mentionRows = 6

type mentionCand struct {
	user, label string
	online      bool
}

func isHandleRune(r rune) bool {
	return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// mentionQuery reports whether the text before the caret ends in "@word" (the @ at the start or
// after whitespace) and returns that word and the rune index of the "@".
func mentionQuery(before []rune) (query string, at int, ok bool) {
	i := len(before)
	for i > 0 && isHandleRune(before[i-1]) {
		i--
	}
	if i == 0 || before[i-1] != '@' {
		return "", 0, false
	}
	if i-1 > 0 && !unicode.IsSpace(before[i-2]) {
		return "", 0, false
	}
	return string(before[i:]), i - 1, true
}

// textBeforeCaret is everything in the box up to the caret.
func textBeforeCaret(b *textarea.Model) []rune {
	lines := strings.Split(b.Value(), "\n")
	row := b.Line()
	if row >= len(lines) {
		row = len(lines) - 1
	}
	var out []rune
	for i := 0; i < row; i++ {
		out = append(out, []rune(lines[i])...)
		out = append(out, '\n')
	}
	li := b.LineInfo()
	cur := []rune(lines[row])
	col := li.StartColumn + li.ColumnOffset
	if col > len(cur) {
		col = len(cur)
	}
	return append(out, cur[:col]...)
}

// mentionState is the open popup's query, "@" position and candidates (none when closed).
func (m *Model) mentionState() (cands []mentionCand, query string, at int) {
	if m.overlay != overlayNone || m.selectMode || (m.pane != paneInput && m.pane != paneThreadInput) {
		return nil, "", 0
	}
	query, at, ok := mentionQuery(textBeforeCaret(m.box()))
	if !ok || m.mentOffAt == at+1 {
		return nil, "", 0
	}
	conv := m.st.Active()
	var pool []string
	if r, isRoom := m.st.RoomOf(conv); isRoom {
		pool = r.Members
	} else if strings.HasPrefix(conv, "dm:") {
		pool = strings.Split(conv[3:], "|")
	} else {
		for u := range m.st.Users() {
			pool = append(pool, u)
		}
	}
	users := m.st.Users()
	q := strings.ToLower(query)
	type ranked struct {
		c    mentionCand
		rank int
	}
	var rs []ranked
	for _, u := range pool {
		if u == m.st.Me() {
			continue
		}
		label := m.st.DisplayName(u)
		ln, lu := strings.ToLower(label), strings.ToLower(u)
		rank := 2
		switch {
		case q == "" || strings.HasPrefix(lu, q) || strings.HasPrefix(ln, q):
			rank = 0
		case strings.Contains(lu, q) || strings.Contains(ln, q):
			rank = 1
		default:
			continue
		}
		rs = append(rs, ranked{mentionCand{u, label, users[u].Online}, rank})
	}
	sort.Slice(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.c.online != b.c.online {
			return a.c.online
		}
		return a.c.user < b.c.user
	})
	for _, r := range rs {
		if r.c.user == q { // a complete name needs no help: Enter sends, as in "/whois @bob"
			return nil, "", 0
		}
		cands = append(cands, r.c)
	}
	return cands, query, at
}

func (m *Model) mentionCands() []mentionCand {
	c, _, _ := m.mentionState()
	return c
}

// mentionKey handles the keys the open popup owns; handled is false when it is closed or the key is not its.
func (m *Model) mentionKey(k tea.KeyMsg) (handled bool, cmd tea.Cmd) {
	cands, query, at := m.mentionState()
	if len(cands) == 0 {
		return false, nil
	}
	if m.mentSel >= len(cands) {
		m.mentSel = 0
	}
	switch k.String() {
	case "up", "ctrl+p":
		m.mentSel = (m.mentSel + len(cands) - 1) % len(cands)
	case "down", "ctrl+n":
		m.mentSel = (m.mentSel + 1) % len(cands)
	case "esc":
		m.mentOffAt = at + 1
	case "tab", "enter":
		b := m.box()
		for range []rune(query) {
			*b, _ = b.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		}
		c := cands[m.mentSel]
		b.InsertString(m.mentionToken(c) + " ")
		m.mentSel = 0
	default:
		return false, nil
	}
	return true, nil
}

// mentionToken is what a picked mention looks like in the textbox: the display name (spaces made
// non-breaking so it wraps as one word), remembered so that sending turns it back into the
// "@username" the wire format carries. Ambiguous or plain names stay as the username.
func (m *Model) mentionToken(c mentionCand) string {
	label := chipLabel(c.label)
	if c.label == c.user || label == "" {
		return c.user
	}
	for u := range m.st.Users() {
		if u != c.user && chipLabel(m.st.DisplayName(u)) == label {
			return c.user // two people share this name: be exact
		}
	}
	if m.mentionNames == nil {
		m.mentionNames = map[string]string{}
	}
	m.mentionNames["@"+label] = c.user
	return label
}

// expandMentions rewrites the picked "@Display Name" tokens in text back to "@username".
func (m *Model) expandMentions(text string) string {
	toks := make([]string, 0, len(m.mentionNames))
	for t := range m.mentionNames {
		toks = append(toks, t)
	}
	sort.Slice(toks, func(i, j int) bool { return len(toks[i]) > len(toks[j]) })
	for _, t := range toks {
		var out strings.Builder
		rest := text
		for {
			i := strings.Index(rest, t)
			if i < 0 {
				break
			}
			end := i + len(t)
			next, _ := utf8.DecodeRuneInString(rest[end:])
			okStart := i == 0 || unicode.IsSpace([]rune(rest[:i])[len([]rune(rest[:i]))-1])
			if okStart && (end == len(rest) || !(unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_')) {
				out.WriteString(rest[:i] + "@" + m.mentionNames[t])
			} else {
				out.WriteString(rest[:end])
			}
			rest = rest[end:]
		}
		out.WriteString(rest)
		text = out.String()
	}
	return text
}

// mentionTyped keeps the highlighted row sensible after the box changed.
func (m *Model) mentionTyped(before string) {
	if m.box().Value() != before {
		m.mentSel = 0
	}
}

// mentionPopup draws the candidate list (nil when closed or when `box` is not the focused one).
func (m *Model) mentionPopup(maxW int, forThread bool) []string {
	if forThread != (m.pane == paneThreadInput) {
		return nil
	}
	cands, _, _ := m.mentionState()
	if len(cands) == 0 {
		return nil
	}
	sel := m.mentSel
	if sel >= len(cands) {
		sel = 0
	}
	off := 0
	if sel >= mentionRows {
		off = sel - mentionRows + 1
	}
	end := minInt(len(cands), off+mentionRows)
	var rows []string
	w := 0
	for i := off; i < end; i++ {
		c := cands[i]
		dot := dim.Render("○")
		if c.online {
			dot = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("●")
		}
		row := dot + " " + nameStyle(c.user).Render(c.label) + " " + dim.Render("@"+c.user)
		rows = append(rows, row)
		w = maxInt(w, width(row))
	}
	w = minInt(maxInt(w, 16), maxInt(4, maxW-2))
	for i := range rows {
		line := " " + padTo(truncateStyled(rows[i], w-2), w-1)
		if off+i == sel {
			line = bgFill(line, w, activeBg)
		} else {
			line = padTo(line, w)
		}
		rows[i] = line
	}
	return strings.Split(paneBox(strings.Join(rows, "\n"), w, true), "\n")
}

// truncateStyled cuts an already-styled line to w cells.
func truncateStyled(s string, w int) string { return fit([]string{s}, w, 1) }

// overlayAbove draws box over lines so that it ends at row `end` (exclusive), starting at column left.
func overlayAbove(lines, box []string, end, left, w int) []string {
	bw := 0
	for _, l := range box {
		bw = maxInt(bw, width(l))
	}
	for i, l := range box {
		y := end - len(box) + i
		if y < 1 || y >= len(lines) {
			continue
		}
		row := padTo(lines[y], w)
		lines[y] = sliceCells(row, 0, left) + padTo(l, bw) + sliceCells(row, left+bw, w)
	}
	return lines
}
