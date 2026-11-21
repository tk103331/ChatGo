package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/gorilla/websocket"

	"chatgo/internal/mobileapi"
	"chatgo/internal/relay"
	"chatgo/internal/relayclient"
	"chatgo/pkg/models"
)

// Exercise the native app through real pairing, E2E, relay transport and
// desktop connector. Only the desktop's model output is deterministic.
func TestMobileRelayReconnectConversation(t *testing.T) {
	var creates, sends, reads atomic.Int32
	var complete atomic.Bool
	events := make(chan struct{}, 8)
	desktop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer desktop-test" {
			http.Error(w, "missing desktop token", http.StatusUnauthorized)
			return
		}
		encode := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/api/meta":
			encode(map[string]any{"ready": true})
		case "/api/agents":
			encode(map[string]any{"agents": []map[string]any{
				{"ref": "acp:OpenCode", "name": "OpenCode", "kind": "acp"},
			}})
		case "/api/agents/icon":
			if r.URL.Query().Get("ref") != "acp:OpenCode" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#0af"/></svg>`))
		case "/api/events":
			ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer ws.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					if _, _, err := ws.ReadMessage(); err != nil {
						return
					}
				}
			}()
			for {
				select {
				case <-done:
					return
				case <-events:
					if err := ws.WriteJSON(mobileapi.Event{V: 1, Type: "session.updated", SessionID: "c1"}); err != nil {
						return
					}
				}
			}
		case "/api/sessions":
			if r.Method == http.MethodPost {
				creates.Add(1)
				encode(mobileapi.SessionResponse{Session: models.Conversation{ID: "c1"}})
			} else {
				encode(map[string]any{"sessions": []any{}})
			}
		case "/api/sessions/c1/messages":
			var in struct {
				Text string `json:"text"`
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.Text == "" {
				http.Error(w, "empty message", 400)
				return
			}
			sends.Add(1)
			reads.Store(0)
			complete.Store(false)
			encode(mobileapi.SendResult{OK: true})
		case "/api/sessions/c1":
			text := "正在输出"
			if complete.Load() {
				text = "全链路测试完成"
			}
			reads.Add(1)
			encode(mobileapi.SessionResponse{Running: !complete.Load(), Session: models.Conversation{ID: "c1", Messages: []models.Message{
				{ID: "u", Role: "user", Content: "测试"},
				{ID: "a", Role: "assistant", Content: text},
			}}})
		default:
			encode(map[string]any{})
		}
	}))
	defer desktop.Close()
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r := httptest.NewServer(relay.NewWithStore(relay.Config{}, store).Handler())
	defer r.Close()
	connector := relayclient.New(relayclient.Config{
		RelayURL: "ws" + strings.TrimPrefix(r.URL, "http"), DesktopID: "mobile-flow",
		Secret: "test-secret", LocalURL: desktop.URL, LocalToken: "desktop-test", AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go connector.Run(ctx)
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for connector.State() != relayclient.StateConnected {
		select {
		case <-deadline:
			t.Fatal("desktop did not connect")
		case <-tick.C:
		}
	}
	code, _ := connector.NewPairCode(time.Minute)
	pair, err := mobileapi.Pair(ctx, r.URL, "mobile-flow", code, "test phone", "android")
	if err != nil {
		t.Fatal(err)
	}
	if !pair.E2E {
		t.Fatal("pairing did not negotiate E2E")
	}
	updates := make(chan func(), 256)
	a := testApp()
	a.storageDir = t.TempDir()
	a.dispatch = func(fn func()) { updates <- fn }
	s := server{RelayURL: r.URL, DesktopID: pair.DesktopID, Token: pair.Token, E2EKey: pair.E2EKey, DeviceID: pair.DeviceID}
	// Use the same in-memory/save path as pairing: this used to erase the
	// token, so the first connection worked but reconnect was unauthorized.
	a.servers = []server{{DesktopID: "other"}, s}
	a.addServer(s)
	s = a.currentServer()
	if s.Token == "" {
		t.Fatal("saving the server discarded its in-memory token")
	}
	defer a.disconnect()
	await := func(label string, cond func() bool) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		for !cond() {
			select {
			case fn := <-updates:
				fn()
			case <-deadline.C:
				t.Fatalf("timeout: %s (status %q)", label, a.status)
			}
		}
	}
	a.openConv = ""
	a.conv = nil
	a.connect(s)
	await("connect", func() bool { return a.connected })
	await("agent icon", func() bool { return a.agentIcons["acp:OpenCode"] != nil })
	a.disconnect()
	if a.client != nil || a.eventCancel != nil || a.watchCancel != nil {
		t.Fatal("disconnect left active connection")
	}
	a.connect(s)
	await("reconnect with saved pairing", func() bool { return a.connected })
	a.newDraft = "测试"
	a.createNewChat()
	a.createNewChat() // repeated taps must create only one session
	await("first output", func() bool { return a.conv != nil && a.conv.ID == "c1" && len(a.conv.Messages) == 2 })
	if a.conv.Messages[1].Content != "正在输出" || !a.running {
		t.Fatal("partial output missing")
	}
	complete.Store(true)
	events <- struct{}{}
	await("final output", func() bool { return !a.running && a.conv.Messages[1].Content == "全链路测试完成" })
	tr := ui.NewTester(a.view, 390, 844)
	tr.Frame()
	if !tr.HasText("全链路测试完成") {
		t.Fatal("output was not rendered by native UI")
	}
	if creates.Load() != 1 || sends.Load() != 1 {
		t.Fatalf("duplicate create/send: %d/%d", creates.Load(), sends.Load())
	}
	a.draft = "继续测试"
	a.sendCurrent()
	await("follow-up send", func() bool { return sends.Load() == 2 && a.running && a.status == "" })
	await("follow-up partial output", func() bool { return a.conv.Messages[1].Content == "正在输出" })
	complete.Store(true)
	events <- struct{}{}
	await("follow-up final output", func() bool { return !a.running && a.conv.Messages[1].Content == "全链路测试完成" })
	// Reconnecting an open conversation must restore its output as well.
	a.disconnect()
	a.conv = nil
	a.connect(s)
	await("restore open conversation", func() bool {
		return a.connected && a.conv != nil && len(a.conv.Messages) == 2 && a.conv.Messages[1].Content == "全链路测试完成"
	})
}

