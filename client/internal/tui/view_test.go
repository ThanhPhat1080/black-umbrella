package tui

import (
	"context"
	"encoding/base64"
	"fmt"
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
	v := msgView{Name: "Bob", Me: "bob", Width: 60}
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
	if !strings.Contains(got, linkSt.Italic(true).Render("https://a.b/c")+"\x1b[3m,") || !strings.Contains(got, linkSt.Italic(true).Render("https://x.y/z")+"\x1b[3m).") {
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
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // closes the thread, focus on the list
	m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // back to the input
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

func TestOverlayCompositesByCellsOverBackground(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // real terminals emit styles; so must the test
	defer lipgloss.SetColorProfile(old)
	m, me, bob := newTestModel(t)
	pm := sealedMsg(t, bob, me, "bob", "a", "hell", "", "(☞ﾟヮﾟ)☞ Tiếng Việt")
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	for _, size := range [][2]int{{120, 35}, {80, 24}, {40, 12}} {
		w, h := size[0], size[1]
		m.Update(tea.WindowSizeMsg{Width: w, Height: h})
		m.overlay = overlayNone
		bg := strings.Split(m.View(), "\n")
		for _, ov := range []overlay{overlayPicker, overlayProfile} {
			m.overlay, m.pickIdx, m.profileUser = ov, 3, "bob"
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d ov=%d: %d lines", w, h, ov, len(lines))
			}
			boxRows := 0
			for i, l := range lines {
				if strings.ContainsRune(l, '\uFFFD') {
					t.Fatalf("%dx%d ov=%d row %d has U+FFFD: %q", w, h, ov, i, l)
				}
				if rw := runewidth.StringWidth(ansi.Strip(l)); rw != w {
					t.Fatalf("%dx%d ov=%d row %d width %d != %d: %q", w, h, ov, i, rw, w, l)
				}
				// outside the box the background must still be there (first two cells = sidebar edge)
				if ansi.Strip(sliceCells(l, 0, 2)) != ansi.Strip(sliceCells(bg[i], 0, 2)) {
					t.Fatalf("%dx%d ov=%d row %d lost background: %q vs %q", w, h, ov, i, sliceCells(l, 0, 2), sliceCells(bg[i], 0, 2))
				}
				plain := ansi.Strip(l)
				if strings.ContainsRune(plain, '╭') {
					if i > 1 { // rows 0/1 hold the sidebar/chat borders themselves
						boxRows++
						if !strings.Contains(plain[strings.IndexRune(plain, '╭'):], "╮") {
							t.Fatalf("%dx%d ov=%d: box top missing ╮: %q", w, h, ov, plain)
						}
					}
				}
			}
			if ov == overlayPicker {
				found := false
				for _, l := range lines {
					p := ansi.Strip(l)
					if i := strings.Index(p, "╭──"); i >= 0 && i > 1 && strings.Contains(p[i:], "╮") && !strings.Contains(p, "Hell") {
						found = true
					}
				}
				if !found {
					t.Fatalf("%dx%d: no picker top border with both corners", w, h)
				}
			}
		}
		m.overlay = overlayNone
	}
}

func TestSliceCells(t *testing.T) {
	cases := []struct {
		in       string
		from, to int
		want     string
	}{
		{"abcdef", 1, 4, "bcd"},
		{"日本語", 0, 3, "日 "},
		{"日本語", 1, 4, " 本"},
		{"\x1b[31mred\x1b[0m", 1, 3, "\x1b[31med\x1b[0m"},
		{"ab", 0, 10, "ab"},
	}
	for _, c := range cases {
		if got := sliceCells(c.in, c.from, c.to); got != c.want {
			t.Errorf("sliceCells(%q,%d,%d) = %q want %q", c.in, c.from, c.to, got, c.want)
		}
	}
}

func TestUpFromInputSelectsLastMessage(t *testing.T) {
	m, me, bob := newTestModel(t)
	for i, txt := range []string{"one", "two"} {
		pm := sealedMsg(t, bob, me, "bob", string(rune('a'+i)), "hell", "", txt)
		m.onEvent(proto.Event{T: "msg", Message: &pm})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.pane != paneMain || m.selMain != 1 {
		t.Fatalf("pane=%v sel=%d", m.pane, m.selMain)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.threadRoot != "b" || m.input.Value() != "" {
		t.Fatalf("thread=%q input=%q", m.threadRoot, m.input.Value())
	}
	// text on the first line: Up still leaves the input; on a later line it moves the cursor
	m2, _, _ := newTestModel(t)
	m2.conn = &fakeLink{}
	m2.input.SetValue("one\ntwo")
	m2.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m2.pane != paneInput {
		t.Fatal("cursor on line 2: Up must stay in the textarea")
	}
}

func TestAnchorOnlyOnOwnMessages(t *testing.T) {
	v := msgView{Name: "Alice", Me: "bob", Width: 60}
	other := msg("hi") // sender bob? use alice
	other.Sender = "alice"
	if got := strings.Join(renderMessage(other, v), "\n"); strings.Contains(got, "⚓") {
		t.Fatalf("anchor shown on someone else's unkept message:\n%s", got)
	}
	other.Kept = true
	got := zone.Scan(strings.Join(renderMessage(other, v), "\n"))
	if !strings.Contains(got, "⚓") || strings.Contains(got, "[⚓]") {
		t.Fatalf("kept marker on others' message should be plain ⚓ without a button:\n%s", got)
	}
	mine := msg("hi")
	mine.Sender = "bob"
	if got := strings.Join(renderMessage(mine, v), "\n"); !strings.Contains(got, "[⚓]") {
		t.Fatalf("own message needs the anchor button:\n%s", got)
	}
}

func TestHellRoomLabel(t *testing.T) {
	m, _, _ := newTestModel(t)
	if v := m.View(); strings.Count(v, "Hell room") < 2 {
		t.Fatalf("sidebar and header should both say Hell room:\n%s", v)
	}
}

// waitZone renders (so zones are registered) and waits for bubblezone's async scanner.
func waitZone(t *testing.T, m *Model, id string) *zone.ZoneInfo {
	t.Helper()
	m.View()
	for i := 0; i < 200; i++ {
		if z := zone.Get(id); !z.IsZero() {
			return z
		}
		time.Sleep(5 * time.Millisecond)
		m.View()
	}
	t.Fatalf("zone %q was never registered by View()", id)
	return nil
}

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft}
}

func TestClickInsideInputBoxFocusesIt(t *testing.T) {
	m, me, bob := newTestModel(t)
	m.conn = &fakeLink{}
	pm := sealedMsg(t, bob, me, "bob", "r1", "hell", "", "root")
	m.onEvent(proto.Event{T: "msg", Message: &pm})
	m.openThread(m.st.Timeline("hell")[0])
	m.pane = paneMain
	m.input.Blur()
	z := waitZone(t, m, "input")
	if z.EndY-z.StartY != 4 { // top border + 3 text lines + bottom border
		t.Fatalf("input box spans %d rows, want 5: %+v", z.EndY-z.StartY+1, z)
	}
	for _, toMain := range []bool{false, true} {
		m.toMain = toMain
		m.pane = paneMain
		m.input.Blur()
		// the border itself and the text area both count
		for _, pt := range [][2]int{{z.StartX, z.StartY}, {z.StartX + 5, z.StartY + 2}, {z.EndX, z.EndY}} {
			m.pane = paneMain
			m.input.Blur()
			m.Update(click(pt[0], pt[1]))
			if m.pane != paneInput || !m.input.Focused() {
				t.Fatalf("click %v: pane=%v focused=%v", pt, m.pane, m.input.Focused())
			}
			if m.toMain != toMain {
				t.Fatalf("click changed the reply target (toMain=%v)", m.toMain)
			}
		}
	}
	// typing now lands in the input
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.input.Value() != "x" {
		t.Fatalf("%q", m.input.Value())
	}
	// a click just outside the box does not
	m.pane = paneMain
	m.input.Blur()
	m.Update(click(z.StartX, z.StartY-2))
	if m.pane == paneInput {
		t.Fatal("click above the box focused the input")
	}
}

func TestClickPlusAndSidebarEntry(t *testing.T) {
	m, _, _ := newTestModel(t)
	z := waitZone(t, m, "plus")
	m.Update(click(z.StartX+1, z.StartY))
	if m.overlay != overlayPicker {
		t.Fatal("[+] did not open the picker")
	}
	m.overlay = overlayNone
	sz := waitZone(t, m, "side:dm:bob|me")
	m.Update(click(sz.StartX+2, sz.StartY))
	if m.st.Active() != "dm:bob|me" {
		t.Fatalf("active %q", m.st.Active())
	}
	az := waitZone(t, m, "avs:bob")
	m.Update(click(az.StartX+1, az.StartY))
	if m.overlay != overlayProfile || m.profileUser != "bob" {
		t.Fatalf("avatar click: overlay=%v user=%q", m.overlay, m.profileUser)
	}
}

func TestClickMessageButtons(t *testing.T) {
	m, me, bob := newTestModel(t)
	f := &fakeLink{}
	m.conn = f
	other := sealedMsg(t, bob, me, "bob", "m1", "hell", "", "see https://a.b/c")
	m.onEvent(proto.Event{T: "msg", Message: &other})
	mine := sealedMsg(t, me, me, "me", "m2", "hell", "", "mine")
	m.onEvent(proto.Event{T: "msg", Message: &mine})
	z := waitZone(t, m, "keep:m2")
	m.Update(click(z.StartX+1, z.StartY))
	if k := f.sent[len(f.sent)-1]; k != proto.NewKeep("m2", true) {
		t.Fatalf("keep click sent %+v", k)
	}
	z = waitZone(t, m, "thr:m1")
	m.Update(click(z.StartX+1, z.StartY))
	if m.threadRoot != "m1" {
		t.Fatalf("thread %q", m.threadRoot)
	}
	if z := zone.Get("keep:m1"); !z.IsZero() {
		t.Fatal("others' messages must not have a keep button")
	}
}

func TestInputBoxRendersBorderedAndFocusStyled(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	m, _, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	focused := m.View()
	m.pane = paneMain
	m.input.Blur()
	blurred := m.View()
	plain := ansi.Strip(focused)
	lines := strings.Split(plain, "\n")
	var top int = -1
	for i, l := range lines {
		if strings.Contains(l, "Message") && strings.Contains(l, "│") && i > 0 && strings.Contains(lines[i-1], "╭") {
			top = i - 1
		}
	}
	if top < 0 {
		t.Fatalf("no bordered input with placeholder:\n%s", plain)
	}
	if !strings.Contains(lines[top+4], "╰") || !strings.Contains(lines[top+5], "kaomoji") {
		t.Fatalf("box must be 3 text lines, with the [+] line right below:\n%s", strings.Join(lines[top:top+6], "\n"))
	}
	if focused == blurred {
		t.Fatal("focus must change the border colour")
	}
	if !strings.Contains(focused, accent.Render("╭")) && !strings.Contains(focused, "\x1b[38;5;39m╭") {
		t.Fatal("focused border should use the accent colour")
	}
}

func TestMessageStylingItalicBodyColouredName(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(old)

	nameEsc := nameStyle("alice").Render("A")
	nameEsc = nameEsc[:strings.Index(nameEsc, "A")] // the opening escape sequence for alice's name
	if !strings.Contains(nameEsc, "38;2;") {
		t.Fatalf("expected a truecolor escape, got %q", nameEsc)
	}
	m := chat.Msg{ID: "m1", Conv: "hell", Sender: "alice", Text: "hello @bob see https://a.b/c.", CreatedAt: time.Unix(1700000000, 0)}
	lines := renderMessage(m, msgView{Name: "Alice", Me: "bob", Width: 80})
	if !strings.Contains(lines[0], nameEsc) || !strings.Contains(lines[0], "\x1b[1") {
		t.Fatalf("header should have alice's bold colour %q: %q", nameEsc, lines[0])
	}
	if strings.Contains(lines[0], "\x1b[3m") {
		t.Fatalf("header must not be italic: %q", lines[0])
	}
	body := lines[1]
	if !strings.Contains(body, "\x1b[3m") {
		t.Fatalf("body should be italic: %q", body)
	}
	if !strings.Contains(body, linkSt.Italic(true).Render("https://a.b/c")+"\x1b[3m.") && !strings.Contains(body, "https://a.b/c") {
		t.Fatalf("link lost: %q", body)
	}
	for _, l := range lines {
		if strings.Contains(l, "📋") && strings.Contains(l[strings.Index(l, "[📋]")-1:], "\x1b[3m[") {
			t.Fatalf("buttons must not be italic: %q", l)
		}
	}
	// system lines stay dim and upright
	for _, root := range []*chat.Msg{nil} {
		for _, b := range renderThread(root, nil, func(s string) string { return s }, "me", 40, -1)[:1] {
			if strings.Contains(strings.Join(b, ""), "\x1b[3m") || !strings.Contains(strings.Join(b, ""), "[message expired]") {
				t.Fatalf("expired placeholder: %q", b)
			}
		}
	}
	u := chat.Msg{ID: "m2", Conv: "hell", Sender: "alice", Text: chat.UndecryptableText, Undecryptable: true, CreatedAt: time.Now()}
	if l := strings.Join(renderMessage(u, msgView{Name: "Alice", Width: 60}), "\n"); strings.Contains(l, "\x1b[3m") {
		t.Fatalf("undecryptable must not be italic: %q", l)
	}
}

func TestNamesColouredEverywhere(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(old)
	m, _, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	esc := nameStyle("bob").Render("B")
	esc = esc[:strings.Index(esc, "B")]
	if v := m.View(); !strings.Contains(v, esc) { // sidebar label
		t.Fatalf("sidebar label not in bob's colour")
	}
	m.onEvent(proto.Event{T: "typing", Conv: "hell", Username: "bob"})
	if v := m.View(); strings.Count(v, esc) < 2 {
		t.Fatalf("typing indicator not in bob's colour")
	}
	m.overlay, m.profileUser = overlayProfile, "bob"
	if v := m.View(); !strings.Contains(v, esc) {
		t.Fatal("profile name not in bob's colour")
	}
}

func fillTimeline(t *testing.T, m *Model, me, bob e2e.KeyPair, n int) []string {
	t.Helper()
	var ids []string
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("m%03d", i)
		pm := sealedMsg(t, bob, me, "bob", id, "hell", "", fmt.Sprintf("tin so %d", i))
		pm.CreatedAt = int64(1_700_000_000_000 + i*1000)
		m.onEvent(proto.Event{T: "msg", Message: &pm})
		ids = append(ids, id)
	}
	return ids
}

