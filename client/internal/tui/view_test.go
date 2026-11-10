package tui

import (
	"context"
	"encoding/base64"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/api"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func msg(text string) chat.Msg {
	return chat.Msg{ID: "m1", Conv: "hell", Sender: "bob", Text: text, CreatedAt: time.Unix(1700000000, 0)}
}

func TestLinkMarkerOnlyWithLink(t *testing.T) {
	v := msgView{Name: "Bob", Me: "me", Width: 60}
	if got := strings.Join(renderMessage(msg("see https://a.b/c"), v), "\n"); !strings.Contains(got, "📋") {
		t.Fatalf("missing 📋:\n%s", got)
	}
	if got := strings.Join(renderMessage(msg("no link here"), v), "\n"); strings.Contains(got, "📋") {
		t.Fatalf("unexpected 📋:\n%s", got)
	}
}

func TestKeptAnchorStyled(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	v := msgView{Name: "Bob", Width: 60}
	plain := msg("hi")
	kept := msg("hi")
	kept.Kept = true
	a := strings.Join(renderMessage(plain, v), "\n")
	b := strings.Join(renderMessage(kept, v), "\n")
	if !strings.Contains(a, "⚓") || !strings.Contains(b, "⚓") {
		t.Fatal("anchor button missing")
	}
	if a == b || !strings.Contains(b, keptStyle.Render("[⚓]")) {
		t.Fatalf("kept anchor not styled differently:\n%q\n%q", a, b)
	}
}

func TestWrapKeepsRunesAndWidth(t *testing.T) {
	text := "Tiếng Việt có dấu ạ, Tiếng Việt có dấu ạ ( ˘ ³˘)♥ ʕ•ᴥ•ʔ 🌂🌂🌂 supercalifragilisticexpialidocious_Việt_Nam_đẹp_lắm_thật_sự"
	for _, w := range []int{5, 8, 12, 20, 37} {
		lines := wrapText(text, w)
		var rebuilt strings.Builder
		for _, l := range lines {
			if runewidth.StringWidth(l) > w && w >= 2 {
				t.Fatalf("w=%d line %q width %d", w, l, runewidth.StringWidth(l))
			}
			if !validRunes(l) {
				t.Fatalf("invalid utf8 in %q", l)
			}
			rebuilt.WriteString(l)
		}
		strip := func(s string) string { return strings.NewReplacer(" ", "", "\n", "").Replace(s) }
		if strip(rebuilt.String()) != strip(text) {
			t.Fatalf("w=%d text changed:\n%q\n%q", w, rebuilt.String(), text)
		}
	}
	// composed vs decomposed Vietnamese
	dec := "Tiếng Việt"
	for _, l := range wrapText(dec, 4) {
		if strings.HasPrefix(l, "́") || strings.HasPrefix(l, "̂") || strings.HasPrefix(l, "̣") {
			t.Fatalf("line starts with combining mark: %q", l)
		}
	}
}

func validRunes(s string) bool { return !strings.ContainsRune(s, '�') }

func TestRenderMessageLinesFit(t *testing.T) {
	m := msg("Tiếng Việt có dấu ạ rất dài rất dài rất dài rất dài https://example.com/very/long/path ( ˘ ³˘)♥")
	m.Kept = true
	m.Replies = 12
	for _, w := range []int{20, 30, 50} {
		for _, l := range renderMessage(m, msgView{Name: "Phát 🌂", Width: w}) {
			if lipgloss.Width(l) > w {
				t.Fatalf("w=%d line width %d: %q", w, lipgloss.Width(l), l)
			}
		}
	}
}

func TestThreadNilRootShowsExpired(t *testing.T) {
	name := func(s string) string { return s }
	reply := chat.Msg{ID: "r1", Conv: "hell", Sender: "bob", ParentID: "gone", Text: "kept reply", Kept: true, CreatedAt: time.Now()}
	for _, root := range []*chat.Msg{nil, {ID: "gone", Text: chat.ExpiredText}} {
		blocks := renderThread(root, []chat.Msg{reply}, name, "me", 40, -1)
		var all []string
		for _, b := range blocks {
			all = append(all, b...)
		}
		out := strings.Join(all, "\n")
		if !strings.Contains(out, "[message expired]") || !strings.Contains(out, "kept reply") {
			t.Fatalf("got:\n%s", out)
		}
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30}
	for i, w := range want {
		if backoff(i) != w*time.Second {
			t.Fatalf("attempt %d: %v", i, backoff(i))
		}
	}
}

