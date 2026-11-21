package mobileapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"chatgo/internal/e2e"
)

// Event is one change from GET /api/events.
type Event struct {
	V         int             `json:"v"`
	Seq       int64           `json:"seq"`
	Type      string          `json:"type"`
	SessionID string          `json:"session_id,omitempty"`
	TurnID    string          `json:"turn_id,omitempty"`
	At        time.Time       `json:"at"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// EventsURL is the WebSocket URL of the stream, with its replay cursor.
func (c *Client) EventsURL(since int64, session string) string {
	base := c.BaseURL
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	path := "/api/events?since=" + strconv.FormatInt(since, 10)
	if session != "" {
		path += "&session=" + url.QueryEscape(session)
	}
	return strings.TrimRight(base, "/") + path
}

// DialEvents opens the change stream. The caller reads Events until the
// connection closes and reconnects with the last Seq.
func (c *Client) DialEvents(ctx context.Context, since int64, session string) (*websocket.Conn, error) {
	headers := http.Header{}
	if c.Token != "" {
		headers.Set("Authorization", "Bearer "+c.Token)
	}
	if c.DesktopID != "" {
		headers.Set("X-ChatGo-Desktop-ID", c.DesktopID)
	}
	if c.E2EKey != nil {
		headers.Set(e2e.HeaderVersion, e2e.Version)
		headers.Set(e2e.HeaderTime, strconv.FormatInt(e2e.Timestamp(), 10))
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, c.EventsURL(since, session), headers)
	return conn, err
}

// ReadEvent reads one plaintext or E2E event frame.
func ReadEvent(conn *websocket.Conn, key []byte, deviceID string) (Event, error) {
	mt, msg, err := conn.ReadMessage()
	if err != nil {
		return Event{}, err
	}
	if mt == websocket.BinaryMessage {
		if len(key) == 0 || len(msg) < 14 || msg[0] != 1 {
			return Event{}, fmt.Errorf("encrypted event without a key")
		}
		msg, err = e2e.Open(key, msg[1:13], msg[13:], e2e.EventAAD(deviceID))
		if err != nil {
			return Event{}, err
		}
	}
	var event Event
	if err := json.Unmarshal(msg, &event); err != nil {
		return Event{}, err
	}
	return event, nil
}
