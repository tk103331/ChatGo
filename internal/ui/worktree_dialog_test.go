package ui

import (
	"path/filepath"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// A Git project's header carries the worktree button; a plain workspace does
// not.
func TestProjectHeaderWorktreeButtonAppearsForProjects(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-hdr")
	if out, err := gitRun(dir, "worktree", "add", "-b", "hdr", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{{ID: "c", Title: "t", Workspace: dir}}
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir, Worktrees: []string{dir, wt}}}

	tr := cw.tester(1000, 700, "t")
	if _, ok := tr.Find("新建/打开 Worktree"); !ok {
		t.Fatal("the project header lacks the worktree button")
	}

	// A workspace outside a repository keeps just the new-chat action.
	plain := testWindow()
	plain.mode = modeChat
	plain.cur = &models.Conversation{ID: "p", Title: "plain", Workspace: "/tmp/plain"}
	plain.convs = []models.Conversation{*plain.cur}
	plain.convGit = newConvGitState()
	tr2 := mgo.NewTester(plain.view, 1000, 700)
	for range 3 {
		tr2.Frame()
	}
	if _, ok := tr2.Find("新建/打开 Worktree"); ok {
		t.Fatal("a non-repository workspace must not offer worktrees")
	}
}

// Opening the worktree button shows the create-or-open dialog with the
// project's existing trees listed.
func TestProjectWorktreeDialogRenders(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-dlg")
	if out, err := gitRun(dir, "worktree", "add", "-b", "dlg", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{{ID: "c", Title: "t", Workspace: dir}}
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{
		Main: dir, Worktrees: []string{dir, wt},
		Branches:     []string{"dlg", "free-branch"},
		TreeBranches: map[string]string{dir: "main", wt: "dlg"},
	}}

	tr := cw.tester(1000, 700, "t")
	if err := tr.Click("新建/打开 Worktree"); err != nil {
		t.Fatalf("open the dialog: %v", err)
	}
	for range 6 {
		tr.Frame()
	}
	for _, want := range []string{"新建工作树", "选择分支", "目录名", "打开已有工作树", "设为工作区"} {
		if !tr.HasText(want) {
			t.Fatalf("the dialog lacks %q; texts: %v", want, tr.Texts())
		}
	}
	// The open list shows each tree's branch: the main tree on main, the
	// linked one on dlg.
	if !tr.HasText("主工作树 · main · 002") || !tr.HasText("dlg · wt-dlg") {
		t.Fatalf("the open list lacks the branches; texts: %v", tr.Texts())
	}

	// The branch is picked from a searchable dropdown: opening it lists the
	// branches, and picking a free one fills the form.
	if err := tr.Click("选择分支"); err != nil {
		t.Fatalf("open the branch dropdown: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	if !tr.HasText("free-branch") {
		t.Fatalf("the dropdown lacks the free branch; texts: %v", tr.Texts())
	}
	if err := tr.Click("free-branch"); err != nil {
		t.Fatalf("pick the branch: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	if !tr.HasText("free-branch") {
		t.Fatalf("the picked branch is not shown; texts: %v", tr.Texts())
	}
}
