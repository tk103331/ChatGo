package ui

import (
	"os"
	"path/filepath"
	"testing"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// The "新会话" tab renders its choice rows and the composer.
func TestCreateTabRenders(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	// The model picker's trigger reads as the chosen model, so it is matched
	// by that rather than by the word 模型.
	for _, want := range []string{"Agent", "项目", "工作树", "OpenAI · gpt-4", "发送"} {
		if !tr.HasText(want) {
			t.Fatalf("the create tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

// Opening the create tab from a project header preselects that project.
func TestCreateTabPreselectsProject(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{{ID: "c", Title: "t", Workspace: dir}}
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir}}
	cw.openCreateTabIn(dir)
	if cw.create.ws != dir {
		t.Fatalf("the project is %q, want %q", cw.create.ws, dir)
	}
}

// Picking "新建 worktree" shows the branch picker; picking a branch back
// there is fed to the create flow.
func TestCreateTabNewWorktreeShowsBranch(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{Main: dir, Branches: []string{"main", "next"}}}
	cw.create.ws = dir
	cw.create.wt = createNewWorktree
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	if !tr.HasText("分支") || !tr.HasText("选择分支") {
		t.Fatalf("the branch picker is missing; texts: %v", tr.Texts())
	}
}

// The create block is centered in the pane in both directions and
// width-capped, so a wide window leaves even margins and the block does not
// sit against the top edge.
func TestCreateTabIsCentered(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.leftHidden = true
	cw.openCreateTab()
	const width, height = 1600, 1000
	tr := cw.tester(width, height, "新会话")
	head, ok := tr.Find("选择 Agent、项目与工作树，输入消息后回车创建会话。")
	if !ok {
		t.Fatalf("the create heading is missing; texts: %v", tr.Texts())
	}
	send, ok := tr.Find("发送")
	if !ok {
		t.Fatalf("the send button is missing; texts: %v", tr.Texts())
	}
	if head.W >= createWidth+8 {
		t.Fatalf("the block is %.0f wide, want at most %.0f", head.W, createWidth)
	}
	// Even side margins mean it sits in the middle horizontally.
	left, right := head.X, float32(width)-(head.X+head.W)
	if diff := left - right; diff > 80 || diff < -80 {
		t.Fatalf("side margins differ: left %.0f, right %.0f", left, right)
	}
	// And room above the heading as well as below the composer means it is
	// centered vertically rather than pinned to the top.
	above := head.Y
	below := float32(height) - (send.Y + send.H)
	if above < 120 {
		t.Fatalf("only %.0f above the block in %d of height: it is pinned to the top", above, height)
	}
	if diff := above - below; diff > 160 || diff < -160 {
		t.Fatalf("vertical margins differ: above %.0f, below %.0f", above, below)
	}
}

// A session started with a new worktree lands in the created directory.
func TestCreateSessionLandsInNewWorktree(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.draft = "hello"
	cw.create.ws = dir
	cw.create.wt = createNewWorktree
	cw.create.branch = "fresh"
	cw.create.newBranch = true
	cw.startFromCreate()
	want := defaultWorktreePath(dir, "fresh")
	cw.waitFor("the new worktree to exist", func() bool {
		if info, err := os.Stat(want); err != nil || !info.IsDir() {
			return false
		}
		return cw.cur != nil && cw.cur.Workspace == want
	})
}

// Worktree entries show only the branch, the main tree marked [主], and are
// searchable through the same dropdown the branch picker uses.
func TestCreateWorktreeEntriesAreCompact(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-entry")
	if out, err := gitRun(dir, "worktree", "add", "-b", "feat/login", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	branch := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
	cw := worktreeWindow(t, dir)
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{
		Main: dir, Worktrees: []string{dir, wt},
		TreeBranches: map[string]string{dir: branch, wt: "feat/login"},
	}}
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")

	// The trigger defaults to the main tree, so it shows "[主] <branch>";
	// opening it lists every tree.
	if err := tr.Click("[主] " + branch); err != nil {
		t.Fatalf("open the worktree dropdown: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	texts := tr.Texts()
	if !containsText(texts, "[主] "+branch) {
		t.Fatalf("the main tree entry is not compact: %v", texts)
	}
	if !containsText(texts, "feat/login") {
		t.Fatalf("the linked tree's branch is missing: %v", texts)
	}
	for _, want := range []string{"新建 worktree…"} {
		if !containsText(texts, want) {
			t.Fatalf("the dropdown lacks %q: %v", want, texts)
		}
	}
	// The directory name is not repeated on the entry, since the project is
	// already chosen above.
	for _, unwanted := range []string{"主工作树 ·", " · "} {
		if containsText(texts, unwanted) {
			t.Fatalf("the entry still shows %q: %v", unwanted, texts)
		}
	}
}

// Opening the tab with a project already set defaults its worktree to the
// main tree, so a session starts in the project itself rather than nowhere.
func TestCreateWorktreeDefaultsToMainTree(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.convs = []models.Conversation{{ID: "c", Title: "t", Workspace: dir}}
	cw.convGit = newConvGitState()
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	cw.waitFor("the main tree to be defaulted", func() bool {
		return cw.create.wt != "" && cw.create.wt != createNewWorktree
	})
	for range 4 {
		tr.Frame()
	}
	// The trigger now reads as the main tree's branch, not the placeholder.
	if !tr.HasText("工作树") || !tr.HasText("[主] main") {
		t.Fatalf("no worktree selector for a Git project; texts: %v", tr.Texts())
	}
	p := cw.convGitProject(dir)
	if p == nil {
		t.Fatal("the project summary never loaded")
	}
	if cw.create.wt != p.Main {
		t.Fatalf("the worktree is %q, want the main tree %q", cw.create.wt, p.Main)
	}
}

// containsText reports whether texts holds s exactly.
func containsText(texts []string, s string) bool {
	for _, t := range texts {
		if t == s {
			return true
		}
	}
	return false
}

// A path longer than the width keeps both ends and elides its middle; a
// short one is untouched. Counting runes keeps CJK paths from being cut
// mid-character.
func TestMiddleEllipsis(t *testing.T) {
	for _, c := range []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"short stays", "/a/b", 10, "/a/b"},
		{"exact stays", "/abc", 4, "/abc"},
		// width 11 keeps 10 characters: five from each end around the "…".
		{"odd width", "/home/user/code/project", 11, "/home" + "…" + "oject"},
		// Counted in runes, so a CJK path is never cut mid-character.
		{"cjk by rune", "/用户/代码/项目/工作树", 7, "/用户" + "…" + "工作树"},
		{"degenerate width", "/a/b/c", 1, "/a/b/c"},
	} {
		got := middleEllipsis(c.in, c.width)
		if got != c.want {
			t.Errorf("%s: middleEllipsis(%q, %d) = %q, want %q", c.name, c.in, c.width, got, c.want)
		}
		if r := []rune(got); c.width > 1 && len(r) > c.width && got != c.in {
			t.Errorf("%s: result is %d runes, over the width %d", c.name, len(r), c.width)
		}
	}
}

// The worktree entry carries the branch on the first line and the path on
// the second, with the path's middle elided when it is long.
func TestCreateWorktreeEntryShowsPath(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-path")
	if out, err := gitRun(dir, "worktree", "add", "-b", "pathy", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	branch := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
	cw := worktreeWindow(t, dir)
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{
		Main: dir, Worktrees: []string{dir, wt},
		TreeBranches: map[string]string{dir: branch, wt: "pathy"},
	}}
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	if err := tr.Click("[主] " + branch); err != nil {
		t.Fatalf("open the worktree dropdown: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	texts := tr.Texts()
	if !containsText(texts, "pathy") {
		t.Fatalf("the branch line is missing: %v", texts)
	}
	if !containsText(texts, middleEllipsis(wt, worktreePathWidth)) {
		t.Fatalf("the path line is missing %q: %v", middleEllipsis(wt, worktreePathWidth), texts)
	}
}

// Every picker's label sits on one baseline, however tall a picker grows.
// Nesting the branch picker inside the worktree column once pushed the other
// labels out of line, so the row's alignment is asserted here.
func TestCreatePickerLabelsAlign(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.leftHidden = true
	cw.convGit.info[dir] = &convGitInfo{Project: &Project{
		Main: dir, Worktrees: []string{dir},
		Branches:     []string{"main", "next"},
		TreeBranches: map[string]string{dir: "main"},
	}}
	cw.create.ws = dir
	cw.create.wt = createNewWorktree // the branch picker shows too
	cw.openCreateTab()
	tr := cw.tester(1400, 900, "新会话")

	// The nav rail also shows an "Agent" entry, so each label carries its own
	// accessible name and is matched by that.
	labels := map[string]float32{}
	for _, name := range []string{"Agent", "项目", "工作树", "分支"} {
		r, ok := tr.Find("新会话 " + name)
		if !ok {
			t.Fatalf("the %q label is missing; texts: %v", name, tr.Texts())
		}
		labels[name] = r.Y
	}
	var min, max float32 = labels["项目"], labels["项目"]
	for _, y := range labels {
		if y < min {
			min = y
		}
		if y > max {
			max = y
		}
	}
	if max-min > 2 {
		t.Fatalf("labels are not on one baseline: %v (spread %.1f)", labels, max-min)
	}
}

// The agent dropdown carries each agent's mark and its own search box.
func TestCreateAgentPickerIsSearchableWithIcons(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.cfg.Agents = []config.AgentDef{{Name: "Helper"}}
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "PiAgent", Command: "pi"}}
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	if err := tr.Click(chatAgentName); err != nil {
		t.Fatalf("open the agent dropdown: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	for _, want := range []string{"Helper", "PiAgent", "搜索 Agent"} {
		if !containsText(tr.Texts(), want) {
			t.Fatalf("the agent dropdown lacks %q: %v", want, tr.Texts())
		}
	}
}

// The model picker in the composer is the same searchable component, so it
// has a search box the old bespoke selector lacked.
func TestCreateModelPickerIsSearchable(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	if err := tr.Click("OpenAI · gpt-4"); err != nil {
		t.Fatalf("open the model dropdown: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	if !containsText(tr.Texts(), "搜索模型") {
		t.Fatalf("the model picker is not the searchable component: %v", tr.Texts())
	}
}
