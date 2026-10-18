package gitapp

import (
	"image"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
)

type fakeHost struct {
	root   string
	toasts []string
	opened []string
}

func (h *fakeHost) Root() string                                        { return h.root }
func (h *fakeHost) Post(fn func())                                      { fn() }
func (h *fakeHost) Invalidate()                                         {}
func (h *fakeHost) Toast(msg string, _ mcore.Severity)                  { h.toasts = append(h.toasts, msg) }
func (h *fakeHost) Confirm(_ string, _ string, action func())           { action() }
func (h *fakeHost) Prompt(_ string, _ string, _ string, a func(string)) { a("") }
func (h *fakeHost) OpenFile(path string)                                { h.opened = append(h.opened, path) }
func (h *fakeHost) Language(ext string) string                          { return strings.TrimPrefix(ext, ".") }
func (h *fakeHost) Workspace() string                                   { return h.root }
func (h *fakeHost) NewID() string                                       { return "id" }
func (h *fakeHost) NativeWindow() *mygo.Window                          { return nil }

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "a@b.c"},
		{"config", "user.name", "T"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if err := os.WriteFile(dir+"/a.txt", []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("add: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	return dir
}

func TestReadModel(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(dir+"/a.txt", []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := readModel(dir)
	if !m.inside || m.branch != "main" {
		t.Fatalf("want inside on main, got %+v", m)
	}
	if len(m.changes) != 1 || m.changes[0].Path != "a.txt" {
		t.Fatalf("want one change, got %+v", m.changes)
	}
	if len(m.commits) != 1 || m.commits[0].Subject != "init" {
		t.Fatalf("want one commit, got %+v", m.commits)
	}
	if m.stat[0] != 1 {
		t.Fatalf("want 1 added line, got %+v", m.stat)
	}
}

func TestReadModelNonRepo(t *testing.T) {
	if m := readModel(t.TempDir()); m.inside {
		t.Fatal("temp dir should not be a repo")
	}
	if m := readModel(""); m.inside {
		t.Fatal("empty root should not be a repo")
	}
}

func TestToolsStatusCommit(t *testing.T) {
	dir := newRepo(t)
	cl := New(&fakeHost{root: dir})
	status, err := cl.status()
	if err != nil || !strings.Contains(status, "main") {
		t.Fatalf("status: %q %v", status, err)
	}
	if err := os.WriteFile(dir+"/b.txt", []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.add(""); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.commitMessage("feat: 添加文件"); err != nil {
		t.Fatal(err)
	}
	if m := readModel(dir); len(m.changes) != 0 || len(m.commits) != 2 {
		t.Fatalf("want clean tree and 2 commits, got %+v", m)
	}
}

func TestToolsReject(t *testing.T) {
	cl := New(&fakeHost{root: t.TempDir()})
	if _, err := cl.status(); err == nil {
		t.Fatal("status outside a repo should fail")
	}
	if _, err := cl.commitMessage("  "); err == nil {
		t.Fatal("empty commit message should fail")
	}
	if _, err := cl.stash("bogus", ""); err == nil {
		t.Fatal("unknown stash action should fail")
	}
}

func TestToolsExposed(t *testing.T) {
	cl := New(&fakeHost{root: t.TempDir()})
	tools := cl.Tools()
	if len(tools) != 7 {
		t.Fatalf("want 7 tools, got %d", len(tools))
	}
	for _, tl := range tools {
		if tl.Name == "" || tl.Run == nil {
			t.Fatalf("tool %+v incomplete", tl)
		}
	}
}

func TestViewRenders(t *testing.T) {
	dir := newRepo(t)
	host := &fakeHost{root: dir}
	cl := New(host)
	render := func() *image.RGBA {
		return mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{Mode: mcore.Light})
			cl.View(c)
		}, 1200, 800, 1)
	}
	for i := 0; i < 100 && !cl.InRepo(); i++ {
		render()
		time.Sleep(10 * time.Millisecond)
	}
	if !cl.InRepo() {
		t.Fatal("client should be inside the repo after loading")
	}
	for _, page := range []int{PageChanges, PageHistory, PageBranches} {
		cl.SetPage(page)
		if img := render(); img == nil {
			t.Fatalf("page %d render failed", page)
		}
	}
}

func TestParseStashes(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(dir+"/a.txt", []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "stash", "push", "-m", "wip").CombinedOutput(); err != nil {
		t.Fatalf("stash: %v %s", err, out)
	}
	stashes := parseStashes(dir)
	if len(stashes) != 1 || stashes[0].Ref != "stash@{0}" {
		t.Fatalf("want one stash, got %+v", stashes)
	}
	if stashes[0].Message != "wip" {
		t.Fatalf("want message wip, got %q", stashes[0].Message)
	}
}

func TestRepoPaneRenders(t *testing.T) {
	dir := newRepo(t)
	if out, err := exec.Command("git", "-C", dir, "tag", "-a", "v1.0.0", "-m", "v1.0.0").CombinedOutput(); err != nil {
		t.Fatalf("tag: %v %s", err, out)
	}
	os.WriteFile(dir+"/a.txt", []byte("one\ntwo\n"), 0o644)
	host := &fakeHost{root: dir}
	cl := New(host)
	render := func() *image.RGBA {
		return mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{Mode: mcore.Light})
			cl.View(c)
		}, 1300, 850, 1)
	}
	for i := 0; i < 100 && !cl.InRepo(); i++ {
		render()
		time.Sleep(10 * time.Millisecond)
	}
	if !cl.InRepo() {
		t.Fatal("repo not loaded")
	}
	cl.SetPage(PageTags)
	if img := render(); img == nil {
		t.Fatal("tags page render failed")
	}
	cl.SetPage(PageRepo)
	if img := render(); img == nil {
		t.Fatal("repo page render failed")
	}
	if len(cl.st.tags) != 1 {
		t.Fatalf("want 1 tag, got %d", len(cl.st.tags))
	}
}