func press(m *Model, k tea.KeyType) { m.Update(tea.KeyMsg{Type: k}) }

func rowOf(view, needle string) string {
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, needle) {
			return l
		}
	}
	return ""
}

// Selection is only a highlight: nothing above or below it may disappear.
func TestSelectionDoesNotHideMessagesBelow(t *testing.T) {
	m, me, bob := newTestModel(t)
	fillTimeline(t, m, me, bob, 7)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	press(m, tea.KeyUp)
	press(m, tea.KeyUp)
	if m.selMain != 5 {
		t.Fatalf("sel %d", m.selMain)
	}
	v := m.View()
	for i := 1; i <= 7; i++ {
		if !strings.Contains(v, fmt.Sprintf("tin so %d", i)) {
			t.Fatalf("message %d vanished:\n%s", i, v)
		}
	}
	if r := rowOf(v, "tin so 6"); !strings.Contains(r, "▌") {
		t.Fatalf("selected message not highlighted: %q", r)
	}
	if r := rowOf(v, "tin so 7"); strings.Contains(r, "▌") {
		t.Fatalf("newest wrongly highlighted: %q", r)
	}
}

func TestLongTimelineScrollsOnlyAtTheEdge(t *testing.T) {
	m, me, bob := newTestModel(t)
	fillTimeline(t, m, me, bob, 40)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 22})
	m.View()
	if !strings.Contains(m.View(), "tin so 40") {
		t.Fatal("default view must be pinned to the newest message")
	}
	press(m, tea.KeyUp) // selects the newest
	m.View()
	top0 := m.topMain
	moved := false
	for i := 0; i < 38 && !moved; i++ {
		press(m, tea.KeyUp)
		v := m.View()
		sel := fmt.Sprintf("tin so %d", m.selMain+1)
		r := rowOf(v, sel)
		if r == "" || !strings.Contains(r, "▌") {
			t.Fatalf("selected %q not visible/highlighted:\n%s", sel, v)
		}
		if m.topMain != top0 {
			moved = true
			// scrolled only because the selection hit the top edge
			if !strings.Contains(strings.Split(v, "\n")[2], "▌") && !strings.Contains(strings.Split(v, "\n")[3], "▌") {
				t.Fatalf("scrolled before the selection reached the top edge:\n%s", v)
			}
		} else if i > 30 {
			t.Fatal("never scrolled")
		}
	}
	if !moved {
		t.Fatal("never scrolled")
	}
	for m.selMain < 39 {
		press(m, tea.KeyDown)
		m.View()
	}
	if v := m.View(); !strings.Contains(v, "tin so 40") || !strings.Contains(rowOf(v, "tin so 40"), "▌") {
		t.Fatalf("newest not visible after going back down:\n%s", v)
	}
}

