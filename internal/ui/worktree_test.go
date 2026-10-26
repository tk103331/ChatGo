package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// isolateHome points HOME at a temp directory for the duration of a test.
// It is mandatory for any test touching workspaces: addWorkspace and
// removeWorktree reach config.SaveConfig, which rewrites
// $HOME/.chatgo/config.yaml, and worktreeRoot resolves under $HOME as well.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := resolved(t, t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// resolved is a path with its symlinks expanded. macOS hands out /var/folders
// temp paths that reach through a symlink, and git prints the expanded form,
// so comparing a path git reported against one built here needs both in the
// same form.
func resolved(t *testing.T, path string) string {
	t.Helper()
	if out, err := filepath.EvalSymlinks(path); err == nil {
		return out
	}
	return path
}

// newTestRepo builds a repository with one commit and returns its directory.
func newTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	isolateHome(t)
	dir := resolved(t, t.TempDir())
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "init")
	return dir
}

// A repository with no linked trees still reports its own, marked as main.
func TestGitWorktreesListsMainTree(t *testing.T) {
	dir := newTestRepo(t)
	trees := gitWorktrees(dir)
	if len(trees) != 1 {
		t.Fatalf("expected the main working tree alone, got %+v", trees)
	}
	w := trees[0]
	if !w.Main {
		t.Error("the first tree git lists is the main one")
	}
	if w.Path != dir {
		t.Errorf("path %q is not the repository %q", w.Path, dir)
	}
	if w.Branch == "" {
		t.Error("the main tree has a branch checked out")
	}
	if w.Head == "" {
		t.Error("the main tree has a HEAD")
	}
}

// Every record the porcelain form prints is read: the branch of a linked
// tree, a detached HEAD, a lock with its reason, and a stale record.
func TestGitWorktreesReadsEveryState(t *testing.T) {
	dir := newTestRepo(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := gitRun(dir, args...); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	feat := filepath.Join(filepath.Dir(dir), "wt-feat")
	det := filepath.Join(filepath.Dir(dir), "wt-det")
	git("worktree", "add", "-b", "feat", feat)
	git("worktree", "add", "--detach", det)
	git("worktree", "lock", det, "--reason", "测试锁定")

	byPath := map[string]worktreeInfo{}
	for _, w := range gitWorktrees(dir) {
		byPath[w.Path] = w
	}
	if len(byPath) != 3 {
		t.Fatalf("expected three working trees, got %d: %+v", len(byPath), byPath)
	}
	if w := byPath[feat]; w.Branch != "feat" || w.Main || w.Detached {
		t.Errorf("the linked tree should be branch feat and not main: %+v", w)
	}
	w := byPath[det]
	if !w.Detached {
		t.Errorf("the detached tree is not marked detached: %+v", w)
	}
	if !w.Locked || w.Reason != "测试锁定" {
		t.Errorf("the lock and its reason were not read: %+v", w)
	}
	if w.Branch != "" {
		t.Errorf("a detached tree has no branch, got %q", w.Branch)
	}

	// A tree whose directory is gone is reported as prunable, which is what
	// the panel offers to clean up.
	git("worktree", "unlock", det)
	if err := os.RemoveAll(det); err != nil {
		t.Fatal(err)
	}
	for _, w := range gitWorktrees(dir) {
		if w.Path != det {
			continue
		}
		if !w.Prunable || w.Prune == "" {
			t.Errorf("the removed directory is not prunable: %+v", w)
		}
		return
	}
	t.Fatal("the stale record is no longer listed")
}

// A tree is named by its branch, by its short hash when detached, and bare
// repositories say so.
func TestWorktreeTitle(t *testing.T) {
	for _, c := range []struct {
		name string
		in   worktreeInfo
		want string
	}{
		{"branch", worktreeInfo{Branch: "feat", Head: "0123456789"}, "feat"},
		{"detached", worktreeInfo{Head: "0123456789abcdef", Detached: true}, "0123456 (detached)"},
		{"bare", worktreeInfo{Bare: true}, "(bare)"},
		{"neither", worktreeInfo{Path: "/tmp/wt-x"}, "wt-x"},
	} {
		if got := c.in.Title(); got != c.want {
			t.Errorf("%s: title %q, want %q", c.name, got, c.want)
		}
	}
}

// New trees land under ~/.chatgo/worktrees, one directory per repository, and
// a branch with a slash stays a single path element.
func TestDefaultWorktreePathStaysUnderRoot(t *testing.T) {
	isolateHome(t)
	path := defaultWorktreePath("/code/ChatGo", "feature/login")
	root := worktreeRoot()
	if !strings.HasPrefix(path, root+string(filepath.Separator)) {
		t.Fatalf("%q is not under the worktree root %q", path, root)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) != 2 {
		t.Fatalf("expected <repo>/<name>, got %q", rel)
	}
	if parts[0] != "ChatGo" {
		t.Errorf("the repository directory is %q, want ChatGo", parts[0])
	}
	if parts[1] != "feature-login" {
		t.Errorf("a slashed branch must flatten, got %q", parts[1])
	}
}

// A name that is all separators still yields a usable directory.
func TestWorktreeSlugNeverEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "///", "...", "-"} {
		if got := worktreeSlug(in); got == "" || strings.ContainsRune(got, filepath.Separator) {
			t.Errorf("slug of %q is %q, which is not a usable path element", in, got)
		}
	}
}

