package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

func motion(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonNone}
}

// roomWithMention opens a room with bob and shows a message mentioning bob in it.
func roomWithMention(t *testing.T) (*Model, string) {
	m, me, bob := newTestModel(t)
	m.conn = &fakeLink{}
	joinRoom(m, "bob")
	m.switchConv(testRoom)
	pm := sealedMsg(t, me, bob, "me", "m1", testRoom, "", "hi @bob and @nobody")
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	return m, "men:m1:0"
}

func TestMentionChipShowsDotAndIsPlainForUnknownUsers(t *testing.T) {
	m, _ := roomWithMention(t)
	m.View()
	screen := strings.Join(m.screen, "\n")
	if !strings.Contains(screen, "● @bob") {
		t.Fatalf("chip missing:\n%s", screen)
	}
	if strings.Contains(screen, "● @nobody") {
		t.Fatal("unknown users get no chip")
	}
	if !strings.Contains(screen, " @nobody") {
		t.Fatal("unknown mention should stay as typed")
	}
}

func TestClickingMentionChipOpensThatChat(t *testing.T) {
	m, id := roomWithMention(t)
	z := waitZone(t, m, id)
	m.Update(click(z.StartX+2, z.StartY))
	if m.st.Active() != "dm:bob|me" {
		t.Fatalf("active = %q", m.st.Active())
	}
}

func TestClickingRoomMemberOpensThatChat(t *testing.T) {
	m, _ := roomWithMention(t)
	z := waitZone(t, m, "rm:bob")
	m.Update(click(z.StartX+3, z.StartY))
	if m.st.Active() != "dm:bob|me" {
		t.Fatalf("member click: active = %q", m.st.Active())
	}
}

func TestClickingYourOwnMentionOrNameShowsYourProfile(t *testing.T) {
	m, _ := roomWithMention(t)
	z := waitZone(t, m, "nm:m1") // m1 was sent by "me"
	m.Update(click(z.StartX, z.StartY))
	if m.overlay != overlayProfile || m.profileUser != "me" {
		t.Fatalf("overlay=%v user=%q", m.overlay, m.profileUser)
	}
}

func TestHoverTracksClickableZones(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256) // tests run without a terminal: switch styling on
	defer lipgloss.SetColorProfile(old)
	m, id := roomWithMention(t)
	z := waitZone(t, m, id)
	m.Update(motion(z.StartX+1, z.StartY))
	if m.hover != id {
		t.Fatalf("hover = %q, want %q", m.hover, id)
	}
	// the hovered chip is drawn differently (underlined) from the same frame without hover
	hovered := m.View()
	m.hover = ""
	if plain := m.View(); plain == hovered {
		t.Fatal("hovered chip should look different")
	}
	m.Update(motion(z.StartX+1, z.StartY))
	m.Update(motion(0, m.h-1)) // the status line is not clickable
	if m.hover != "" {
		t.Fatalf("hover = %q, want none", m.hover)
	}
	sz := waitZone(t, m, "side:dm:bob|me")
	m.Update(motion(sz.StartX+2, sz.StartY))
	if m.hover != "side:dm:bob|me" {
		t.Fatalf("sidebar hover = %q", m.hover)
	}
}

func TestPointerShapeSequences(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "tty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c := &cursorOut{File: f}
	c.setPointer(true)
	c.setPointer(true) // no change: nothing written
	c.setPointer(false)
	b, _ := os.ReadFile(f.Name())
	if got, want := string(b), "\x1b]22;pointer\x1b\\\x1b]22;default\x1b\\"; got != want {
		t.Fatalf("wrote %q, want %q", got, want)
	}
	(*cursorOut)(nil).setPointer(true) // nil-safe (tests run without a terminal)
}

func mustMsg(text string) chat.Msg { return chat.Msg{ID: "z", Conv: "hell", Sender: "bob", Text: text} }

func TestChipsKeepLinesWithinWidth(t *testing.T) {
	m, _ := roomWithMention(t)
	look := m.chipLookup()
	for w := 12; w < 40; w++ {
		for _, l := range renderMessage(mustMsg("see @bob @bob @bob @bob and @bob!"), msgView{Name: "X", Me: "me", Width: w, User: look}) {
			if cw := ansi.StringWidth(l); cw > w {
				t.Fatalf("width %d: line is %d cells: %q", w, cw, ansi.Strip(l))
			}
		}
	}
}
