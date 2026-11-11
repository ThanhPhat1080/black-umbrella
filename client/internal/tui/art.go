package tui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const (
	danceDur      = 4 * time.Second
	danceStep     = 300 * time.Millisecond
	danceCooldown = 10 * time.Second
	maxCowText    = 40
	maxCowLines   = 8
)

// danceAnimals: each dances between two poses; the animal is picked from the message ID so
// every client shows the same one.
var danceAnimals = []struct {
	name   string
	frames [2][3]string
}{
	{"rabbit", [2][3]string{
		{` (\_/)   ♪`, ` (^o^)/    `, ` (")_(")   `},
		{`♪ (\_/)    `, `\(^.^)     `, ` (")_(")   `},
	}},
	{"penguin", [2][3]string{
		{`   _     ♪`, `  ('v')/   `, ` //-=-\\    `},
		{`♪  _       `, `\('v')     `, ` //-=-\\    `},
	}},
	{"frog", [2][3]string{
		{`  @..@   ♪`, ` (----)/   `, `( >__< )   `},
		{`♪ @..@     `, `\(----)     `, `( >__< )   `},
	}},
}

func danceIndex(id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32()) % len(danceAnimals)
}

// danceBody draws the dancing animal for message id, already coloured. While anim is active the
// animal swaps poses and the colours run; afterwards it rests in its first pose.
func danceBody(id, name string, anim buzzAnim) []string {
	a := danceAnimals[danceIndex(id)]
	pose := 0
	if anim.Active {
		pose = int(anim.Elapsed/danceStep) % 2
	}
	out := []string{styleBuzz(fmt.Sprintf("💃 %s is dancing! (%s)", name, a.name), 0, anim)}
	for _, l := range a.frames[pose] {
		out = append(out, styleBuzz(l, 0, anim))
	}
	return out
}

// danceState tracks the latest dance animation (like buzzState, without the shake).
type danceState struct {
	id    string
	start time.Time
	sent  time.Time
}

func (d *danceState) begin(id string, now time.Time) tea.Cmd {
	d.id, d.start = id, now
	return buzzTick()
}

func (d *danceState) next(now time.Time) tea.Cmd {
	if d.id != "" && now.Sub(d.start) < danceDur {
		return buzzTick()
	}
	return nil
}

func (d *danceState) anim(id string, now time.Time) buzzAnim {
	if id == "" || id != d.id || now.Sub(d.start) >= danceDur {
		return buzzAnim{}
	}
	return buzzAnim{Active: true, Elapsed: now.Sub(d.start)}
}

// cowArt draws a cow saying text, like cowsay: the text is wrapped at maxCowText columns and
// put in a speech bubble ( < > for one line, / \ | | \ / for several ).
func cowArt(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		words = []string{"Moo!"}
	}
	var lines []string
	cur := ""
	for _, w := range words {
		for runewidth.StringWidth(w) > maxCowText { // a word wider than the bubble is split
			head := runewidth.Truncate(w, maxCowText, "")
			if cur != "" {
				lines, cur = append(lines, cur), ""
			}
			lines = append(lines, head)
			w = strings.TrimPrefix(w, head)
		}
		switch {
		case w == "":
		case cur == "":
			cur = w
		case runewidth.StringWidth(cur)+1+runewidth.StringWidth(w) <= maxCowText:
			cur += " " + w
		default:
			lines, cur = append(lines, cur), w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if len(lines) > maxCowLines {
		lines = lines[:maxCowLines]
	}
	w := 0
	for _, l := range lines {
		w = maxInt(w, runewidth.StringWidth(l))
	}
	var sb strings.Builder
	sb.WriteString(" " + strings.Repeat("_", w+2) + "\n")
	for i, l := range lines {
		open, close := "|", "|"
		switch {
		case len(lines) == 1:
			open, close = "<", ">"
		case i == 0:
			open, close = "/", "\\"
		case i == len(lines)-1:
			open, close = "\\", "/"
		}
		sb.WriteString(open + " " + runewidth.FillRight(l, w) + " " + close + "\n")
	}
	sb.WriteString(" " + strings.Repeat("-", w+2) + "\n")
	sb.WriteString(`        \   ^__^` + "\n")
	sb.WriteString(`         \  (oo)\_______` + "\n")
	sb.WriteString(`            (__)\       )\/\` + "\n")
	sb.WriteString(`                ||----w |` + "\n")
	sb.WriteString(`                ||     ||`)
	return sb.String()
}
