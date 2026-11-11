package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// dragSel is a mouse text selection over the rendered screen: stream selection (like a
// terminal) confined to the pane where the drag started, borders excluded.
type dragSel struct {
	on, moved, shown bool
	ax, ay, cx, cy   int // anchor and current cell
	x0, x1           int // selectable columns [x0, x1) of the anchor's pane
}

// minDragCells keeps a slightly shaky click from being mistaken for a drag.
const minDragCells = 2

// paneBounds returns the selectable columns (inside the border) of the pane containing x.
func (m *Model) paneBounds(x int) (x0, x1 int, ok bool) {
	sideW, chatW, threadW := m.layout()
	edges := []int{0, sideW, sideW + chatW, sideW + chatW + threadW}
	for i := 0; i+1 < len(edges); i++ {
		if x >= edges[i] && x < edges[i+1] && edges[i+1]-edges[i] > 2 {
			return edges[i] + 1, edges[i+1] - 1, true
		}
	}
	return 0, 0, false
}

func (m *Model) dragRow(y int) int { return clampInt(y, 1, maxInt(1, m.h-3)) }

// dragMouse handles press/motion/release for text selection. It returns true when the event
// was consumed (so it must not also act as a click).
func (m *Model) dragMouse(msg tea.MouseMsg) bool {
	if msg.Button != tea.MouseButtonLeft {
		return false
	}
	d := &m.drag
	switch msg.Action {
	case tea.MouseActionPress:
		d.shown = false
		x0, x1, ok := m.paneBounds(msg.X)
		if !ok || msg.Y < 1 || msg.Y > m.h-3 {
			d.on = false
			return false
		}
		*d = dragSel{on: true, ax: clampInt(msg.X, x0, x1-1), ay: msg.Y, x0: x0, x1: x1}
		d.cx, d.cy = d.ax, d.ay
	case tea.MouseActionMotion:
		if !d.on {
			return false
		}
		d.cx, d.cy = clampInt(msg.X, d.x0, d.x1-1), m.dragRow(msg.Y)
		if d.cy != d.ay || absInt(d.cx-d.ax) >= minDragCells {
			d.moved = true
		}
		d.shown = d.moved
		return true
	case tea.MouseActionRelease:
		was := d.on && d.moved
		d.on = false
		if !was {
			d.shown = false
			return false
		}
		d.cx, d.cy = clampInt(msg.X, d.x0, d.x1-1), m.dragRow(msg.Y)
		if text := m.selectedText(); text != "" { // selecting never copies by itself
			m.setStatus(fmt.Sprintf("Selected %d characters · %s to copy.", utf8.RuneCountInString(text), copyKeyName()))
		}
		return true
	}
	return false
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// span returns the selected cell range [a, b) on screen row y.
func (d dragSel) span(y int) (a, b int, ok bool) {
	sy, sx, ey, ex := d.ay, d.ax, d.cy, d.cx
	if ey < sy || ey == sy && ex < sx {
		sy, sx, ey, ex = ey, ex, sy, sx
	}
	if y < sy || y > ey {
		return 0, 0, false
	}
	a, b = d.x0, d.x1
	if y == sy {
		a = sx
	}
	if y == ey {
		b = ex + 1
	}
	return a, b, b > a
}

// selectedText is the plain text under the selection, one line per row, trimmed.
func (m *Model) selectedText() string {
	var out []string
	for y := 0; y < len(m.screen); y++ {
		a, b, ok := m.drag.span(y)
		if !ok {
			continue
		}
		out = append(out, strings.TrimSpace(sliceCells(m.screen[y], a, b)))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

var dragHl = lipgloss.NewStyle().Reverse(true)

// highlight paints the selection onto the final screen lines (m.screen holds their plain text).
func (m *Model) highlight(lines []string) {
	if !m.drag.shown {
		return
	}
	for y := range lines {
		a, b, ok := m.drag.span(y)
		if !ok {
			continue
		}
		seg := sliceCells(ansi.Strip(lines[y]), a, b)
		lines[y] = sliceCells(lines[y], 0, a) + dragHl.Render(seg) + sliceCells(lines[y], b, m.w)
	}
}