func newTestModel(t *testing.T) (*Model, e2e.KeyPair, e2e.KeyPair) {
	t.Helper()
	me, _ := e2e.GenerateKeyPair()
	bob, _ := e2e.GenerateKeyPair()
	enc := func(k e2e.KeyPair) string { return base64.StdEncoding.EncodeToString(k.Public[:]) }
	st := chat.New("me", me)
	st.Apply(proto.Event{T: "hello", Me: "me", Users: []proto.User{
		{Username: "me", DisplayName: "Me", PublicKey: enc(me), Online: true},
		{Username: "bob", DisplayName: "Phát 🌂", PublicKey: enc(bob), Online: true},
	}})
	m := NewModel(context.Background(), nil, st, me, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, me, bob
}

func TestViewFitsTerminalWithVietnameseAndEmoji(t *testing.T) {
	m, me, bob := newTestModel(t)
	for i, text := range []string{"Tiếng Việt có dấu ạ", "( ˘ ³˘)♥ ʕ•ᴥ•ʔ 🌂 " + strings.Repeat("rất dài ", 30)} {
		s, _ := e2e.Seal(text, bob, map[string][32]byte{"me": me.Public, "bob": bob.Public})
		pm := proto.Message{ID: string(rune('a' + i)), Conv: "hell", Sender: "bob", CreatedAt: time.Now().UnixMilli(), Nonce: s.Nonce, CT: s.CT, Key: s.Keys["me"]}
		m.st.Apply(proto.Event{T: "msg", Message: &pm})
	}
	for _, size := range [][2]int{{100, 30}, {80, 24}, {70, 20}, {50, 16}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.threadRoot = ""
		for _, open := range []bool{false, true} {
			if open {
				m.threadRoot = "a"
				m.resize()
			}
			view := m.View()
			lines := strings.Split(view, "\n")
			if len(lines) > size[1] {
				t.Fatalf("%v: %d lines", size, len(lines))
			}
			for _, l := range lines {
				if lipgloss.Width(l) > size[0] {
					t.Fatalf("%v: line width %d > %d: %q", size, lipgloss.Width(l), size[0], l)
				}
			}
		}
	}
}

type fakeLink struct {
	sent   []any
	closed int
	code   int
}

func (f *fakeLink) Send(v any) error           { f.sent = append(f.sent, v); return nil }
func (f *fakeLink) Close()                     { f.closed++ }
func (f *fakeLink) Events() <-chan proto.Event { return nil }
func (f *fakeLink) CloseCode() int             { return f.code }

func (f *fakeLink) sends() (out []proto.Send) {
	for _, v := range f.sent {
		if s, ok := v.(proto.Send); ok {
			out = append(out, s)
		}
	}
	return
}

func typeAndEnter(m *Model, text string) {
	m.input.SetValue(text)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func joinCarol(t *testing.T, m *Model) {
	carol, _ := e2e.GenerateKeyPair()
	m.onEvent(proto.Event{T: "user", User: &proto.User{Username: "carol", DisplayName: "C", PublicKey: base64.StdEncoding.EncodeToString(carol.Public[:])}})
}

// Review focus 1: a user registers while we send -> recipients_mismatch -> retry exactly once.
func TestMismatchResendsImmediatelyWhenUsersChanged(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "hello")
	first := f.sends()
	if len(first) != 1 || len(first[0].Keys) != 2 {
		t.Fatalf("first send: %+v", first)
	}
	joinCarol(t, m) // user event processed before the server's error arrives
	m.onEvent(proto.Event{T: "error", Ref: first[0].Ref, Error: "recipients_mismatch"})
	all := f.sends()
	if len(all) != 2 || len(all[1].Keys) != 3 {
		t.Fatalf("expected a resend sealed for 3 users, got %+v", all)
	}
	if f.closed != 0 {
		t.Fatal("should not reconnect")
	}
	// the retry fails too -> user-visible error, no third attempt
	m.onEvent(proto.Event{T: "error", Ref: all[1].Ref, Error: "recipients_mismatch"})
	if len(f.sends()) != 2 || !strings.Contains(m.status, "send again") {
		t.Fatalf("sends=%d status=%q", len(f.sends()), m.status)
	}
}

func TestMismatchWithUnchangedUsersRefreshesThenRetriesOnce(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "hello")
	ref := f.sends()[0].Ref
	m.onEvent(proto.Event{T: "error", Ref: ref, Error: "recipients_mismatch"})
	if f.closed != 1 || len(f.sends()) != 1 {
		t.Fatalf("closed=%d sends=%d", f.closed, len(f.sends()))
	}
	// reconnect delivers a fresh snapshot that now includes carol
	carol, _ := e2e.GenerateKeyPair()
	users := m.st.Users()
	var us []proto.User
	for _, u := range users {
		us = append(us, u)
	}
	us = append(us, proto.User{Username: "carol", DisplayName: "C", PublicKey: base64.StdEncoding.EncodeToString(carol.Public[:])})
	m.onEvent(proto.Event{T: "hello", Me: "me", Users: us})
	all := f.sends()
	if len(all) != 2 || len(all[1].Keys) != 3 {
		t.Fatalf("expected one retry with 3 keys, got %+v", all)
	}
	m.onEvent(proto.Event{T: "hello", Me: "me", Users: us})
	if len(f.sends()) != 2 {
		t.Fatal("retried more than once")
	}
}

