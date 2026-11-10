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
	Key       string `json:"key"`
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