func TestNewMessageKeepsSelectionOnSameID(t *testing.T) {
	m, me, bob := newTestModel(t)
	ids := fillTimeline(t, m, me, bob, 10)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	press(m, tea.KeyUp)
	press(m, tea.KeyUp)
	want := ids[8]
	if sel, _ := m.selectedMsg(); sel.ID != want {
		t.Fatalf("setup: %s", sel.ID)
	}
	// an older message arrives (sorts before the selection) and one expires
	old := sealedMsg(t, bob, me, "bob", "m000", "hell", "", "older")
	old.CreatedAt = 1_699_999_999_000
	m.onEvent(proto.Event{T: "msg", Message: &old})
	m.onEvent(proto.Event{T: "expired", IDs: []string{ids[0]}})
	m.onEvent(proto.Event{T: "expired", IDs: []string{ids[1]}})
	if sel, ok := m.selectedMsg(); !ok || sel.ID != want {
		t.Fatalf("selection moved to %q", sel.ID)
	}
}

func TestRightOpensThreadLeftClosesKeepingSelection(t *testing.T) {
	m, me, bob := newTestModel(t)
	ids := fillTimeline(t, m, me, bob, 5)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	press(m, tea.KeyUp)
	press(m, tea.KeyUp)
	press(m, tea.KeyRight)
	if m.threadRoot != ids[3] || m.pane != paneThread || m.toMain {
		t.Fatalf("root=%q pane=%v toMain=%v", m.threadRoot, m.pane, m.toMain)
	}
	press(m, tea.KeyLeft)
	if m.threadRoot != "" || m.pane != paneMain {
		t.Fatalf("root=%q pane=%v", m.threadRoot, m.pane)
	}
	if sel, _ := m.selectedMsg(); sel.ID != ids[3] {
		t.Fatalf("selection lost: %s", sel.ID)
	}
	// Esc with a thread open does the same; a second Esc returns to the input
	press(m, tea.KeyRight)
	press(m, tea.KeyEsc)
	if m.threadRoot != "" || m.pane != paneMain {
		t.Fatalf("esc: root=%q pane=%v", m.threadRoot, m.pane)
	}
	if sel, _ := m.selectedMsg(); sel.ID != ids[3] {
		t.Fatalf("esc lost selection: %s", sel.ID)
	}
	press(m, tea.KeyEsc)
	if m.pane != paneInput {
		t.Fatal("second Esc should focus the input")
	}
}

