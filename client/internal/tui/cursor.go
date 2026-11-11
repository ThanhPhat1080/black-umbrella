package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

// Input methods (IBus, fcitx, …) place their pre-edit / candidate popup at the
// terminal's real cursor. bubbletea hides that cursor and, after every frame,
// parks it at the bottom-left cell, so the popup showed up there instead of at
// the textbox. cursorOut wraps the program's output and, after every write,
// moves the real cursor onto the focused textbox's (virtual) cursor cell.
type cursorOut struct {
	*os.File // keeps Fd(), so bubbletea still detects the TTY and its size

	mu       sync.Mutex
	row, col int  // 1-based screen position of the cursor
	want     bool // whether the real cursor should be visible at row/col
	shown    bool // what we last told the terminal
}

func (c *cursorOut) set(row, col int, want bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.row, c.col, c.want = row, col, want
	c.mu.Unlock()
}

// Write forwards p, then re-applies the cursor position that bubbletea's own
// end-of-frame cursor reset just overwrote.
func (c *cursorOut) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var tail string
	switch {
	case c.want:
		// steady bar so it does not clash with the reverse-video block of the textarea
		tail = fmt.Sprintf("\x1b[%d;%dH\x1b[?25h\x1b[6 q", c.row, c.col)
		c.shown = true
	case c.shown:
		tail = "\x1b[?25l\x1b[0 q"
		c.shown = false
	}
	if tail == "" {
		return c.File.Write(p)
	}
	n, err := c.File.Write(append(append([]byte{}, p...), tail...))
	if n > len(p) {
		n = len(p)
	}
	return n, err
}

// reset restores the user's cursor shape on exit.
func (c *cursorOut) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.File.WriteString("\x1b[0 q")
}

// cursorCell finds the cell of the textarea's virtual cursor (the reverse-video
// cell) inside the rows [y0,y1] of the frame. It returns the 0-based column and row.
func cursorCell(lines []string, y0, y1 int) (x, y int, ok bool) {
	for y = y0; y <= y1 && y < len(lines); y++ {
		l := lines[y]
		i := strings.Index(l, "\x1b[7m")
		if j := strings.Index(l, "\x1b[7;"); i < 0 || (j >= 0 && j < i) {
			i = j
		}
		if i >= 0 {
			return ansi.StringWidth(l[:i]), y, true
		}
	}
	return 0, 0, false
}

// placeCursor tells the output wrapper where the real cursor belongs for this frame.
func (m *Model) placeCursor(lines []string) {
	if m.curOut == nil {
		return
	}
	id := ""
	switch {
	case m.overlay != overlayNone:
	case m.pane == paneInput:
		id = "input"
	case m.pane == paneThreadInput:
		id = "tinput"
	}
	if id == "" || m.selectMode {
		m.curOut.set(0, 0, false)
		return
	}
	z := zone.Get(id)
	if z.IsZero() {
		m.curOut.set(0, 0, false)
		return
	}
	if x, y, ok := cursorCell(lines, z.StartY, z.EndY); ok {
		m.curOut.set(y+1, x+1, true)
		return
	}
	// Cursor cell not drawn (e.g. whole text selected): fall back to the first text cell.
	m.curOut.set(z.StartY+2, z.StartX+3, true)
}
