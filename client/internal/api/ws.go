package api

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/ThanhPhat1080/black-umbrella/client/internal/proto"
)

// WSURL derives the WebSocket URL from the HTTP base URL.
func WSURL(baseURL, token string) (string, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/ws"
	u.RawQuery = url.Values{"token": {token}}.Encode()
	return u.String(), nil
}

type Conn struct {
	ws     *websocket.Conn
	events chan proto.Event
	cancel context.CancelFunc
	ctx    context.Context
	once   sync.Once
}

// Dial opens the WebSocket. The Events channel is closed when the connection ends.
func Dial(ctx context.Context, baseURL, token string) (*Conn, error) {
	u, err := WSURL(baseURL, token)
	if err != nil {
		return nil, err
	}
	ws, _, err := websocket.Dial(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(32 << 20)
	cctx, cancel := context.WithCancel(context.Background())
	c := &Conn{ws: ws, events: make(chan proto.Event, 64), cancel: cancel, ctx: cctx}
	go c.readLoop()
	return c, nil
}

func (c *Conn) readLoop() {
	defer close(c.events)
	for {
		_, data, err := c.ws.Read(c.ctx)
		if err != nil {
			return
		}
		var ev proto.Event
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		select {
		case c.events <- ev:
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Conn) Events() <-chan proto.Event { return c.events }

func (c *Conn) Send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.ws.Write(c.ctx, websocket.MessageText, data)
}

func (c *Conn) Close() {
	c.once.Do(func() {
		c.cancel()
		_ = c.ws.CloseNow()
	})
}