func TestArrowsInsideInputMoveTheCursorOnly(t *testing.T) {
	m, me, bob := newTestModel(t)
	m.conn = &fakeLink{}
	fillTimeline(t, m, me, bob, 3)
	m.openThread(m.st.Timeline("hell")[0])
	m.pane = paneInput
	m.input.Focus()
	m.input.SetValue("abc")
	before := m.input.LineInfo().CharOffset
	press(m, tea.KeyLeft)
	if m.input.LineInfo().CharOffset != before-1 || m.threadRoot == "" || m.pane != paneInput {
		t.Fatalf("left: offset %d->%d root=%q pane=%v", before, m.input.LineInfo().CharOffset, m.threadRoot, m.pane)
	}
	press(m, tea.KeyRight)
	if m.input.LineInfo().CharOffset != before || m.threadRoot == "" || m.pane != paneInput {
		t.Fatalf("right: offset %d root=%q pane=%v", m.input.LineInfo().CharOffset, m.threadRoot, m.pane)
	}
}

func TestThreadPanelScrollsWithoutHidingReplies(t *testing.T) {
	m, me, bob := newTestModel(t)
	ids := fillTimeline(t, m, me, bob, 1)
	for i := 1; i <= 4; i++ {
		pm := sealedMsg(t, bob, me, "bob", fmt.Sprintf("r%d", i), "hell", ids[0], fmt.Sprintf("tra loi %d", i))
		pm.CreatedAt = int64(1_700_000_100_000 + i*1000)
		m.onEvent(proto.Event{T: "msg", Message: &pm})
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	m.openThread(m.st.Timeline("hell")[0])
	m.pane = paneThread
	m.selThread = 3
	v := m.View()
	for i := 1; i <= 4; i++ {
		if !strings.Contains(v, fmt.Sprintf("tra loi %d", i)) {
			t.Fatalf("reply %d hidden:\n%s", i, v)
		}
	}
}

func sidebarModel(t *testing.T) *Model {
	t.Helper()
	m, me, bob := newTestModel(t)
	zed, _ := e2e.GenerateKeyPair()
	m.onEvent(proto.Event{T: "user", User: &proto.User{Username: "zed", DisplayName: "Zed Zed", PublicKey: base64.StdEncoding.EncodeToString(zed.Public[:]), Online: false}})
	_, _ = me, bob
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	return m
}

func sideRows(m *Model) []string {
	var rows []string
	for _, l := range strings.Split(m.viewSidebar(24, 33), "\n") {
		rows = append(rows, l)
	}
	return rows
}

func TestSidebarOneLinePerUserWithSeparator(t *testing.T) {
	m := sidebarModel(t) // users: me (not listed), bob, zed
	rows := sideRows(m)
	var content []string
	for _, r := range rows[1 : len(rows)-1] {
		if p := strings.TrimSpace(strings.Trim(ansi.Strip(r), "│")); p != "" {
			content = append(content, p)
		}
	}
	// Hell room, separator, bob, zed: exactly one line per user
	if len(content) != 4 {
		t.Fatalf("want 4 content rows, got %d: %q", len(content), content)
	}
	if !strings.Contains(content[0], "Hell room") || strings.Trim(content[1], "─") != "" {
		t.Fatalf("separator missing under Hell room: %q", content)
	}
	if !strings.Contains(content[2], "Phát") || !strings.Contains(content[3], "Zed Zed") {
		t.Fatalf("user rows: %q", content)
	}
	if w := width(strings.Trim(ansi.Strip(rows[2]), "│")); w != 22 {
		t.Fatalf("separator width %d, want 22", w)
	}
}

func TestSidebarActiveRowHasBackgroundAcrossWidth(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(old)
	m := sidebarModel(t)
	for _, active := range []string{"hell", "dm:bob|me"} {
		m.st.SetActive(active)
		var activeRows, otherRows int
		for _, r := range sideRows(m) {
			plain := ansi.Strip(r)
			if !strings.ContainsAny(plain, "#ZP") || strings.Contains(plain, "─") {
				continue
			}
			isActive := (active == "hell" && strings.Contains(plain, "Hell room")) || (active != "hell" && strings.Contains(plain, "Phát"))
			if isActive {
				activeRows++
				if !strings.Contains(r, "48;2;") {
					t.Fatalf("active row %q has no background SGR: %q", active, r)
				}
				if w := width(plain); w != 24 {
					t.Fatalf("row width %d", w)
				}
				// the background must still be on after every inner style reset
				if strings.Contains(r, "\x1b[0m\x1b[0m") {
					t.Fatal("odd reset sequence")
				}
			} else {
				otherRows++
				if strings.Contains(r, "48;2;") {
					t.Fatalf("inactive row has a background: %q", r)
				}
			}
		}
		if activeRows != 1 || otherRows == 0 {
			t.Fatalf("active=%q: activeRows=%d otherRows=%d", active, activeRows, otherRows)
		}
	}
}

func TestSelectedMessageHasFullWidthBackground(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer lipgloss.SetColorProfile(old)
	mm := msg("hello https://a.b/c\nsecond line @me")
	for _, sel := range []bool{true, false} {
		for _, l := range renderMessage(mm, msgView{Name: "Bob", Me: "me", Width: 50, Selected: sel}) {
			has := strings.Contains(l, "48;2;")
			if has != sel {
				t.Fatalf("selected=%v line background=%v: %q", sel, has, l)
			}
			if sel && width(l) != 50 {
				t.Fatalf("selected line width %d, want 50: %q", width(l), l)
			}
		}
	}
}
