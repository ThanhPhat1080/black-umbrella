package chat

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

type actor struct {
	name string
	kp   e2e.KeyPair
}

func newActor(t *testing.T, name string) actor {
	t.Helper()
	kp, err := e2e.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return actor{name, kp}
}

func (a actor) user(online bool) proto.User {
	return proto.User{Username: a.name, DisplayName: "Name " + a.name, PublicKey: base64.StdEncoding.EncodeToString(a.kp.Public[:]), Online: online}
}

var seq int

// msgFor seals text from `from` to everyone in all and returns the wire message for `to`.
func msgFor(t *testing.T, from actor, to string, all []actor, conv, parent, text string) proto.Message {
	t.Helper()
	rec := map[string][32]byte{}
	for _, a := range all {
		rec[a.name] = a.kp.Public
	}
	s, err := e2e.Seal(text, from.kp, rec)
	if err != nil {
		t.Fatal(err)
	}
	seq++
	return proto.Message{ID: string(rune('a'+seq/26)) + string(rune('a'+seq%26)), Conv: conv, Sender: from.name, ParentID: parent,
		CreatedAt: int64(1000 + seq), Nonce: s.Nonce, CT: s.CT, Key: s.Keys[to]}
}

func setup(t *testing.T) (*State, actor, actor, actor, actor) {
	me, bob, zed, amy := newActor(t, "me"), newActor(t, "bob"), newActor(t, "zed"), newActor(t, "amy")
	st := New("me", me.kp)
	st.Apply(proto.Event{T: "hello", Me: "me", Users: []proto.User{me.user(true), bob.user(true), zed.user(true), amy.user(false)}})
	return st, me, bob, zed, amy
}

func TestHelloReplacesNoDuplicates(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	all := []actor{me, bob}
	m := msgFor(t, bob, "me", all, "hell", "", "hi")
	hello := proto.Event{T: "hello", Me: "me", Users: []proto.User{me.user(true), bob.user(true)}, Messages: []proto.Message{m}}
	st.Apply(hello)
	st.Apply(hello)
	tl := st.Timeline("hell")
	if len(tl) != 1 || tl[0].Text != "hi" || tl[0].Undecryptable {
		t.Fatalf("%+v", tl)
	}
	// a live msg duplicate of the same id is ignored too
	st.Apply(proto.Event{T: "msg", Message: &m})
	if len(st.Timeline("hell")) != 1 {
		t.Fatal("duplicate")
	}
}

func TestUnreadAndMentions(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	all := []actor{me, bob}
	st.SetActive("dm:bob|me")
	m1 := msgFor(t, bob, "me", all, "hell", "", "plain")
	if e := st.Apply(proto.Event{T: "msg", Message: &m1}); e.Bell {
		t.Fatal("bell on plain")
	}
	m2 := msgFor(t, bob, "me", all, "hell", "", "hey @me look")
	if e := st.Apply(proto.Event{T: "msg", Message: &m2}); !e.Bell {
		t.Fatal("no bell on mention")
	}
	if u := unread(st, "hell"); u != 2 {
		t.Fatalf("unread %d", u)
	}
	st.SetActive("hell")
	if u := unread(st, "hell"); u != 0 {
		t.Fatalf("unread after active %d", u)
	}
	// own messages never count
	mine := msgFor(t, me, "me", all, "dm:bob|me", "", "x @me")
	st.SetActive("hell")
	if e := st.Apply(proto.Event{T: "msg", Message: &mine}); e.Bell || unread(st, "dm:bob|me") != 0 {
		t.Fatal("own message counted")
	}
}

func unread(st *State, conv string) int {
	for _, e := range st.Sidebar() {
		if e.Conv == conv {
			return e.Unread
		}
	}
	return -1
}

func TestMentionsFn(t *testing.T) {
	cases := map[string]bool{"@me hi": true, "hi @me": true, "(@me)": true, "@meow": false, "x@me": false, "@ME": true, "": false}
	for in, want := range cases {
		if Mentions(in, "me") != want {
			t.Errorf("%q", in)
		}
	}
}

