package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var terminalUpgrader = websocket.Upgrader{
	HandshakeTimeout: 15 * time.Second,
	CheckOrigin:      func(*http.Request) bool { return true },
}

type terminalControl struct {
	Type  string `json:"type"`
	Cols  int    `json:"cols,omitempty"`
	Rows  int    `json:"rows,omitempty"`
	Title string `json:"title,omitempty"`
	Code  int    `json:"code,omitempty"`
}

// handleTerminalWS attaches a WebSocket to a persistent shell: the server
// sends the scrollback and then every byte the shell prints; the phone sends
// keystrokes as binary frames and resize requests as JSON text frames.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	m, err := s.terminalManager()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, ok := m.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("终端不存在"))
		return
	}
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	info := session.Info()
	if err := conn.WriteJSON(terminalControl{
		Type: "reset", Cols: info.Cols, Rows: info.Rows, Title: info.Title,
	}); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		})
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				if err := session.Write(msg); err != nil {
					return
				}
			case websocket.TextMessage:
				var c terminalControl
				if json.Unmarshal(msg, &c) != nil {
					continue
				}
				switch c.Type {
				case "resize":
					_ = session.Resize(c.Cols, c.Rows)
				case "ping":
					// The writer's own ping keeps the connection alive.
				}
			}
		}
	}()

	// Send what the shell printed while nobody was attached, then follow it.
	out := session.Output(0, 0)
	since := out.Seq
	if out.Reset {
		_ = conn.WriteJSON(terminalControl{Type: "reset", Cols: info.Cols, Rows: info.Rows, Title: info.Title})
	}
	if len(out.Data) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, out.Data); err != nil {
			return
		}
	}
	if out.Exited {
		_ = conn.WriteJSON(terminalControl{Type: "exit", Code: out.ExitCode})
		return
	}

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		out := session.Output(since, 5*time.Second)
		since = out.Seq
		if out.Reset {
			_ = conn.WriteJSON(terminalControl{Type: "reset", Cols: info.Cols, Rows: info.Rows, Title: info.Title})
		}
		if len(out.Data) > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, out.Data); err != nil {
				return
			}
		}
		if out.Exited {
			_ = conn.WriteJSON(terminalControl{Type: "exit", Code: out.ExitCode})
			return
		}
		select {
		case <-done:
			return
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		default:
		}
	}
}
