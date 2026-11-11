package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestInputFooterShowsHelpHintAndRightAlignedCounter(t *testing.T) {
	m, _, _ := newTestModel(t)
	if m.input.Placeholder != "" {
		t.Fatalf("the textbox placeholder should be gone, got %q", m.input.Placeholder)
	}
	var foot, borderRow string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, "[+]") {
			foot = l
		}
		if strings.Contains(l, "╰") {
			borderRow = l
		}
	}
	if !strings.Contains(foot, "[+] kaomoji  /h for shortcut") {
		t.Fatalf("footer = %q", foot)
	}
	i := strings.Index(foot, "0/4000")
	if i < 0 || strings.TrimRight(foot[i:], " │") != "0/4000" {
		t.Fatalf("the counter should end the row: %q", foot)
	}
	// its right end lines up with (or sits just inside) the right border of the box
	end := lipgloss.Width(foot[:i]) + len("0/4000")
	right := strings.LastIndex(borderRow, "╯")
	if right < 0 {
		t.Fatalf("no box corner in %q", borderRow)
	}
	boxRight := lipgloss.Width(borderRow[:right]) + 1
	if end > boxRight || boxRight-end > 2 {
		t.Fatalf("counter ends at col %d, box border at col %d\n%s\n%s", end, boxRight, borderRow, foot)
	}
}
