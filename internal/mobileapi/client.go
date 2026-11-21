// Package mobileapi is the JSON client the native phone app uses. It talks to
// the same /api as the browser, either directly to a desktop or through the
// self-hosted relay at /d/<desktop_id>.
package mobileapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/e2e"
	"chatgo/pkg/models"
)

// Client is one server connection.
type Client struct {
	BaseURL   string
	Token     string
	DesktopID string
	// DeviceID is the relay device id used in the E2E AAD.
	DeviceID string
	// E2EKey, when set, encrypts API bodies and event frames. nil keeps the
	// plaintext path for older desktops.
	E2EKey []byte
	HTTP   *http.Client
	// OnAuthError is called when the relay or the desktop refuses the
	// device token. The phone uses it to drop the dead token and ask for a
	// new pairing instead of retrying forever.
	OnAuthError func(error)
}

// New returns a client for a relay base URL such as
// https://relay.example.com/d/d_abc.
func New(baseURL, token, desktopID string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Token:     token,
		DesktopID: desktopID,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var plaintext []byte
	if in != nil {
		var err error
		plaintext, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	data, err := c.send(ctx, method, path, plaintext, in != nil)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// send performs one request and returns the body, decrypting it when the
// desktop sealed it. A JSON body is marked as such unless it was sealed.
func (c *Client) send(ctx context.Context, method, path string, plaintext []byte, jsonBody bool) ([]byte, error) {
	body := plaintext
	var nonce []byte
	var ts int64
	if c.E2EKey != nil && len(plaintext) > 0 {
		var ciphertext []byte
		var err error
		ts = e2e.Timestamp()
		nonce, ciphertext, err = e2e.Seal(c.E2EKey, plaintext, e2e.AAD(method, path, c.DeviceID, ts))
		if err != nil {
			return nil, err
		}
		body = ciphertext
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if jsonBody && (c.E2EKey == nil || len(nonce) == 0) {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.E2EKey != nil {
		if len(nonce) > 0 {
			req.Header.Set("Content-Type", "application/octet-stream")
			req.Header.Set(e2e.HeaderNonce, base64.StdEncoding.EncodeToString(nonce))
		}
		req.Header.Set(e2e.HeaderVersion, e2e.Version)
		// The timestamp travels even without a body: the desktop checks the
		// window before it accepts the envelope.
		if ts == 0 {
			ts = e2e.Timestamp()
		}
		req.Header.Set(e2e.HeaderTime, strconv.FormatInt(ts, 10))
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.DesktopID != "" {
		req.Header.Set("X-ChatGo-Desktop-ID", c.DesktopID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.Header.Get(e2e.HeaderVersion) == e2e.Version {
		if c.E2EKey == nil {
			return nil, fmt.Errorf("response is encrypted but this client has no key")
		}
		nonce, err := base64.StdEncoding.DecodeString(resp.Header.Get(e2e.HeaderNonce))
		if err != nil {
			return nil, err
		}
		data, err = e2e.Open(c.E2EKey, nonce, data, e2e.ResponseAAD(c.DesktopID))
		if err != nil {
			return nil, fmt.Errorf("decrypt response: %w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			err := APIError{
				Status:  resp.StatusCode,
				Code:    e.Error.Code,
				Message: e.Error.Message,
				Hint:    hintFor(e.Error.Code),
			}
			c.reportAuthError(err)
			return nil, err
		}
		err := APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
		c.reportAuthError(err)
		return nil, err
	}
	return data, nil
}

// AgentIcon fetches the icon an agent was configured with. A 404 means the
// agent has none, which the phone draws as its own mark.
func (c *Client) AgentIcon(ctx context.Context, ref string) ([]byte, error) {
	return c.send(ctx, http.MethodGet, "/api/agents/icon?ref="+url.QueryEscape(ref), nil, false)
}

// APIError is an error response from the relay or desktop.
type APIError struct {
	Status  int
	Code    string
	Message string
	// Hint is the phone's own wording for a known error code. It is what
	// the user reads; Message keeps the raw text for the details view.
	Hint string
}

// apiErrorHints turns the error codes the relay and the desktop use into a
// line the phone user can act on.
var apiErrorHints = map[string]string{
	"unauthorized":    "设备令牌已失效，请在设置里重新配对",
	"desktop_offline": "桌面当前离线，请确认桌面上的 ChatGo 正在运行",
	"rate_limited":    "请求过于频繁，请稍后再试",
	"pair_failed":     "配对失败，请确认配对码是否已过期",
	"e2e_clock_skew":  "手机与桌面时间相差过大，请校准手机时间后重试",
	"e2e_rejected":    "加密请求被桌面拒绝，请重新配对后再试",
}

func hintFor(code string) string { return apiErrorHints[code] }
func (e APIError) Error() string {
	if e.Hint != "" {
		return e.Hint
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("HTTP %d", e.Status)
}

// reportAuthError tells the app that the device token is no longer good.
func (c *Client) reportAuthError(err error) {
	if c.OnAuthError == nil {
		return
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		return
	}
	c.OnAuthError(err)
}

// Meta is GET /api/meta.
type Meta struct {
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	Ready        bool            `json:"ready"`
	Error        string          `json:"error,omitempty"`
	Note         string          `json:"note,omitempty"`
	Models       []ModelChoice   `json:"models,omitempty"`
	Capabilities map[string]bool `json:"capabilities"`
	// Theme mirrors the desktop's appearance so the phone uses the same
	// accent and light/dark choice.
	Theme struct {
		Mode  string `json:"mode"`
		Color string `json:"color"`
	} `json:"theme"`
}

type ModelChoice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// SessionSummary is one row of GET /api/sessions.
type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  int       `json:"messages"`
	Running   bool      `json:"running"`
}

// SessionResponse is GET /api/sessions/{id}.
type SessionResponse struct {
	Session     models.Conversation `json:"session"`
	Running     bool                `json:"running"`
	Interaction *Interaction        `json:"interaction,omitempty"`
}

// Interaction is a pending question or ACP permission request.
type Interaction struct {
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Items      []AskItem          `json:"items,omitempty"`
	Permission *PermissionRequest `json:"permission,omitempty"`
}

type AskItem struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
}

type PermissionRequest struct {
	Title   string             `json:"title"`
	Detail  string             `json:"detail,omitempty"`
	Options []PermissionOption `json:"options"`
}

type PermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type SendResult struct {
	OK             bool   `json:"ok"`
	UserMessageID  string `json:"user_message_id,omitempty"`
	ReplyMessageID string `json:"reply_message_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
}

type Attachment struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Agent struct {
	Ref         string   `json:"ref"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        string   `json:"kind"`
	Disabled    bool     `json:"disabled,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Models      []string `json:"models,omitempty"`
	Tools       []string `json:"tools,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

type Model struct {
	Ref          string `json:"ref"`
	Label        string `json:"label"`
	Provider     string `json:"provider,omitempty"`
	ProviderType string `json:"provider_type,omitempty"`
}

type Room struct {
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	Workspace          string       `json:"workspace,omitempty"`
	Activation         string       `json:"activation"`
	Master             string       `json:"master,omitempty"`
	AllowSelfResponses bool         `json:"allow_self_responses,omitempty"`
	Members            []RoomMember `json:"members"`
	Sessions           int          `json:"sessions"`
	Updated            time.Time    `json:"updated,omitempty"`
	Running            bool         `json:"running,omitempty"`
}

type RoomMember struct {
	Agent         string `json:"agent"`
	Name          string `json:"name,omitempty"`
	Muted         bool   `json:"muted,omitempty"`
	Talkativeness int    `json:"talkativeness,omitempty"`
}

type RoomInput struct {
	Name               string       `json:"name"`
	Workspace          string       `json:"workspace,omitempty"`
	Activation         string       `json:"activation,omitempty"`
	Master             string       `json:"master,omitempty"`
	AllowSelfResponses bool         `json:"allow_self_responses,omitempty"`
	Members            []RoomMember `json:"members"`
}

type RoomSession struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Updated time.Time `json:"updated"`
}

type Schedule struct {
	Kind    string `json:"kind"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Weekday int    `json:"weekday,omitempty"`
	Day     int    `json:"day,omitempty"`
}

type Task struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Agent     string    `json:"agent,omitempty"`
	Prompt    string    `json:"prompt"`
	Workspace string    `json:"workspace,omitempty"`
	Schedule  Schedule  `json:"schedule"`
	Reuse     bool      `json:"reuse,omitempty"`
	ConvID    string    `json:"conversation_id,omitempty"`
	Enabled   bool      `json:"enabled"`
	Running   bool      `json:"running,omitempty"`
	LastRun   time.Time `json:"last_run,omitempty"`
	LastNote  string    `json:"last_note,omitempty"`
	Sessions  int       `json:"sessions"`
}

type TaskInput struct {
	Name      string   `json:"name"`
	Agent     string   `json:"agent,omitempty"`
	Prompt    string   `json:"prompt"`
	Workspace string   `json:"workspace,omitempty"`
	Schedule  Schedule `json:"schedule"`
	Reuse     bool     `json:"reuse,omitempty"`
	Enabled   bool     `json:"enabled"`
}

// PairResult is the relay's pairing response.
type PairResult struct {
	DesktopID     string          `json:"desktop_id"`
	DeviceID      string          `json:"device_id"`
	Token         string          `json:"token"`
	Name          string          `json:"name"`
	Capabilities  map[string]bool `json:"capabilities"`
	Protocol      int             `json:"protocol"`
	DesktopPubKey string          `json:"desktop_pubkey,omitempty"`
	E2E           bool            `json:"e2e,omitempty"`
	// E2EKey is derived from the pair's X25519 exchange; never sent on the
	// wire in this form.
	E2EKey []byte `json:"-"`
}

// Pair exchanges a desktop's one-time code for a device token.
func Pair(ctx context.Context, relayURL, desktopID, code, deviceName, platform string) (PairResult, error) {
	priv, pub, err := e2e.Generate()
	if err != nil {
		return PairResult{}, err
	}
	body, _ := json.Marshal(map[string]string{
		"desktop_id":    desktopID,
		"code":          code,
		"device_name":   deviceName,
		"platform":      platform,
		"device_pubkey": base64.StdEncoding.EncodeToString(pub),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(relayURL, "/")+"/v1/pair", bytes.NewReader(body))
	if err != nil {
		return PairResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return PairResult{}, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			return PairResult{}, APIError{Status: resp.StatusCode, Code: e.Error.Code, Message: e.Error.Message}
		}
		return PairResult{}, APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	var out PairResult
	if err := json.Unmarshal(data, &out); err != nil {
		return PairResult{}, err
	}
	if out.E2E && out.DesktopPubKey != "" {
		peer, err := base64.StdEncoding.DecodeString(out.DesktopPubKey)
		if err != nil {
			return PairResult{}, err
		}
		key, err := e2e.Shared(priv, peer)
		if err != nil {
			return PairResult{}, err
		}
		out.E2EKey = key
	}
	return out, nil
}

// RegisterPush tells the relay where to send this device's notifications.
func (c *Client) RegisterPush(ctx context.Context, relayURL, provider, token, env string) error {
	body, _ := json.Marshal(map[string]string{"provider": provider, "token": token, "env": env})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(relayURL, "/")+"/v1/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("X-ChatGo-Desktop-ID", c.DesktopID)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	return nil
}

func isNotFound(err error) bool {
	var e APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}
