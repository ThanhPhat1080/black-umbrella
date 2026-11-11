// Package proto holds the wire types shared with the server.
package proto

type User struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	PublicKey   string `json:"publicKey"`
	JoinedAt    int64  `json:"joinedAt"`
	Online      bool   `json:"online"`
	LastSeen    *int64 `json:"lastSeen"`
}

type Message struct {
	ID        string `json:"id"`
	Conv      string `json:"conv"`
	Sender    string `json:"sender"`
	ParentID  string `json:"parentId"` // "" for roots (null on the wire)
	CreatedAt int64  `json:"createdAt"`
	Kept      bool   `json:"kept"`
	Nonce     string `json:"nonce"`
	CT        string `json:"ct"`
	SenderKey string `json:"senderKey"` // sender's public key at send time ("" for old messages)
	Key       string `json:"key"`
}

// Room is a private group conversation; Members are ordered by join time.
type Room struct {
	ID      string   `json:"id"` // also the conversation id ("room:<hex>")
	Name    string   `json:"name"`
	Owner   string   `json:"owner"`
	Members []string `json:"members"`
}

// Note says what changed in a room (attached to a "room" event).
type Note struct {
	Kind     string   `json:"kind"` // created, added, left, kicked, renamed
	By       string   `json:"by"`
	Users    []string `json:"users,omitempty"`
	NewOwner string   `json:"newOwner,omitempty"`
	Name     string   `json:"name,omitempty"`
}

// Event is any server -> client WebSocket frame, discriminated by T.
type Event struct {
	T        string    `json:"t"`
	Me       string    `json:"me,omitempty"`
	Users    []User    `json:"users,omitempty"`
	Messages []Message `json:"messages,omitempty"`
	Message  *Message  `json:"message,omitempty"`
	Ref      string    `json:"ref,omitempty"`
	ID       string    `json:"id,omitempty"`
	Conv     string    `json:"conv,omitempty"`
	Username string    `json:"username,omitempty"`
	Error    string    `json:"error,omitempty"`
	Kept     bool      `json:"kept,omitempty"`
	IDs      []string  `json:"ids,omitempty"`
	User     *User     `json:"user,omitempty"`
	Until    int64     `json:"until,omitempty"`
	Room     *Room     `json:"room,omitempty"`
	Rooms    []Room    `json:"rooms,omitempty"`
	Note     *Note     `json:"note,omitempty"`
	Detail   string    `json:"detail,omitempty"` // extra context for an error (usually a username)
	Reason   string    `json:"reason,omitempty"` // room_gone: left, kicked, closed
	By       string    `json:"by,omitempty"`
}

type Send struct {
	T        string            `json:"t"`
	Ref      string            `json:"ref"`
	Conv     string            `json:"conv"`
	ParentID string            `json:"parentId,omitempty"`
	Nonce    string            `json:"nonce"`
	CT       string            `json:"ct"`
	Keys     map[string]string `json:"keys"`
}

type Keep struct {
	T    string `json:"t"`
	ID   string `json:"id"`
	Kept bool   `json:"kept"`
}

type Typing struct {
	T    string `json:"t"`
	Conv string `json:"conv"`
}

type Name struct {
	T           string `json:"t"`
	DisplayName string `json:"displayName"`
}

func NewSend(ref, conv, parentID, nonce, ct string, keys map[string]string) Send {
	return Send{T: "send", Ref: ref, Conv: conv, ParentID: parentID, Nonce: nonce, CT: ct, Keys: keys}
}
func NewKeep(id string, kept bool) Keep { return Keep{T: "keep", ID: id, Kept: kept} }
func NewTyping(conv string) Typing      { return Typing{T: "typing", Conv: conv} }
func NewName(displayName string) Name   { return Name{T: "name", DisplayName: displayName} }

type RoomCreate struct {
	T       string   `json:"t"`
	Ref     string   `json:"ref"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
}
type RoomInvite struct {
	T     string   `json:"t"`
	Ref   string   `json:"ref"`
	Room  string   `json:"room"`
	Users []string `json:"users"`
}
type RoomKick struct {
	T    string `json:"t"`
	Ref  string `json:"ref"`
	Room string `json:"room"`
	User string `json:"user"`
}
type RoomOp struct { // room_leave and room_close
	T    string `json:"t"`
	Ref  string `json:"ref"`
	Room string `json:"room"`
}
type RoomName struct {
	T    string `json:"t"`
	Ref  string `json:"ref"`
	Room string `json:"room"`
	Name string `json:"name"`
}

func NewRoomCreate(ref, name string, members []string) RoomCreate {
	return RoomCreate{T: "room_create", Ref: ref, Name: name, Members: members}
}
func NewRoomInvite(ref, room string, users []string) RoomInvite {
	return RoomInvite{T: "room_invite", Ref: ref, Room: room, Users: users}
}
func NewRoomKick(ref, room, user string) RoomKick {
	return RoomKick{T: "room_kick", Ref: ref, Room: room, User: user}
}
func NewRoomLeave(ref, room string) RoomOp { return RoomOp{T: "room_leave", Ref: ref, Room: room} }
func NewRoomClose(ref, room string) RoomOp { return RoomOp{T: "room_close", Ref: ref, Room: room} }
func NewRoomName(ref, room, name string) RoomName {
	return RoomName{T: "room_name", Ref: ref, Room: room, Name: name}
}
