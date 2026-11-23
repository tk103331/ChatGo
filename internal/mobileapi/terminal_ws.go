package mobileapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"chatgo/internal/e2e"
)

// TerminalWSURL is the WebSocket address of a persistent terminal session.
func (c *Client) TerminalWSURL(id string) string {
	base := c.BaseURL
	base = strings.Replace(base, "https://", "wss://", 1)
	base = strings.Replace(base, "http://", "ws://", 1)
	return strings.TrimRight(base, "/") + "/api/terminals/" + url.PathEscape(id) + "/ws"
}

// DialTerminal attaches to a shell. With E2E, frames are sealed with its key;
// without it, the desktop's plaintext frames are read directly.
func (c *Client) DialTerminal(ctx context.Context, id string) (*websocket.Conn, error) {
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
	conn, _, err := dialer.DialContext(ctx, c.TerminalWSURL(id), headers)
	return conn, err
}

// ReadTerminalFrame reads one terminal frame: kind 0 is shell output, kind 1
// is JSON control.
func ReadTerminalFrame(conn *websocket.Conn, key []byte, deviceID, direction string) (byte, []byte, error) {
	mt, msg, err := conn.ReadMessage()
	if err != nil {
		return 0, nil, err
	}
	if key == nil {
		if mt == websocket.TextMessage {
			return 1, msg, nil
		}
		return 0, msg, nil
	}
	if mt != websocket.BinaryMessage || len(msg) < 14 || msg[0] != 1 {
		return 0, nil, fmt.Errorf("encrypted terminal frame without a key")
	}
	plain, err := e2e.Open(key, msg[1:13], msg[13:], e2e.TerminalAAD(deviceID, direction))
	if err != nil {
		return 0, nil, err
	}
	if len(plain) == 0 {
		return 0, nil, fmt.Errorf("empty terminal frame")
	}
	return plain[0], plain[1:], nil
}

// WriteTerminalFrame sends one terminal frame, sealing it when the session has
// an E2E key.
func WriteTerminalFrame(conn *websocket.Conn, key []byte, deviceID, direction string, kind byte, payload []byte) error {
	if key == nil {
		mt := websocket.BinaryMessage
		if kind == 1 {
			mt = websocket.TextMessage
		}
		return conn.WriteMessage(mt, payload)
	}
	plain := append([]byte{kind}, payload...)
	nonce, ciphertext, err := e2e.Seal(key, plain, e2e.TerminalAAD(deviceID, direction))
	if err != nil {
		return err
	}
	frame := make([]byte, 0, 1+len(nonce)+len(ciphertext))
	frame = append(frame, 1)
	frame = append(frame, nonce...)
	frame = append(frame, ciphertext...)
	return conn.WriteMessage(websocket.BinaryMessage, frame)
}
