package ui

import (
	"testing"

	"chatgo/pkg/models"
)

// A workspace that is not a git tree of its own — a plain directory, the
// root, the empty workspace — shows one layer in the sidebar: its sessions
// sit directly under the project header instead of a second header that
// repeats the same title. A git tree keeps its branch header.
func TestSidebarSkipsDuplicateTreeHeader(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.convs = []models.Conversation{
		{ID: "a", Title: "无区会话"},
		{ID: "b", Title: "根目录会话", Workspace: "/"},
		{ID: "c", Title: "仓库会话", Workspace: "/code/repo"},
	}
	cw.convGit.info["/code/repo"] = &convGitInfo{
		Branch:  "main",
		Project: &Project{Main: "/code/repo", Worktrees: []string{"/code/repo"}, TreeBranches: map[string]string{"/code/repo": "main"}},
	}

	rows := cw.buildConvRows(nil)
	type seen struct {
		groups, subHeaders int
	}
	counts := map[string]seen{}
	for _, r := range rows {
		switch {
		case r.header && !r.sub:
			s := counts[r.ws]
			s.groups++
			counts[r.ws] = s
		case r.header:
			s := counts[r.ws]
			s.subHeaders++
			counts[r.ws] = s
		}
	}
	if s := counts[""]; s.groups != 1 || s.subHeaders != 0 {
		t.Fatalf("the empty workspace should be one layer, got %+v", s)
	}
	if s := counts["/"]; s.groups != 1 || s.subHeaders != 0 {
		t.Fatalf("the root directory should be one layer, got %+v", s)
	}
	if s := counts["/code/repo"]; s.groups != 1 || s.subHeaders != 1 {
		t.Fatalf("a git tree keeps its branch header, got %+v", s)
	}
}

// The workspace a recent row names must tell the reader something: the
// empty workspace and the root directory show no folder line.
func TestWorkspaceMeaningful(t *testing.T) {
	for _, ws := range []string{"", "/"} {
		if workspaceMeaningful(ws) {
			t.Fatalf("%q carries no place and should be hidden", ws)
		}
	}
	for _, ws := range []string{"/code/repo", "/Users/tk/code"} {
		if !workspaceMeaningful(ws) {
			t.Fatalf("%q names a place and should show", ws)
		}
	}
}
