package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
	"github.com/coder/websocket"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginLocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(423)
		_, _ = w.Write([]byte(`{"error":"locked","until":1234567}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	_, err := c.Login(context.Background(), "alice", "123456")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 423 || ae.Code != "locked" || ae.Until != 1234567 {
		t.Fatalf("got %#v", err)
	}
}

func TestRegisterWireFields(t *testing.T) {
	var got map[string]any
	var path, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"token":"tok","user":{}}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/"}
	tok, err := c.Register(context.Background(), RegisterReq{Invite: "i", Username: "alice", DisplayName: "Alice", Pin: "123456", PublicKey: "pk"})
	if err != nil || tok != "tok" {
		t.Fatal(tok, err)
	}
	want := map[string]any{"invite": "i", "username": "alice", "displayName": "Alice", "pin": "123456", "publicKey": "pk"}
	if len(got) != len(want) || path != "/api/register" || method != "POST" {
		t.Fatalf("%v %s %s", got, path, method)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("field %s: %v", k, got[k])
		}
	}
}

func TestAdminBearerAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sekret" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		switch r.URL.Path {
		case "/api/admin/invite", "/api/admin/reset":
			_, _ = w.Write([]byte(`{"code":"abc"}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL}
	ctx := context.Background()
	if code, err := c.AdminInvite(ctx, "sekret"); err != nil || code != "abc" {
		t.Fatal(code, err)
	}
	if code, err := c.AdminReset(ctx, "sekret", "bob"); err != nil || code != "abc" {
		t.Fatal(code, err)
	}
	if err := c.AdminUnlock(ctx, "sekret", "bob"); err != nil {
		t.Fatal(err)
	}
	_, err := c.AdminInvite(ctx, "wrong")
	var ae *APIError
	if !errors.As(err, &ae) || ae.Status != 401 || ae.Code != "unauthorized" {
		t.Fatalf("%#v", err)
	}
}

func TestWSURL(t *testing.T) {
	cases := map[string]string{
		"https://h.dev":          "wss://h.dev/api/ws?token=a%2Bb",
		"http://localhost:8787/": "ws://localhost:8787/api/ws?token=a%2Bb",
	}
	for in, want := range cases {
		got, err := WSURL(in, "a+b")
		if err != nil || got != want {
			t.Fatalf("%s -> %s %v", in, got, err)
		}
	}
}

func TestDialReceivesEventsAndSends(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ws" || r.URL.Query().Get("token") != "tok" {
			w.WriteHeader(401)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_ = c.Write(r.Context(), websocket.MessageText, []byte(`{"t":"hello","me":"alice","messages":[{"id":"1","parentId":null,"conv":"hell"}]}`))
		_, data, err := c.Read(r.Context())
		if err == nil {
			got <- string(data)
		}
	}))
	defer srv.Close()
	conn, err := Dial(context.Background(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ev := <-conn.Events()
	if ev.T != "hello" || ev.Me != "alice" || len(ev.Messages) != 1 || ev.Messages[0].ParentID != "" {
		t.Fatalf("%+v", ev)
	}
	if err := conn.Send(proto.NewTyping("hell")); err != nil {
		t.Fatal(err)
	}
	if s := <-got; s != `{"t":"typing","conv":"hell"}` {
		t.Fatal(s)
	}
	if _, err := Dial(context.Background(), srv.URL, "bad"); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestSendDoesNotHTMLEscape(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, data, _ := c.Read(r.Context())
		got <- string(data)
		_ = c.Close(websocket.StatusCode(4001), "revoked")
	}))
	defer srv.Close()
	conn, err := Dial(context.Background(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.Send(proto.NewName("a<b>&c"))
	if s := <-got; s != `{"t":"name","displayName":"a<b>&c"}` {
		t.Fatal(s)
	}
	// the close code is exposed once the event channel is closed
	for range conn.Events() {
	}
	if conn.CloseCode() != 4001 {
		t.Fatalf("close code %d", conn.CloseCode())
	}
}
