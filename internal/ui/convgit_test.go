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

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\ny\nz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info = readConvGit(dir)
	if info.Changes != 1 {
		t.Fatalf("one untracked file is one change, got %d", info.Changes)
	}
	if info.Untracked != 1 {
		t.Errorf("one untracked file counted, got %+v", info)
	}
	if info.DiffAdded != 3 {
		t.Errorf("three added lines counted, got %d", info.DiffAdded)
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
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{Branch: "feat"}
	if got := cw.convGitSubtitle("/code/ChatGo"); got != "feat" {
		t.Errorf("the subtitle is the branch, got %q", got)
	}
	// An empty workspace has no directory to read, whatever the cache holds.
	if got := cw.convGitSubtitle(""); got != "" {
		t.Errorf("the empty workspace shows nothing, got %q", got)
	}
}

// The line counts ride beside the branch, and a missing summary reads zero.
func TestConvGitDiffFormats(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.convGit.info["/code/ChatGo"] = &convGitInfo{DiffAdded: 555, DiffDeleted: 777}
	added, deleted := cw.convGitDiff("/code/ChatGo")
	if added != 555 || deleted != 777 {
		t.Errorf("diff %d/+%d, want +555 -777", added, deleted)
	}
	if a, d := cw.convGitDiff(""); a != 0 || d != 0 {
		t.Errorf("the empty workspace reads %d/+%d, want 0", a, d)
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
	if info.Project.TreeBranches[dir] == "" {
		t.Errorf("the main tree's branch is missing: %+v", info.Project.TreeBranches)
	}
	foundMain := false
	for _, b := range info.Project.Branches {
		if b == info.Project.TreeBranches[dir] {
			foundMain = true
		}
	}
	if !foundMain {
		t.Errorf("the branch list misses the checked-out branch: %v", info.Project.Branches)
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

	if got := projectHeaders(cw.groupedConvRows()); len(got) != 1 || got[0] != dir {
		t.Fatalf("one group under the main tree %q expected, got %v", dir, got)
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

	if got := projectHeaders(cw.groupedConvRows()); len(got) != 1 || got[0] != dir {
		t.Fatalf("one group under the main tree %q expected, got %v", dir, got)
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
	if got := projectHeaders(rows); len(got) != 1 || got[0] != dir {
		t.Fatalf("one group expected under %q, got %v", dir, got)
	}
	// Each tree is labelled under the project, so the two sessions are not
	// drawn as one undifferentiated group.
	var trees, titles []string
	for _, r := range rows {
		switch {
		case r.header && r.sub:
			trees = append(trees, r.ws)
		case !r.header:
			titles = append(titles, cw.convs[r.index].Title)
		}
	}
	if len(trees) != 2 || trees[0] != dir || trees[1] != wt {
		t.Fatalf("both trees must be labelled, main first: %v", trees)
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

// The sidebar shows the workspace's branch and its colored line counts under
// the conversation title.
func TestConversationRowShowsGitSubtitle(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "feat", DiffAdded: 555, DiffDeleted: 777}

	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("t") {
		t.Fatal("the conversation title is missing")
	}
	if !tr.HasText("feat") || !tr.HasText("+555") || !tr.HasText("-777") {
		t.Fatalf("the branch or the diff is missing; texts: %v", tr.Texts())
	}
}

// A diff over a thousand lines is compacted, so the row stays short.
func TestConversationRowShowsCompactDiff(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "feat", DiffAdded: 3900, DiffDeleted: 12000}

	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("+3.9k") || !tr.HasText("-12.0k") {
		t.Fatalf("the compact diff is missing; texts: %v", tr.Texts())
	}
}

// The conversation row's menu offers the workspace actions: copy the path,
// copy the branch and reveal the directory.
func TestConversationRowMenuOffersWorkspaceActions(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c", Title: "menu", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "main"}

	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 3 {
		tr.Frame()
	}
	if err := tr.Click("会话操作"); err != nil {
		t.Fatalf("open the row menu: %v", err)
	}
	for range 2 {
		tr.Frame()
	}
	for _, want := range []string{"复制路径", "复制分支", "在文件管理器中查看"} {
		if !tr.HasText(want) {
			t.Fatalf("the menu lacks %q; texts: %v", want, tr.Texts())
		}
	}
	// Copy path puts the workspace directory on the clipboard.
	if err := tr.Click("复制路径"); err != nil {
		t.Fatalf("choose copy path: %v", err)
	}
	for range 2 {
		tr.Frame()
	}
	if got := tr.Clipboard(); got != dir {
		t.Fatalf("copy path put %q on the clipboard, want %q", got, dir)
	}
}

// projectHeaders lists the project headers of a row list, leaving out the
// per-tree headers drawn under them.
func projectHeaders(rows []convRow) []string {
	var out []string
	for _, r := range rows {
		if r.header && !r.sub {
			out = append(out, r.ws)
		}
	}
	return out
}

// A project whose sessions all live in one linked tree shows that tree alone
// under its header: the main tree, holding nothing, gets no empty row.
func TestGroupedConvRowsSkipsTreesWithoutSessions(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-only")
	if out, err := gitRun(dir, "worktree", "add", "-b", "only", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[wt] = &convGitInfo{Project: &Project{Main: dir}}

	rows := cw.groupedConvRows()
	if got := projectHeaders(rows); len(got) != 1 || got[0] != dir {
		t.Fatalf("one project group under %q expected, got %v", dir, got)
	}
	var headers []string
	for _, r := range rows {
		if r.header {
			headers = append(headers, r.ws)
		}
	}
	if len(headers) != 1 {
		t.Fatalf("a single tree needs no header of its own, got %v", headers)
	}
}

// A project with sessions in two trees labels each tree, and the main tree,
// sorted first, gets a collapse state of its own rather than sharing the
// project header's.
func TestGroupedConvRowsLabelsEachTree(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-label")
	if out, err := gitRun(dir, "worktree", "add", "-b", "label", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}
	cw.convGit.info[wt] = &convGitInfo{Project: &Project{Main: dir}}

	rows := cw.groupedConvRows()
	if len(rows) != 5 {
		t.Fatalf("project, two tree headers and two sessions expected, got %d rows", len(rows))
	}
	if !rows[1].sub || !rows[1].first || rows[1].key == rows[0].key {
		t.Fatalf("the main tree's header must be its own row with its own key: %+v", rows[1])
	}
	cw.wsCollapsed[rows[0].ws] = true
	if got := cw.groupedConvRows(); len(got) != 1 {
		t.Fatalf("collapsing the project must hide its trees, got %d rows", len(got))
	}
	cw.wsCollapsed[rows[0].ws] = false
	cw.wsCollapsed[rows[1].key] = true
	got := cw.groupedConvRows()
	var titles []string
	for _, r := range got {
		if !r.header {
			titles = append(titles, cw.convs[r.index].Title)
		}
	}
	if len(titles) != 1 || titles[0] != "linked" {
		t.Fatalf("collapsing the main tree must hide only its own session, got %v", titles)
	}
}

// Collapsing a project hides its sessions, and the chevron toggle must write
// the same key the row builder reads: a project header folds under its own
// group path, exactly like the "wt:" keys the tree headers use.
func TestProjectHeaderCollapsesUnderItsOwnKey(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-collapse")
	if out, err := gitRun(dir, "worktree", "add", "-b", "coll", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}
	cw.convGit.info[wt] = &convGitInfo{Project: &Project{Main: dir}}

	rows := cw.groupedConvRows()
	if len(rows) != 5 {
		t.Fatalf("project + two tree headers + two sessions expected, got %d rows", len(rows))
	}
	proj := rows[0]
	if !proj.header || proj.sub || proj.key == "" {
		t.Fatalf("the project header must carry its own collapse key: %+v", proj)
	}
	// The header's toggle target is the same key the builder checks, so the
	// two agree by construction.
	cw.wsCollapsed[proj.key] = true
	if got := cw.groupedConvRows(); len(got) != 1 {
		t.Fatalf("collapsing the project left %d rows, want just its header", len(got))
	}
	cw.wsCollapsed[proj.key] = false
	if got := cw.groupedConvRows(); len(got) != 5 {
		t.Fatalf("expanding the project did not restore its rows: %d", len(got))
	}
}

// A single-tree project folds under its own path too, which is the common
// case where no tree headers exist at all.
func TestSingleTreeProjectCollapses(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}

	rows := cw.groupedConvRows()
	if len(rows) != 2 {
		t.Fatalf("header + one session expected, got %d rows", len(rows))
	}
	if rows[0].key != dir {
		t.Fatalf("the project header folds under %q, want %q", rows[0].key, dir)
	}
	cw.wsCollapsed[rows[0].key] = true
	if got := cw.groupedConvRows(); len(got) != 1 {
		t.Fatalf("collapsing a single-tree project left %d rows", len(got))
	}
}

// A working tree is named by the branch it has checked out, not by its
// directory: the main tree's directory is the project's own, so a path would
// print the project's name twice in a row.
func TestWorktreeHeaderNamesTheBranch(t *testing.T) {
	dir := newTestRepo(t)
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "main", Project: &Project{Main: dir}}

	if got := cw.worktreeHeaderLabel(dir); got != "main" {
		t.Fatalf("the tree header reads %q, want its branch %q", got, "main")
	}
	// Before the summary loads there is no branch to show, so the directory
	// stands in rather than leaving the row blank.
	cw.convGit = newConvGitState()
	if got := cw.worktreeHeaderLabel(dir); got != filepath.Base(dir) {
		t.Fatalf("without a summary the header reads %q, want the directory", got)
	}
}

// A session under a labelled tree leaves the branch out, since the header
// above it already says which branch it is.
func TestSessionRowDropsBranchUnderLabelledTree(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-branchdup")
	if out, err := gitRun(dir, "worktree", "add", "-b", "dup", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{Branch: "main", Project: &Project{Main: dir}}
	cw.convGit.info[wt] = &convGitInfo{Branch: "dup", Project: &Project{Main: dir}}

	for _, r := range cw.groupedConvRows() {
		if r.header {
			continue
		}
		if !r.branchShown {
			t.Fatalf("a session under a labelled tree must know its branch is already shown: %+v", r)
		}
	}

	// A project with a single tree has no tree header, so its sessions keep
	// showing the branch themselves.
	cw.convs = []models.Conversation{{ID: "c1", Title: "main", Workspace: dir}}
	for _, r := range cw.groupedConvRows() {
		if r.header {
			continue
		}
		if r.branchShown {
			t.Fatalf("a session with no tree header above it must show its branch: %+v", r)
		}
	}
}

// The change counts belong to the working tree, so a tree header carries them
// and the sessions under it do not repeat them.
func TestWorktreeHeaderCarriesChangeSummary(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-diffsum")
	if out, err := gitRun(dir, "worktree", "add", "-b", "diffsum", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "main", Workspace: dir},
		{ID: "c2", Title: "linked", Workspace: wt},
	}
	cw.convGit = newConvGitState()
	cw.convGit.info[dir] = &convGitInfo{
		Branch: "main", DiffAdded: 4600, DiffDeleted: 416, Project: &Project{Main: dir},
	}
	cw.convGit.info[wt] = &convGitInfo{
		Branch: "diffsum", DiffAdded: 12, DiffDeleted: 3, Project: &Project{Main: dir},
	}

	// Each tree reports its own counts, which is what the header draws.
	if a, d := cw.convGitDiff(dir); a != 4600 || d != 416 {
		t.Fatalf("the main tree reports +%d -%d", a, d)
	}
	if a, d := cw.convGitDiff(wt); a != 12 || d != 3 {
		t.Fatalf("the linked tree reports +%d -%d", a, d)
	}
	// Every session sits under a labelled tree, so none of them repeats the
	// counts the header already shows.
	for _, r := range cw.groupedConvRows() {
		if !r.header && !r.branchShown {
			t.Fatalf("a session under a tree header must drop the counts: %+v", r)
		}
	}
}

// The sidebar abbreviates counts the way the conversation rows always have.
func TestShortLineCount(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1000: "1.0k", 4600: "4.6k", 438800: "438.8k", 2_500_000: "2.5m"}
	for in, want := range cases {
		if got := shortLineCount(in); got != want {
			t.Errorf("shortLineCount(%d) = %q, want %q", in, got, want)
		}
	}
}

// The sidebar's order is fixed: projects keep the order they were configured
// in and trees the order git reports, so neither running a session nor
// switching projects moves a group. A list that reorders itself under the
// pointer is what this guards against.
func TestSidebarOrderIgnoresActivity(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	wtB := filepath.Join(filepath.Dir(projB), "wt-order")
	if out, err := gitRun(projB, "worktree", "add", "-b", "ordered", wtB); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	now := time.Now()
	cw := testWindow()
	cw.wsCollapsed = map[string]bool{}
	cw.cfg.Workspaces = []string{projA, projB}
	cw.convGit = newConvGitState()
	cw.convGit.info[projA] = &convGitInfo{Branch: "main", Project: &Project{Main: projA, Worktrees: []string{projA}}}
	cw.convGit.info[projB] = &convGitInfo{Branch: "main", Project: &Project{Main: projB, Worktrees: []string{projB, wtB}}}
	cw.convGit.info[wtB] = &convGitInfo{Branch: "ordered", Project: &Project{Main: projB, Worktrees: []string{projB, wtB}}}

	// B's linked tree has the newest session, A's the oldest; the order must
	// not follow that.
	cw.convs = []models.Conversation{
		{ID: "newest", Title: "n", Workspace: wtB, CreatedAt: now},
		{ID: "mid", Title: "m", Workspace: projB, CreatedAt: now.Add(-time.Hour)},
		{ID: "oldest", Title: "o", Workspace: projA, CreatedAt: now.Add(-2 * time.Hour)},
	}
	// A has one tree, so it draws only its project header; B has two, so its
	// project header is followed by a header per tree, main first.
	want := []string{projA, projB, projB, wtB}
	if got := headerOrder(cw.groupedConvRows()); !equalStrings(got, want) {
		t.Fatalf("headers are %v, want the configured order %v", got, want)
	}

	// A gains the newest session and the workspace moves to B's tree, while
	// every tree keeps a session of its own. Neither changes the order.
	cw.convs = append(cw.convs, models.Conversation{
		ID: "freshest", Title: "f", Workspace: projA, CreatedAt: now.Add(time.Hour),
	})
	cw.workspace = wtB
	if got := headerOrder(cw.groupedConvRows()); !equalStrings(got, want) {
		t.Fatalf("activity reordered the sidebar to %v, want %v", got, want)
	}
}

// headerOrder lists every header's path, projects and trees alike, in the
// order the list draws them.
func headerOrder(rows []convRow) []string {
	var out []string
	for _, r := range rows {
		if r.header {
			out = append(out, r.ws)
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