func TestDisconnectCancelsPendingConnect(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer ts.Close()
	updates := make(chan func(), 16)
	a := &app{dispatch: func(fn func()) { updates <- fn }}
	a.connect(server{RelayURL: ts.URL, DesktopID: "slow"})
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	a.disconnect()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel HTTP request")
	}
	select {
	case fn := <-updates:
		fn()
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled request did not finish")
	}
	if a.client != nil || a.connecting || a.connected || a.connErr != "" || a.status != "已断开连接" {
		t.Fatal("stale connect result replaced disconnected state")
	}
}

func TestRevokedTokenAsksForNewPairing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"code": "unauthorized", "message": "device is revoked"},
		})
	}))
	defer ts.Close()
	updates := make(chan func(), 16)
	a := testApp()
	a.storageDir = t.TempDir()
	a.dispatch = func(fn func()) { updates <- fn }
	a.servers = []server{{DesktopID: "d1", RelayURL: ts.URL, Token: "dead-token", DeviceID: "dev_1"}}
	a.server = 0
	a.connect(a.currentServer())
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !a.pairRequired {
		select {
		case fn := <-updates:
			fn()
		case <-deadline.C:
			t.Fatal("a refused token did not ask for a new pairing")
		}
	}
	if a.client != nil {
		t.Fatal("the dead connection is still in place")
	}
	tr := ui.NewTester(a.view, 390, 844)
	tr.Frame()
	if !tr.HasText("需要重新配对") {
		t.Fatal("the phone does not ask for a new pairing")
	}
}

func TestNewSessionControlsFitNarrowScreen(t *testing.T) {
	a := testApp()
	a.screen = screenNew
	a.client = mobileapi.New("http://127.0.0.1:1", "test", "test")
	a.newDraft = "hello"
	a.newModel = "provider::a-very-long-model-name"
	a.meta.Models = []mobileapi.ModelChoice{{Provider: "provider", Model: "a-very-long-model-name"}}
	tr := ui.NewTester(a.view, 320, 640)
	tr.Frame()
	tr.Frame()
	send, ok := tr.AccessByRole(ui.RoleButton, "创建会话")
	if !ok {
		t.Fatal("send control missing")
	}
	if send.Bounds.X < 0 || send.Bounds.X+send.Bounds.W > 320 {
		t.Fatalf("send control outside screen: %+v", send.Bounds)
	}
}

func TestEnsureNewChoicesDropsStaleServerChoices(t *testing.T) {
	a := &app{
		newAgent: "agent:old", newModel: "old::m", newWorkspace: "/old",
		agents: []mobileapi.Agent{{Ref: "agent:new"}},
		meta:   mobileapi.Meta{Models: []mobileapi.ModelChoice{{Provider: "new", Model: "m"}}},
	}
	a.ensureNewChoices()
	if a.newAgent != "" {
		t.Fatalf("stale agent kept: %q", a.newAgent)
	}
	if a.newModel != "new::m" {
		t.Fatalf("model = %q, want the new desktop's first", a.newModel)
	}
}

// A chat sends with the model its composer picked, so the desktop answers
// with that model rather than its startup default.
func TestChatSendsWithTheChosenModel(t *testing.T) {
	type sent struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	got := make(chan sent, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/sessions/c1/messages":
			var in sent
			_ = json.NewDecoder(r.Body).Decode(&in)
			got <- in
			_ = json.NewEncoder(w).Encode(mobileapi.SendResult{OK: true})
		default:
			_ = json.NewEncoder(w).Encode(mobileapi.SessionResponse{
				Session: models.Conversation{ID: "c1", Provider: "deepseek", Model: "deepseek-chat"},
			})
		}
	}))
	defer ts.Close()
	a := testApp()
	a.client = mobileapi.New(ts.URL, "test", "d1")
	a.screen = screenChat
	a.chatModel = "deepseek::deepseek-chat"
	a.draft = "你好"
	a.sendCurrent()
	select {
	case in := <-got:
		if in.Provider != "deepseek" || in.Model != "deepseek-chat" {
			t.Fatalf("sent model = %s/%s, want deepseek/deepseek-chat", in.Provider, in.Model)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("send did not reach the desktop")
	}
}
