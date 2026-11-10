package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

func TestBuzzTimeline(t *testing.T) {
	var b buzzState
	t0 := time.Now()
	if b.shake(t0) != 0 || b.anim("x", t0).Active {
		t.Fatal("idle state must not animate")
	}
	b.begin("x", t0)
	if b.shake(t0) == 0 {
		t.Fatal("shake should start immediately")
	}
	if b.shake(t0.Add(buzzShakeDur)) != 0 {
		t.Fatal("shake must stop after 1s")
	}
	if !b.anim("x", t0.Add(1500*time.Millisecond)).Active || b.anim("y", t0).Active {
		t.Fatal("text animates for its own message only")
	}
	if b.anim("x", t0.Add(buzzTextDur)).Active {
		t.Fatal("text animation must stop after 2s")
	}
	if b.next(t0.Add(time.Second)) == nil || b.next(t0.Add(buzzTextDur)) != nil {
		t.Fatal("ticker should run exactly until the text animation ends")
	}
}

func TestShiftLinesKeepsWidth(t *testing.T) {
	in := []string{"abcdef", ansi.Strip("xy")}
	for _, dx := range []int{3, -3, 1, -1} {
		for _, l := range shiftLines(in, dx, 6) {
			if lipgloss.Width(l) > 6 {
				t.Fatalf("dx=%d width %d: %q", dx, lipgloss.Width(l), l)
			}
		}
	}
}

func TestBuzzMessageRendersBannerAndShakes(t *testing.T) {
	m, me, bob := newTestModel(t)
	s, _ := e2e.Seal(chat.BuzzText, bob, map[string][32]byte{"me": me.Public, "bob": bob.Public})
	pm := proto.Message{ID: "z", Conv: "hell", Sender: "bob", CreatedAt: time.Now().UnixMilli(), Nonce: s.Nonce, CT: s.CT, Key: s.Keys["me"]}
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	if m.buzz.id != "z" {
		t.Fatal("incoming buzz should start the animation")
	}
	view := m.View()
	for _, l := range strings.Split(view, "\n") {
		if lipgloss.Width(l) > 100 {
			t.Fatalf("line too wide: %d", lipgloss.Width(l))
		}
	}
	if !strings.Contains(ansi.Strip(view), "Phát 🌂 buzzzz!") {
		t.Fatal("banner missing")
	}
	m.buzz.start = time.Now().Add(-3 * time.Second) // animation over: still shows the banner
	if !strings.Contains(ansi.Strip(m.View()), "buzzzz!") {
		t.Fatal("banner should persist")
	}
}

func TestIamOpensOwnProfile(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.input.SetValue("/iam")
	m.submit()
	if m.overlay != overlayProfile || m.profileUser != "me" {
		t.Fatalf("overlay=%v user=%q", m.overlay, m.profileUser)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Me") {
		t.Fatal("profile should show the display name")
	}
}
