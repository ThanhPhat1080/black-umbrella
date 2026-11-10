package tui

import (
	"fmt"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Terminals cannot send the Cmd key, so editing shortcuts are the ones they can:
//
//	word left/right   Ctrl+←/→  Alt/Option+←/→
//	line start/end    Home/End  Ctrl+A/Ctrl+E (iTerm2 & friends send these for Cmd+←/→)
//	delete word       Alt+Backspace, Ctrl+W      delete to line start: Ctrl+U (Cmd+Backspace)
//	select all        Ctrl+L or Alt+A (Cmd+A is swallowed by the terminal; map it to send 0x0c)
const keySelectAll = "ctrl+l"

// ctrlASelectsAll: on Windows and Linux Ctrl+A is "select all" as users expect there; on macOS it
// stays "line start" because iTerm2-style Cmd+← mappings send it.
var ctrlASelectsAll = runtime.GOOS != "darwin"

// editKeys adds the common word-jump bindings the textarea lacks by default (Ctrl+←/→).
func editKeys(km *textarea.KeyMap) {
	km.WordBackward = key.NewBinding(key.WithKeys("ctrl+left", "alt+left", "alt+b"))
	km.WordForward = key.NewBinding(key.WithKeys("ctrl+right", "alt+right", "alt+f"))
	km.DeleteWordBackward = key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w", "ctrl+backspace"))
	km.DeleteWordForward = key.NewBinding(key.WithKeys("alt+delete", "alt+d", "ctrl+delete"))
}

// selectAllKey implements select-all for a textbox: the whole text is highlighted and copied;
// typing or pasting replaces it, Backspace/Delete removes it, a cursor key just deselects.
func (m *Model) selectAllKey(b *textarea.Model, k tea.KeyMsg) (bool, tea.Cmd) {
	s := k.String()
	if s == keySelectAll || s == "alt+a" || s == "ctrl+a" && ctrlASelectsAll {
		if b.Value() == "" {
			return true, nil
		}
		m.selAllBox = b
		if err := copyText(b.Value()); err == nil {
			m.setStatus(fmt.Sprintf("Selected all (%d characters, copied). Type to replace, Backspace to cut.", utf8.RuneCountInString(b.Value())))
		}
		return true, nil
	}
	if m.selAllBox != b {
		return false, nil
	}
	m.selAllBox = nil
	switch s {
	case "backspace", "delete", "ctrl+h", "ctrl+d", "ctrl+w", "ctrl+u", "ctrl+k":
		b.Reset()
		return true, nil
	case "left", "up", "home", "ctrl+home":
		toBegin(b)
		return true, nil
	case "right", "down", "end", "ctrl+e", "ctrl+end":
		b.SetValue(b.Value()) // cursor goes to the end
		return true, nil
	}
	if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
		b.Reset() // typing or pasting replaces the selection; the key itself is then inserted
	}
	return false, nil
}

// boxView renders a textbox, painting the text as selected after select-all.
func (m *Model) boxView(b *textarea.Model) string {
	v := b.View()
	if m.selAllBox != b || b.Value() == "" {
		return v
	}
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		if plain := strings.TrimRight(ansi.Strip(l), " "); plain != "" {
			lines[i] = dragHl.Render(plain) + strings.Repeat(" ", maxInt(0, width(l)-width(plain)))
		}
	}
	return strings.Join(lines, "\n")
}

// toBegin moves the cursor to the very start of the text.
func toBegin(b *textarea.Model) {
	for i := 0; i < 10000 && (b.Line() > 0 || b.LineInfo().RowOffset > 0); i++ {
		b.CursorUp()
	}
	b.CursorStart()
}
