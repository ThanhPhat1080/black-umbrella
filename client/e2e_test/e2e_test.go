//go:build e2e

// End-to-end test against a running server: UMBRELLA_E2E_SERVER (e.g. http://localhost:8787),
// UMBRELLA_E2E_ADMIN_TOKEN (default "dev").
package e2e_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/api"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

type peer struct {
	name string
	kp   e2e.KeyPair
	conn *api.Conn
	st   *chat.State
}

type env struct {
	t     *testing.T
	ctx   context.Context
	c     *api.Client
	admin string
	sfx   string
}

func setup(t *testing.T) *env {
	srv := os.Getenv("UMBRELLA_E2E_SERVER")
	if srv == "" {
		t.Skip("UMBRELLA_E2E_SERVER not set")
	}
	admin := os.Getenv("UMBRELLA_E2E_ADMIN_TOKEN")
	if admin == "" {
		admin = "dev"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return &env{t: t, ctx: ctx, c: &api.Client{BaseURL: srv}, admin: admin, sfx: fmt.Sprintf("%04x", rand.Intn(1<<16))}
}

func (e *env) register(name string) (*peer, string) {
	e.t.Helper()
	inv, err := e.c.AdminInvite(e.ctx, e.admin)
	if err != nil {
		e.t.Fatal("invite:", err)
	}
	kp, _ := e2e.GenerateKeyPair()
	user := name + "_" + e.sfx
	tok, err := e.c.Register(e.ctx, api.RegisterReq{Invite: inv, Username: user, DisplayName: "Tiếng " + name, Pin: "123456",
		PublicKey: base64.StdEncoding.EncodeToString(kp.Public[:])})
	if err != nil {
		e.t.Fatal("register:", err)
	}
	return &peer{name: user, kp: kp, st: chat.New(user, kp)}, tok
}

func (e *env) dial(p *peer, tok string) {
	e.t.Helper()
	conn, err := api.Dial(e.ctx, e.c.BaseURL, tok)
	if err != nil {
		e.t.Fatal("dial:", err)
	}
	e.t.Cleanup(conn.Close)
	p.conn = conn
	e.expect(p, func(ev proto.Event) bool { return ev.T == "hello" })
}

// expect reads events for p (applying them to its state) until pred matches.
func (e *env) expect(p *peer, pred func(proto.Event) bool) proto.Event {
	e.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-p.conn.Events():
			if !ok {
				e.t.Fatalf("%s: connection closed", p.name)
			}
			p.st.Apply(ev)
			if pred(ev) {
				return ev
			}
		case <-timeout:
			e.t.Fatalf("%s: timed out waiting for event", p.name)
		}
	}
}

func (e *env) nothing(p *peer, d time.Duration, pred func(proto.Event) bool) {
	e.t.Helper()
	timeout := time.After(d)
	for {
		select {
		case ev, ok := <-p.conn.Events():
			if !ok {
				return
			}
			p.st.Apply(ev)
			if pred(ev) {
				e.t.Fatalf("%s: unexpected event %+v", p.name, ev)
			}
		case <-timeout:
			return
		}
	}
}

func (e *env) say(from *peer, conv, parent, text string) {
	e.t.Helper()
	s, err := e2e.Seal(text, from.kp, from.st.Recipients(conv))
	if err != nil {
		e.t.Fatal(err)
	}
	if err := from.conn.Send(proto.NewSend("ref-"+text, conv, parent, s.Nonce, s.CT, s.Keys)); err != nil {
		e.t.Fatal(err)
	}
}

func isMsg(text string, p *peer) func(proto.Event) bool {
	return func(ev proto.Event) bool {
		if ev.T != "msg" {
			return false
		}
		for _, m := range p.st.Timeline(ev.Message.Conv) {
			if m.Text == text {
				return true
			}
		}
		_, replies := p.st.Thread(ev.Message.ParentID)
		for _, m := range replies {
			if m.Text == text {
				return true
			}
		}
		return false
	}
}

