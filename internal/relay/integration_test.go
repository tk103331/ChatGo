package relay_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"chatgo/internal/mobileapi"
	"chatgo/internal/relay"
	"chatgo/internal/relayclient"
)

func newLocalWeb(t *testing.T, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/meta" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": true, "path": r.URL.Path})
	}))
}

func waitState(t *testing.T, c *relayclient.Connector, want relayclient.State) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.State() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("state = %s, want %s", c.State(), want)
}

func TestRelayPairProxyAndRevoke(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	local := newLocalWeb(t, "local-token")
	defer local.Close()

	conn := relayclient.New(relayclient.Config{
		RelayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		DesktopID:    "desktop-1",
		DesktopName:  "Test Mac",
		Secret:       "desktop-secret",
		LocalURL:     local.URL,
		LocalToken:   "local-token",
		AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	waitState(t, conn, relayclient.StateConnected)

	code, _ := conn.NewPairCode(time.Minute)
	body := `{"desktop_id":"desktop-1","code":"` + code + `","device_name":"iPhone","platform":"ios"}`
	resp, err := http.Post(ts.URL+"/v1/pair", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("pair status = %d: %s", resp.StatusCode, data)
	}
	var pair struct {
		DeviceID string `json:"device_id"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	if pair.DeviceID == "" || pair.Token == "" {
		t.Fatalf("empty pair response: %+v", pair)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/d/desktop-1/api/meta", nil)
	req.Header.Set("Authorization", "Bearer "+pair.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("proxy status = %d: %s", resp.StatusCode, data)
	}
	var meta map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta["path"] != "/api/meta" {
		t.Fatalf("path = %v", meta["path"])
	}

	// A request without a token is rejected before the tunnel.
	resp, err = http.Get(ts.URL + "/d/desktop-1/api/meta")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", resp.StatusCode)
	}

	if err := conn.Revoke(context.Background(), pair.DeviceID); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/d/desktop-1/api/meta", nil)
	req.Header.Set("Authorization", "Bearer "+pair.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked status = %d", resp.StatusCode)
	}
}

func TestRelayE2EMeta(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	local := newLocalWeb(t, "local-token")
	defer local.Close()

	conn := relayclient.New(relayclient.Config{
		RelayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		DesktopID:    "desktop-e2e",
		DesktopName:  "E2E Mac",
		Secret:       "desktop-secret",
		LocalURL:     local.URL,
		LocalToken:   "local-token",
		AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	waitState(t, conn, relayclient.StateConnected)

	code, _ := conn.NewPairCode(time.Minute)
	pair, err := mobileapi.Pair(ctx, ts.URL, "desktop-e2e", code, "iPhone", "ios")
	if err != nil {
		t.Fatal(err)
	}
	if !pair.E2E || len(pair.E2EKey) == 0 {
		t.Fatalf("pair did not negotiate e2e: %+v", pair)
	}
	client := mobileapi.New(ts.URL+"/d/desktop-e2e", pair.Token, pair.DesktopID)
	client.E2EKey = pair.E2EKey
	client.DeviceID = pair.DeviceID
	meta, err := client.Meta(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Ready {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRelayE2EPost(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/sessions" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]any{"id": "c1", "title": in.Title},
		})
	}))
	defer local.Close()

	connector := relayclient.New(relayclient.Config{
		RelayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		DesktopID:    "desktop-post",
		DesktopName:  "Post",
		Secret:       "post-secret",
		LocalURL:     local.URL,
		LocalToken:   "local-token",
		AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go connector.Run(ctx)
	waitState(t, connector, relayclient.StateConnected)
	code, _ := connector.NewPairCode(time.Minute)
	pair, err := mobileapi.Pair(ctx, ts.URL, "desktop-post", code, "iPhone", "ios")
	if err != nil {
		t.Fatal(err)
	}
	client := mobileapi.New(ts.URL+"/d/desktop-post", pair.Token, pair.DesktopID)
	client.E2EKey = pair.E2EKey
	client.DeviceID = pair.DeviceID
	resp, err := client.CreateSession(ctx, "加密标题", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Session.ID != "c1" || resp.Session.Title != "加密标题" {
		t.Fatalf("session = %+v", resp.Session)
	}
}

func TestRelayE2EEvents(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/events" {
			http.NotFound(w, r)
			return
		}
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"v":1,"seq":1,"type":"hello"}`))
	}))
	defer local.Close()

	connector := relayclient.New(relayclient.Config{
		RelayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		DesktopID:    "desktop-events",
		DesktopName:  "Events",
		Secret:       "events-secret",
		LocalURL:     local.URL,
		LocalToken:   "local-token",
		AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go connector.Run(ctx)
	waitState(t, connector, relayclient.StateConnected)

	code, _ := connector.NewPairCode(time.Minute)
	pair, err := mobileapi.Pair(ctx, ts.URL, "desktop-events", code, "iPhone", "ios")
	if err != nil {
		t.Fatal(err)
	}
	client := mobileapi.New(ts.URL+"/d/desktop-events", pair.Token, pair.DesktopID)
	client.E2EKey = pair.E2EKey
	ws, err := client.DialEvents(ctx, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	event, err := mobileapi.ReadEvent(ws, pair.E2EKey, pair.DesktopID)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "hello" || event.Seq != 1 {
		t.Fatalf("event = %+v", event)
	}
}

func TestRelayDesktopOffline(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/v1/pair", "application/json",
		strings.NewReader(`{"desktop_id":"missing","code":"ABCDEFGH"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("offline status = %d", resp.StatusCode)
	}
}

// A paired phone learns which desktop it reached, and a disabled one is
// refused with a code that keeps its pairing.
func TestRelayPairDesktopNameAndDeviceDisabled(t *testing.T) {
	store, err := relay.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := relay.NewWithStore(relay.Config{}, store)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	local := newLocalWeb(t, "local-token")
	defer local.Close()

	conn := relayclient.New(relayclient.Config{
		RelayURL:     "ws" + strings.TrimPrefix(ts.URL, "http"),
		DesktopID:    "desktop-1",
		DesktopName:  "Test Mac",
		Secret:       "desktop-secret",
		LocalURL:     local.URL,
		LocalToken:   "local-token",
		AllowPairing: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	waitState(t, conn, relayclient.StateConnected)

	code, _ := conn.NewPairCode(time.Minute)
	body := `{"desktop_id":"desktop-1","code":"` + code + `","device_name":"iPhone","platform":"ios"}`
	resp, err := http.Post(ts.URL+"/v1/pair", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var pair struct {
		DeviceID    string `json:"device_id"`
		Token       string `json:"token"`
		DesktopName string `json:"desktop_name"`
		Platform    string `json:"platform"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	if pair.DesktopName != "Test Mac" || pair.Platform == "" {
		t.Fatalf("pair desktop = %q/%q", pair.DesktopName, pair.Platform)
	}

	metaReq := func() int {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/d/desktop-1/api/meta", nil)
		req.Header.Set("Authorization", "Bearer "+pair.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	if status := metaReq(); status != http.StatusOK {
		t.Fatalf("paired status = %d", status)
	}
	if err := conn.SetDeviceDisabled(ctx, pair.DeviceID, true); err != nil {
		t.Fatal(err)
	}
	if status := metaReq(); status != http.StatusForbidden {
		t.Fatalf("disabled status = %d, want 403", status)
	}
	devices := conn.Devices()
	if len(devices) != 1 || !devices[0].Disabled {
		t.Fatalf("devices = %+v", devices)
	}
	if err := conn.SetDeviceDisabled(ctx, pair.DeviceID, false); err != nil {
		t.Fatal(err)
	}
	if status := metaReq(); status != http.StatusOK {
		t.Fatalf("enabled status = %d", status)
	}
	devices = conn.Devices()
	if len(devices) != 1 || devices[0].Disabled {
		t.Fatalf("devices after enable = %+v", devices)
	}
}