// The filter matches a tree by its path or its branch.
func TestWorktreeFilter(t *testing.T) {
	cw := testWindow()
	cw.wt.trees = []worktreeInfo{
		{Path: "/code/ChatGo", Branch: "main"},
		{Path: "/home/u/.chatgo/worktrees/ChatGo/feat", Branch: "feat"},
	}
	cw.wt.query = "feat"
	if got := cw.worktreeVisible(); len(got) != 1 || got[0].Branch != "feat" {
		t.Fatalf("the branch filter matched %+v", got)
	}
	cw.wt.query = "worktrees"
	if got := cw.worktreeVisible(); len(got) != 1 || got[0].Branch != "feat" {
		t.Fatalf("the path filter matched %+v", got)
	}
	cw.wt.query = ""
	if got := cw.worktreeVisible(); len(got) != 2 {
		t.Fatalf("an empty query shows every tree, got %+v", got)
	}
}

// A branch checked out somewhere cannot be taken a second time, which is
// what git refuses.
func TestGitBranchCheckedOut(t *testing.T) {
	dir := newTestRepo(t)
	if !gitBranchCheckedOut(dir, gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")) {
		t.Error("the main tree's branch must count as checked out")
	}
	if gitBranchCheckedOut(dir, "unused") {
		t.Error("an unused branch must be free")
	}
	wt := filepath.Join(filepath.Dir(dir), "wt-checked")
	if out, err := gitRun(dir, "worktree", "add", "-b", "checked", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	if !gitBranchCheckedOut(dir, "checked") {
		t.Error("a linked tree's branch must count as checked out")
	}
}

// The create form shows where the tree will land before anything is made.
func TestWorktreeFormTarget(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.wt.dir = "/code/ChatGo"
	if got := cw.worktreeFormTarget(); got != "" {
		t.Errorf("with no branch the target is unknown, got %q", got)
	}
	cw.wt.branch = "feat"
	if got, want := cw.worktreeFormTarget(), defaultWorktreePath("/code/ChatGo", "feat"); got != want {
		t.Errorf("target %q, want %q", got, want)
	}
	// An explicit directory name wins over the branch.
	cw.wt.name = "sandbox"
	if got, want := cw.worktreeFormTarget(), defaultWorktreePath("/code/ChatGo", "sandbox"); got != want {
		t.Errorf("target %q, want %q", got, want)
	}
}

// A lock reason survives verbatim however git chose to print it. With
// newline-terminated output git C-quotes a reason holding non-ASCII, a quote
// or a backslash, and a reader that keeps the quoting shows the escapes to
// the user.
func TestWorktreeLockReasonIsUnquoted(t *testing.T) {
	for _, reason := range []string{
		"测试锁定",
		"plain ascii reason",
		`has "quote" and \back`,
		"混合 ascii 与中文",
	} {
		t.Run(reason, func(t *testing.T) {
			dir := newTestRepo(t)
			wt := filepath.Join(filepath.Dir(dir), "wt-lock")
			if out, err := gitRun(dir, "worktree", "add", "--detach", wt); err != nil {
				t.Fatalf("worktree add: %v: %s", err, out)
			}
			if out, err := gitRun(dir, "worktree", "lock", wt, "--reason", reason); err != nil {
				t.Fatalf("worktree lock: %v: %s", err, out)
			}
			for _, w := range gitWorktrees(dir) {
				if w.Path != wt {
					continue
				}
				if !w.Locked {
					t.Fatalf("the tree is locked but was not read as locked: %+v", w)
				}
				if w.Reason != reason {
					t.Fatalf("reason %q, want %q", w.Reason, reason)
				}
				return
			}
			t.Fatalf("the locked tree %q is not listed", wt)
		})
	}
}

// The newline form is read even when -z is unavailable, which is the only
// reason the fallback exists.
func TestParseWorktreesReadsNewlineForm(t *testing.T) {
	out := strings.Join([]string{
		"worktree /code/ChatGo",
		"HEAD 1111111111111111111111111111111111111111",
		"branch refs/heads/main",
		"",
		"worktree /wt/det",
		"HEAD 2222222222222222222222222222222222222222",
		"detached",
		`locked "\351\224\201\345\256\232"`,
		"",
	}, "\n")
	trees := parseWorktrees(out, "\n", true)
	if len(trees) != 2 {
		t.Fatalf("expected two records, got %+v", trees)
	}
	if !trees[0].Main || trees[0].Branch != "main" {
		t.Errorf("the first record is the main tree on main: %+v", trees[0])
	}
	w := trees[1]
	if !w.Detached || !w.Locked || w.Reason != "锁定" {
		t.Errorf("the second record was misread: %+v", w)
	}
}

// mainQueue stands in for the window's own main-thread dispatch. A window
// marshals a postMain callback onto the thread that draws; a test has no
// window, so without a queue the callback would run on the background
// goroutine and write the state the render loop is reading.
type mainQueue struct {
	mu      sync.Mutex
	pending []func()
}

func (q *mainQueue) post(fn func()) {
	q.mu.Lock()
	q.pending = append(q.pending, fn)
	q.mu.Unlock()
}

// drain runs the queued callbacks on the caller's goroutine, which for a test
// is the one that renders.
func (q *mainQueue) drain() {
	q.mu.Lock()
	due := q.pending
	q.pending = nil
	q.mu.Unlock()
	for _, fn := range due {
		fn()
	}
}

// wtEnv is a chat window under test together with the queue its background
// loads post through, so a test can let those callbacks run between frames
// rather than on the loading goroutine.
type wtEnv struct {
	*ChatWindow
	t *testing.T
	q *mainQueue
}

// worktreeWindow is a chat window in a repository, ready to open the tab.
func worktreeWindow(t *testing.T, dir string) *wtEnv {
	t.Helper()
	cw := testWindow()
	cw.convGit = newConvGitState()
	cw.mode = modeChat
	cw.workspace = dir
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: dir}
	cw.convs = []models.Conversation{*cw.cur}
	q := &mainQueue{}
	cw.dispatch = q.post
	return &wtEnv{ChatWindow: cw, t: t, q: q}
}

