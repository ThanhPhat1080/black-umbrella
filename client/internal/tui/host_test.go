package tui

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/host"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

// asElectron switches the model to desktop-app behaviour and captures what it writes to
// stdout; the returned function stops capturing and decodes every OSC 777 notice.
func asElectron(t *testing.T) func() []Notice {
	t.Helper()
	old := hostElectron
	hostElectron = true
	t.Cleanup(func() { hostElectron = old })
	t.Setenv("UMBRELLA_HOST", "electron")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = stdout })
	return func() []Notice {
		os.Stdout = stdout
		w.Close()
		buf, _ := io.ReadAll(r)
		r.Close()
		var out []Notice
		for _, part := range strings.Split(string(buf), "\x1b]777;umbrella;") {
			i := strings.Index(part, "\x07")
			if i < 0 {
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(part[:i])
			if err != nil {
				t.Fatalf("bad base64 %q: %v", part[:i], err)
			}
			var n Notice
			if err := json.Unmarshal(raw, &n); err != nil {
				t.Fatal(err)
			}
			out = append(out, n)
		}
		return out
	}
}

func TestHostSequenceRoundTrips(t *testing.T) {
	seq := host.Sequence(Notice{K: "notify", Kind: "dm", From: "Phát 🌂", Text: "xin chào"})
	if !strings.HasPrefix(seq, "\x1b]777;umbrella;") || !strings.HasSuffix(seq, "\x07") {
		t.Fatalf("bad framing %q", seq)
	}
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]777;umbrella;"), "\x07"))
	var n Notice
	if err := json.Unmarshal(raw, &n); err != nil || n.From != "Phát 🌂" || n.Kind != "dm" {
		t.Fatalf("round trip %+v %v", n, err)
	}
}

func TestElectronCtrlCCopiesAndNeverQuits(t *testing.T) {
	_ = asElectron(t)
	copied := stubClipboard(t)
	m, _, _ := newTestModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}) // nothing selected: does nothing
	if m.overlay == overlayQuit || len(*copied) != 0 {
		t.Fatalf("Ctrl+C with no selection must be a no-op (overlay %v, copied %q)", m.overlay, *copied)
	}
	m.input.SetValue("xin chào Việt Nam")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.selAllBox == nil {
		t.Fatal("Ctrl+A should select all in the textbox")
	}
	if len(*copied) != 0 {
		t.Fatal("selecting all must not copy by itself")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if got := *copied; len(got) != 1 || got[0] != "xin chào Việt Nam" {
		t.Fatalf("copied %q", got)
	}
	if m.input.Value() == "" {
		t.Fatal("copy must not remove the text")
	}
}

func TestElectronCtrlXCutsSelectedTextbox(t *testing.T) {
	_ = asElectron(t)
	copied := stubClipboard(t)
	m, _, _ := newTestModel(t)
	m.input.SetValue("cắt tôi")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if got := *copied; len(got) != 1 || got[0] != "cắt tôi" {
		t.Fatalf("copied %q", got)
	}
	if m.input.Value() != "" || m.selAllBox != nil {
		t.Fatalf("cut should empty the box, got %q", m.input.Value())
	}
	if m.selectMode {
		t.Fatal("Ctrl+X must not toggle select mode in the desktop app")
	}
}

func TestElectronTypingReplacesSelectAll(t *testing.T) {
	_ = asElectron(t)
	m, _, _ := newTestModel(t)
	m.conn = &fakeLink{}
	m.input.SetValue("cũ")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mới"), Paste: true}) // a Ctrl+V paste
	if m.input.Value() != "mới" {
		t.Fatalf("paste should replace the selection, got %q", m.input.Value())
	}
}

func TestTerminalKeepsCtrlCQuitConfirm(t *testing.T) {
	m, _, _ := newTestModel(t) // not electron
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.overlay != overlayQuit {
		t.Fatal("the terminal client must still ask before quitting")
	}
}

func TestElectronHelpAndUpdate(t *testing.T) {
	rows := electronHelp(helpEntries)
	var all []string
	for _, r := range rows {
		all = append(all, r[0])
	}
	joined := strings.Join(all, "|")
	for _, gone := range []string{"/update", keyCopySel} {
		if strings.Contains(joined, gone) {
			t.Fatalf("%q should not be listed in the desktop app help", gone)
		}
	}
	for _, want := range []string{"Ctrl+A", "Ctrl+C", "Ctrl+X", "Ctrl+V"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("help misses %s: %v", want, all)
		}
	}
	_ = asElectron(t)
	m, _, _ := newTestModel(t)
	if strings.Contains(helpHint(), "select mode") {
		t.Fatal("status hint must not mention select mode")
	}
	typeAndEnter(m, "/update")
	if !strings.Contains(m.status, "desktop app") {
		t.Fatalf("status %q", m.status)
	}
}

func TestElectronNotifiesDMMentionBuzzAndUnread(t *testing.T) {
	drain := asElectron(t)
	m, me, bob := newTestModel(t)
	// a direct message while another conversation is open: notify + unread 1
	dm := sealedMsg(t, bob, me, "bob", "d1", "dm:bob|me", "", "chào bạn\nxuống dòng")
	m.onEvent(proto.Event{T: "msg", Message: &dm})
	m.hostUnread()
	// a mention in the Hell room (active): notify with active=true
	mention := sealedMsg(t, bob, me, "bob", "m1", "hell", "", "ê @me ơi")
	m.onEvent(proto.Event{T: "msg", Message: &mention})
	// a plain Hell message: no notification
	plain := sealedMsg(t, bob, me, "bob", "p1", "hell", "", "bình thường")
	m.onEvent(proto.Event{T: "msg", Message: &plain})
	// a buzz
	buzz := sealedMsg(t, bob, me, "bob", "z1", "hell", "", "/buzz")
	m.onEvent(proto.Event{T: "msg", Message: &buzz})
	m.hostUnread()

	var kinds []string
	var unread int
	for _, n := range drain() {
		switch n.K {
		case "notify":
			kinds = append(kinds, n.Kind)
			if n.Kind == "dm" && (n.From != "Phát 🌂" || n.Text != "chào bạn xuống dòng" || n.Active) {
				t.Fatalf("dm notice %+v", n)
			}
			if n.Kind == "mention" && !n.Active {
				t.Fatalf("mention in the open room should be active: %+v", n)
			}
		case "unread":
			unread = n.N
		}
	}
	if strings.Join(kinds, ",") != "dm,mention,buzz" {
		t.Fatalf("notify kinds %v", kinds)
	}
	if unread != 1 {
		t.Fatalf("unread = %d, want 1", unread)
	}
}
