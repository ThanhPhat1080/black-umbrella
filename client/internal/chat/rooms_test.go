package chat

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

func roomState(t *testing.T) *State {
	t.Helper()
	kp, err := e2e.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	s := New("alice", kp)
	now := time.Unix(1000, 0)
	s.Now = func() time.Time { return now }
	return s
}

func room(id, name, owner string, members ...string) *proto.Room {
	return &proto.Room{ID: id, Name: name, Owner: owner, Members: members}
}

func TestHelloRoomsAndSidebarOrder(t *testing.T) {
	s := roomState(t)
	s.Apply(proto.Event{T: "hello", Me: "alice",
		Users: []proto.User{{Username: "alice"}, {Username: "bobby", DisplayName: "Bob"}},
		Rooms: []proto.Room{*room("room:aaaaaaaaaaaa", "Dev", "alice", "alice", "bobby"), *room("room:bbbbbbbbbbbb", "Ops", "bobby", "alice", "bobby")}})
	sb := s.Sidebar()
	if len(sb) != 4 || sb[0].Conv != "hell" || sb[1].Conv != "room:aaaaaaaaaaaa" || sb[2].Conv != "room:bbbbbbbbbbbb" || sb[3].Username != "bobby" {
		t.Fatalf("sidebar = %+v", sb)
	}
	if !sb[1].Room || sb[1].Label != "Dev" || sb[3].Room {
		t.Fatalf("room flags wrong: %+v", sb)
	}
}

func TestRoomEventsAndNotices(t *testing.T) {
	s := roomState(t)
	s.Apply(proto.Event{T: "hello", Me: "alice", Users: []proto.User{{Username: "alice"}, {Username: "bobby", DisplayName: "Bob"}, {Username: "carol"}}})
	// someone else adds me
	fx := s.Apply(proto.Event{T: "room", Room: room("room:aaaaaaaaaaaa", "Dev", "bobby", "alice", "bobby"), Note: &proto.Note{Kind: "created", By: "bobby", Users: []string{"alice"}}})
	if fx.Status != "Bob added you to Dev" {
		t.Fatalf("status = %q", fx.Status)
	}
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now()); got != "Bob created the room" {
		t.Fatalf("notice = %q", got)
	}
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now().Add(time.Minute)); got != "" {
		t.Fatalf("notice should expire, got %q", got)
	}
	// a member is added
	s.Apply(proto.Event{T: "room", Room: room("room:aaaaaaaaaaaa", "Dev", "bobby", "alice", "bobby", "carol"), Note: &proto.Note{Kind: "added", By: "bobby", Users: []string{"carol"}}})
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now()); got != "Bob added carol" {
		t.Fatalf("notice = %q", got)
	}
	if r, _ := s.RoomOf("room:aaaaaaaaaaaa"); len(r.Members) != 3 {
		t.Fatalf("members = %v", r.Members)
	}
	// rename, leave with a new owner, kick
	s.Apply(proto.Event{T: "room", Room: room("room:aaaaaaaaaaaa", "Ops", "bobby", "alice", "bobby", "carol"), Note: &proto.Note{Kind: "renamed", By: "bobby", Name: "Ops"}})
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now()); got != "Bob renamed the room to Ops" {
		t.Fatalf("notice = %q", got)
	}
	s.Apply(proto.Event{T: "room", Room: room("room:aaaaaaaaaaaa", "Ops", "alice", "alice", "carol"), Note: &proto.Note{Kind: "left", By: "bobby", Users: []string{"bobby"}, NewOwner: "alice"}})
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now()); got != "Bob left. You are the owner now" {
		t.Fatalf("notice = %q", got)
	}
	s.Apply(proto.Event{T: "room", Room: room("room:aaaaaaaaaaaa", "Ops", "alice", "alice"), Note: &proto.Note{Kind: "kicked", By: "alice", Users: []string{"carol"}}})
	if got := s.Notice("room:aaaaaaaaaaaa", s.Now()); got != "You removed carol" {
		t.Fatalf("notice = %q", got)
	}
}

func TestRoomGoneDropsEverything(t *testing.T) {
	s := roomState(t)
	id := "room:aaaaaaaaaaaa"
	s.Apply(proto.Event{T: "hello", Me: "alice", Users: []proto.User{{Username: "alice"}, {Username: "bobby", DisplayName: "Bob"}}, Rooms: []proto.Room{*room(id, "Dev", "bobby", "alice", "bobby")}})
	s.msgs["m1"] = &Msg{ID: "m1", Conv: id}
	s.msgs["m2"] = &Msg{ID: "m2", Conv: "hell"}
	s.unread[id] = 3
	s.SetActive(id)
	fx := s.Apply(proto.Event{T: "room_gone", ID: id, Reason: "kicked", By: "bobby"})
	if fx.Gone != id || fx.Status != "Bob removed you from Dev" {
		t.Fatalf("fx = %+v", fx)
	}
	if _, ok := s.msgs["m1"]; ok {
		t.Fatal("room messages should be dropped")
	}
	if _, ok := s.msgs["m2"]; !ok {
		t.Fatal("other messages must stay")
	}
	if len(s.Sidebar()) != 2 {
		t.Fatalf("sidebar = %+v", s.Sidebar())
	}
	if s.Active() != "hell" {
		t.Fatalf("active = %q, want hell", s.Active())
	}
	s.Apply(proto.Event{T: "hello", Me: "alice", Users: []proto.User{{Username: "bobby", DisplayName: "Bob"}}, Rooms: []proto.Room{*room(id, "Dev", "bobby", "alice", "bobby")}})
	fx = s.Apply(proto.Event{T: "room_gone", ID: id, Reason: "closed", By: "bobby"})
	if fx.Status != "Dev was closed by Bob" {
		t.Fatalf("status = %q", fx.Status)
	}
}

func TestRoomRecipientsAndMentions(t *testing.T) {
	s := roomState(t)
	id := "room:aaaaaaaaaaaa"
	bk, _ := e2e.GenerateKeyPair()
	pub := func(k [32]byte) string { return b64(k[:]) }
	ck, _ := e2e.GenerateKeyPair()
	s.Apply(proto.Event{T: "hello", Me: "alice",
		Users: []proto.User{{Username: "alice", PublicKey: pub(s.kp.Public)}, {Username: "bobby", PublicKey: pub(bk.Public)}, {Username: "carol", PublicKey: pub(ck.Public)}},
		Rooms: []proto.Room{*room(id, "Dev", "alice", "alice", "bobby")}})
	rec := s.Recipients(id)
	if len(rec) != 2 || rec["bobby"] != bk.Public {
		t.Fatalf("recipients = %v", rec)
	}
	if s.Recipients("room:ffffffffffff")["carol"] != ([32]byte{}) {
		t.Fatal("unknown room must not leak other users")
	}
	// mention in a room rings the bell
	sealed, err := e2e.Seal("hi @alice", bk, map[string][32]byte{"alice": s.kp.Public, "bobby": bk.Public})
	if err != nil {
		t.Fatal(err)
	}
	fx := s.Apply(proto.Event{T: "msg", Message: &proto.Message{ID: "x1", Conv: id, Sender: "bobby", SenderKey: pub(bk.Public), CreatedAt: 5, Nonce: sealed.Nonce, CT: sealed.CT, Key: sealed.Keys["alice"]}})
	if !fx.Bell {
		t.Fatal("room mention should ring the bell")
	}
	if s.unread[id] != 1 {
		t.Fatalf("unread = %d", s.unread[id])
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