func TestSlashCommandsAndLimits(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "/i")
	if m.overlay != overlayPicker {
		t.Fatal("picker not open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	typeAndEnter(m, "/whois @bob")
	if m.overlay != overlayProfile || m.profileUser != "bob" {
		t.Fatal("profile not open")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	typeAndEnter(m, "/name Tiếng Việt 🌂")
	if len(f.sent) != 1 || f.sent[0].(proto.Name).DisplayName != "Tiếng Việt 🌂" {
		t.Fatalf("%+v", f.sent)
	}
	if m.input.CharLimit != 4000 {
		t.Fatal("limit")
	}
}

func sealedMsg(t *testing.T, from, to e2e.KeyPair, fromName, id, conv, parent, text string) proto.Message {
	t.Helper()
	s, err := e2e.Seal(text, from, map[string][32]byte{"me": to.Public, fromName: from.Public})
	if err != nil {
		t.Fatal(err)
	}
	return proto.Message{ID: id, Conv: conv, Sender: fromName, ParentID: parent, CreatedAt: time.Now().UnixMilli(), Nonce: s.Nonce, CT: s.CT, Key: s.Keys["me"]}
}

func noEscapes(t *testing.T, what, s string) {
	t.Helper()
	if strings.ContainsAny(s, "\x1b\a\x00") {
		t.Fatalf("%s contains terminal control bytes: %q", what, s)
	}
}

// Review: terminal escape injection through message text and display names.
func TestHostileTextAndNamesNeverReachTheScreen(t *testing.T) {
	m, me, bob := newTestModel(t)
	evil := "Bob\x1b[2J\x1b]0;pwn\x07"
	m.onEvent(proto.Event{T: "user", User: &proto.User{Username: "bob", DisplayName: evil, PublicKey: base64.StdEncoding.EncodeToString(bob.Public[:]), Online: true}})
	pm := sealedMsg(t, bob, me, "bob", "e1", "hell", "", "\x1b]0;X\x07\x1b[2Jhi \x1b[31mred")
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	noEscapes(t, "render", zone.Scan(strings.Join(renderMessage(m.st.Timeline("hell")[0], msgView{Name: m.nameOf("bob"), Width: 60}), "\n")))
	noEscapes(t, "view", m.View())
	m.overlay, m.profileUser = overlayProfile, "bob"
	noEscapes(t, "profile", m.View())
	if !strings.Contains(m.View(), "hi red") && m.overlay == overlayNone {
		t.Fatal("body missing")
	}
}

// Review: one width function (runewidth) everywhere; no line may overflow the terminal.
func TestEveryLineFitsByRunewidth(t *testing.T) {
	m, me, bob := newTestModel(t)
	for i, text := range []string{"(☞ﾟヮﾟ)☞ ☜(ﾟヮﾟ☜) (ﾉ◕ヮ◕)ﾉ*:･ﾟ✧", "Tiếng Việt có dấu ạ ( ˘ ³˘)♥ ʕ•ᴥ•ʔ 🌂", strings.Repeat("ﾟ✧ ", 40)} {
		pm := sealedMsg(t, bob, me, "bob", string(rune('a'+i)), "hell", "", text)
		m.onEvent(proto.Event{T: "msg", Message: &pm})
	}
	check := func(label string, w, h int) {
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			t.Fatalf("%s %dx%d: %d lines", label, w, h, len(lines))
		}
		for _, l := range lines {
			if rw := runewidth.StringWidth(ansi.Strip(l)); rw > w {
				t.Fatalf("%s %dx%d: width %d: %q", label, w, h, rw, l)
			}
		}
	}
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {60, 20}, {40, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.overlay, m.threadRoot = overlayNone, ""
		check("main", size[0], size[1])
		m.threadRoot = "c"
		m.resize()
		check("thread", size[0], size[1])
		m.threadRoot = ""
		m.overlay, m.pickIdx = overlayPicker, 7
		check("picker", size[0], size[1])
		m.overlay, m.profileUser = overlayProfile, "bob"
		check("profile", size[0], size[1])
		m.overlay = overlayNone
	}
}

func TestStyleBodyDoesNotStylePunctuationAfterLink(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	got := styleBody("see https://a.b/c, ok (https://x.y/z).", "me")
	if !strings.Contains(got, linkSt.Render("https://a.b/c")+",") || !strings.Contains(got, linkSt.Render("https://x.y/z")+").") {
		t.Fatalf("%q", got)
	}
}

func TestListFocusTypingGoesToInput(t *testing.T) {
	m, _, _ := newTestModel(t)
	m.pane = paneMain
	m.input.Blur()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.pane != paneInput || m.input.Value() != "x" {
		t.Fatalf("pane=%v value=%q", m.pane, m.input.Value())
	}
	// defined hotkeys stay hotkeys
	m.pane = paneMain
	m.input.Blur()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.pane != paneMain {
		t.Fatal("j should stay a hotkey")
	}
}

func TestThreadReplyTargetIsExplicit(t *testing.T) {
	m, me, bob := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	pm := sealedMsg(t, bob, me, "bob", "root1", "hell", "", "root")
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	m.openThread(m.st.Timeline("hell")[0])
	if v := m.View(); !strings.Contains(v, "Replying in thread") {
		t.Fatalf("no indicator:\n%s", v)
	}
	typeAndEnter(m, "in thread")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if v := m.View(); strings.Contains(v, "Replying in thread") || !strings.Contains(v, "main chat") {
		t.Fatalf("indicator did not switch:\n%s", v)
	}
	typeAndEnter(m, "in main")
	s := f.sends()
	if len(s) != 2 || s[0].ParentID != "root1" || s[1].ParentID != "" {
		t.Fatalf("%+v", s)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	typeAndEnter(m, "after close")
	if s = f.sends(); s[2].ParentID != "" || m.threadRoot != "" {
		t.Fatal("thread should be closed and target main")
	}
}

func TestSendErrorRestoresText(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	typeAndEnter(m, "Tiếng Việt dài")
	if m.input.Value() != "" {
		t.Fatal("input should clear on send")
	}
	for _, code := range []string{"too_large", "bad_parent", "bad_conv"} {
		m.input.Reset()
		typeAndEnter(m, "keep me "+code)
		ref := f.sends()[len(f.sends())-1].Ref
		m.onEvent(proto.Event{T: "error", Ref: ref, Error: code})
		if m.input.Value() != "keep me "+code {
			t.Fatalf("%s: input %q", code, m.input.Value())
		}
	}
	// with text already typed, the failed message is put back in front of it
	typeAndEnter(m, "first")
	ref := f.sends()[len(f.sends())-1].Ref
	m.input.SetValue("draft")
	m.onEvent(proto.Event{T: "error", Ref: ref, Error: "too_large"})
	if m.input.Value() != "first\ndraft" {
		t.Fatalf("%q", m.input.Value())
	}
}

func TestRevokedSessionStopsReconnecting(t *testing.T) {
	m, _, _ := newTestModel(t)
	f := &fakeLink{code: 4001}
	m.conn = f
	_, cmd := m.Update(connClosedMsg{f})
	if cmd != nil || m.reconnecting || !strings.Contains(m.fatal, "revoked") {
		t.Fatalf("cmd=%v reconnecting=%v fatal=%q", cmd != nil, m.reconnecting, m.fatal)
	}
	if v := m.View(); !strings.Contains(v, "revoked") || strings.Contains(v, "Reconnecting") {
		t.Fatalf("status:\n%s", v)
	}
}

func TestReconnectGivesUpOnAuthFailures(t *testing.T) {
	for code, want := range map[string]string{"bad_credentials": "register", "locked": "locked until"} {
		m, _, _ := newTestModel(t)
		m.reconnect = func(context.Context) (*api.Conn, error) {
			return nil, &api.APIError{Status: 401, Code: code, Until: time.Now().Add(time.Hour).UnixMilli()}
		}
		msg := m.doReconnect()()
		fm, ok := msg.(fatalMsg)
		if !ok {
			t.Fatalf("%s: got %T", code, msg)
		}
		m.Update(fm)
		if !strings.Contains(m.fatal, want) || m.reconnecting {
			t.Fatalf("%s: fatal=%q", code, m.fatal)
		}
	}
}
