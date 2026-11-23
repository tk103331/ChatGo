package mobileapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// Terminal frames keep their kind through the encrypted transport.
func TestTerminalFrames(t *testing.T) {
	key := make([]byte, 32)
	up := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		kind, payload, err := ReadTerminalFrame(conn, key, "d1", "c2s")
		if err != nil || kind != 0 || string(payload) != "input" {
			return
		}
		_ = WriteTerminalFrame(conn, key, "d1", "s2c", 1, []byte(`{"type":"reset"}`))
		_ = WriteTerminalFrame(conn, key, "d1", "s2c", 0, payload)
	}))
	defer ts.Close()
	c := New(ts.URL, "dev", "d1")
	c.E2EKey = key
	conn, err := c.DialTerminal(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := WriteTerminalFrame(conn, key, "d1", "c2s", 0, []byte("input")); err != nil {
		t.Fatal(err)
	}
	kind, payload, err := ReadTerminalFrame(conn, key, "d1", "s2c")
	if err != nil || kind != 1 || string(payload) != `{"type":"reset"}` {
		t.Fatalf("control = %v %q %v", kind, payload, err)
	}
	kind, payload, err = ReadTerminalFrame(conn, key, "d1", "s2c")
	if err != nil || kind != 0 || string(payload) != "input" {
		t.Fatalf("output = %v %q %v", kind, payload, err)
	}
}

// A plaintext server's message types map to kinds the same way.
func TestTerminalFramesPlaintext(t *testing.T) {
	up := websocket.Upgrader{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		kind, payload, err := ReadTerminalFrame(conn, nil, "", "s2c")
		if err != nil || kind != 0 || string(payload) != "input" {
			return
		}
		_ = WriteTerminalFrame(conn, nil, "", "s2c", 1, []byte(`{"type":"reset"}`))
		_ = WriteTerminalFrame(conn, nil, "", "s2c", 0, payload)
	}))
	defer ts.Close()
	c := New(ts.URL, "dev", "d1")
	conn, err := c.DialTerminal(context.Background(), "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := WriteTerminalFrame(conn, nil, "", "c2s", 0, []byte("input")); err != nil {
		t.Fatal(err)
	}
	if kind, payload, err := ReadTerminalFrame(conn, nil, "", "c2s"); err != nil || kind != 1 || string(payload) != `{"type":"reset"}` {
		t.Fatalf("control = %v %q %v", kind, payload, err)
	}
	if kind, payload, err := ReadTerminalFrame(conn, nil, "", "c2s"); err != nil || kind != 0 || string(payload) != "input" {
		t.Fatalf("output = %v %q %v", kind, payload, err)
	}
}
