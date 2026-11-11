package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/chat"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

// roomErrorText words a server error that answers a room command.
func roomErrorText(code, detail string) string {
	switch code {
	case "invalid_room_name":
		return "Room names are 1-32 letters, digits, spaces or ! @ # $ % ^ & * ( ) _ - + = ~ `"
	case "unknown_user":
		return "No such user: " + detail
	case "already_member":
		return detail + " is already in this room."
	case "not_member":
		return detail + " is not in this room."
	case "room_full":
		return "A room holds at most 20 people."
	case "room_limit":
		return "You are in 10 rooms already. Leave one first."
	case "user_room_limit":
		return detail + " is in 10 rooms already."
	case "not_owner":
		return "Only the room owner can do that."
	case "not_found":
		return "That room doesn't exist."
	case "bad_request":
		return "That isn't allowed."
	}
	return "Server error: " + code
}

// atNames splits trailing @user words off a command argument: "My room @bob @eve" -> ("My room", [bob eve]).
func atNames(arg string) (rest string, users []string) {
	f := strings.Fields(arg)
	i := len(f)
	for i > 0 && len(f[i-1]) > 1 && f[i-1][0] == '@' {
		i--
	}
	for _, u := range f[i:] {
		users = append(users, strings.ToLower(u[1:]))
	}
	return strings.Join(f[:i], " "), users
}

// userArgs reads "@a @b" or "a b" (every word is a username).
func userArgs(arg string) []string {
	var us []string
	for _, w := range strings.Fields(arg) {
		us = append(us, strings.ToLower(strings.TrimPrefix(w, "@")))
	}
	return us
}

// activeRoom is the open conversation when it is a room.
func (m *Model) activeRoom() (proto.Room, bool) { return m.st.RoomOf(m.st.Active()) }

func (m *Model) newRoomRef() string {
	m.refN++
	ref := fmt.Sprintf("m%d", m.refN)
	m.roomRefs[ref] = true
	return ref
}

// roomSend transmits a room command built by mk; it forgets the ref when the send fails.
func (m *Model) roomSend(mk func(ref string) any) bool {
	ref := m.newRoomRef()
	if err := m.conn.Send(mk(ref)); err != nil {
		delete(m.roomRefs, ref)
		m.setStatus("Not connected. Reconnecting…")
		return false
	}
	return true
}

// roomCommand handles the room slash commands; handled is false for any other command.
func (m *Model) roomCommand(cmd, arg string) (handled bool, c tea.Cmd) {
	needRoom := func() (proto.Room, bool) {
		r, ok := m.activeRoom()
		if !ok {
			m.setStatus("Open a room first.")
		}
		return r, ok
	}
	switch cmd {
	case "/room":
		name, users := atNames(arg)
		if name == "" {
			m.setStatus("Usage: /room <name> [@user …]")
			return true, nil
		}
		if m.roomSend(func(ref string) any { return proto.NewRoomCreate(ref, name, users) }) {
			m.box().Reset()
		}
	case "/invite":
		r, ok := needRoom()
		if !ok {
			return true, nil
		}
		users := userArgs(arg)
		if len(users) == 0 {
			m.setStatus("Usage: /invite @user [@user …]")
			return true, nil
		}
		if m.roomSend(func(ref string) any { return proto.NewRoomInvite(ref, r.ID, users) }) {
			m.box().Reset()
		}
	case "/kick":
		r, ok := needRoom()
		if !ok {
			return true, nil
		}
		users := userArgs(arg)
		if len(users) != 1 {
			m.setStatus("Usage: /kick @user")
			return true, nil
		}
		if m.roomSend(func(ref string) any { return proto.NewRoomKick(ref, r.ID, users[0]) }) {
			m.box().Reset()
		}
	case "/leave":
		r, ok := needRoom()
		if !ok {
			return true, nil
		}
		if m.roomSend(func(ref string) any { return proto.NewRoomLeave(ref, r.ID) }) {
			m.box().Reset()
		}
	case "/closeroom":
		if _, ok := needRoom(); !ok {
			return true, nil
		}
		m.box().Reset()
		m.overlay = overlayClose
	case "/roomname":
		r, ok := needRoom()
		if !ok {
			return true, nil
		}
		name := strings.TrimSpace(arg)
		if name == "" {
			m.setStatus("Usage: /roomname <new name>")
			return true, nil
		}
		if m.roomSend(func(ref string) any { return proto.NewRoomName(ref, r.ID, name) }) {
			m.box().Reset()
		}
	case "/members":
		if _, ok := needRoom(); !ok {
			return true, nil
		}
		m.box().Reset()
		m.openMembers()
	default:
		return false, nil
	}
	return true, nil
}

// closeKey answers the "close this room?" confirmation.
func (m *Model) closeKey(s string) (tea.Model, tea.Cmd) {
	m.overlay = overlayNone
	if s == "y" || s == "Y" || s == keyEnter {
		if r, ok := m.activeRoom(); ok {
			m.roomSend(func(ref string) any { return proto.NewRoomClose(ref, r.ID) })
		}
	}
	return m, nil
}

// onRoomEvent reacts to room-related server events after the state has absorbed them.
func (m *Model) onRoomEvent(ev proto.Event, fx chat.Effects) {
	if fx.Status != "" {
		m.setStatus(fx.Status)
	}
	if ev.T == "room" && ev.Room != nil && ev.Ref != "" && m.roomRefs[ev.Ref] {
		delete(m.roomRefs, ev.Ref)
		if ev.Note != nil && ev.Note.Kind == "created" {
			m.switchConv(ev.Room.ID)
		}
	}
}

// roomNotice is the short-lived "X joined / left" line for the open room ("" when none).
func (m *Model) roomNotice() string {
	return m.st.Notice(m.st.Active(), time.Now())
}
