package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// A workspace outside a repository has no summary, so its row stays one line.
func TestReadConvGitOutsideRepo(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	if info := readConvGit(dir); info != nil {
		t.Fatalf("a plain directory must yield no summary, got %+v", info)
	}
}

// A repository reports its branch and how many files are changed.
func TestReadConvGitReadsRepository(t *testing.T) {
	dir := newTestRepo(t)
	info := readConvGit(dir)
	if info == nil {
		t.Fatal("a repository must have a summary")
	}
	if info.Branch == "" {
		t.Error("the repository's branch was not read")
	}
	if info.Changes != 0 {
		t.Errorf("a clean repository has %d changes", info.Changes)
	}

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info = readConvGit(dir)
	if info.Changes != 1 {
		t.Fatalf("one untracked file is one change, got %d", info.Changes)
	}
	if info.Untracked != 1 {
		t.Errorf("one untracked file counted, got %+v", info)
	}
}

// The analysis breaks the changes down by status, and calls a clean tree
// clean.
func TestConvGitAnalysisFormats(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	if got := cw.convGitAnalysis("/code/ChatGo"); got != "" {
		t.Errorf("no summary means no analysis, got %q", got)
	}
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{Modified: 3, Added: 1}
	if got := cw.convGitAnalysis("/code/ChatGo"); got != "3 修改 · 1 新增" {
		t.Errorf("the breakdown is wrong, got %q", got)
	}
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{}
	if got := cw.convGitAnalysis("/code/ChatGo"); got != "工作区干净" {
		t.Errorf("a clean tree says so, got %q", got)
	}
}

// Only the main working tree of a project is its main tree.
func TestConvGitMainFlagsMainTree(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-mainflag")
	if out, err := gitRun(dir, "worktree", "add", "-b", "mf", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}
	cw.convGit.info[wt] = &convGitInfo{Project: &Project{Main: dir}}
	if !cw.convGitMain(dir) {
		t.Error("the main tree must be flagged main")
	}
	if cw.convGitMain(wt) {
		t.Error("a linked tree must not be flagged main")
	}
}

// Resting the pointer on a conversation shows the hover card to its right,
// with the analysis, the directory and the worktree kind.
func TestConversationRowHoverCardShows(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c", Title: "hovered", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{
		Branch: "feat", Changes: 2, Modified: 1, Added: 1,
		Project: &Project{Main: dir},
	}

	tr := mgo.NewTester(cw.view, 1000, 700)
	tr.Frame()
	at, ok := tr.Find("hovered")
	if !ok {
		t.Fatalf("no conversation row to hover; texts: %v", tr.Texts())
	}
	tr.Move(at.X+at.W/2, at.Y+at.H/2)
	tr.Advance(700 * time.Millisecond)
	tr.Frame()
	if !tr.HasText("分析：1 修改 · 1 新增") {
		t.Fatalf("the hover card does not show the analysis; texts: %v", tr.Texts())
	}
	if !tr.HasText("主工作树") {
		t.Fatalf("the main tree kind is missing; texts: %v", tr.Texts())
	}
	if !tr.HasText(dir) {
		t.Fatalf("the directory is missing; texts: %v", tr.Texts())
	}
}

// The subtitle is empty without a summary, the bare branch when nothing
// changed, and branch plus count when files are dirty.
func TestConvGitSubtitleFormats(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	if got := cw.convGitSubtitle("/code/ChatGo"); got != "" {
		t.Errorf("no summary must mean no subtitle, got %q", got)
	}
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{Branch: "main", Changes: 0}
	if got := cw.convGitSubtitle("/code/ChatGo"); got != "main" {
		t.Errorf("a clean branch stands alone, got %q", got)
	}
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{Branch: "feat", Changes: 3}
	if got := cw.convGitSubtitle("/code/ChatGo"); got != "feat · 3 个变更" {
		t.Errorf("dirty branch must count changes, got %q", got)
	}
	// An empty workspace has no directory to read, whatever the cache holds.
	if got := cw.convGitSubtitle(""); got != "" {
		t.Errorf("the empty workspace shows nothing, got %q", got)
	}
}