func TestSideBySideToggle(t *testing.T) {
	dir := newRepo(t)
	host := &fakeHost{root: dir}
	cl := New(host)
	render := func() *image.RGBA {
		return mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{Mode: mcore.Light})
			cl.View(c)
		}, 1300, 800, 1)
	}
	for i := 0; i < 100 && !cl.InRepo(); i++ {
		render()
		time.Sleep(10 * time.Millisecond)
	}
	if cl.st.sideBySide {
		t.Fatal("default should be unified")
	}
	cl.st.sideBySide = true
	if img := render(); img == nil {
		t.Fatal("side-by-side render failed")
	}
	cl.st.sideBySide = false
	if img := render(); img == nil {
		t.Fatal("unified render failed")
	}
}

func TestCommitFilesParsing(t *testing.T) {
	dir := newRepo(t)
	os.WriteFile(dir+"/x.go", []byte("package x\n"), 0o644)
	os.WriteFile(dir+"/y.txt", []byte("hello\n"), 0o644)
	exec.Command("git", "-C", dir, "add", "-A").Run()
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "add two").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	hash := strings.TrimSpace(gitOut(dir, "rev-parse", "HEAD"))
	files, stat := parseCommitFiles(dir, hash)
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %+v", files)
	}
	if stat[0] == 0 {
		t.Fatalf("want added lines, got %+v", stat)
	}
	for _, f := range files {
		if f.Status != mgit.GitAdded {
			t.Fatalf("want added, got %+v", f)
		}
	}
}

func TestCommitOldPathOnRename(t *testing.T) {
	dir := newRepo(t)
	exec.Command("git", "-C", dir, "mv", "a.txt", "b.txt").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "rename").Run()
	hash := strings.TrimSpace(gitOut(dir, "rev-parse", "HEAD"))
	if old := commitOldPath(dir, hash, "b.txt"); old != "a.txt" {
		t.Fatalf("want a.txt, got %q", old)
	}
	if old := commitOldPath(dir, hash, "a.txt"); old != "a.txt" {
		t.Fatalf("unchanged path should stay, got %q", old)
	}
}

func TestBuildTreeNests(t *testing.T) {
	files := []mgit.ChangedFile{
		{Path: "internal/ui/a.go"},
		{Path: "internal/gitapp/b.go"},
		{Path: "main.go"},
	}
	expanded := buildTree(files, map[string]bool{})
	var names []string
	for _, r := range expanded {
		names = append(names, r.name)
	}
	want := []string{"internal", "gitapp", "b.go", "ui", "a.go", "main.go"}
	if len(names) != len(want) {
		t.Fatalf("want %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("want %v, got %v", want, names)
		}
	}
	collapsed := buildTree(files, map[string]bool{"internal": true})
	if len(collapsed) != 2 {
		t.Fatalf("collapsed tree rows: %+v", collapsed)
	}
	if !collapsed[0].dir || collapsed[0].count != 2 || collapsed[1].index != 2 {
		t.Fatalf("collapsed tree shape: %+v", collapsed)
	}
}

func TestCommitOpensFirstFile(t *testing.T) {
	dir := newRepo(t)
	os.WriteFile(dir+"/z.go", []byte("package z\n"), 0o644)
	exec.Command("git", "-C", dir, "add", "-A").Run()
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "add z").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	hash := strings.TrimSpace(gitOut(dir, "rev-parse", "HEAD"))
	cl := New(&fakeHost{root: dir})
	render := func() *image.RGBA {
		return mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{Mode: mcore.Light})
			cl.View(c)
		}, 800, 600, 1)
	}
	for i := 0; i < 200; i++ {
		if cl.InRepo() && len(cl.st.commits) > 0 {
			break
		}
		render()
		time.Sleep(10 * time.Millisecond)
	}
	cl.openDiff(cl.st, "commit", hash)
	v := cl.st.viewers[tab{kind: "commit", key: hash}]
	for i := 0; i < 200; i++ {
		if v != nil && len(v.files) > 0 {
			break
		}
		render()
		time.Sleep(10 * time.Millisecond)
	}
	if v == nil || len(v.files) == 0 {
		t.Fatal("commit files not loaded")
	}
	if v.picked != 0 {
		t.Fatalf("want first file picked, got %d", v.picked)
	}
}

