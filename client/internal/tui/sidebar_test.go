package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func mouse(x, y int, a tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: a, Button: tea.MouseButtonLeft}
}

func sideNow(m *Model) int { s, _, _ := m.layout(); return s }

func TestDragSidebarEdgeResizes(t *testing.T) {
	m, _, _ := newTestModel(t) // 100 wide
	if sideNow(m) != 24 {
		t.Fatalf("default = %d", sideNow(m))
	}
	m.Update(mouse(23, 5, tea.MouseActionPress)) // the border column
	if !m.resizing {
		t.Fatal("pressing the edge should start resizing")
	}
	m.Update(mouse(34, 5, tea.MouseActionMotion))
	if sideNow(m) != 35 {
		t.Fatalf("side = %d, want 35", sideNow(m))
	}
	m.Update(mouse(34, 5, tea.MouseActionRelease))
	if m.resizing || sideNow(m) != 35 {
		t.Fatalf("resizing=%v side=%d", m.resizing, sideNow(m))
	}
	_, cw, tw := m.layout()
	if 35+cw+tw != 100 {
		t.Fatalf("columns don't add up: %d+%d+%d", 35, cw, tw)
	}
}

func TestSidebarWidthIsClamped(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.Update(mouse(23, 5, tea.MouseActionPress))
	m.Update(mouse(2, 5, tea.MouseActionMotion))
	if sideNow(m) != minSideW {
		t.Fatalf("min: %d", sideNow(m))
	}
	m.Update(mouse(90, 5, tea.MouseActionMotion))
	if got := sideNow(m); got != 50 {
		t.Fatalf("max: %d", got)
	}
	m.Update(mouse(90, 5, tea.MouseActionRelease))
	// a narrow window never leaves the chat under 40 cells
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if _, cw, _ := m.layout(); cw < 40 {
		t.Fatalf("chat only %d wide", cw)
	}
}

func TestPressElsewhereDoesNotResize(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.Update(mouse(10, 5, tea.MouseActionPress)) // inside the sidebar
	m.Update(mouse(40, 5, tea.MouseActionMotion))
	if m.resizing || sideNow(m) != 24 {
		t.Fatalf("resizing=%v side=%d", m.resizing, sideNow(m))
	}
}

func TestViewStillFitsAfterResize(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.Update(mouse(23, 5, tea.MouseActionPress))
	m.Update(mouse(40, 5, tea.MouseActionMotion))
	m.Update(mouse(40, 5, tea.MouseActionRelease))
	m.View()
	for i, l := range m.screen {
		if w := lipgloss.Width(l); w != 100 {
			t.Fatalf("row %d is %d cells wide", i, w)
		}
	}
	if len(m.screen) != 30 {
		t.Fatalf("%d rows", len(m.screen))
	}
}
