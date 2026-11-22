package webui

import (
	"context"
	"net/http"
	"strings"
	"testing"
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
