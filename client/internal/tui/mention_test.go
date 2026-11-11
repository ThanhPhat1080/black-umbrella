package tui

import (
	"time"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

func TestMentionQuery(t *testing.T) {
	cases := []struct {
		before string
		q      string
		ok     bool
	}{
		{"@", "", true},
		{"hi @bo", "bo", true},
		{"hi\n@bo", "bo", true},
		{"mail me at a@b", "", false}, // @ inside a word is not a mention
		{"hi @bob ", "", false},       // finished with a space
		{"hi", "", false},
		{"@Bob_1", "Bob_1", true},
	}
	for _, c := range cases {
		q, _, ok := mentionQuery([]rune(c.before))
		if ok != c.ok || (ok && q != c.q) {
			t.Errorf("%q -> %q %v, want %q %v", c.before, q, ok, c.q, c.ok)
		}
	}
}

func mentionModel(t *testing.T) *Model {
	m, _, _ := newTestModel(t)
	m.conn = &fakeLink{}
	joinCarol(t, m)
	return m
}

func kmsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeKeys(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(kmsg(k))
	}
}

func candNames(m *Model) []string {
	var n []string
	for _, c := range m.mentionCands() {
		n = append(n, c.user)
	}
	return n
}

func TestMentionPopupListsUsersAndFilters(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "h", "i", " ", "@")
	if got := candNames(m); !reflect.DeepEqual(got, []string{"bob", "carol"}) { // never myself
		t.Fatalf("cands = %v", got)
	}
	typeKeys(m, "c")
	if got := candNames(m); !reflect.DeepEqual(got, []string{"carol"}) {
		t.Fatalf("cands = %v", got)
	}
	typeKeys(m, "z")
	if len(candNames(m)) != 0 {
		t.Fatal("no match -> no popup")
	}
}

func TestMentionAcceptWithEnterAndTab(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "h", "i", " ", "@", "b", "down", "up", "enter") // enter accepts, does not send
	if m.input.Value() != "hi @bob " {
		t.Fatalf("value %q", m.input.Value())
	}
	if len(m.conn.(*fakeLink).sends()) != 0 {
		t.Fatal("Enter must not send while the popup is open")
	}
	m.input.Reset()
	typeKeys(m, "@", "down", "tab") // second candidate
	if m.input.Value() != "@carol " {
		t.Fatalf("value %q", m.input.Value())
	}
	typeKeys(m, "enter") // popup gone: now Enter sends
	if len(m.conn.(*fakeLink).sends()) != 1 {
		t.Fatal("Enter should send once the popup is closed")
	}
}

func TestMentionEscDismissesAndKeepsText(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "@", "b", "esc")
	if m.input.Value() != "@b" || len(candNames(m)) != 0 {
		t.Fatalf("value %q cands %v", m.input.Value(), candNames(m))
	}
	typeKeys(m, "o") // stays dismissed for this @
	if len(candNames(m)) != 0 {
		t.Fatal("popup should stay closed after Esc")
	}
	typeKeys(m, " ", "@")
	if len(candNames(m)) == 0 {
		t.Fatal("a new @ opens it again")
	}
}

func TestMentionBackspaceRemovesAtAndPopup(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "@")
	if len(candNames(m)) == 0 {
		t.Fatal("popup expected")
	}
	typeKeys(m, "backspace")
	if m.input.Value() != "" || len(candNames(m)) != 0 {
		t.Fatalf("value %q cands %v", m.input.Value(), candNames(m))
	}
}

func TestMentionScopedToRoomAndDM(t *testing.T) {
	m := mentionModel(t)
	m.onEvent(proto.Event{T: "room", Room: &proto.Room{ID: testRoom, Name: "Dev", Owner: "me", Members: []string{"me", "bob"}}, Note: &proto.Note{Kind: "created", By: "me"}})
	m.switchConv(testRoom)
	typeKeys(m, "@")
	if got := candNames(m); !reflect.DeepEqual(got, []string{"bob"}) {
		t.Fatalf("room cands = %v", got)
	}
	m.input.Reset()
	m.switchConv("dm:bob|me")
	typeKeys(m, "@")
	if got := candNames(m); !reflect.DeepEqual(got, []string{"bob"}) {
		t.Fatalf("dm cands = %v", got)
	}
}

func TestMentionPopupIsDrawn(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "@")
	m.View()
	screen := strings.Join(m.screen, "\n")
	if !strings.Contains(screen, "@bob") || !strings.Contains(screen, "@carol") {
		t.Fatalf("popup missing:\n%s", screen)
	}
	typeKeys(m, "esc")
	m.View()
	if strings.Contains(strings.Join(m.screen, "\n"), "@carol") {
		t.Fatal("popup should be gone after Esc")
	}
}

func TestMentionClosedOnceNameIsComplete(t *testing.T) {
	m := mentionModel(t)
	typeKeys(m, "/", "w", "h", "o", "i", "s", " ", "@", "b", "o", "b")
	if len(candNames(m)) != 0 {
		t.Fatal("a complete @name must not hijack Enter")
	}
}

func TestNudgeTimelineIsGentle(t *testing.T) {
	var n nudgeState
	t0 := time.Now()
	if n.shake(t0) != 0 || n.next(t0) != nil {
		t.Fatal("idle must not shake")
	}
	n.begin(t0)
	for d := time.Duration(0); d < nudgeDur; d += 10 * time.Millisecond {
		if x := n.shake(t0.Add(d)); x < -1 || x > 1 {
			t.Fatalf("offset %d at %v is too strong", x, d)
		}
	}
	if n.shake(t0) == 0 {
		t.Fatal("should start immediately")
	}
	if n.shake(t0.Add(nudgeDur)) != 0 || n.next(t0.Add(nudgeDur)) != nil {
		t.Fatal("must stop")
	}
}

func TestMentionNudgesButPlainMessageDoesNot(t *testing.T) {
	m, me, bob := newTestModel(t)
	send := func(id, text string) {
		s, _ := e2e.Seal(text, bob, map[string][32]byte{"me": me.Public, "bob": bob.Public})
		pm := proto.Message{ID: id, Conv: "hell", Sender: "bob", CreatedAt: time.Now().UnixMilli(), Nonce: s.Nonce, CT: s.CT, Key: s.Keys["me"]}
		m.onEvent(proto.Event{T: "msg", Message: &pm})
	}
	send("a", "hello there")
	if !m.nudge.start.IsZero() {
		t.Fatal("no nudge without a mention")
	}
	send("b", "hey @me look")
	if m.nudge.start.IsZero() || m.nudge.shake(m.nudge.start) == 0 {
		t.Fatal("mention should nudge")
	}
	if m.buzz.id != "" {
		t.Fatal("a mention is not a buzz")
	}
}
