package webui

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// toolTestRunner adds the optional tool surface to the feature fake.
type toolTestRunner struct {
	*fakeRunner
	project string
}

func (f *toolTestRunner) WorkspaceDir(dir string) (string, error) { return dir, nil }
func (f *toolTestRunner) Files(dir string) ([]FileEntry, error) {
	return []FileEntry{{Name: "a.txt", Path: dir + "/a.txt", Size: 5}}, nil
}
func (f *toolTestRunner) ReadFile(path string) (FileContent, error) {
	return FileContent{Name: "a.txt", Path: path, Content: "hello", Size: 5}, nil
}
func (f *toolTestRunner) GitStatus(dir string) (GitStatusInfo, error) {
	return GitStatusInfo{Branch: "main", Changes: []GitChange{{Path: "a.txt", Status: " M"}}}, nil
}
func (f *toolTestRunner) GitDiff(dir, path string) (string, error) { return "+hello", nil }
func (f *toolTestRunner) GitLog(dir string) ([]GitCommitInfo, error) {
	return []GitCommitInfo{{Hash: "abc", Subject: "init"}}, nil
}
func (f *toolTestRunner) GitStage(dir, path string) error     { return nil }
func (f *toolTestRunner) GitCommit(dir, message string) error { return nil }
func (f *toolTestRunner) RunCommand(ctx context.Context, dir, command string) CommandResult {
	return CommandResult{Output: "hi", ExitCode: 0}
}
func (f *toolTestRunner) CreateProject(parent, name string, git bool) (string, error) {
	f.project = parent + "/" + name
	return f.project, nil
}

// The tool routes answer with the runtime's files, Git, terminal and project
// data.
func TestToolRoutes(t *testing.T) {
	r := &toolTestRunner{fakeRunner: &fakeRunner{}}
	s, _ := runnerServer(t, r)
	for _, tc := range []struct {
		method, path, body, want string
	}{
		{"GET", "/api/files?dir=/w", "", "a.txt"},
		{"GET", "/api/file?path=/w/a.txt", "", "hello"},
		{"GET", "/api/git/status?dir=/w", "", "\"main\""},
		{"GET", "/api/git/diff?dir=/w&path=a.txt", "", "+hello"},
		{"GET", "/api/git/log?dir=/w", "", "init"},
		{"POST", "/api/git/stage", `{"dir":"/w"}`, "\"ok\":true"},
		{"POST", "/api/git/commit", `{"dir":"/w","message":"m"}`, "\"ok\":true"},
		{"POST", "/api/terminal", `{"dir":"/w","command":"echo hi"}`, "\"hi\""},
		{"POST", "/api/projects", `{"parent":"/w","name":"new","git":true}`, "\"path\":\"/w/new\""},
	} {
		w := serve(t, s, tc.method, tc.path, tc.body)
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
		if !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s %s: %s", tc.method, tc.path, w.Body)
		}
	}
	if r.project != "/w/new" {
		t.Fatalf("created project = %q", r.project)
	}
}

// Without the optional surface the routes say so instead of panicking.
func TestToolRoutesNeedRuntime(t *testing.T) {
	s, _ := runnerServer(t, &fakeRunner{})
	w := serve(t, s, http.MethodGet, "/api/files?dir=/w", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
}

// The terminal routes start a persistent shell, feed it input and return
// output; the session outlives the client.
func TestTerminalRoutes(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	r := &toolTestRunner{fakeRunner: &fakeRunner{}}
	s, _ := runnerServer(t, r)
	w := serve(t, s, http.MethodPost, "/api/terminals", `{"dir":"`+dir+`","cols":40,"rows":10}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	var start struct {
		Session TerminalInfo `json:"session"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &start); err != nil || start.Session.ID == "" {
		t.Fatalf("start: %v %s", err, w.Body)
	}
	id := start.Session.ID
	if w := serve(t, s, http.MethodGet, "/api/terminals", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), id) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	input := base64.StdEncoding.EncodeToString([]byte("echo 7\n"))
	if w := serve(t, s, http.MethodPost, "/api/terminals/"+id+"/input", `{"data":"`+input+`"}`); w.Code != http.StatusOK {
		t.Fatalf("input: %d %s", w.Code, w.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := serve(t, s, http.MethodGet, "/api/terminals/"+id+"/output?since=0&wait_ms=100", "")
		var out TerminalOutput
		if w.Code == http.StatusOK && jsonUnmarshal(w.Body.Bytes(), &out) == nil && strings.Contains(string(out.Data), "7") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("output did not arrive: %s", w.Body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if w := serve(t, s, http.MethodDelete, "/api/terminals/"+id, ""); w.Code != http.StatusOK {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}
}

// The WebSocket terminal replays its screen and carries live input.
func TestTerminalWebSocket(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	r := &toolTestRunner{fakeRunner: &fakeRunner{}}
	s, _ := runnerServer(t, r)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	auth := "Bearer " + s.Token()
	w := serve(t, s, http.MethodPost, "/api/terminals", `{"dir":"`+dir+`","cols":40,"rows":10}`)
	var start struct {
		Session TerminalInfo `json:"session"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &start); err != nil || start.Session.ID == "" {
		t.Fatalf("start: %v %s", err, w.Body)
	}
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/terminals/" + start.Session.ID + "/ws"
	conn, _, err := (&websocket.Dialer{}).Dial(wsURL, http.Header{"Authorization": {auth}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// The first frame is the reset control; then the shell's prompt.
	_, first, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(first), "\"reset\"") {
		t.Fatalf("first frame = %s %v", first, err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("echo 7\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if strings.Contains(string(msg), "7") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("terminal output did not arrive: %s", msg)
		}
	}
}