func TestFilters(t *testing.T) {
	changes := []mgit.ChangedFile{{Path: "internal/ui/a.go"}, {Path: "README.md"}}
	if got := filterChanges(changes, "UI/A"); len(got) != 1 || got[0].Path != "internal/ui/a.go" {
		t.Fatalf("changes filter: %+v", got)
	}
	if got := filterChanges(changes, "  "); len(got) != 2 {
		t.Fatalf("blank filter should pass all: %+v", got)
	}

	branches := []mgit.Branch{{Name: "main"}, {Name: "origin/feature/x", Upstream: "origin/feature/x"}}
	if got := filterBranches(branches, "feat"); len(got) != 1 {
		t.Fatalf("branch filter: %+v", got)
	}

	tags := []mgit.GitTag{{Name: "v1.0.0", Target: "a", Annotation: "release one"}, {Name: "nightly", Target: "b"}}
	if got := filterTags(tags, "release"); len(got) != 1 || got[0].Name != "v1.0.0" {
		t.Fatalf("tag filter: %+v", got)
	}

	now := time.Now()
	commits := []mgit.Commit{
		{Hash: "abc123", Author: "Alice", Subject: "feat: add", When: now.Add(-2 * time.Hour)},
		{Hash: "def456", Author: "Bob", Subject: "fix: bug", When: now.Add(-10 * 24 * time.Hour)},
	}
	if got := filterCommits(commits, "feat", "", ""); len(got) != 1 || got[0].Author != "Alice" {
		t.Fatalf("message filter: %+v", got)
	}
	if got := filterCommits(commits, "", "bob", ""); len(got) != 1 || got[0].Hash != "def456" {
		t.Fatalf("author filter: %+v", got)
	}
	if got := filterCommits(commits, "", "", "7d"); len(got) != 1 || got[0].Hash != "abc123" {
		t.Fatalf("relative time filter: %+v", got)
	}
	if got := filterCommits(commits, "abc", "", ""); len(got) != 1 {
		t.Fatalf("hash filter: %+v", got)
	}
	if got := filterCommits(commits, "", "", "2006-01-01"); len(got) != 0 {
		t.Fatalf("date filter should exclude old commits: %+v", got)
	}
}

func TestShortPath(t *testing.T) {
	if got := shortPath("/a/b/c/d/e", 2); got != "…/d/e" {
		t.Fatalf("shortPath: %q", got)
	}
	if got := shortPath("/a/b", 3); got != "/a/b" {
		t.Fatalf("short path should stay: %q", got)
	}
}

func TestPushRecentCaps(t *testing.T) {
	cl := New(&fakeHost{root: t.TempDir()})
	for i := 0; i < 15; i++ {
		cl.pushRecent("/repo/" + itoaTest(i))
	}
	if len(cl.st.recent) != recentMax {
		t.Fatalf("want %d recent, got %d", recentMax, len(cl.st.recent))
	}
	if cl.st.recent[0] != "/repo/14" {
		t.Fatalf("newest first, got %q", cl.st.recent[0])
	}
	cl.pushRecent("/repo/14")
	if len(cl.st.recent) != recentMax || cl.st.recent[0] != "/repo/14" {
		t.Fatalf("duplicate should move to front without growing: %+v", cl.st.recent)
	}
}

func TestSwitchRootUpdatesSelection(t *testing.T) {
	cl := New(&fakeHost{root: t.TempDir()})
	cl.SwitchRoot("/repo/x")
	if !cl.st.pinned || cl.st.root != "/repo/x" || cl.st.pickRoot != "/repo/x" {
		t.Fatalf("switch root state: %+v", cl.st)
	}
	if !cl.st.dirty {
		t.Fatal("switching should mark the model stale")
	}
}

func TestRepoLabel(t *testing.T) {
	if got := repoLabel("/Users/a/code/ChatGo"); got != "code/ChatGo" {
		t.Fatalf("repoLabel: %q", got)
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestTreeRowCount(t *testing.T) {
	files := []mgit.ChangedFile{
		{Path: "a/b/c.txt"},
		{Path: "a/b/d.go"},
		{Path: "e.txt"},
	}
	rows := buildTree(files, map[string]bool{})
	byPath := map[string]treeRow{}
	for _, r := range rows {
		if r.dir {
			byPath[r.path] = r
		}
	}
	if byPath["a"].count != 2 {
		t.Fatalf("a count: %d", byPath["a"].count)
	}
	if byPath["a/b"].count != 2 {
		t.Fatalf("a/b count: %d", byPath["a/b"].count)
	}
	if !byPath["a"].dir {
		t.Fatal("a should be a directory row")
	}
}
