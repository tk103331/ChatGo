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

// A session can be renamed and deleted from the phone.
func TestRenameAndDeleteSession(t *testing.T) {
	var patch map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PATCH" && r.URL.Path == "/api/sessions/c1":
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == "DELETE" && r.URL.Path == "/api/sessions/c1":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	c := New(ts.URL, "dev", "d1")
	if err := c.RenameSession(context.Background(), "c1", "新名字"); err != nil {
		t.Fatal(err)
	}
	if patch["title"] != "新名字" {
		t.Fatalf("patch = %#v", patch)
	}
	if err := c.DeleteSession(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
}

// Files, Git, terminal and project creation decode their responses.
func TestProjectTools(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/files":
			_ = json.NewEncoder(w).Encode(map[string]any{"files": []map[string]any{
				{"name": "a.txt", "path": "/w/a.txt", "size": 5},
			}})
		case "/api/file":
			_ = json.NewEncoder(w).Encode(map[string]any{"file": map[string]any{"name": "a.txt", "content": "hello", "size": 5}})
		case "/api/git/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"branch": "main", "changes": []map[string]any{{"path": "a.txt", "status": " M"}}}})
		case "/api/git/diff":
			_ = json.NewEncoder(w).Encode(map[string]any{"diff": "+hello"})
		case "/api/git/log":
			_ = json.NewEncoder(w).Encode(map[string]any{"commits": []map[string]any{{"hash": "abc", "subject": "init"}}})
		case "/api/git/stage", "/api/git/commit":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/api/terminal":
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"output": "hi", "exit_code": 0}})
		case "/api/projects":
			_ = json.NewEncoder(w).Encode(map[string]any{"path": "/w/new"})
		case "/api/fs":
			_ = json.NewEncoder(w).Encode(map[string]any{"dir": "/home/tk", "parent": "/home", "entries": []map[string]any{{"name": "code", "path": "/home/tk/code"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	c := New(ts.URL, "dev", "d1")
	ctx := context.Background()
	if files, err := c.Files(ctx, "/w"); err != nil || len(files) != 1 || files[0].Name != "a.txt" {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if file, err := c.ReadFile(ctx, "/w/a.txt"); err != nil || file.Content != "hello" {
		t.Fatalf("file = %+v, %v", file, err)
	}
	if status, err := c.GitStatus(ctx, "/w"); err != nil || status.Branch != "main" || len(status.Changes) != 1 {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if diff, err := c.GitDiff(ctx, "/w", "a.txt"); err != nil || diff != "+hello" {
		t.Fatalf("diff = %q, %v", diff, err)
	}
	if log, err := c.GitLog(ctx, "/w"); err != nil || len(log) != 1 || log[0].Hash != "abc" {
		t.Fatalf("log = %+v, %v", log, err)
	}
	if err := c.GitStage(ctx, "/w", ""); err != nil {
		t.Fatal(err)
	}
	if err := c.GitCommit(ctx, "/w", "msg"); err != nil {
		t.Fatal(err)
	}
	if result, err := c.RunCommand(ctx, "/w", "echo hi"); err != nil || result.Output != "hi" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if path, err := c.CreateProject(ctx, "/w", "new", true); err != nil || path != "/w/new" {
		t.Fatalf("path = %q, %v", path, err)
	}
	if listing, err := c.Directories(ctx, ""); err != nil || listing.Dir != "/home/tk" || len(listing.Entries) != 1 {
		t.Fatalf("listing = %+v, %v", listing, err)
	}
}
