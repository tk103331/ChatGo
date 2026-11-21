package webui

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const eventRing = 4096

// Event is one change a client may want to see. The stream is an optimization:
// the conversation files and GET /api/sessions/{id} remain the source of truth.
type Event struct {
	V         int       `json:"v"`
	Seq       int64     `json:"seq"`
	Type      string    `json:"type"`
	SessionID string    `json:"session_id,omitempty"`
	TurnID    string    `json:"turn_id,omitempty"`
	At        time.Time `json:"at"`
	Data      any       `json:"data,omitempty"`
}

// EventHub keeps a bounded replay buffer and fans events out to subscribers.
type EventHub struct {
	mu   sync.Mutex
	seq  int64
	ring []Event
	subs map[*eventSub]struct{}
}

type eventSub struct {
	session string
	ch      chan Event
}

func newEventHub() *EventHub {
	return &EventHub{subs: map[*eventSub]struct{}{}}
}

// Publish records an event and sends it to every subscriber. A subscriber
// that cannot keep up misses deltas; its next reconnect asks for `since` and
// the hub replays what it still has, or tells it to resync.
func (h *EventHub) Publish(typ, sessionID, turnID string, data any) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	e := Event{
		V: 1, Seq: h.seq, Type: typ, SessionID: sessionID,
		TurnID: turnID, At: time.Now(), Data: data,
	}
	h.ring = append(h.ring, e)
	if len(h.ring) > eventRing {
		h.ring = append([]Event(nil), h.ring[len(h.ring)-eventRing:]...)
	}
	for sub := range h.subs {
		if sub.session != "" && sub.session != sessionID {
			continue
		}
		select {
		case sub.ch <- e:
		default:
			// A slow client gets a resync on reconnect instead of blocking
			// the agent.
		}
	}
	return e
}

// Subscribe returns a hello event, replay since, whether replay fell outside
// the ring, and a live channel. cancel must be called when the connection
// ends.
func (h *EventHub) Subscribe(since int64, session string) (Event, []Event, bool, <-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hello := Event{V: 1, Seq: h.seq, Type: "hello", At: time.Now(), Data: map[string]any{"protocol": 1}}
	resync := false
	var replay []Event
	if since > 0 {
		if len(h.ring) > 0 && since+1 < h.ring[0].Seq {
			resync = true
		} else {
			for _, e := range h.ring {
				if e.Seq > since && (session == "" || e.SessionID == session) {
					replay = append(replay, e)
				}
			}
		}
	}
	sub := &eventSub{session: session, ch: make(chan Event, 64)}
	h.subs[sub] = struct{}{}
	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[sub]; ok {
			delete(h.subs, sub)
			close(sub.ch)
		}
		h.mu.Unlock()
	}
	return hello, replay, resync, sub.ch, cancel
}

var eventUpgrader = websocket.Upgrader{
	HandshakeTimeout: 15 * time.Second,
	ReadBufferSize:   8 << 10,
	WriteBufferSize:  8 << 10,
	CheckOrigin:      func(*http.Request) bool { return true },
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := eventUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	session := r.URL.Query().Get("session")
	hello, replay, resync, ch, cancel := s.events.Subscribe(since, session)
	defer cancel()

	if err := conn.WriteJSON(hello); err != nil {
		return
	}
	if resync {
		if err := conn.WriteJSON(Event{V: 1, Type: "resync", At: time.Now()}); err != nil {
			return
		}
	}
	for _, e := range replay {
		if err := conn.WriteJSON(e); err != nil {
			return
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteJSON(e); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		}
	}
}

// Publish records an event for this server.
func (s *Server) Publish(typ, sessionID, turnID string, data any) {
	if s.events != nil {
		s.events.Publish(typ, sessionID, turnID, data)
	}
}
