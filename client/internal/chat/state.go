// Package chat is the pure (UI-free) client state: users, decrypted messages, unread, typing.
package chat

import (
	"encoding/base64"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/e2e"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

const (
	ExpiredText       = "[message expired]"
	UndecryptableText = "[unable to decrypt]"
	typingTTL         = 4 * time.Second
	// BuzzText is the body of a buzz message. It travels as ordinary encrypted text, so the
	// server needs no changes and older clients just show "/buzz".
	BuzzText = "/buzz"
	// DanceText is the body of a /dance message; like a buzz it is plain encrypted text.
	DanceText = "/dance"
)

type Msg struct {
	ID, Conv, Sender, ParentID, Text string
	CreatedAt                        time.Time
	Kept, Undecryptable              bool
	Replies                          int
}

type Effects struct{ Bell, Buzz, Dance bool }

type Entry struct {
	Conv, Label, Username string
	Online                bool
	Unread                int
}

type State struct {
	Now func() time.Time

	me     string
	kp     e2e.KeyPair
	users  map[string]proto.User
	msgs   map[string]*Msg
	unread map[string]int
	typing map[string]map[string]time.Time // conv -> user -> when
	active string

	newest   time.Time
	hadHello bool
}

func New(me string, kp e2e.KeyPair) *State {
	return &State{
		Now: time.Now, me: me, kp: kp,
		users: map[string]proto.User{}, msgs: map[string]*Msg{},
		unread: map[string]int{}, typing: map[string]map[string]time.Time{}, active: "hell",
	}
}

func (s *State) Me() string { return s.me }

// DMConv returns the canonical conversation id for two users.
func DMConv(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return "dm:" + a + "|" + b
}

func pubKey(u proto.User) ([32]byte, bool) { return decodeKey(u.PublicKey) }

func decodeKey(b64 string) ([32]byte, bool) {
	var k [32]byte
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(b) != 32 {
		return k, false
	}
	copy(k[:], b)
	return k, true
}

func (s *State) decrypt(m proto.Message) *Msg {
	out := &Msg{ID: m.ID, Conv: m.Conv, Sender: SanitizeLine(m.Sender), ParentID: m.ParentID, Kept: m.Kept, CreatedAt: time.UnixMilli(m.CreatedAt)}
	var sp [32]byte
	ok := false
	if m.Sender == s.me {
		sp, ok = s.kp.Public, true
	} else if k, good := decodeKey(m.SenderKey); good {
		sp, ok = k, true // the key the sender had when sending (survives a later re-registration)
	} else if u, found := s.users[m.Sender]; found {
		sp, ok = pubKey(u)
	}
	if ok {
		if text, err := e2e.Open(m.Nonce, m.CT, m.Key, s.kp, sp); err == nil {
			out.Text = Sanitize(text)
			return out
		}
	}
	out.Undecryptable = true
	out.Text = UndecryptableText
	return out
}

var mentionCache = map[string]*regexp.Regexp{}

// Mentions reports whether text contains @me as a whole word (case-insensitive).
func Mentions(text, me string) bool {
	re, ok := mentionCache[me]
	if !ok {
		re = regexp.MustCompile(`(?i)(^|[^a-z0-9_])@` + regexp.QuoteMeta(me) + `($|[^a-z0-9_])`)
		mentionCache[me] = re
	}
	return re.MatchString(text)
}

// Apply folds a server event into the state.
func (s *State) Apply(ev proto.Event) Effects {
	var fx Effects
	switch ev.T {
	case "hello":
		known, newest, had := s.msgs, s.newest, s.hadHello
		s.users = map[string]proto.User{}
		s.msgs = map[string]*Msg{}
		s.typing = map[string]map[string]time.Time{}
		for _, u := range ev.Users {
			u = cleanUser(u)
			s.users[u.Username] = u
		}
		for _, m := range ev.Messages {
			dm := s.decrypt(m)
			s.msgs[m.ID] = dm
			if dm.CreatedAt.After(s.newest) {
				s.newest = dm.CreatedAt
			}
			// A reconnect snapshot can contain messages we never saw live: count those as new.
			if _, seen := known[m.ID]; had && !seen && dm.CreatedAt.After(newest) && dm.Sender != s.me {
				s.noteIncoming(dm, &fx)
			}
		}
		s.hadHello = true
	case "msg":
		if ev.Message == nil {
			break
		}
		m := s.decrypt(*ev.Message)
		if _, dup := s.msgs[m.ID]; dup {
			break
		}
		s.msgs[m.ID] = m
		fx.Buzz = m.Text == BuzzText && !m.Undecryptable
		fx.Dance = m.Text == DanceText && !m.Undecryptable
		if m.Sender == s.me {
			break
		}
		if m.CreatedAt.After(s.newest) {
			s.newest = m.CreatedAt
		}
		if t := s.typing[m.Conv]; t != nil {
			delete(t, m.Sender)
		}
		s.noteIncoming(m, &fx)
	case "kept":
		if m, ok := s.msgs[ev.ID]; ok {
			m.Kept = ev.Kept
		}
	case "expired":
		for _, id := range ev.IDs {
			delete(s.msgs, id)
		}
	case "typing":
		if ev.Username == s.me {
			break
		}
		if s.typing[ev.Conv] == nil {
			s.typing[ev.Conv] = map[string]time.Time{}
		}
		s.typing[ev.Conv][ev.Username] = s.Now()
	case "user":
		if ev.User != nil {
			u := cleanUser(*ev.User)
			s.users[u.Username] = u
		}
	}
	return fx
}

// noteIncoming does the unread / mention bookkeeping for a message from someone else.
func (s *State) noteIncoming(m *Msg, fx *Effects) {
	if m.Conv == "hell" && !m.Undecryptable && Mentions(m.Text, s.me) {
		fx.Bell = true
	}
	if m.Conv != s.active {
		s.unread[m.Conv]++
	}
}

func (s *State) replyCounts() map[string]int {
	c := map[string]int{}
	for _, m := range s.msgs {
		if m.ParentID != "" {
			c[m.ParentID]++
		}
	}
	return c
}

func sortMsgs(l []Msg) {
	sort.Slice(l, func(i, j int) bool {
		if !l[i].CreatedAt.Equal(l[j].CreatedAt) {
			return l[i].CreatedAt.Before(l[j].CreatedAt)
		}
		return l[i].ID < l[j].ID
	})
}

// Timeline returns the conversation's top-level messages, oldest first. Replies whose
// root has expired are included so they stay reachable (they open a thread with a
// "[message expired]" root).
func (s *State) Timeline(conv string) []Msg {
	counts := s.replyCounts()
	var out []Msg
	for _, m := range s.msgs {
		if m.Conv != conv {
			continue
		}
		if m.ParentID != "" {
			if _, ok := s.msgs[m.ParentID]; ok {
				continue
			}
		}
		c := *m
		c.Replies = counts[m.ID]
		out = append(out, c)
	}
	sortMsgs(out)
	return out
}

// Thread returns the root (never nil: an expired root is a placeholder) and its replies.
func (s *State) Thread(rootID string) (*Msg, []Msg) {
	counts := s.replyCounts()
	var replies []Msg
	conv := ""
	for _, m := range s.msgs {
		if m.ParentID == rootID {
			replies = append(replies, *m)
			conv = m.Conv
		}
	}
	sortMsgs(replies)
	if r, ok := s.msgs[rootID]; ok {
		c := *r
		c.Replies = counts[rootID]
		return &c, replies
	}
	return &Msg{ID: rootID, Conv: conv, Text: ExpiredText, Replies: len(replies)}, replies
}

func (s *State) Find(id string) (Msg, bool) {
	m, ok := s.msgs[id]
	if !ok {
		return Msg{}, false
	}
	return *m, true
}

func (s *State) Sidebar() []Entry {
	out := []Entry{{Conv: "hell", Label: "Hell room", Unread: s.unread["hell"]}}
	var us []proto.User
	for _, u := range s.users {
		if u.Username != s.me {
			us = append(us, u)
		}
	}
	sort.Slice(us, func(i, j int) bool {
		if us[i].Online != us[j].Online {
			return us[i].Online
		}
		return us[i].Username < us[j].Username
	})
	for _, u := range us {
		c := DMConv(s.me, u.Username)
		label := u.DisplayName
		if label == "" {
			label = u.Username
		}
		out = append(out, Entry{Conv: c, Label: label, Username: u.Username, Online: u.Online, Unread: s.unread[c]})
	}
	return out
}

func (s *State) SetActive(conv string) {
	s.active = conv
	delete(s.unread, conv)
}

func (s *State) Active() string { return s.active }

// Recipients lists the public keys a message in conv must be sealed for (always including me).
func (s *State) Recipients(conv string) map[string][32]byte {
	out := map[string][32]byte{s.me: s.kp.Public}
	if conv == "hell" {
		for name, u := range s.users {
			if k, ok := pubKey(u); ok {
				out[name] = k
			}
		}
		out[s.me] = s.kp.Public
		return out
	}
	if strings.HasPrefix(conv, "dm:") {
		for _, name := range strings.Split(conv[3:], "|") {
			if name == s.me {
				continue
			}
			if u, ok := s.users[name]; ok {
				if k, ok := pubKey(u); ok {
					out[name] = k
				}
			}
		}
	}
	return out
}

// Typing lists the users typing in conv at `now`.
func (s *State) Typing(conv string, now time.Time) []string {
	var out []string
	for u, at := range s.typing[conv] {
		if now.Sub(at) <= typingTTL {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out
}

func (s *State) Users() map[string]proto.User {
	out := make(map[string]proto.User, len(s.users))
	for k, v := range s.users {
		out[k] = v
	}
	return out
}

// DisplayName returns the user's display name, falling back to the username.
func (s *State) DisplayName(username string) string {
	if u, ok := s.users[username]; ok && u.DisplayName != "" {
		return u.DisplayName
	}
	return username
}

func cleanUser(u proto.User) proto.User {
	u.Username = SanitizeLine(u.Username)
	u.DisplayName = SanitizeLine(u.DisplayName)
	return u
}

// Art wraps pre-formatted text (ASCII art) in a fenced block; clients draw it without
// re-wrapping or trimming, older ones just show the fences.
func Art(body string) string { return "```\n" + body + "\n```" }

// Preformatted returns the body of a fenced art message.
func Preformatted(text string) (string, bool) {
	if strings.HasPrefix(text, "```\n") && strings.HasSuffix(text, "\n```") && len(text) >= 8 {
		return text[4 : len(text)-4], true
	}
	return "", false
}