func TestExpiredAndReplyCounts(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	all := []actor{me, bob}
	root := msgFor(t, bob, "me", all, "hell", "", "root")
	r1 := msgFor(t, me, "me", all, "hell", root.ID, "r1")
	r2 := msgFor(t, bob, "me", all, "hell", root.ID, "r2")
	for _, m := range []proto.Message{root, r1, r2} {
		m := m
		st.Apply(proto.Event{T: "msg", Message: &m})
	}
	if tl := st.Timeline("hell"); len(tl) != 1 || tl[0].Replies != 2 {
		t.Fatalf("%+v", tl)
	}
	st.Apply(proto.Event{T: "expired", IDs: []string{r1.ID}})
	if tl := st.Timeline("hell"); tl[0].Replies != 1 {
		t.Fatalf("%+v", tl)
	}
	_, replies := st.Thread(root.ID)
	if len(replies) != 1 || replies[0].Text != "r2" {
		t.Fatalf("%+v", replies)
	}
}

func TestExpiredRootPlaceholder(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	all := []actor{me, bob}
	root := msgFor(t, bob, "me", all, "hell", "", "root")
	rep := msgFor(t, bob, "me", all, "hell", root.ID, "kept reply")
	st.Apply(proto.Event{T: "hello", Me: "me", Users: []proto.User{me.user(true), bob.user(true)}, Messages: []proto.Message{rep}})
	r, replies := st.Thread(root.ID)
	if r == nil || r.Text != "[message expired]" || len(replies) != 1 {
		t.Fatalf("%+v %+v", r, replies)
	}
	// unknown thread entirely: still no nil deref
	if r, rs := st.Thread("nope"); r == nil || len(rs) != 0 {
		t.Fatal("nil root")
	}
	// the orphan reply stays reachable from the timeline
	tl := st.Timeline("hell")
	if len(tl) != 1 || tl[0].Text != "kept reply" || tl[0].ParentID != root.ID {
		t.Fatalf("%+v", tl)
	}
}

func TestKept(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	m := msgFor(t, bob, "me", []actor{me, bob}, "hell", "", "x")
	st.Apply(proto.Event{T: "msg", Message: &m})
	st.Apply(proto.Event{T: "kept", ID: m.ID, Kept: true})
	if !st.Timeline("hell")[0].Kept {
		t.Fatal("not kept")
	}
	st.Apply(proto.Event{T: "kept", ID: m.ID, Kept: false})
	if st.Timeline("hell")[0].Kept {
		t.Fatal("still kept")
	}
}

func TestSidebarOrder(t *testing.T) {
	st, _, _, _, _ := setup(t)
	var got []string
	for _, e := range st.Sidebar() {
		got = append(got, e.Conv)
	}
	want := []string{"hell", "dm:bob|me", "dm:me|zed", "dm:amy|me"}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v != %v", got, want)
		}
	}
}

func TestTypingExpires(t *testing.T) {
	st, _, _, _, _ := setup(t)
	now := time.Unix(1000, 0)
	st.Now = func() time.Time { return now }
	st.Apply(proto.Event{T: "typing", Conv: "hell", Username: "bob"})
	st.Apply(proto.Event{T: "typing", Conv: "hell", Username: "me"})
	if got := st.Typing("hell", now.Add(3*time.Second)); len(got) != 1 || got[0] != "bob" {
		t.Fatalf("%v", got)
	}
	if got := st.Typing("hell", now.Add(4*time.Second+time.Millisecond)); len(got) != 0 {
		t.Fatalf("%v", got)
	}
}

func TestRecipients(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	r := st.Recipients("hell")
	if len(r) != 4 || r["bob"] != bob.kp.Public || r["me"] != me.kp.Public {
		t.Fatalf("%v", r)
	}
	if d := st.Recipients("dm:bob|me"); len(d) != 2 || d["bob"] != bob.kp.Public {
		t.Fatalf("%v", d)
	}
	if DMConv("me", "bob") != "dm:bob|me" {
		t.Fatal("dm conv")
	}
	// a user without a key is excluded from hell
	st.Apply(proto.Event{T: "user", User: &proto.User{Username: "nokey", DisplayName: "n"}})
	if len(st.Recipients("hell")) != 4 {
		t.Fatal("keyless user included")
	}
}

