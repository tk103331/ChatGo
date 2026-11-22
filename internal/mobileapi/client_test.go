package mobileapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSessionDecodesConversation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer dev" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]any{"id": "c1", "title": "hello"},
		})
	}))
	defer ts.Close()

	c := New(ts.URL, "dev", "d1")
	resp, err := c.CreateSession(context.Background(), "hello", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Session.ID != "c1" || resp.Session.Title != "hello" {
		t.Fatalf("session = %+v", resp.Session)
	}
}

func TestRegisterPushSendsDeviceHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/push" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer dev" || r.Header.Get("X-ChatGo-Desktop-ID") != "d1" {
			http.Error(w, "headers", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := New(ts.URL+"/d/d1", "dev", "d1")
	if err := c.RegisterPush(context.Background(), ts.URL, "apns", "push-token", "prod"); err != nil {
		t.Fatal(err)
	}
}

func TestEventsURL(t *testing.T) {
	c := New("https://relay.example.com/d/d1", "t", "d1")
	got := c.EventsURL(42, "c1")
	want := "wss://relay.example.com/d/d1/api/events?since=42&session=c1"
	if got != want {
		t.Fatalf("EventsURL = %q, want %q", got, want)
	}
}

func TestMetaTheme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ready": true,
			"theme": map[string]string{"mode": "dark", "color": "wine"},
		})
	}))
	defer ts.Close()
	c := New(ts.URL, "", "d1")
	meta, err := c.Meta(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if meta.Theme.Mode != "dark" || meta.Theme.Color != "wine" {
		t.Fatalf("theme = %+v", meta.Theme)
	}
}

// An agent's icon comes back as the file's bytes, and a missing one as an
// error the app ignores.
func TestAgentIconFetchesTheFile(t *testing.T) {
	icon := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agents/icon" || r.URL.Query().Get("ref") != "acp:OpenCode" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(icon)
	}))
	defer ts.Close()

	c := New(ts.URL, "dev", "d1")
	data, err := c.AgentIcon(context.Background(), "acp:OpenCode")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(icon) {
		t.Fatalf("icon = %q", data)
	}
	if _, err := c.AgentIcon(context.Background(), "agent:None"); err == nil {
		t.Fatal("missing icon did not fail")
	}
}

// An ACP agent's model list comes from its runtime options, and a choice is
// applied to the live conversation.
func TestACPOptionsAndSetConfig(t *testing.T) {
	var setBody map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/acp-options":
			if r.URL.Query().Get("agent") != "acp:Claude" {
				t.Errorf("agent = %q", r.URL.Query().Get("agent"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"options": []map[string]any{
				{"id": "model", "name": "模型", "current": "m2", "kind": "model", "options": []map[string]string{
					{"value": "m1", "name": "Opus"}, {"value": "m2", "name": "Sonnet"},
				}},
			}})
		case r.Method == "GET" && r.URL.Path == "/api/sessions/c1/acp-options":
			_ = json.NewEncoder(w).Encode(map[string]any{"options": []map[string]any{}})
		case r.Method == "PUT" && r.URL.Path == "/api/sessions/c1/acp-config":
			if err := json.NewDecoder(r.Body).Decode(&setBody); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "dev", "d1")
	opts, err := c.ACPOptions(context.Background(), "acp:Claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 1 || opts[0].Kind != "model" || opts[0].Current != "m2" || len(opts[0].Options) != 2 {
		t.Fatalf("options = %+v", opts)
	}
	if _, err := c.SessionACPOptions(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSessionConfig(context.Background(), "c1", "model", "m1"); err != nil {
		t.Fatal(err)
	}
	if setBody["id"] != "model" || setBody["value"] != "m1" {
		t.Fatalf("set body = %#v", setBody)
	}
}

// A new ACP conversation carries the model its first turn starts with.
func TestCreateSessionCarriesACPConfig(t *testing.T) {
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{"id": "c1"}})
	}))
	defer ts.Close()

	c := New(ts.URL, "dev", "d1")
	if _, err := c.CreateSessionWithACP(context.Background(), "t", "acp:Claude", "", "", "", map[string]string{"model": "m1"}); err != nil {
		t.Fatal(err)
	}
	cfg, ok := body["acp_config"].(map[string]any)
	if !ok || cfg["model"] != "m1" {
		t.Fatalf("acp_config = %#v", body["acp_config"])
	}
}
