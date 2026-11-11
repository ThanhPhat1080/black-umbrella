package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	buzzFmt      = "⚡ ~~ %s buzzzz! (ﾉ°Д°)ﾉ ~~ ⚡"
	buzzShakeDur = time.Second
	buzzTextDur  = 10 * time.Second // the banner keeps its colour wave for ten seconds
	buzzShakeGap = 6 * time.Second  // ...and the screen shakes again every few seconds meanwhile
	buzzFrame    = 50 * time.Millisecond
	buzzCooldown = 10 * time.Second
	slowFrame    = 150 * time.Millisecond // frame time while nothing shakes
)

// Horizontal screen offsets, one per frame; the amplitude decays so the shake settles.
var shakeOffsets = []int{3, -3, 3, -3, 2, -2, 2, -2, 2, -2, 1, -1, 1, -1, 1, -1, 1, 0, 0, 0}

var buzzPalette = []lipgloss.Style{
	lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
	lipgloss.NewStyle().Foreground(lipgloss.Color("202")).Bold(true),
	lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true),
	lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
	lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),
	lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true),
}

// buzzState tracks the most recent buzz animation and when we last sent one.
type buzzState struct {
	id    string
	start time.Time
	sent  time.Time
}

// buzzAnim describes how to draw a buzz banner: Active while it should animate.
type buzzAnim struct {
	Active  bool
	Elapsed time.Duration
}

func (b *buzzState) begin(id string, now time.Time) tea.Cmd {
	b.id, b.start = id, now
	return buzzTick()
}

func buzzTick() tea.Cmd {
	return tea.Tick(buzzFrame, func(time.Time) tea.Msg { return buzzTickMsg{} })
}

// animTick schedules the next animation frame, or nothing when every animation is over. There is
// exactly one ticker chain: starting one per active animation would double the work every frame.
// Frames come fast (50ms) only while the screen is shaking; colour waves, dancers and fish look
// fine at a much lower rate, which keeps the terminal from lagging.
func (m *Model) animTick(now time.Time) tea.Cmd {
	if m.buzz.next(now) == nil && m.dance.next(now) == nil && m.fish.next(now) == nil && m.nudge.next(now) == nil {
		return nil
	}
	if m.buzz.shake(now) != 0 || m.nudge.shake(now) != 0 {
		return buzzTick()
	}
	return tea.Tick(slowFrame, func(time.Time) tea.Msg { return buzzTickMsg{} })
}

// next keeps the frame ticker going until the longest animation (the text) is over.
func (b *buzzState) next(now time.Time) tea.Cmd {
	if b.id != "" && now.Sub(b.start) < buzzTextDur {
		return buzzTick()
	}
	return nil
}

func (b *buzzState) anim(id string, now time.Time) buzzAnim {
	if id == "" || id != b.id || now.Sub(b.start) >= buzzTextDur {
		return buzzAnim{}
	}
	return buzzAnim{Active: true, Elapsed: now.Sub(b.start)}
}

// shake returns the horizontal offset for the whole screen at now (0 when idle).
func (b *buzzState) shake(now time.Time) int {
	if b.id == "" {
		return 0
	}
	d := now.Sub(b.start)
	if d < 0 || d >= buzzTextDur {
		return 0
	}
	d %= buzzShakeGap
	if d >= buzzShakeDur {
		return 0
	}
	return shakeOffsets[int(d*time.Duration(len(shakeOffsets))/buzzShakeDur)]
}

// styleBuzz colours one wrapped line of the banner; a colour wave runs along it while animating,
// then it settles to plain yellow. off is the line's first rune index in the whole banner.
func styleBuzz(line string, off int, a buzzAnim) string {
	if !a.Active {
		return buzzPalette[4].Render(line)
	}
	shift := int(a.Elapsed / (70 * time.Millisecond))
	var sb strings.Builder
	i := off
	for _, r := range line {
		sb.WriteString(buzzPalette[((i-shift)%len(buzzPalette)+len(buzzPalette))%len(buzzPalette)].Render(string(r)))
		i++
	}
	return sb.String()
}

// shiftLines moves every line dx cells right (negative: left), keeping each at most w cells.
func shiftLines(lines []string, dx, w int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if dx > 0 {
			out[i] = strings.Repeat(" ", dx) + ansi.Truncate(l, w-dx, "")
		} else {
			out[i] = ansi.Cut(l, -dx, w)
		}
	}
	return out
}