func TestFlow(t *testing.T) {
	e := setup(t)
	alice, atok := e.register("alice")
	bob, btok := e.register("bob")
	e.dial(alice, atok)
	e.dial(bob, btok)

	// hell message: bob decrypts it
	text := "Tiếng Việt có dấu ạ ( ˘ ³˘)♥ 🌂"
	e.say(alice, "hell", "", text)
	e.expect(bob, isMsg(text, bob))
	e.expect(alice, func(ev proto.Event) bool { return ev.T == "msg" && ev.Ref != "" })
	root := bob.st.Timeline("hell")[0]
	if root.Text != text || root.Sender != alice.name || root.Undecryptable {
		t.Fatalf("bob sees %+v", root)
	}

	// thread reply
	e.say(bob, "hell", root.ID, "reply from bob")
	e.expect(alice, isMsg("reply from bob", alice))
	if r, replies := alice.st.Thread(root.ID); r.Text != text || len(replies) != 1 || replies[0].Sender != bob.name {
		t.Fatalf("thread: %+v %+v", r, replies)
	}
	if tl := alice.st.Timeline("hell"); len(tl) != 1 || tl[0].Replies != 1 {
		t.Fatalf("timeline: %+v", tl)
	}

	// keep
	if err := alice.conn.Send(proto.NewKeep(root.ID, true)); err != nil {
		t.Fatal(err)
	}
	e.expect(bob, func(ev proto.Event) bool { return ev.T == "kept" && ev.ID == root.ID && ev.Kept })
	if !bob.st.Timeline("hell")[0].Kept {
		t.Fatal("kept not applied")
	}
	// non-owner cannot keep
	if err := bob.conn.Send(proto.NewKeep(root.ID, false)); err != nil {
		t.Fatal(err)
	}
	if ev := e.expect(bob, func(ev proto.Event) bool { return ev.T == "error" }); ev.Error != "not_owner" {
		t.Fatalf("got %s", ev.Error)
	}

	// carol registers while alice and bob are online: they must get a user event,
	// and a send sealed before that fails with recipients_mismatch.
	stale := alice.st.Recipients("hell")
	carol, ctok := e.register("carol")
	e.expect(alice, func(ev proto.Event) bool { return ev.T == "user" && ev.User.Username == carol.name })
	e.expect(bob, func(ev proto.Event) bool { return ev.T == "user" && ev.User.Username == carol.name })
	s, _ := e2e.Seal("too early", alice.kp, stale)
	_ = alice.conn.Send(proto.NewSend("stale", "hell", "", s.Nonce, s.CT, s.Keys))
	if ev := e.expect(alice, func(ev proto.Event) bool { return ev.T == "error" }); ev.Error != "recipients_mismatch" || ev.Ref != "stale" {
		t.Fatalf("got %+v", ev)
	}
	e.say(alice, "hell", "", "welcome carol")
	e.dial(carol, ctok)
	e.expect(bob, isMsg("welcome carol", bob))

	// DM alice -> bob is invisible to carol, now and in her hello
	dm := chat.DMConv(alice.name, bob.name)
	e.say(alice, dm, "", "secret between us")
	e.expect(bob, isMsg("secret between us", bob))
	e.nothing(carol, 1500*time.Millisecond, func(ev proto.Event) bool { return ev.T == "msg" && ev.Message.Conv == dm })
	if tl := carol.st.Timeline(dm); len(tl) != 0 {
		t.Fatalf("carol sees DM: %+v", tl)
	}

	// reconnect: a fresh hello replaces state with no duplicates; carol's own hello holds no DMs
	conn2, err := api.Dial(e.ctx, e.c.BaseURL, atok)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	hello := <-conn2.Events()
	if hello.T != "hello" {
		t.Fatalf("got %s", hello.T)
	}
	alice.st.Apply(hello)
	alice.st.Apply(hello)
	if n := len(alice.st.Timeline("hell")); n != 2 {
		t.Fatalf("hell timeline after hello: %d", n)
	}
	if n := len(alice.st.Timeline(dm)); n != 1 {
		t.Fatalf("dm timeline after hello: %d", n)
	}

	// lockout: 5 bad PINs then 423 with until
	for i := 0; i < 5; i++ {
		_, err := e.c.Login(e.ctx, bob.name, "000000")
		var ae *api.APIError
		if !errors.As(err, &ae) || ae.Status != 401 {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	_, err = e.c.Login(e.ctx, bob.name, "123456")
	var ae *api.APIError
	if !errors.As(err, &ae) || ae.Status != 423 || ae.Until <= time.Now().UnixMilli() {
		t.Fatalf("expected lock, got %v", err)
	}
	if err := e.c.AdminUnlock(e.ctx, e.admin, bob.name); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Login(e.ctx, bob.name, "123456"); err != nil {
		t.Fatal("login after unlock:", err)
	}
}
