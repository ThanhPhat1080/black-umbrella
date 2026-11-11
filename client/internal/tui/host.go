package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/host"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

// hostElectron is true inside the desktop app. There the keys behave like a web app (Ctrl+C
// copies, Ctrl+X cuts, Ctrl+A selects all), the host owns the clipboard and the window shake,
// and notifications go out as OSC 777 events. The plain terminal client is unchanged.
var hostElectron = host.Electron()

func init() {
	if hostElectron {
		helpEntries = electronHelp(helpEntries)
	}
}

// Notice is what the desktop app receives for notifications, shakes and the unread count.
type Notice struct {
	K      string `json:"k"`                // notify | unread
	Kind   string `json:"kind,omitempty"`   // dm | mention | buzz
	From   string `json:"from,omitempty"`   // sender's display name
	Conv   string `json:"conv,omitempty"`   // conversation id
	Text   string `json:"text,omitempty"`   // short message preview
	Active bool   `json:"active,omitempty"` // the conversation is the one on screen
	N      int    `json:"n,omitempty"`      // unread: total unread messages
}

// copyKeyName is how the help and status lines name the copy shortcut.
func copyKeyName() string {
	if hostElectron {
		return "Ctrl+C"
	}
	return keyCopySel
}

// electronHelp rewrites the shortcut cheat sheet for the desktop app.
func electronHelp(in [][2]string) [][2]string {
	var out [][2]string
	for _, e := range in {
		switch e[0] {
		case "/update", "Ctrl+C", keyCopySel:
			continue
		}
		if strings.Contains(e[1], "select all in the textbox") {
			e = [2]string{"Ctrl+A", "select all in the textbox (type to replace, Backspace to cut)"}
			out = append(out, e,
				[2]string{"Ctrl+C", "copy the selection (drag with the mouse, or Ctrl+A in the textbox)"},
				[2]string{"Ctrl+X", "cut the selected text in the textbox"},
				[2]string{"Ctrl+V", "paste"})
			continue
		}
		out = append(out, e)
	}
	return out
}

// helpHint is the one-line shortcut hint in the status bar.
func helpHint() string {
	if hostElectron {
		return strings.Replace(helpLine, "Ctrl+X select mode", "Ctrl+C copy", 1)
	}
	return helpLine
}

// copySel copies the current selection (a dragged range, or the textbox after select-all).
// With cut, a select-all selection in the textbox is removed afterwards.
func (m *Model) copySel(cut bool) (tea.Model, tea.Cmd) {
	text := ""
	switch {
	case m.drag.shown:
		text = m.selectedText()
	case m.selAllBox != nil:
		text = m.selAllBox.Value()
	}
	n := utf8.RuneCountInString(text)
	switch {
	case text == "":
		m.setStatus("Nothing selected.")
	case copyText(text) != nil:
		m.setStatus("Could not copy.")
	case cut && m.selAllBox != nil:
		m.selAllBox.Reset()
		m.selAllBox = nil
		m.setStatus(fmt.Sprintf("Cut %d characters.", n))
	default:
		m.setStatus(fmt.Sprintf("Copied %d characters.", n))
	}
	return m, nil
}

// hostNotify tells the desktop app about an incoming message worth a desktop notification
// (a direct message, a mention) or a buzz. The app decides what to do from its focus state.
func (m *Model) hostNotify(ev proto.Event, fx chat.Effects) {
	if !hostElectron || ev.Message == nil {
		return
	}
	msg := ev.Message
	n := Notice{K: "notify", From: m.st.DisplayName(msg.Sender), Conv: msg.Conv, Active: msg.Conv == m.st.Active()}
	switch {
	case fx.Buzz:
		n.Kind = "buzz"
	case fx.Dance || fx.Fish || fx.Incoming == nil || fx.Incoming.Undecryptable:
		return
	case fx.Bell:
		n.Kind = "mention"
	case strings.HasPrefix(msg.Conv, "dm:"):
		n.Kind = "dm"
	default:
		return
	}
	if n.Kind != "buzz" {
		n.Text = preview(fx.Incoming.Text)
	}
	host.Emit(n)
}

// hostUnread sends the total unread count to the desktop app whenever it changes.
func (m *Model) hostUnread() {
	if !hostElectron {
		return
	}
	if n := m.st.UnreadTotal(); n != m.sentUnread {
		m.sentUnread = n
		host.Emit(Notice{K: "unread", N: n})
	}
}

// preview collapses whitespace and cuts a message to a notification-sized snippet.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 140 {
		s = string(r[:140]) + "…"
	}
	return s
}
