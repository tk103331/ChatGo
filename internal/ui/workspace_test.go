package ui

import (
	"path/filepath"
	"testing"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// Picking a conversation of another project moves every workspace-bound
// surface at once: the file root, the Git panel, the Worktree tab and the
// GitGraph tab all point at the new directory, so none of them keeps showing
// the previous project.
func TestSwitchWorkspaceSurfacesFollowConversation(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	convB, err := cm.CreateConversation("in B", "OpenAI", "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	convB.Workspace = projB
	if err := cm.SaveConversation(convB); err != nil {
		t.Fatal(err)
	}
	cw.reloadConvs()

	cw.selectConversation(convB.ID)
	for what, got := range map[string]string{
		"workspace": cw.workspace,
		"file root": cw.fileRoot(),
		"Git panel": cw.gitFor,
		"worktree":  cw.wt.dir,
		"GitGraph":  cw.gg.dir,
	} {
		if got != projB {
			t.Fatalf("%s points at %q, want %q", what, got, projB)
		}
	}
}

// Applying a custom agent bound to its own workspace moves every
// workspace-bound surface to it, and applying a later agent without one
// leaves the project alone.
func TestApplyAgentSwitchesWorkspaceSurfaces(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cw.cfg.Agents = []config.AgentDef{
		{Name: "b", Workspace: projB},
		{Name: "loose"},
	}

	cw.applyAgent("b")
	for what, got := range map[string]string{
		"workspace": cw.workspace,
		"file root": cw.fileRoot(),
		"Git panel": cw.gitFor,
		"worktree":  cw.wt.dir,
		"GitGraph":  cw.gg.dir,
	} {
		if got != projB {
			t.Fatalf("%s points at %q, want the agent's %q", what, got, projB)
		}
	}

	cw.applyAgent("loose")
	if cw.workspace != projB {
		t.Fatalf("an agent without a workspace moved the workspace to %q, want %q kept", cw.workspace, projB)
	}
	if cw.wt.dir != projB || cw.gg.dir != projB || cw.gitFor != projB {
		t.Fatalf("an agent without a workspace disturbed the surfaces: wt=%q gg=%q git=%q",
			cw.wt.dir, cw.gg.dir, cw.gitFor)
	}
}

// The built-in Chat's saved workspace moves the surfaces too, on the same
// path a custom agent's does.
func TestBuiltinChatWorkspaceSwitchesSurfaces(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cw.cfg.Chat.Workspace = projB

	cw.applyAgent("")
	if cw.workspace != projB {
		t.Fatalf("workspace is %q, want the Chat's %q", cw.workspace, projB)
	}
	if cw.gitFor != projB || cw.wt.dir != projB || cw.gg.dir != projB {
		t.Fatalf("the Git panel, Worktree and GitGraph did not follow: git=%q wt=%q gg=%q",
			cw.gitFor, cw.wt.dir, cw.gg.dir)
	}
}

// Starting a conversation in another project refreshes the Git panel even
// though the window has one already, so the new session never shows the
// previous project's changes.
func TestNewConversationRefreshesGitPanel(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm

	cw.newConversationIn(projB, "")
	if cw.workspace != projB {
		t.Fatalf("workspace is %q, want %q", cw.workspace, projB)
	}
	if cw.gitFor != projB || cw.wt.dir != projB || cw.gg.dir != projB {
		t.Fatalf("the surfaces did not follow the new conversation: git=%q wt=%q gg=%q",
			cw.gitFor, cw.wt.dir, cw.gg.dir)
	}
}

// The center area's tabs follow the working directory: each workspace keeps
// its own tabs, and moving to another project's conversation points the
// center at that directory's tab set while the previous one is kept for
// coming back.
func TestCenterTabsFollowWorkspace(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	convA := workspaceConv(t, cm, "in A", projA)
	convB := workspaceConv(t, cm, "in B", projB)
	cw.reloadConvs()
	keyA, keyB := centerStateKey(projA), centerStateKey(projB)

	cw.selectConversation(convA.ID)
	cw.openPreview(filepath.Join(projA, "a.txt"))
	if cw.center != cw.centerStates[keyA] {
		t.Fatal("the center is not A's tab set while A is active")
	}
	if !containsStr(tabsKinds(cw.centerStates[keyA]), "text") {
		t.Fatal("A's preview did not land in A's tab set")
	}

	// A conversation of B points the center at B's own tab set.
	cw.selectConversation(convB.ID)
	if cw.workspace != projB {
		t.Fatalf("workspace is %q, want %q", cw.workspace, projB)
	}
	if cw.center != cw.centerStates[keyB] {
		t.Fatal("the center did not follow the workspace to B's tab set")
	}
	if containsStr(tabsKinds(cw.centerStates[keyB]), "text") {
		t.Fatal("B's tab set inherited A's preview")
	}

	// Coming back to A restores its tabs.
	cw.selectConversation(convA.ID)
	if cw.center != cw.centerStates[keyA] {
		t.Fatal("the center did not return to A's tab set")
	}
	if !containsStr(tabsKinds(cw.centerStates[keyA]), "text") {
		t.Fatal("A's preview tab was not restored")
	}
}

func workspaceConv(t *testing.T, cm *models.ConversationManager, title, ws string) *models.Conversation {
	t.Helper()
	conv, err := cm.CreateConversation(title, "OpenAI", "gpt-4")
	if err != nil {
		t.Fatal(err)
	}
	conv.Workspace = ws
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

// The Code Search tab is bound to the workspace its results were found in:
// opening it after moving to another project discards the previous project's
// query and hits instead of listing them.
func TestCodeSearchFollowsWorkspace(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := testWindow()
	cw.workspace = projA
	cw.search = codeSearchState{
		ws: projA, query: "needle",
		results: []searchHit{{Path: filepath.Join(projA, "a.txt"), Line: 1, Text: "needle"}},
	}

	cw.workspace = projB
	cw.resetCodeSearch(projB)
	if cw.search.ws != projB {
		t.Fatalf("the search is bound to %q, want %q", cw.search.ws, projB)
	}
	if len(cw.search.results) != 0 || cw.search.query != "" {
		t.Fatalf("the previous workspace's matches survived: results=%d query=%q",
			len(cw.search.results), cw.search.query)
	}

	// The same workspace keeps its results.
	cw.resetCodeSearch(projB)
	if cw.search.ws != projB {
		t.Fatal("a same-workspace reset should leave the tab alone")
	}
}

// Opening a terminal for another working tree moves the workspace to it, so
// the terminal and the rest of the center area always share one directory.
func TestBringWorkspaceMovesCenterForCrossTreeTerminal(t *testing.T) {
	dir := newTestRepo(t)
	a := filepath.Join(filepath.Dir(dir), "wt-term-a")
	if out, err := gitRun(dir, "worktree", "add", "-b", "ta", a); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)

	// A terminal for the tree that already is the workspace stays put.
	cw.bringWorkspace(dir)
	if cw.workspace != dir {
		t.Fatalf("bringWorkspace moved off the active tree to %q", cw.workspace)
	}

	// A terminal for another tree moves the workspace, and with it the
	// center tabs and git surfaces.
	cw.bringWorkspace(a)
	if cw.workspace != a {
		t.Fatalf("bringWorkspace left the workspace at %q, want the tree %q", cw.workspace, a)
	}
	if cw.center != cw.centerStates[centerStateKey(a)] {
		t.Fatal("the center did not follow the tree the terminal was opened for")
	}
}

// Opening a conversation of the workspace the center already shows adds its
// chat tab next to the open ones, so switching between sessions of one
// project keeps the files and tools of that project at hand.
func TestSelectConversationSameWorkspaceKeepsTabs(t *testing.T) {
	proj := newTestRepo(t)
	cw := worktreeWindow(t, proj)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	convA := workspaceConv(t, cm, "in A", proj)
	convB := workspaceConv(t, cm, "in B", proj)
	cw.reloadConvs()

	cw.selectConversation(convA.ID)
	cw.selectConversation(convB.ID)
	if !containsStr(tabsKinds(cw.centerStates[centerStateKey(proj)]), "chat") {
		t.Fatal("the project lost its tab set")
	}
	if got := cw.center.activeTab(); got == nil || got.ConvID != convB.ID {
		t.Fatalf("the center does not show the session just opened: %+v", got)
	}
}

// Opening a conversation of another project switches to that project's tab
// set; the previous project's tabs stay with it instead of being carried over.
func TestSelectConversationOtherWorkspaceSwitchesTabSets(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	convA := workspaceConv(t, cm, "in A", projA)
	convB := workspaceConv(t, cm, "in B", projB)
	cw.reloadConvs()

	cw.selectConversation(convA.ID)
	cw.openPreview(filepath.Join(projA, "a.txt"))
	cw.selectConversation(convB.ID)
	if got := tabsKinds(cw.center); len(got) != 1 || got[0] != "chat" {
		t.Fatalf("the previous project's tabs survived the switch: %v", got)
	}
	if ids := chatTabIDs(cw.center); len(ids) != 1 || ids[0] != convB.ID {
		t.Fatalf("the center does not show only the new project's session: %v", ids)
	}
	// A's tab set is still there for coming back to A.
	if !containsStr(tabsKinds(cw.centerStates[centerStateKey(projA)]), "text") {
		t.Fatal("A's tabs were closed while A was not the active project")
	}
}

// Conversations of one working tree share that tree's tab group across
// visits, and each tree keeps a group of its own.
func TestWorktreeChatTabsStayPerWorktree(t *testing.T) {
	dir := newTestRepo(t)
	wt := filepath.Join(filepath.Dir(dir), "wt-chat-tabs")
	if out, err := gitRun(dir, "worktree", "add", "-b", "chattabs", wt); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	cw := worktreeWindow(t, dir)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	mainConv := workspaceConv(t, cm, "main", dir)
	linkedA := workspaceConv(t, cm, "linked A", wt)
	linkedB := workspaceConv(t, cm, "linked B", wt)
	cw.reloadConvs()

	cw.selectConversation(linkedA.ID)
	cw.selectConversation(mainConv.ID)
	cw.selectConversation(linkedB.ID)
	if cw.workspace != wt {
		t.Fatalf("workspace is %q, want the linked tree %q", cw.workspace, wt)
	}
	if ids := chatTabIDs(cw.center); len(ids) != 2 || ids[0] != linkedA.ID || ids[1] != linkedB.ID {
		t.Fatalf("the linked tree's sessions do not share one tab group: %v", ids)
	}
	if ids := chatTabIDs(cw.centerStates[centerStateKey(dir)]); len(ids) != 1 || ids[0] != mainConv.ID {
		t.Fatalf("the main tree's tab group picked up another tree's sessions: %v", ids)
	}
}

// chatTabIDs lists the conversation IDs of the center's chat tabs, in order.
func chatTabIDs(s *centerState) []string {
	var out []string
	for _, pi := range s.visiblePanes() {
		for _, t := range s.panes[pi].tabs {
			if t.Kind == centerChat {
				out = append(out, t.ConvID)
			}
		}
	}
	return out
}

// The "新会话" tab opened for a project's header moves the window to that
// project, so the center never lists another project's tabs beside the
// session about to be started.
func TestCreateTabForProjectMovesWorkspace(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cw.openPreview(filepath.Join(projA, "a.txt"))

	cw.openCreateTabIn(projB)
	if cw.workspace != projB {
		t.Fatalf("workspace is %q, want the tab's project %q", cw.workspace, projB)
	}
	if kinds := tabsKinds(cw.center); len(kinds) != 1 || kinds[0] != "create" {
		t.Fatalf("the create tab must sit alone in the new project, got %v", kinds)
	}
	if !containsStr(tabsKinds(cw.centerStates[centerStateKey(projA)]), "text") {
		t.Fatal("the previous project lost its tabs")
	}
}

// The right panel's Git tab describes the selected session's directory, so a
// session of another project never shows the window's own repository beside
// it.
func TestGitPanelFollowsSelectedConversation(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: projB}
	if got := cw.selectedDir(); got != projB {
		t.Fatalf("the panel points at %q, want the session's %q", got, projB)
	}
	// Without a session, the window's own workspace is described.
	cw.cur = nil
	if got := cw.selectedDir(); got != projA {
		t.Fatalf("the panel points at %q, want the workspace %q", got, projA)
	}
	// A session saved without a workspace does not blank the panel.
	cw.cur = &models.Conversation{ID: "c2", Title: "t2"}
	if got := cw.selectedDir(); got != projA {
		t.Fatalf("the panel points at %q, want the workspace %q", got, projA)
	}
}

// A workspace switch always leaves the Git panel marked as describing the new
// directory, never the one it was showing before.
func TestPointAtWorkspaceRepointsGitPanel(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	cw.gitFor = projA

	cw.pointAtWorkspace(projB)
	if cw.gitFor != projB {
		t.Fatalf("the Git panel still points at %q, want %q", cw.gitFor, projB)
	}
	// Pointing again at the directory already shown changes nothing.
	cw.pointAtWorkspace(projB)
	if cw.gitFor != projB {
		t.Fatalf("the Git panel moved to %q", cw.gitFor)
	}
}

// Opening a conversation restores its agent without letting that agent's own
// workspace move the window: the conversation's directory decides which tab
// set the session lands in, or a Chat session of another project would be
// added to the tabs of the project the built-in Chat last stood in.
func TestSelectConversationKeepsItsOwnWorkspaceOverAgent(t *testing.T) {
	projA := newTestRepo(t)
	projB := newTestRepo(t)
	cw := worktreeWindow(t, projA)
	// The built-in Chat is bound to another project, as it is after the user
	// worked there.
	cw.cfg.Chat.Workspace = projB
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	convA := workspaceConv(t, cm, "in A", projA)
	cw.reloadConvs()

	cw.selectConversation(convA.ID)
	if cw.workspace != projA {
		t.Fatalf("workspace is %q, want the session's own %q", cw.workspace, projA)
	}
	if got := cw.centerStates[centerStateKey(projA)]; got != cw.center {
		t.Fatal("the session did not land in its own project's tab set")
	}
	if ids := chatTabIDs(cw.center); len(ids) != 1 || ids[0] != convA.ID {
		t.Fatalf("the center shows %v, want only the opened session", ids)
	}
}
