package channel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The WeCom protocol is spoken directly, so the test stands in for the
// platform: it checks the subscribe frame, sends a callback, and reads the
// reply frame the driver writes.
func TestWeComSubscribesReceivesAndReplies(t *testing.T) {
	upgrader := websocket.Upgrader{}
	var gotSubscribe map[string]any
	replies := make(chan map[string]any, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var sub map[string]any
		if err := conn.ReadJSON(&sub); err != nil {
			return
		}
		gotSubscribe = sub
		_ = conn.WriteJSON(map[string]any{
			"headers": map[string]string{"req_id": "sub"},
			"errcode": 0, "errmsg": "ok",
		})
		// Deliver one text message callback.
		body := map[string]any{
			"msgid": "m1", "chatid": "chat-1", "chattype": "single", "msgtype": "text",
			"from": map[string]string{"userid": "u1"},
			"text": map[string]string{"content": "hi there"},
		}
		raw, _ := json.Marshal(body)
		_ = conn.WriteJSON(map[string]any{
			"cmd":     "aibot_msg_callback",
			"headers": map[string]string{"req_id": "req-1"},
			"body":    json.RawMessage(raw),
		})
		// Read whatever the driver writes back.
		for {
			var reply map[string]any
			if err := conn.ReadJSON(&reply); err != nil {
				return
			}
			replies <- reply
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	d := NewWeCom()
	got := make(chan Inbound, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx, Config{Type: "wecom", AppID: "bot", AppSecret: "sec", BaseURL: wsURL},
		func(in Inbound) { got <- in }); err != nil {
		t.Fatal(err)
	}

	select {
	case in := <-got:
		if in.Text != "hi there" || in.ChatID != "chat-1" || in.FromUser != "u1" {
			t.Fatalf("unexpected inbound: %+v", in)
		}
		if in.ReplyToken != "req-1" {
			t.Fatalf("reply token is %q, want the callback's req_id", in.ReplyToken)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message delivered")
	}

	if gotSubscribe["cmd"] != "aibot_subscribe" {
		t.Fatalf("the first frame is %v, want a subscribe", gotSubscribe)
	}
	body, _ := gotSubscribe["body"].(map[string]any)
	if body["bot_id"] != "bot" || body["secret"] != "sec" {
		t.Fatalf("subscribe carried %v", body)
	}

	// The reply quotes the callback's req_id.
	if err := d.SendReply(context.Background(), "chat-1", "req-1", "hello back"); err != nil {
		t.Fatal(err)
	}
	select {
	case reply := <-replies:
		if reply["cmd"] != "aibot_respond_msg" {
			t.Fatalf("reply cmd is %v", reply["cmd"])
		}
		hdr, _ := reply["headers"].(map[string]any)
		if hdr["req_id"] != "req-1" {
			t.Fatalf("reply quoted %v, want req-1", hdr["req_id"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no reply frame written")
	}
}

// Missing credentials are refused before any connection is attempted.
func TestWeComRequiresCredentials(t *testing.T) {
	err := NewWeCom().Start(context.Background(), Config{Type: "wecom"}, func(Inbound) {})
	if err == nil {
		t.Fatal("a driver without credentials must refuse to start")
	}
}
