package webui

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func readEvent(t *testing.T, conn *websocket.Conn) Event {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var e Event
	if err := conn.ReadJSON(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestEventsReplayAndLive(t *testing.T) {
	srv := protocolServerWithToken(t, nil, testWebToken)
	first := srv.events.Publish("session.updated", "c1", "", map[string]any{"n": 1})
	second := srv.events.Publish("session.created", "c2", "", nil)
	if second.Seq != first.Seq+1 {
		t.Fatalf("seq = %d, want %d", second.Seq, first.Seq+1)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/events?since=" + itoa(first.Seq)
	headers := websocket.Dialer{HandshakeTimeout: time.Second}
	conn, _, err := headers.Dial(wsURL, map[string][]string{
		"Authorization": {"Bearer " + testWebToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if hello := readEvent(t, conn); hello.Type != "hello" || hello.Seq < second.Seq {
		t.Fatalf("hello = %+v", hello)
	}
	if replay := readEvent(t, conn); replay.Type != "session.created" || replay.SessionID != "c2" {
		t.Fatalf("replay = %+v", replay)
	}
	srv.Publish("room.updated", "", "", nil)
	if live := readEvent(t, conn); live.Type != "room.updated" {
		t.Fatalf("live = %+v", live)
	}
}

func TestEventsResyncWhenSinceIsOld(t *testing.T) {
	srv := protocolServerWithToken(t, nil, testWebToken)
	for i := 0; i < eventRing+2; i++ {
		srv.Publish("session.updated", "c1", "", nil)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/events?since=1"
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: time.Second}).Dial(wsURL, map[string][]string{
		"Authorization": {"Bearer " + testWebToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if hello := readEvent(t, conn); hello.Type != "hello" {
		t.Fatalf("hello = %+v", hello)
	}
	if resync := readEvent(t, conn); resync.Type != "resync" {
		t.Fatalf("resync = %+v", resync)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
