package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

const testRoom = "room:aaaaaaaaaaaa"

func TestAtNamesAndUserArgs(t *testing.T) {
	name, us := atNames("Phòng họp 1 @Bob @eve")
	if name != "Phòng họp 1" || !reflect.DeepEqual(us, []string{"bob", "eve"}) {
		t.Fatalf("%q %v", name, us)
	}
	if name, us := atNames("a@b"); name != "a@b" || us != nil {
		t.Fatalf("%q %v", name, us)
	}
	if name, _ := atNames("@"); name != "@" {
		t.Fatal("a lone @ is part of the name")
	}
	if got := userArgs("@Bob eve"); !reflect.DeepEqual(got, []string{"bob", "eve"}) {
		t.Fatal(got)
	}
}

func lastRoomSend(f *fakeLink) any {
	for i := len(f.sent) - 1; i >= 0; i-- {
		if _, ok := f.sent[i].(proto.Send); !ok {
			return f.sent[i]
		}
	}
	return nil
}

func joinRoom(m *Model, owner string) {
	m.onEvent(proto.Event{T: "room", Room: &proto.Room{ID: testRoom, Name: "Dev", Owner: owner, Members: []string{"me", "bob"}},
		Note: &proto.Note{Kind: "created", By: "bob", Users: []string{"me"}}})
}

func TestRoomCommandsSendProtocolMessages(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "/room Dev team @bob")
	rc, ok := lastRoomSend(f).(proto.RoomCreate)
	if !ok || rc.Name != "Dev team" || !reflect.DeepEqual(rc.Members, []string{"bob"}) || rc.T != "room_create" {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
	if m.input.Value() != "" {
		t.Fatal("input should be cleared")
	}
	// the created event answering our ref switches to the room
	m.onEvent(proto.Event{T: "room", Ref: rc.Ref, Room: &proto.Room{ID: testRoom, Name: "Dev team", Owner: "me", Members: []string{"me", "bob"}}, Note: &proto.Note{Kind: "created", By: "me", Users: []string{"bob"}}})
	if m.st.Active() != testRoom {
		t.Fatalf("active = %q", m.st.Active())
	}
	typeAndEnter(m, "/invite @carol @dave")
	if iv, ok := lastRoomSend(f).(proto.RoomInvite); !ok || iv.Room != testRoom || !reflect.DeepEqual(iv.Users, []string{"carol", "dave"}) {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
	typeAndEnter(m, "/kick @bob")
	if k, ok := lastRoomSend(f).(proto.RoomKick); !ok || k.User != "bob" || k.Room != testRoom {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
	typeAndEnter(m, "/roomname Mới nè")
	if n, ok := lastRoomSend(f).(proto.RoomName); !ok || n.Name != "Mới nè" {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
	typeAndEnter(m, "/leave")
	if l, ok := lastRoomSend(f).(proto.RoomOp); !ok || l.T != "room_leave" || l.Room != testRoom {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
}

func TestRoomCommandsNeedAnOpenRoom(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	for _, c := range []string{"/invite @bob", "/kick @bob", "/leave", "/closeroom", "/roomname x", "/members"} {
		typeAndEnter(m, c)
		if m.status != "Open a room first." || len(f.sent) != 0 {
			t.Fatalf("%s: status %q sent %v", c, m.status, f.sent)
		}
		m.status = ""
	}
	typeAndEnter(m, "/room")
	if !strings.HasPrefix(m.status, "Usage: /room") {
		t.Fatalf("status %q", m.status)
	}
}

func TestCloseRoomAsksFirst(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	joinRoom(m, "me")
	m.switchConv(testRoom)
	typeAndEnter(m, "/closeroom")
	if m.overlay != overlayClose || len(f.sent) != 0 {
		t.Fatalf("overlay %v sent %v", m.overlay, f.sent)
	}
	if !strings.Contains(strings.Join(m.overlayView(), "\n"), "Close Dev?") {
		t.Fatal("confirmation text missing")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.overlay != overlayNone || len(f.sent) != 0 {
		t.Fatal("any other key must cancel")
	}
	typeAndEnter(m, "/closeroom")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if op, ok := lastRoomSend(f).(proto.RoomOp); !ok || op.T != "room_close" {
		t.Fatalf("sent %+v", lastRoomSend(f))
	}
}

func TestRemovedFromOpenRoomFallsBackToHell(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.conn = &fakeLink{}
	joinRoom(m, "bob")
	m.switchConv(testRoom)
	m.onEvent(proto.Event{T: "room_gone", ID: testRoom, Reason: "kicked", By: "bob"})
	if m.st.Active() != "hell" || !strings.Contains(m.status, "removed you from Dev") {
		t.Fatalf("active %q status %q", m.st.Active(), m.status)
	}
}

func TestRoomErrorsUseRoomWording(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "/room X @ghost")
	rc := lastRoomSend(f).(proto.RoomCreate)
	m.onEvent(proto.Event{T: "error", Ref: rc.Ref, Error: "unknown_user", Detail: "ghost"})
	if m.status != "No such user: ghost" {
		t.Fatalf("status %q", m.status)
	}
}

func TestRoomShownInSidebarMembersAndTitle(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.conn = &fakeLink{}
	joinRoom(m, "bob")
	m.switchConv(testRoom)
	view := m.View()
	for _, want := range []string{"Rooms", "◈ Dev", "2 members"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view lacks %q", want)
		}
	}
	m.overlay = overlayMembers
	box := strings.Join(m.overlayView(), "\n")
	if !strings.Contains(box, "@bob") || !strings.Contains(box, "owner") {
		t.Fatalf("members overlay: %s", box)
	}
}

func TestRoomHeaderListsMembersWithOwnerAndSeparator(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.conn = &fakeLink{}
	joinRoom(m, "bob")
	m.switchConv(testRoom)
	lines := strings.Split(m.View(), "\n")
	plain := func(i int) string { return m.screen[i] }
	var title int
	for i := range lines {
		if strings.Contains(plain(i), "◈ Dev") && strings.Contains(plain(i), "2 members") {
			title = i
		}
	}
	if title == 0 && !strings.Contains(plain(0), "◈ Dev") {
		t.Fatal("room title not found")
	}
	if got := plain(title + 1); !strings.Contains(got, "[ ") || !strings.Contains(got, "● Me") || !strings.Contains(got, "● Phát 🌂 (owner)") {
		t.Fatalf("member line = %q", got)
	}
	if got := plain(title + 2); !strings.Contains(got, "────────") {
		t.Fatalf("separator = %q", got)
	}
	// Hell has neither
	m.switchConv("hell")
	m.View()
	for i := range m.screen {
		if strings.Contains(m.screen[i], "(owner)") {
			t.Fatal("hell must not list members")
		}
	}
}
