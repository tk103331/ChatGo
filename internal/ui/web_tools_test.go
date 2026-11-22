package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/config"
)

func testToolRuntime(t *testing.T) (webRuntime, string) {
	t.Helper()
	// addWorkspace saves the desktop config; keep it in the test's home.
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cw := &ChatWindow{cfg: &config.Config{Workspaces: []string{dir}}, workspace: dir}
	return webRuntime{cw: cw}, dir
}

// The phone may list and read files inside a workspace, but not outside it.
func TestWebToolsFiles(t *testing.T) {
	r, dir := testToolRuntime(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files, err := r.Files(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].Dir || files[0].Name != "sub" || files[1].Name != "a.txt" {
		t.Fatalf("files = %+v", files)
	}
	file, err := r.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if file.Content != "hello" || file.Binary {
		t.Fatalf("file = %+v", file)
	}
	if _, err := r.Files("/"); err == nil {
		t.Fatal("listed a directory outside the workspace")
	}
	if _, err := r.ReadFile("/etc/passwd"); err == nil {
		t.Fatal("read a file outside the workspace")
	}
}

// Git status, staging, committing and the log work in a workspace.
func TestWebToolsGit(t *testing.T) {
	r, dir := testToolRuntime(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "phone@example.com")
	run("config", "user.name", "Phone")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := r.GitStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Changes) != 1 || status.Changes[0].Path != "a.txt" {
		t.Fatalf("status = %+v", status)
	}
	if _, err := r.GitDiff(dir, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := r.GitStage(dir, ""); err != nil {
		t.Fatal(err)
	}
	if err := r.GitCommit(dir, "first"); err != nil {
		t.Fatal(err)
	}
	log, err := r.GitLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0].Subject != "first" {
		t.Fatalf("log = %+v", log)
	}
}

// A terminal command runs in the project and returns its output and code.
func TestWebToolsTerminal(t *testing.T) {
	r, dir := testToolRuntime(t)
	res := r.RunCommand(context.Background(), dir, "printf hello; exit 3")
	if res.Output != "hello" || res.ExitCode != 3 {
		t.Fatalf("result = %+v", res)
	}
	if out := r.RunCommand(context.Background(), dir, ""); out.ExitCode == 0 {
		t.Fatalf("empty command = %+v", out)
	}
}

// Creating a project makes the directory and registers it as a workspace.
func TestWebToolsCreateProject(t *testing.T) {
	r, dir := testToolRuntime(t)
	path, err := r.CreateProject(dir, "new-project", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Fatalf("git was not initialized: %v", err)
	}
	found := false
	for _, w := range r.cw.cfg.Workspaces {
		if w == path {
			found = true
		}
	}
	if !found || !strings.HasSuffix(path, "new-project") {
		t.Fatalf("workspaces = %v, path = %s", r.cw.cfg.Workspaces, path)
	}
	if _, err := r.CreateProject(dir, "new-project", false); err == nil {
		t.Fatal("created an existing directory")
	}
}
