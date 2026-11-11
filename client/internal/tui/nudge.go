package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const nudgeDur = 450 * time.Millisecond

// A mention nudges the screen one cell left and right a few times: much gentler than a buzz.
var nudgeOffsets = []int{1, -1, 1, -1, 1, 0, 0, 0, 0}

// nudgeState tracks the latest mention nudge.
type nudgeState struct{ start time.Time }

func (n *nudgeState) begin(now time.Time) tea.Cmd {
	n.start = now
	return buzzTick()
}

// next keeps the frame ticker going until the nudge is over.
func (n *nudgeState) next(now time.Time) tea.Cmd {
	if !n.start.IsZero() && now.Sub(n.start) < nudgeDur {
		return buzzTick()
	}
	return nil
}

// shake is the horizontal offset for the whole screen at now (0 when idle).
func (n *nudgeState) shake(now time.Time) int {
	d := now.Sub(n.start)
	if n.start.IsZero() || d < 0 || d >= nudgeDur {
		return 0
	}
	return nudgeOffsets[int(d*time.Duration(len(nudgeOffsets))/nudgeDur)]
}