// waitWorktrees renders until the tab's list has loaded.
func (e *wtEnv) waitWorktrees() {
	e.t.Helper()
	e.waitFor("the worktree list to load", func() bool { return e.wt.loaded })
}

// waitFor renders until cond holds. The panel's operations run in the
// background and post their result to the main thread, so a test cannot
// simply read the state after calling one: the list it must see is the one
// the reload that follows the operation brings in.
func (e *wtEnv) waitFor(what string, cond func() bool) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.q.drain()
		if cond() {
			return
		}
		mgo.Render(e.view, 1400, 900, 1)
		time.Sleep(2 * time.Millisecond)
	}
	e.t.Fatalf("timed out waiting for %s", what)
}

// tester renders the window until want appears, draining the queue between
// frames, and returns the tester so the caller can inspect the layout.
func (e *wtEnv) tester(width, height int, want string) *mgo.Tester {
	e.t.Helper()
	tr := mgo.NewTester(e.view, width, height)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.q.drain()
		tr.Frame()
		if tr.HasText(want) {
			return tr
		}
	}
	e.t.Fatalf("%q never appeared; texts: %v", want, tr.Texts())
	return nil
}

// worktreeAt is the listed working tree at path, if the panel knows one.
func (cw *ChatWindow) worktreeAt(path string) (worktreeInfo, bool) {
	for _, w := range cw.wt.trees {
		if w.Path == path {
			return w, true
		}
	}
	return worktreeInfo{}, false
}

