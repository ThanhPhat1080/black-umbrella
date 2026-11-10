// Package api talks to the Umbrella server over HTTP and WebSocket.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIError struct {
	Status int
	Code   string
	Until  int64
}

func (e *APIError) Error() string { return fmt.Sprintf("server error %d: %s", e.Status, e.Code) }

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type RegisterReq struct {
	Invite      string `json:"invite"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Pin         string `json:"pin"`
	PublicKey   string `json:"publicKey"`
}

func (c *Client) post(ctx context.Context, path, bearer string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	} else {
		buf.WriteString("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	h := c.HTTP
	if h == nil {
		h = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := h.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
			Until int64  `json:"until"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = http.StatusText(res.StatusCode)
		}
		return &APIError{Status: res.StatusCode, Code: e.Error, Until: e.Until}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) Register(ctx context.Context, r RegisterReq) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	if err := c.post(ctx, "/api/register", "", r, &out); err != nil {
		return "", err
	}
	return out.Token, nil
}

func (c *Client) Login(ctx context.Context, user, pin string) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	err := c.post(ctx, "/api/login", "", map[string]string{"username": user, "pin": pin}, &out)
	return out.Token, err
}

func (c *Client) AdminInvite(ctx context.Context, adminToken string) (string, error) {
	var out struct {
		Code string `json:"code"`
	}
	err := c.post(ctx, "/api/admin/invite", adminToken, nil, &out)
	return out.Code, err
}

func (c *Client) AdminReset(ctx context.Context, adminToken, user string) (string, error) {
	var out struct {
		Code string `json:"code"`
	}
	err := c.post(ctx, "/api/admin/reset", adminToken, map[string]string{"username": user}, &out)
	return out.Code, err
}

func (c *Client) AdminUnlock(ctx context.Context, adminToken, user string) error {
	return c.post(ctx, "/api/admin/unlock", adminToken, map[string]string{"username": user}, nil)
}