func TestUserEventAndUndecryptable(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	evil := newActor(t, "bob")
	m := msgFor(t, evil, "me", []actor{me, evil}, "hell", "", "forged")
	st.Apply(proto.Event{T: "msg", Message: &m})
	tl := st.Timeline("hell")
	if len(tl) != 1 || !tl[0].Undecryptable {
		t.Fatalf("%+v", tl)
	}
	nu := bob.user(false)
	nu.DisplayName = "Robert"
	st.Apply(proto.Event{T: "user", User: &nu})
	if st.Users()["bob"].DisplayName != "Robert" || st.Users()["bob"].Online {
		t.Fatal("user not updated")
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b]0;X\x07\x1b[2Jhi", "hi"},
		{"a\x1b[31mred\x1b[0m", "ared"},
		{"x\x1b]8;;http://e\x1b\\link\x1b]8;;\x1b\\", "xlink"},
		{"bell\x07 nul\x00 del\x7f c1\u009b31m", "bell nul del c131m"},
		{"tab\there", "tab    here"},
		{"line1\r\nline2", "line1\nline2"},
		{"Tiếng Việt 🌂", "Tiếng Việt 🌂"},
		{"rtl‮evil", "rtlevil"},
	}
	for _, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("Sanitize(%q) = %q want %q", c.in, got, c.want)
		}
	}
	if got := SanitizeLine("a\nb\x1b[1mc"); got != "a bc" {
		t.Errorf("SanitizeLine = %q", got)
	}
}

func TestHostileTextAndNamesAreSanitized(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	evil := bob.user(true)
	evil.DisplayName = "Bob\x1b[2J\x07"
	st.Apply(proto.Event{T: "user", User: &evil})
	m := msgFor(t, bob, "me", []actor{me, bob}, "hell", "", "\x1b]0;X\x07\x1b[2Jhi")
	st.Apply(proto.Event{T: "msg", Message: &m})
	if got := st.Timeline("hell")[0].Text; got != "hi" {
		t.Fatalf("%q", got)
	}
	if got := st.DisplayName("bob"); got != "Bob" {
		t.Fatalf("%q", got)
	}
	for _, e := range st.Sidebar() {
		if strings.ContainsAny(e.Label, "\x1b\x07") {
			t.Fatalf("label %q", e.Label)
		}
	}
}

func TestOldMessagesOpenWithSenderKeyAfterRekey(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	m := msgFor(t, bob, "me", []actor{me, bob}, "hell", "", "from the old key")
	m.SenderKey = base64.StdEncoding.EncodeToString(bob.kp.Public[:])
	newBob := newActor(t, "bob")
	nu := newBob.user(true)
	st.Apply(proto.Event{T: "user", User: &nu}) // bob re-registered with a new key
	st.Apply(proto.Event{T: "msg", Message: &m})
	if tl := st.Timeline("hell"); len(tl) != 1 || tl[0].Undecryptable || tl[0].Text != "from the old key" {
		t.Fatalf("%+v", tl)
	}
	m2 := m
	m2.ID = "zz"
	m2.SenderKey = "" // fallback to the sender's current key: fails here, must not crash
	st.Apply(proto.Event{T: "msg", Message: &m2})
}

func TestHelloAfterReconnectCountsNewMessages(t *testing.T) {
	st, me, bob, _, _ := setup(t)
	all := []actor{me, bob}
	users := []proto.User{me.user(true), bob.user(true)}
	old := msgFor(t, bob, "me", all, "hell", "", "old")
	st.Apply(proto.Event{T: "hello", Me: "me", Users: users, Messages: []proto.Message{old}})
	if unread(st, "hell") != 0 {
		t.Fatal("first snapshot counted")
	}
	st.SetActive("dm:bob|me")
	missed := msgFor(t, bob, "me", all, "hell", "", "while you slept @me")
	own := msgFor(t, me, "me", all, "hell", "", "mine")
	fx := st.Apply(proto.Event{T: "hello", Me: "me", Users: users, Messages: []proto.Message{old, missed, own}})
	if !fx.Bell || unread(st, "hell") != 1 {
		t.Fatalf("bell=%v unread=%d", fx.Bell, unread(st, "hell"))
	}
	fx = st.Apply(proto.Event{T: "hello", Me: "me", Users: users, Messages: []proto.Message{old, missed, own}})
	if fx.Bell || unread(st, "hell") != 1 {
		t.Fatalf("recounted: bell=%v unread=%d", fx.Bell, unread(st, "hell"))
	}
}
