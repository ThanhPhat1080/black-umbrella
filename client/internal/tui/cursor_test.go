package tui

import (
	"encoding/base64"
	"fmt"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestRealCursorFollowsTextbox(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256) // the virtual cursor is reverse video, which needs colour support
	defer lipgloss.SetColorProfile(old)

	m, _, _ := newTestModel(t)
	m.curOut = &cursorOut{}
	m.input.SetValue("xin chào")
	view := m.View()

	if !m.curOut.want {
		t.Fatal("real cursor should be shown while the main textbox is focused")
	}
	lines := strings.Split(ansi.Strip(view), "\n")
	row, col := m.curOut.row-1, m.curOut.col-1
	if row >= len(lines) || !strings.Contains(lines[row], "xin chào") {
		t.Fatalf("cursor row %d is not the textbox row; line=%q", row, lines[min(row, len(lines)-1)])
	}
	// cursor sits right after the typed text
	if got := ansi.StringWidth(lines[row][:strings.Index(lines[row], "xin chào")]) + ansi.StringWidth("xin chào"); got != col {
		t.Fatalf("cursor col %d, want %d", col, got)
	}

	m.overlay = overlayHelp
	m.View()
	if m.curOut.want {
		t.Fatal("real cursor must be hidden while an overlay is open")
	}
}

func TestCtrlCAsksBeforeQuitting(t *testing.T) {
	isQuit := func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}
	m, _, _ := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if isQuit(cmd) || m.overlay != overlayQuit {
		t.Fatal("first Ctrl+C must only open the confirmation")
	}
	if !strings.Contains(ansi.Strip(strings.Join(m.overlayView(), "\n")), "Quit Umbrella?") {
		t.Fatal("confirmation not rendered")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.overlay != overlayNone {
		t.Fatal("another key should dismiss the confirmation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Fatal("second Ctrl+C must quit")
	}
	m.overlay = overlayQuit
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter}); !isQuit(cmd) {
		t.Fatal("Enter must confirm")
	}
	m.overlay, m.fatal = overlayNone, "gone"
	if _, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !isQuit(cmd) {
		t.Fatal("Ctrl+C quits at once when the connection is permanently gone")
	}
}

func TestSidebarPetStaysAndListScrolls(t *testing.T) {
	m := sidebarModel(t)
	for i := 0; i < 30; i++ {
		u := fmt.Sprintf("user%02d", i)
		kp, _ := e2e.GenerateKeyPair()
		m.onEvent(proto.Event{T: "user", User: &proto.User{Username: u, DisplayName: u, PublicKey: base64.StdEncoding.EncodeToString(kp.Public[:])}})
	}
	m.st.SetActive("dm:me|user29")
	var plain []string
	for _, r := range strings.Split(m.viewSidebar(24, 33), "\n") {
		plain = append(plain, ansi.Strip(r))
	}
	if len(plain) != 33 {
		t.Fatalf("sidebar has %d rows, want 33", len(plain))
	}
	all := strings.Join(plain, "\n")
	if !strings.Contains(all, "user29") {
		t.Fatalf("active user scrolled out of view:\n%s", all)
	}
	if !strings.Contains(all, "/\\_/\\") {
		t.Fatalf("pet missing:\n%s", all)
	}
	if strings.Contains(strings.Join(plain[:29], "\n"), "/\\_/\\") {
		t.Fatalf("pet must sit at the bottom:\n%s", all)
	}
	// separator line between Channel and User
	if !strings.Contains(all, "──") {
		t.Fatalf("separator missing")
	}
}