// A detached HEAD is named by its short hash, the way the git graph names
// one.
func TestReadConvGitDetachedHead(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-det")
	if out, err := gitRun(dir, "worktree", "add", "--detach", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	info := readConvGit(wt)
	if info == nil {
		t.Fatal("a detached working tree still has a summary")
	}
	if !strings.HasSuffix(info.Branch, " (detached)") {
		t.Errorf("a detached tree is marked, got %q", info.Branch)
	}
}

// A linked working tree groups under the repository's main tree, so its
// conversations sit with the main tree's.
func TestReadConvGitGroupsUnderMainTree(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-group")
	if out, err := gitRun(dir, "worktree", "add", "-b", "group", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	info := readConvGit(wt)
	if info == nil {
		t.Fatal("a working tree has a summary")
	}
	if info.Project == nil || info.Project.Main != dir {
		t.Errorf("the tree belongs to project %+v, want the main tree %q", info.Project, dir)
	}
	// The main tree belongs to the project headed by itself.
	main := readConvGit(dir)
	if main.Project == nil || main.Project.Main != dir {
		t.Errorf("the main tree belongs to %+v, want itself", main.Project)
	}
	if len(main.Project.Worktrees) != 2 {
		t.Errorf("the project holds two trees, got %v", main.Project.Worktrees)
	}
}

// The project lists every working tree, main first.
func TestProjectCollectsAllWorktrees(t *testing.T) {
	dir := newTestRepo(t)
	w1 := filepath.Join(filepath.Dir(dir), "wt-p1")
	w2 := filepath.Join(filepath.Dir(dir), "wt-p2")
	for _, w := range []string{w1, w2} {
		if out, err := gitRun(dir, "worktree", "add", "--detach", w); err != nil {
			t.Fatalf("worktree add: %v: %s", err, out)
		}
	}
	info := readConvGit(dir)
	if info.Project == nil {
		t.Fatal("a repository has a project")
	}
	if info.Project.Main != dir {
		t.Errorf("the project is headed by %q, want %q", info.Project.Main, dir)
	}
	if len(info.Project.Worktrees) != 3 {
		t.Fatalf("three trees belong to the project, got %v", info.Project.Worktrees)
	}
	if info.Project.Worktrees[0] != dir {
		t.Errorf("the main tree heads the list, got %v", info.Project.Worktrees)
	}
}

// Without a loaded summary a conversation keeps its own workspace as the
// group, which the finished load then replaces.
func TestConvGitGroupFallsBackToWorkspace(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	if got := cw.convGitGroup("/code/ChatGo"); got != "/code/ChatGo" {
		t.Errorf("no summary means the workspace is its own group, got %q", got)
	}
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{Project: &Project{Main: "/wt/main"}}
	if got := cw.convGitGroup("/code/ChatGo"); got != "/wt/main" {
		t.Errorf("the cached project is used once loaded, got %q", got)
	}
}

// A working tree opened through the worktree panel's "设为工作区", holding
// conversations, groups under the main tree like any other linked tree.
func TestWorktreeSetAsWorkspaceGroupsWithMain(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-setas")
	if out, err := gitRun(dir, "worktree", "add", "-b", "saw", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.openWorktreeWorkspace(wt)
	cw.waitFor("the linked tree's summary to load", func() bool {
		return cw.convGitProject(wt) != nil
	})

	rows := cw.groupedConvRows()
	var headers []string
	for _, r := range rows {
		if r.header {
			headers = append(headers, r.ws)
		}
	}
	if len(headers) != 1 || headers[0] != dir {
		t.Fatalf("one group under the main tree %q expected, got %v", dir, headers)
	}
}

// A working tree set as workspace but holding no conversation yet must not
// create a group of its own next to the main tree's.
func TestWorktreeSetAsWorkspaceWithoutConversationsGroups(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-solo")
	if out, err := gitRun(dir, "worktree", "add", "-b", "solo", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
	}
	cw.openWorktreeWorkspace(wt)
	cw.waitFor("the linked tree's summary to load", func() bool {
		return cw.convGitProject(wt) != nil
	})

	rows := cw.groupedConvRows()
	var headers []string
	for _, r := range rows {
		if r.header {
			headers = append(headers, r.ws)
		}
	}
	if len(headers) != 1 || headers[0] != dir {
		t.Fatalf("one group under the main tree %q expected, got %v", dir, headers)
	}
}

// The conversation list puts a working tree's sessions under the project's
// header, one group instead of two.
func TestGroupedConvRowsMergeWorktrees(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-rows")
	if out, err := gitRun(dir, "worktree", "add", "-b", "rows", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}
	cw.convGit.info[wt] = &convGitInfo{Project: &Project{Main: dir}}

	rows := cw.groupedConvRows()
	var headers, titles []string
	for _, r := range rows {
		if r.header {
			headers = append(headers, r.ws)
		} else {
			titles = append(titles, cw.convs[r.index].Title)
		}
	}
	if len(headers) != 1 || headers[0] != dir {
		t.Fatalf("one group expected under %q, got %v", dir, headers)
	}
	if len(titles) != 2 {
		t.Fatalf("both conversations must stay, got %v", titles)
	}
}

// ensureConvGit loads the summary in the background and the list sees it
// once the queued callback runs on the drawing thread.
func TestConvGitLoadsInBackground(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	q := &mainQueue{}
	cw.dispatch = q.post

	cw.ensureConvGit(dir)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		q.drain()
		cw.convGit.mu.Lock()
		loaded := cw.convGit.info[dir] != nil
		cw.convGit.mu.Unlock()
		if loaded {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("the summary never loaded")
}

// A second ensure does not start another load while one is running.
func TestConvGitDoesNotReloadWhileLoading(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.convGit.loading[dir] = true
	cw.ensureConvGit(dir)
	cw.convGit.mu.Lock()
	still := cw.convGit.loading[dir]
	cw.convGit.mu.Unlock()
	if !still {
		t.Fatal("a load already in flight must not be started twice")
	}
}

// The sidebar shows the workspace's branch and change count under the
// conversation title, as a second, muted line.
func TestConversationRowShowsGitSubtitle(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "feat", Changes: 2}

	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("t") {
		t.Fatal("the conversation title is missing")
	}
	if !tr.HasText("feat · 2 个变更") {
		t.Fatalf("the git subtitle is missing; texts: %v", tr.Texts())
	}
}