// The tab lists the repository's trees and names them by branch.
func TestWorktreeTabRenders(t *testing.T) {
	dir := newTestRepo(t)
	feat := filepath.Join(filepath.Dir(dir), "wt-feat")
	if out, err := gitRun(dir, "worktree", "add", "-b", "feat", feat); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.openWorktreeTab(0)
	cw.waitWorktrees()
	if len(cw.wt.trees) != 2 {
		t.Fatalf("expected two working trees, got %+v", cw.wt.trees)
	}
	tr := cw.tester(1400, 900, "2 个工作树")
	if _, ok := tr.Find("Worktree"); !ok {
		t.Fatal("no Worktree tab in the strip")
	}
	if !tr.HasText("feat") {
		t.Fatal("the linked tree's branch is not listed")
	}
}

// Outside a repository the tab says so rather than showing an empty list.
func TestWorktreeTabOutsideRepo(t *testing.T) {
	isolateHome(t)
	dir := t.TempDir()
	cw := worktreeWindow(t, dir)
	cw.openWorktreeTab(0)
	cw.waitWorktrees()
	if cw.wt.repo {
		t.Fatal("a plain directory is not a repository")
	}
	cw.tester(1400, 900, "当前工作区不是 Git 仓库")
}

// Only one Worktree tab exists at a time; opening it again focuses that one.
func TestWorktreeTabOpensOnce(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.openWorktreeTab(0)
	cw.openWorktreeTab(0)
	n := 0
	for _, tab := range cw.center.panes[0].tabs {
		if tab.Kind == centerWorktree {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected one Worktree tab, got %d", n)
	}
}

// Opening a tree as the workspace switches to it and keeps the tab in view,
// even though every workspace holds its own set of center tabs.
func TestOpenWorktreeWorkspaceKeepsTab(t *testing.T) {
	dir := newTestRepo(t)
	feat := filepath.Join(filepath.Dir(dir), "wt-keep")
	if out, err := gitRun(dir, "worktree", "add", "-b", "keep", feat); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cw.openWorktreeTab(0)
	cw.waitWorktrees()

	cw.openWorktreeWorkspace(feat)
	if cw.workspace != feat {
		t.Fatalf("workspace is %q, want %q", cw.workspace, feat)
	}
	if cw.focusCenterTab("worktree") == nil {
		t.Fatal("the Worktree tab vanished with the workspace switch")
	}
	if !containsStr(cw.cfg.Workspaces, feat) {
		t.Error("the tree was not added to the workspace list")
	}
}

// A directory git no longer has cannot become the workspace.
func TestOpenWorktreeWorkspaceRejectsMissingDir(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	gone := filepath.Join(filepath.Dir(dir), "not-there")
	cw.openWorktreeWorkspace(gone)
	if cw.workspace == gone {
		t.Fatal("a missing directory must not become the workspace")
	}
	if !containsStr(cw.cfg.Workspaces, gone) {
		return
	}
	t.Fatal("a missing directory must not be saved as a workspace")
}

// Removing the tree the app is sitting in moves the workspace back to the
// repository, so nothing points at a deleted directory.
func TestForgetWorktreeWorkspaceFallsBack(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.wt.dir = dir
	gone := filepath.Join(filepath.Dir(dir), "wt-gone")
	cw.cfg.Workspaces = []string{dir, gone}
	cw.workspace = gone

	cw.forgetWorktreeWorkspace(gone)
	if cw.workspace != dir {
		t.Fatalf("workspace stayed at %q, want the repository %q", cw.workspace, dir)
	}
	if containsStr(cw.cfg.Workspaces, gone) {
		t.Error("the removed tree is still in the workspace list")
	}
}

// The Git panel's worktree selector appears only once a repository has more
// than its own working tree.
func TestWorktreeSelectorNeedsSeveralTrees(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()
	if len(cw.wt.trees) != 1 {
		t.Fatalf("a fresh repository has one working tree, got %+v", cw.wt.trees)
	}
	if cw.worktreeSelectorVisible() {
		t.Fatal("the selector must stay hidden with a single working tree")
	}

	feat := filepath.Join(filepath.Dir(dir), "wt-sel")
	if out, err := gitRun(dir, "worktree", "add", "-b", "sel", feat); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw.loadWorktrees()
	cw.waitWorktrees()
	if !cw.worktreeSelectorVisible() {
		t.Fatalf("the selector must appear with %d working trees", len(cw.wt.trees))
	}

	// The panel draws it by the working tree's directory, which is what sets
	// two trees of one repository apart.
	cw.rightPanel = true
	cw.rightTab = 1
	cw.tester(1500, 900, worktreeLabel(dir))
}

// Outside a repository the selector is never drawn, whatever stale list the
// state happens to hold.
func TestWorktreeSelectorHiddenOutsideRepo(t *testing.T) {
	cw := testWindow()
	cw.wt = worktreeState{
		dir: "/tmp/plain", loaded: true, repo: false,
		trees: []worktreeInfo{{Path: "/a"}, {Path: "/b"}},
	}
	if cw.worktreeSelectorVisible() {
		t.Fatal("a directory outside a repository has no working trees to offer")
	}
}

// A Worktree tab renders beside the other kinds without upsetting them.
func TestWorktreeTabRendersWithOtherTabs(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.setLayout(layoutCols)
	cw.openWorktreeTab(1)
	cw.openCenterTab(1, &centerTab{Kind: centerGitGraph, ID: "gitgraph", Title: "GitGraph"})
	cw.waitWorktrees()
	cw.frames(4)
}

// The center tabs and the Git panel follow the working tree: every tree of
// one project keeps its own open tabs and its own git view, so switching
// between them is switching directories, not resetting to the project.
func TestCenterStateSwitchesPerWorktree(t *testing.T) {
	dir := newTestRepo(t)
	a := filepath.Join(filepath.Dir(dir), "wt-center-a")
	b := filepath.Join(filepath.Dir(dir), "wt-center-b")
	for _, c := range []struct {
		path, branch string
	}{{a, "ca"}, {b, "cb"}} {
		if out, err := gitRun(dir, "worktree", "add", "-b", c.branch, c.path); err != nil {
			t.Fatalf("worktree add: %v: %s", err, out)
		}
	}
	cw := worktreeWindow(t, dir)

	// The main tree opens a file, so its state holds a tab.
	cw.openPreview(filepath.Join(dir, "a.txt"))
	if n := len(cw.center.panes[0].tabs); n == 0 {
		t.Fatal("the main tree's tab is missing")
	}

	// Opening worktree A swaps to A's empty state and points the Git panel
	// at A; opening a file there gives A its own tab.
	cw.openWorktreeWorkspace(a)
	if cw.workspace != a {
		t.Fatalf("workspace %q, want %q", cw.workspace, a)
	}
	if n := len(cw.center.panes[0].tabs); n != 0 {
		t.Fatalf("a fresh tree starts with %d tabs, want none", n)
	}
	if cw.gitFor != a {
		t.Fatalf("the Git panel points at %q, want %q", cw.gitFor, a)
	}
	cw.openPreview(filepath.Join(a, "a.txt"))
	if n := len(cw.center.panes[0].tabs); n == 0 {
		t.Fatal("A's tab is missing")
	}

	// Worktree B is its own empty state too.
	cw.openWorktreeWorkspace(b)
	if n := len(cw.center.panes[0].tabs); n != 0 {
		t.Fatalf("B starts with %d tabs, want none", n)
	}

	// Coming back to A restores A's tab, and back to the main tree restores
	// the main one's — not one shared set for the project.
	cw.openWorktreeWorkspace(a)
	if n := len(cw.center.panes[0].tabs); n != 1 {
		t.Fatalf("A's tabs were not restored, got %d", n)
	}
	cw.openWorktreeWorkspace(dir)
	if n := len(cw.center.panes[0].tabs); n != 1 {
		t.Fatalf("the main tree's tabs were not restored, got %d", n)
	}
}

// frames renders n frames, running any queued main-thread work between them.
func (e *wtEnv) frames(n int) {
	e.t.Helper()
	for i := range n {
		e.q.drain()
		if img := mgo.Render(e.view, 1500, 900, 1); img == nil {
			e.t.Fatalf("frame %d did not render", i)
		}
	}
}
