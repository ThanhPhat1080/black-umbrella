package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/links"
)

// chipLookup resolves @name to a known user for the mention chips (a snapshot taken per render).
func (m *Model) chipLookup() chipLookup {
	us := m.st.Users()
	return func(name string) (string, string, bool, bool) {
		u, ok := us[strings.ToLower(name)]
		return u.Username, m.st.DisplayName(u.Username), u.Online, ok && u.Username != ""
	}
}

// openDM goes to the chat with user (your own profile when it is you).
func (m *Model) openDM(user string) {
	switch {
	case user == m.st.Me():
		m.showProfile(user)
	default:
		if _, ok := m.st.Users()[user]; ok {
			m.switchConv(chat.DMConv(m.st.Me(), user))
		}
	}
}

// visibleMsgs are the messages whose widgets can be clicked: the open conversation and thread.
func (m *Model) visibleMsgs() []chat.Msg {
	vis := append([]chat.Msg(nil), m.st.Timeline(m.st.Active())...)
	_, it := m.threadItems()
	return append(vis, it...)
}

// clickTargets lists every zone that reacts to a click, with the action it performs. Zones that are
// not on screen simply never match.
func (m *Model) clickTargets() []clickTarget {
	var ts []clickTarget
	add := func(id string, f func()) { ts = append(ts, clickTarget{id, f}) }
	for _, e := range m.st.Sidebar() {
		e := e
		if e.Username != "" {
			add("avs:"+e.Username, func() { m.showProfile(e.Username) })
		}
		add("side:"+e.Conv, func() { m.switchConv(e.Conv) })
	}
	if r, ok := m.activeRoom(); ok {
		for _, u := range r.Members {
			u := u
			add("rm:"+u, func() { m.openDM(u) })
		}
	}
	look := m.chipLookup()
	for _, v := range m.visibleMsgs() {
		v := v
		add("nm:"+v.ID, func() { m.openDM(v.Sender) })
		add("av:"+v.ID, func() { m.showProfile(v.Sender) })
		for k, u := range chipUsers(v.Text, look) {
			u := u
			add(fmt.Sprintf("men:%s:%d", v.ID, k), func() { m.openDM(u) })
		}
		for ui, u := range links.Find(v.Text) {
			u := u
			for f := 0; f < 16; f++ {
				add(fmt.Sprintf("lnk:%s:%d:%d", v.ID, ui, f), func() { m.openLink(u) })
			}
		}
	}
	return ts
}

type clickTarget struct {
	id string
	do func()
}

// clickUser runs the action of the user-related zone under the mouse; it reports whether there was one.
func (m *Model) clickUser(msg tea.MouseMsg) bool {
	for _, t := range m.clickTargets() {
		if strings.HasPrefix(t.id, "lnk:") {
			continue // links are handled with the buttons
		}
		if zone.Get(t.id).InBounds(msg) {
			t.do()
			return true
		}
	}
	return false
}

// hoverAt records which clickable zone the pointer is over and switches the mouse pointer shape.
func (m *Model) hoverAt(msg tea.MouseMsg) {
	id := ""
	if m.overlay == overlayNone && !m.selectMode {
		for _, t := range m.clickTargets() {
			if zone.Get(t.id).InBounds(msg) {
				id = t.id
				break
			}
		}
		if id == "" && zone.Get("plus").InBounds(msg) {
			id = "plus"
		}
	}
	m.hover = id
	m.curOut.setPointer(id != "")
}
