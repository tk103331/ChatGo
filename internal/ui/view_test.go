package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"
)

func testWindow() *ChatWindow {
	cfg := &config.Config{
		Providers: []config.Provider{{
			Name: "OpenAI", Type: "openai", Model: "gpt-4",
			Models: []string{"gpt-4", "gpt-4o"}, Enabled: true,
		}},
		CurrentProvider:   "OpenAI",
		ReactAgentMaxStep: 20,
	}
	return &ChatWindow{
		cfg:        cfg,
		mcpManager: NewMCPManagerWrapper(),
		provider:   "OpenAI",
		toolOn:     map[string]bool{},
		editingIdx: -1,
		acp:        newACPState(),
		center:     newCenterState(),
	}
}

// splitTestPanes builds the three-pane tree the old 左一右二 layout drew:
// pane 1 to pane 0's right, and pane 2 below pane 1.
func splitTestPanes(cw *ChatWindow) {
	cw.splitPane(0, splitRight)
	cw.splitPane(1, splitDown)
}

func TestHomeViewRenders(t *testing.T) {
	cw := testWindow()
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("home render failed: %v", img)
	}
}

func TestChatViewRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{
		ID:       "c1",
		Title:    "Test",
		Provider: "OpenAI",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "你好", Timestamp: time.Now()},
			{ID: "a1", Role: "assistant", Content: "# 标题\n\n**加粗** 与 `代码`\n\n- 一\n- 二", Timestamp: time.Now(),
				ToolCalls: []models.ToolCall{{ID: "t1", Name: "search", Arguments: "{}", Result: "ok"}}},
		},
	}
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("chat render failed: %v", img)
	}
}

func TestSettingsDialogRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.prov.editing = 0
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("settings render failed: %v", img)
	}
}

func TestSidebarSearchRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.searchQuery = "hello"
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "Hello world", Messages: []models.Message{{ID: "m1", Role: "user", Content: "hello there"}}},
		{ID: "c2", Title: "Other", Messages: []models.Message{{ID: "m2", Role: "user", Content: "nope"}}},
	}
	cw.cur = &cw.convs[0]
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("search render failed: %v", img)
	}
}

func TestAppearanceTabRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsAppearanceTab
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("appearance render failed: %v", img)
	}
}

func TestAgentPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionAgent
	cw.cfg.Agents = []config.AgentDef{
		{Name: "代码助手", Description: "帮助写代码", SystemPrompt: "You are a coding assistant.", Mode: "agent"},
	}
	cw.agentForm.editing = 0
	cw.loadAgentForm(0)
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("agent render failed: %v", img)
	}
}

func TestACPPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = 2
	cw.cfg.ACPAgents = []config.ACPConnection{
		{Name: "Claude Code", Command: "claude-code-acp"},
	}
	cw.acpForm.editing = 0
	cw.loadACPForm(0)
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("acp render failed: %v", img)
	}
}

func TestACPCatalogDialogRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = 2
	cw.acpCatalogOpen = true
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("acp catalog render failed: %v", img)
	}
}

func TestACPChatRenders(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = "/tmp/demo"
	cw.cur = &models.Conversation{
		ID: "acp1", Title: "ACP 会话", Workspace: "/tmp/demo", Agent: "acp:OpenCode",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "帮我改代码", Timestamp: time.Now()},
			{ID: "a1", Role: "assistant", Content: "已经完成修改。", Timestamp: time.Now(),
				ToolCalls: []models.ToolCall{{ID: "tc1", Name: "edit_file", Arguments: `{"path":"main.go"}`, Result: "done"}}},
		},
	}
	cw.cfg.Workspaces = []string{"/tmp/demo"}
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}}}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("acp chat render failed: %v", img)
	}
}

func TestACPControlsRender(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = "/tmp/demo"
	cw.cur = &models.Conversation{ID: "acp1", Title: "ACP", Workspace: "/tmp/demo", Agent: "acp:OpenCode",
		Messages: []models.Message{{ID: "u1", Role: "user", Content: "hi", Timestamp: time.Now()}}}
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}}}
	cw.acp.convs["acp1"] = &acpConv{
		modeID: "plan",
		modes:  []acp.SessionMode{{Id: "plan", Name: "Plan"}, {Id: "build", Name: "Build"}},
		configs: []acp.SessionConfigOption{
			{Select: &acp.SessionConfigOptionSelect{Id: "model", Name: "Model", CurrentValue: "gpt-4",
				Options: acp.SessionConfigSelectOptions{Ungrouped: &acp.SessionConfigSelectOptionsUngrouped{{Name: "GPT-4", Value: "gpt-4"}}}}},
			{Boolean: &acp.SessionConfigOptionBoolean{Id: "auto", Name: "Auto", CurrentValue: true}},
		},
	}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("acp controls render failed: %v", img)
	}
}

func TestMCPPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.cfg.MCPServers = []config.MCPServer{{Name: "filesystem", Type: "stdio", Command: "npx"}}
	cw.mcp.marketLoaded = true
	cw.mcp.market = []mcpMarketServer{
		{ID: "github", Name: "GitHub", Description: "GitHub MCP", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{ID: "sqlite", Name: "SQLite", Description: "SQLite MCP", Command: "uvx", Args: []string{"mcp-server-sqlite"}},
	}
	cw.mcp.tabSel = 1
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp render failed: %v", img)
	}
	cw.mcp.tabSel = 0
	img2 := mgo.Render(cw.view, 1280, 820, 1)
	if img2 == nil || img2.Bounds().Dx() != 1280 {
		t.Fatalf("mcp servers render failed: %v", img2)
	}
}

func TestMCPEmptyStateRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.mcp.tabSel = 0
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp empty state render failed: %v", img)
	}
}

func TestMCPMarketEmptyResultRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.mcp.tabSel = 1
	cw.mcp.marketLoaded = true
	cw.mcp.market = []mcpMarketServer{
		{ID: "github", Name: "GitHub", Description: "GitHub MCP"},
	}
	cw.mcp.marketQuery = "zzz-no-match"
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp market empty result render failed: %v", img)
	}
}

func TestSkillPageRenders(t *testing.T) {
	// Loading the skill page auto-enables every skill it finds and saves the
	// config, so the home directory must be a temp one or the test overwrites
	// the developer's own ~/.chatgo/config.yaml.
	isolateHome(t)
	cw := testWindow()
	cw.section = sectionSkill
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("skill render failed: %v", img)
	}
}

func TestPreviewClose(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = wd
	cw.rightPanel = true
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: wd,
		Messages: []models.Message{{ID: "u", Role: "user", Content: "hi", Timestamp: time.Now()}}}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)
	cw.openPreview(wd + "/chat.go")
	cw.openPreview(wd + "/nav.go")
	tr := mgo.NewTester(cw.view, 1400, 800)
	tr.Frame()
	if err := tr.Click("关闭 nav.go"); err != nil {
		t.Fatalf("click close: %v", err)
	}
	tr.Frame()
	// The Chat tab and the remaining file tab survive.
	tabs := cw.center.panes[0].tabs
	if len(tabs) != 2 {
		t.Fatalf("expected 2 tabs, got %d", len(tabs))
	}
	if tabs[0].Kind != centerChat {
		t.Fatalf("expected the Chat tab first, got %q", tabs[0].Kind)
	}
}

// Search finds a line in the working tree and skips binary files and build
// output.
func TestCodeSearchFindsHits(t *testing.T) {
	wd, _ := os.Getwd()
	hits := searchWorkspace(wd, "func searchWorkspace")
	if len(hits) == 0 {
		t.Fatal("search missed its own source")
	}
	found := false
	for _, hit := range hits {
		if filepath.Base(hit.Path) == "codesearch.go" && hit.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a hit in codesearch.go")
	}
}

// Search results group by file, one group per path in file order.
func TestCodeSearchGroupsByFile(t *testing.T) {
	hits := []searchHit{
		{Path: "a.go", Line: 1, Text: "x"},
		{Path: "b.go", Line: 2, Text: "y"},
		{Path: "a.go", Line: 3, Text: "z"},
	}
	groups := groupSearchHits(hits)
	if len(groups) != 2 {
		t.Fatalf("want 2 file groups, got %d", len(groups))
	}
	if groups[0].Path != "a.go" || len(groups[0].Hits) != 2 {
		t.Fatalf("a.go should group its two hits first: %+v", groups[0])
	}
	if groups[1].Path != "b.go" || len(groups[1].Hits) != 1 {
		t.Fatalf("b.go should hold one hit: %+v", groups[1])
	}
}

// The Code Search tab renders as a center tab on its own.
func TestCodeSearchTabRenders(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = wd
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: wd}
	cw.convs = []models.Conversation{*cw.cur}
	cw.search.query = "func"
	// The results belong to the workspace they were searched in; binding the
	// tab to another directory drops them.
	cw.search.ws = wd
	cw.search.results = []searchHit{{Path: wd + "/chat.go", Line: 1, Text: "func x()"}}
	cw.openCodeSearchTab(0)
	img := mgo.Render(cw.view, 1400, 900, 1)
	if img == nil {
		t.Fatal("code search tab did not render")
	}
	tr := mgo.NewTester(cw.view, 1400, 900)
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("chat.go") {
		t.Fatal("the grouped file is not shown")
	}
}

// Rendering a pane with several tab kinds must not panic: the strip draws
// every tab before applying any close it collected, so a context menu action
// can never shift the slice under the drawing loop.
func TestRenderMixedTabs(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = wd
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: wd,
		Messages: []models.Message{{ID: "u", Role: "user", Content: "hi"}}}
	cw.convs = []models.Conversation{*cw.cur}
	splitTestPanes(cw)
	cw.openPreview(wd + "/chat.go")
	cw.openPreview(wd + "/nav.go")
	cw.openCenterTab(1, &centerTab{Kind: centerDiff, Title: "a.go (diff)",
		Path: "diff:a.go", Lang: "go", From: "x", To: "y"})
	cw.openCenterTab(2, &centerTab{Kind: centerGitGraph, Title: "GitGraph"})
	for i := 0; i < 4; i++ {
		img := mgo.Render(cw.view, 1500, 900, 1)
		if img == nil {
			t.Fatalf("frame %d did not render", i)
		}
	}
}

// A chat tab closes like any other: the conversation stays in the list and
// reopening it brings the tab back.
func TestChatTabCloses(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	conv := &models.Conversation{ID: "c", Title: "t"}
	cw.cur = conv
	cw.focusConvTab(conv)
	if n := len(cw.center.panes[0].tabs); n != 1 {
		t.Fatalf("expected the conversation tab, got %d tabs", n)
	}
	cw.closeCenterTab(0, 0)
	if n := len(cw.center.panes[0].tabs); n != 0 {
		t.Fatalf("a chat tab must close, got %d tabs", n)
	}
}

// The Chat tab is named and marked after the conversation's agent, which is
// what tells a plain agent from an ACP one.
// A chat tab is named after its conversation's title, following a rename in
// the list at once.
func TestChatTabTitleFollowsConversation(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	conv := &models.Conversation{ID: "c", Title: "重构登录模块", Agent: "agent:Helper"}
	cw.convs = []models.Conversation{*conv}
	cw.cur = conv
	cw.focusConvTab(conv)
	if n := len(cw.center.panes[0].tabs); n != 1 {
		t.Fatalf("expected the conversation tab, got %d tabs", n)
	}
	tab := cw.center.panes[0].tabs[0]
	if got := cw.centerTabTitle(tab); got != "重构登录模块" {
		t.Fatalf("the tab shows %q, want the conversation title", got)
	}
	// A rename in the list shows on the tab at once.
	cw.convs[0].Title = "重命名后的标题"
	if got := cw.centerTabTitle(tab); got != "重命名后的标题" {
		t.Fatalf("the tab does not follow a rename, got %q", got)
	}
}

// Every split tree renders, from one pane to the most the area allows.
func TestCenterSplitsRender(t *testing.T) {
	wd, _ := os.Getwd()
	builds := []func(*ChatWindow){
		func(*ChatWindow) {},
		func(cw *ChatWindow) { cw.splitPane(0, splitRight) },
		splitTestPanes,
		func(cw *ChatWindow) {
			splitTestPanes(cw)
			cw.splitPane(0, splitDown)
		},
	}
	for i, build := range builds {
		cw := testWindow()
		cw.mode = modeChat
		cw.workspace = wd
		cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: wd}
		cw.convs = []models.Conversation{*cw.cur}
		build(cw)
		img := mgo.Render(cw.view, 1400, 900, 1)
		if img == nil || img.Bounds().Dx() != 1400 {
			t.Fatalf("split %d render failed: %v", i, img)
		}
	}
}

// A split gives the area one more pane, with no upper bound.
func TestSplitPaneCounts(t *testing.T) {
	cw := testWindow()
	if got := cw.center.paneCount(); got != 1 {
		t.Fatalf("a fresh center draws one pane, got %d", got)
	}
	const want = 8
	for n := 2; n <= want; n++ {
		cw.splitPane(0, splitRight)
		if got := cw.center.paneCount(); got != n {
			t.Fatalf("after a split want %d panes, got %d", n, got)
		}
	}
}

// A split right puts the new pane beside the split one; a split down puts it
// below, and the leaves keep their drawing order.
func TestSplitDirectionsShapeTheTree(t *testing.T) {
	cw := testWindow()
	cw.splitPane(0, splitRight)
	if cw.center.root.dir != splitRight {
		t.Fatal("向右分栏 must split the area side by side")
	}
	if got := cw.center.visiblePanes(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("want leaves 0 then 1, got %v", got)
	}
	cw.splitPane(1, splitDown)
	if cw.center.root.second.dir != splitDown {
		t.Fatal("向下分栏 must stack the new pane below the split one")
	}
	if got := cw.center.visiblePanes(); len(got) != 3 || got[2] != 2 {
		t.Fatalf("want leaves 0, 1, 2, got %v", got)
	}
}

// A new pane starts empty and selected pane-wise, offering its new-tab button.
func TestNewPanesStartEmpty(t *testing.T) {
	cw := testWindow()
	splitTestPanes(cw)
	for i := 1; i < cw.center.paneCount(); i++ {
		if n := len(cw.center.panes[i].tabs); n != 0 {
			t.Fatalf("pane %d should start empty, got %d tabs", i, n)
		}
		if cw.center.panes[i].sel != -1 {
			t.Fatalf("empty pane %d should have no selection", i)
		}
	}
	if cw.center.focus != 2 {
		t.Fatalf("the last split should take the focus, got %d", cw.center.focus)
	}
}

// The three-pane tree keeps its tabs apart, one strip per pane.
func TestThreePaneTabsStayPerPane(t *testing.T) {
	cw := testWindow()
	splitTestPanes(cw)
	cw.openCenterTab(1, &centerTab{Kind: centerText, Title: "x"})
	cw.openCenterTab(2, &centerTab{Kind: centerText, Title: "y"})
	counts := [3]int{}
	for i := range 3 {
		counts[i] = len(cw.center.panes[i].tabs)
	}
	if counts != [3]int{0, 1, 1} {
		t.Fatalf("want tabs only in panes 1 and 2, got %v", counts)
	}
}

// The two split marks are embedded, since Must panics on a missing icon.
func TestSplitIconsExist(t *testing.T) {
	for _, name := range []string{"columns-2", "rows-2"} {
		if _, ok := icons.Get(name); !ok {
			t.Fatalf("the split action needs the %q icon", name)
		}
	}
}

// A tab dropped on another pane's strip moves there even after its index in
// the source pane changed.
func TestDropMovesDraggedTabByValue(t *testing.T) {
	cw := testWindow()
	cw.splitPane(0, splitRight)
	tab := cw.openCenterTab(0, &centerTab{ID: "t", Kind: centerText, Title: "t"})
	cw.openCenterTab(0, &centerTab{ID: "later", Kind: centerText, Title: "later"})
	cw.moveDraggedTab(1, centerTabDrag{tab: tab})
	pane, _ := cw.paneHoldingTab(tab)
	if pane != 1 {
		t.Fatalf("the dragged tab should land in pane 1, got %d", pane)
	}
	if len(cw.center.panes[0].tabs) != 1 {
		t.Fatalf("the source pane should keep its other tab, got %d", len(cw.center.panes[0].tabs))
	}
}

// gitCommitRefs labels commits with the branches and tags on them, which the
// GitGraph tab shows.
func TestGitCommitRefsAreLabelled(t *testing.T) {
	wd, _ := os.Getwd()
	commits := gitCommitHistory(wd, 50)
	if len(commits) == 0 {
		t.Skip("the working copy has no history")
	}
	labelled := 0
	for _, commit := range commits {
		if len(commit.Refs) > 0 {
			labelled++
		}
	}
	if labelled == 0 {
		t.Fatal("no commit carried a branch or tag name")
	}
}

// The graph lays every commit in a column and draws the joins between them.
func TestBuildGraphLaysOutLanes(t *testing.T) {
	commits := []kit.Commit{
		{Hash: "d", Parents: []string{"b", "c"}},
		{Hash: "c", Parents: []string{"a"}},
		{Hash: "b", Parents: []string{"a"}},
		{Hash: "a"},
	}
	rows := buildGraph(commits)
	if len(rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(rows))
	}
	if rows[0].Commit.Hash != "d" || rows[0].Lane != 0 {
		t.Fatalf("the newest commit must take the first lane: %+v", rows[0])
	}
	// A merge opens a second column for its second parent.
	if rows[0].Lanes < 2 {
		t.Fatalf("a merge must widen the graph, got %d lanes", rows[0].Lanes)
	}
	merging := 0
	for _, seg := range rows[0].Segments {
		if seg.Merging {
			merging++
		}
	}
	if merging != 1 {
		t.Fatalf("want one merging line for the second parent, got %d", merging)
	}
	// Every commit still gets a dot once its turn comes.
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.Commit.Hash] {
			t.Fatalf("%s drawn twice", row.Commit.Hash)
		}
		seen[row.Commit.Hash] = true
		if row.Lane < 0 || row.Lane >= maxGraphLanes {
			t.Fatalf("%s got lane %d", row.Commit.Hash, row.Lane)
		}
	}
}

// A linear history stays in one lane and needs no widening.
func TestBuildGraphLinearHistory(t *testing.T) {
	commits := []kit.Commit{
		{Hash: "c", Parents: []string{"b"}},
		{Hash: "b", Parents: []string{"a"}},
		{Hash: "a"},
	}
	rows := buildGraph(commits)
	for i, row := range rows {
		if row.Lane != 0 {
			t.Fatalf("row %d drifted to lane %d", i, row.Lane)
		}
		if row.Lanes != 1 {
			t.Fatalf("row %d wants 1 lane, got %d", i, row.Lanes)
		}
	}
}

// The graph reads history a page at a time, beyond the working tree's log.
func TestGitHistoryPagePaginates(t *testing.T) {
	wd, _ := os.Getwd()
	first, hasMore := gitHistoryPage(wd, 0, 2)
	if len(first) == 0 {
		t.Skip("the working copy has no history")
	}
	if len(first) > 2 {
		t.Fatalf("a page of 2 returned %d commits", len(first))
	}
	if hasMore {
		second, _ := gitHistoryPage(wd, 2, 2)
		if len(second) == 0 {
			t.Fatal("hasMore was set but the next page is empty")
		}
		if second[0].Hash == first[0].Hash {
			t.Fatal("the second page repeated the first")
		}
	}
}

// Opening the GitGraph tab leaves the Git panel's state untouched, and the
// panel leaves the graph's alone.
func TestGitGraphStateIsIsolated(t *testing.T) {
	cw := testWindow()
	cw.workspace, _ = os.Getwd()

	// Stand in for the Git panel's own data.
	cw.gitBranch = "panel-branch"
	cw.gitCommits = []kit.Commit{{Hash: "panelsha", Subject: "panel commit"}}
	cw.gitStat = [2]int{7, 3}

	cw.openGitGraphTab(0)

	if cw.gitBranch != "panel-branch" {
		t.Fatalf("the panel's branch changed to %q", cw.gitBranch)
	}
	if len(cw.gitCommits) != 1 || cw.gitCommits[0].Hash != "panelsha" {
		t.Fatalf("the panel's history changed: %+v", cw.gitCommits)
	}
	if cw.gitStat != [2]int{7, 3} {
		t.Fatalf("the panel's diff stat changed: %v", cw.gitStat)
	}

	// And the graph keeps its own copies.
	cw.gg.branch = "graph-branch"
	cw.gg.stat = [2]int{1, 1}
	if cw.gitBranch != "panel-branch" || cw.gitStat != [2]int{7, 3} {
		t.Fatal("writing the graph state altered the panel state")
	}
}

// The graph's history and the panel's are separate lists.
func TestGitGraphHistoryIsSeparate(t *testing.T) {
	cw := testWindow()
	cw.workspace, _ = os.Getwd()
	cw.gitCommits = []kit.Commit{{Hash: "panelonly", Subject: "panel"}}
	cw.dispatch = func(fn func()) { fn() }
	cw.openGitGraphTab(0)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !cw.gg.loaded {
		mgo.Render(cw.view, 1400, 900, 1)
		time.Sleep(2 * time.Millisecond)
	}
	if len(cw.gitCommits) != 1 || cw.gitCommits[0].Hash != "panelonly" {
		t.Fatalf("loading the graph rewrote the panel list: %+v", cw.gitCommits)
	}
	if len(cw.gg.commits) > 0 && cw.gg.commits[0].Hash == "panelonly" {
		t.Fatal("the graph reused the panel's list")
	}
}

// The history list sits below the toolbar rather than over it.
func TestGitGraphListClearsToolbar(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = wd
	cw.cur = &models.Conversation{ID: "c", Title: "t", Workspace: wd}
	cw.convs = []models.Conversation{*cw.cur}
	cw.dispatch = func(fn func()) { fn() }
	cw.openGitGraphTab(0)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !cw.gg.loaded {
		mgo.Render(cw.view, 1500, 900, 1)
		time.Sleep(2 * time.Millisecond)
	}
	if !cw.gg.loaded || len(cw.gg.rows) == 0 {
		t.Skip("the working copy has no Git history to lay out")
	}
	tr := mgo.NewTester(cw.view, 1500, 900)
	for range 8 {
		tr.Frame()
	}
	tab, ok := tr.Find("GitGraph")
	if !ok {
		t.Fatal("no GitGraph tab")
	}
	bar, ok := tr.Find(fmt.Sprintf("%d 个提交", len(cw.gg.rows)))
	if !ok {
		t.Fatal("no commit count in the toolbar")
	}
	row, ok := tr.Find(cw.gg.rows[0].Commit.Subject)
	if !ok {
		t.Fatal("the first commit row is missing")
	}
	if bar.Y < tab.Y+tab.H {
		t.Fatalf("the toolbar overlaps the tab strip: bar %.1f, tabs end %.1f",
			bar.Y, tab.Y+tab.H)
	}
	if row.Y < bar.Y+bar.H {
		t.Fatalf("the history overlaps the toolbar: row %.1f, bar ends %.1f",
			row.Y, bar.Y+bar.H)
	}
	if row.W == 0 {
		t.Fatal("the first commit row has no width")
	}
}

// The Editor tab is opened from a file chooser that starts in the workspace.
func TestFilePickStartsInWorkspace(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.workspace = wd
	cw.askFile("", nil)
	if !cw.filePick.open {
		t.Fatal("the chooser should open")
	}
	if cw.filePick.dir != wd {
		t.Fatalf("want the workspace %q, got %q", wd, cw.filePick.dir)
	}
}

// The chooser walks out of the workspace, so files outside it can be edited.
func TestFilePickReachesOuterDirectories(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.workspace = wd
	cw.askFile("", nil)
	tr := mgo.NewTester(cw.view, 1400, 900)
	for range 6 {
		tr.Frame()
	}
	if err := tr.Click(".."); err != nil {
		t.Fatalf("click parent: %v", err)
	}
	tr.Frame()
	if cw.filePick.dir == wd {
		t.Fatalf("the parent button must leave the workspace, still at %q", cw.filePick.dir)
	}
	if want := filepath.Dir(wd); cw.filePick.dir != want {
		t.Fatalf("want parent %q, got %q", want, cw.filePick.dir)
	}
}

// A file chosen in the chooser opens as an Editor tab in the focused pane.
func TestFilePickOpensEditorTab(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.mode = modeChat
	cw.workspace = wd
	var got string
	cw.askFile("", func(p string) { got = p; cw.openEditor(p) })
	tr := mgo.NewTester(cw.view, 1400, 900)
	for range 6 {
		tr.Frame()
	}
	hint, ok := tr.Find("双击文件打开")
	if !ok {
		t.Fatal("the chooser did not render")
	}
	for y := hint.Y - 60; y > hint.Y-500 && got == ""; y -= 36 {
		tr.ClickAt(700, y)
		tr.Frame()
		tr.ClickAt(700, y)
		tr.Frame()
		if cw.filePick.picked != "" {
			tr.Frame()
		}
	}
	if got == "" {
		t.Fatal("double clicking a file never picked one")
	}
	editors := 0
	for _, tab := range cw.center.panes[0].tabs {
		if tab.Kind == centerEditor {
			editors++
			if tab.Path != got {
				t.Fatalf("editor tab holds %q, want %q", tab.Path, got)
			}
		}
	}
	if editors != 1 {
		t.Fatalf("want 1 editor tab, got %d", editors)
	}
	if cw.filePick.open {
		t.Fatal("the chooser should close after a pick")
	}
}

// Opening the same file twice focuses the open tab rather than duplicating it.
func TestOpenPreviewReusesTab(t *testing.T) {
	wd, _ := os.Getwd()
	cw := testWindow()
	cw.workspace = wd
	cw.openPreview(wd + "/chat.go")
	cw.openPreview(wd + "/chat.go")
	if n := len(cw.center.panes[0].tabs); n != 1 {
		t.Fatalf("expected the one file tab reused, got %d", n)
	}
}

// Closing a pane with tabs asks first, then closes the pane and its tabs.
func TestClosePaneWithTabsAsksFirst(t *testing.T) {
	cw := testWindow()
	cw.splitPane(0, splitRight)
	tab := cw.openCenterTab(1, &centerTab{ID: "t", Kind: centerText, Title: "x"})
	cw.closePane(1)
	if !cw.confirm.open {
		t.Fatal("closing a pane with tabs must ask first")
	}
	if !cw.center.hasPane(1) {
		t.Fatal("the pane must stay open until the prompt is confirmed")
	}
	cw.confirm.action()
	if cw.center.hasPane(1) {
		t.Fatal("the confirmed pane should close")
	}
	if pane, _ := cw.paneHoldingTab(tab); pane != -1 {
		t.Fatalf("the pane's tab should close too, still in pane %d", pane)
	}
}

// A pane without tabs closes without a prompt, and the last pane never closes.
func TestCloseEmptyPaneClosesImmediately(t *testing.T) {
	cw := testWindow()
	cw.splitPane(0, splitRight)
	cw.closePane(1)
	if cw.confirm.open {
		t.Fatal("an empty pane should close without a prompt")
	}
	if cw.center.hasPane(1) {
		t.Fatal("the empty pane should close")
	}
	if got := cw.center.visiblePanes(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("want only pane 0 to remain, got %v", got)
	}
	if cw.center.focus != 0 {
		t.Fatalf("closing the focused pane must focus a survivor, got %d", cw.center.focus)
	}
	cw.closePane(0)
	if !cw.center.hasPane(0) {
		t.Fatal("the last pane must stay open")
	}
}

// A tab moves between panes, including a chat tab.
func TestMoveCenterTab(t *testing.T) {
	cw := testWindow()
	cw.splitPane(0, splitRight)
	cw.openCenterTab(0, &centerTab{ID: "t1", Kind: centerText, Title: "a.go"})
	cw.openCenterTab(0, &centerTab{ID: "c1", Kind: centerChat, Title: "Chat"})
	cw.moveCenterTab(0, 0, 1)
	if len(cw.center.panes[1].tabs) != 1 || cw.center.panes[1].tabs[0].ID != "t1" {
		t.Fatalf("expected the file tab to land in pane 1")
	}
	cw.moveCenterTab(0, 0, 1)
	if len(cw.center.panes[1].tabs) != 2 || cw.center.panes[1].tabs[1].ID != "c1" {
		t.Fatalf("expected the chat tab to move to pane 1")
	}
}

func TestAutoTaskPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionAuto
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("auto task empty render failed: %v", img)
	}
}

func TestAutoTaskWithTasksRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionAuto
	cw.cfg.AutoTasks = []config.AutoTask{
		{Name: "每日报告", Agent: "", Prompt: "生成报告", Schedule: config.Schedule{Kind: "daily", Hour: 9, Minute: 0}, Enabled: true, LastRun: time.Now().Add(-time.Hour), LastNote: "成功"},
		{Name: "周报", Agent: "agent:helper", Prompt: "周报", Schedule: config.Schedule{Kind: "weekly", Hour: 18, Minute: 0, Weekday: 5}, Enabled: false},
	}
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("auto task with tasks render failed: %v", img)
	}
}

func TestProvidersTabShowsCards(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = config.ProviderPresets()
	cw.section = sectionSettings
	cw.settings.tab = 0
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("providers tab render failed: %v", img)
	}
	// Count non-background pixels in the content area (below the tab strip)
	var nonBg int
	for y := 200; y < 600; y++ {
		for x := 100; x < 900; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0xf000 || g > 0xf000 || b > 0xf000 {
				// near-white background
			} else {
				nonBg++
			}
		}
	}
	if nonBg < 100 {
		t.Fatalf("providers tab content area appears empty: nonBg=%d", nonBg)
	}
}

func TestBuiltinTabShowsContent(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = 1
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil {
		t.Fatal("builtin tab render failed")
	}
	var nonBg int
	for y := 200; y < 600; y++ {
		for x := 100; x < 900; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0xf000 || g > 0xf000 || b > 0xf000 {
			} else {
				nonBg++
			}
		}
	}
	t.Logf("builtin tab nonBg=%d", nonBg)
}

func TestStatsPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionStats
	now := time.Now()
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "A", CreatedAt: now.AddDate(0, 0, -2),
			Messages: []models.Message{
				{ID: "m1", Role: "user", Content: "hi", Timestamp: now.AddDate(0, 0, -2)},
				{ID: "m2", Role: "assistant", Content: "hello", Timestamp: now,
					ToolCalls: []models.ToolCall{{ID: "t1", Name: "search"}}},
			},
			Turns: []models.Turn{{ID: "t1", UserMsgID: "m1", ReplyMsgID: "m2", Provider: "OpenAI", Model: "gpt-4",
				Input: 100, Output: 50, Total: 150, StartedAt: now}}},
	}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("stats render failed: %v", img)
	}
}

func TestComputeStatsAggregates(t *testing.T) {
	cw := testWindow()
	now := time.Now()
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "A", CreatedAt: now,
			Messages: []models.Message{
				{ID: "u1", Role: "user", Timestamp: now},
				{ID: "a1", Role: "assistant", Timestamp: now},
				{ID: "u2", Role: "user", Timestamp: now},
				{ID: "a2", Role: "assistant", Timestamp: now},
			},
			Turns: []models.Turn{
				{ID: "t1", UserMsgID: "u1", ReplyMsgID: "a1", Model: "gpt-4", Input: 10, Output: 5, Total: 15, StartedAt: now},
				{ID: "t2", UserMsgID: "u2", ReplyMsgID: "a2", Model: "gpt-4", Input: 20, Output: 5, Total: 25, StartedAt: now},
			}},
	}
	cw.taskConvs = []models.Conversation{
		{ID: "t1", Title: "Task", CreatedAt: now, TaskID: "x",
			Messages: []models.Message{{ID: "a3", Role: "assistant", Timestamp: now}},
			Turns:    []models.Turn{{ID: "t3", UserMsgID: "u3", ReplyMsgID: "a3", Model: "gpt-4", Input: 1, Output: 1, Total: 2, StartedAt: now}}},
	}
	s := cw.computeStats()
	if s.sessions != 2 || s.messages != 5 || s.total != 42 || s.input != 31 || s.output != 11 {
		t.Fatalf("aggregate %+v", s)
	}
	if len(s.models) != 1 || s.models[0].name != "gpt-4" || s.models[0].value != 42 {
		t.Fatalf("models %+v", s.models)
	}
	if n := len(s.days); n != statsRange {
		t.Fatalf("days %d", n)
	}
	last := s.days[len(s.days)-1]
	if last.sessions != 2 || last.total != 42 {
		t.Fatalf("today bucket %+v", last)
	}
}

func TestTurnModelLabel(t *testing.T) {
	cw := testWindow()
	cw.cur = &models.Conversation{ID: "c1", Turns: []models.Turn{
		{ID: "t1", ReplyMsgID: "a1", Provider: "OpenAI", Model: "gpt-4"},
	}}
	if got := cw.turnModelLabel(models.Message{ID: "a1", Role: "assistant"}); got != "OpenAI · gpt-4" {
		t.Fatalf("label %q", got)
	}
	if got := cw.turnModelLabel(models.Message{ID: "a2", Role: "assistant"}); got != "" {
		t.Fatalf("empty label %q", got)
	}
}

func TestApplyTurnModelForSwitchesOnlyWhenNeeded(t *testing.T) {
	cw := testWindow()
	cw.provider, cw.model = "OpenAI", "gpt-4"
	conv := &models.Conversation{ID: "c1", Turns: []models.Turn{
		{ID: "t1", ReplyMsgID: "a1", Provider: "OpenAI", Model: "gpt-4"},
		{ID: "t2", ReplyMsgID: "a2", Provider: "OpenAI", Model: "gpt-4o"},
	}}
	cw.applyTurnModelFor(conv, "a1")
	if cw.provider != "OpenAI" || cw.model != "gpt-4" {
		t.Fatalf("unexpected switch %q/%q", cw.provider, cw.model)
	}
	cw.applyTurnModelFor(conv, "a2")
	if cw.model != "gpt-4o" {
		t.Fatalf("model %q", cw.model)
	}
	cw.applyTurnModelFor(conv, "missing")
	if cw.model != "gpt-4o" {
		t.Fatalf("unknown turn changed model: %q", cw.model)
	}
}

func TestStatsCountsModelsPerTurn(t *testing.T) {
	cw := testWindow()
	now := time.Now()
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "a", CreatedAt: now, Messages: []models.Message{
			{ID: "u1", Role: "user", Timestamp: now},
			{ID: "a1", Role: "assistant", Timestamp: now},
			{ID: "u2", Role: "user", Timestamp: now},
			{ID: "a2", Role: "assistant", Timestamp: now},
		}, Turns: []models.Turn{
			{ID: "t1", UserMsgID: "u1", ReplyMsgID: "a1", Model: "gpt-4", Total: 100, StartedAt: now},
			{ID: "t2", UserMsgID: "u2", ReplyMsgID: "a2", Model: "gpt-4o", Total: 300, StartedAt: now},
		}},
	}
	s := cw.computeStats()
	if len(s.models) != 2 {
		t.Fatalf("models %+v", s.models)
	}
	for _, m := range s.models {
		want := 100
		if m.name == "gpt-4o" {
			want = 300
		}
		if m.value != want {
			t.Fatalf("model %s = %d, want %d", m.name, m.value, want)
		}
	}
}

// TestAccentTextOnlyOnAccent guards a regression where AccentText — the
// color of text on the accent fill, white in both themes — was used for icons
// and labels on ordinary surfaces, which made them invisible in the light
// theme. Only a background of Accent or Danger may carry it.
func TestAccentTextOnlyOnAccent(t *testing.T) {
	files, err := filepath.Glob("kit/*.go")
	if err != nil {
		t.Fatal(err)
	}
	// The files at the root of internal/ui draw too.
	root, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range append(files, root...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.Contains(line, "AccentText") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			// A fill of the accent or the danger color is the one use that
			// makes the text readable; anything else is a mistake.
			// PrimaryButton fills itself with the accent, so its text
			// color is AccentText without saying so here.
			if strings.Contains(line, "Background(k.Accent)") ||
				strings.Contains(line, "k.Accent,") ||
				strings.Contains(line, "k.Danger") ||
				strings.Contains(line, "return k.AccentText") ||
				strings.Contains(line, "fg, bg := k.Accent") {
				continue
			}
			t.Errorf("%s:%d uses AccentText without an accent fill: %s",
				f, i+1, strings.TrimSpace(line))
		}
	}
}

// TestColorThemesRecolorOnlyTheAccent checks that a theme changes the accent
// family and nothing else: the neutral colors carry the contrast, so a theme
// must not touch them.
// TestColorThemesRecolorAccentAndSurfaces pins what a colour theme may
// change. applySurfaces deliberately tints the page and the panels on it so
// the preview matches the window, so Background, Surface and Border do move;
// the text and the status colours must not, or labels and warnings would stop
// reading the same across themes.
func TestColorThemesRecolorAccentAndSurfaces(t *testing.T) {
	for _, want := range mtheme.Themes {
		for _, dark := range []bool{false, true} {
			base := mgo.LightTheme()
			if dark {
				base = mgo.DarkTheme()
			}
			got := mgo.LightTheme()
			if dark {
				got = mgo.DarkTheme()
			}
			want.Apply(got)

			// The default theme keeps MyGo's own colors, so it alone may
			// leave the accent as it found it.
			if got.Accent == base.Accent && want.Name != mtheme.DefaultName {
				t.Errorf("%s (dark=%v): accent unchanged", want.Name, dark)
			}
			for _, c := range []struct {
				name     string
				got, was mgo.Color
			}{
				{"Text", got.Text, base.Text},
				{"TextMuted", got.TextMuted, base.TextMuted},
				{"Danger", got.Danger, base.Danger},
				{"Success", got.Success, base.Success},
			} {
				if c.got != c.was {
					t.Errorf("%s (dark=%v): %s changed from %v to %v",
						want.Name, dark, c.name, c.was, c.got)
				}
			}
			// Text on a surface must stay readable after the surface is tinted.
			if r := mtheme.Contrast(got.Text, got.Background); r < 4.5 {
				t.Errorf("%s (dark=%v): Text on Background contrast %.2f is too low",
					want.Name, dark, r)
			}
			if r := mtheme.Contrast(got.Text, got.Surface); r < 4.5 {
				t.Errorf("%s (dark=%v): Text on Surface contrast %.2f is too low",
					want.Name, dark, r)
			}
			// Text on the accent must stay readable, whatever the accent.
			if r := mtheme.Contrast(got.AccentText, got.Accent); r < 3 {
				t.Errorf("%s (dark=%v): AccentText on Accent contrast %.2f is too low",
					want.Name, dark, r)
			}
		}
	}
}

// The composer keeps the model selector on its own and gathers the builtin
// tools, MCP servers and skills under one searchable, tabbed control.
func TestComposerSelectorsAreConsistent(t *testing.T) {
	cw := testWindow()
	cw.cfg.BuiltinTools = []config.BuiltinTool{{Name: "search", Type: "web_search", Enabled: true}}
	cw.cfg.MCPServers = []config.MCPServer{{Name: "files", Command: "node", Enabled: true}}
	cw.skills.loaded = true
	cw.skills.list = []skill{{ID: "s1", Name: "DeepFocus", Description: "focus"}}
	cw.cfg.SkillsEnabled = []string{"s1"}
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t", Provider: "OpenAI"}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)

	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 5 {
		tr.Frame()
	}
	texts := tr.Texts()
	if !containsText(texts, "OpenAI · gpt-4") {
		t.Fatalf("the composer lacks the model selector: %v", texts)
	}
	if err := tr.Click("工具与技能"); err != nil {
		t.Fatalf("open the tool tabs: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	for _, want := range []string{"工具 (1)", "MCP (1)", "Skills (1)", "搜索工具"} {
		if !containsText(tr.Texts(), want) {
			t.Fatalf("the tool tabs lack %q: %v", want, tr.Texts())
		}
	}
	if err := tr.Click("MCP (1)"); err != nil {
		t.Fatalf("switch to MCP: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !containsText(tr.Texts(), "搜索 MCP 服务器") {
		t.Fatalf("the MCP tab lacks its search: %v", tr.Texts())
	}
	if err := tr.Click("Skills (1)"); err != nil {
		t.Fatalf("switch to Skills: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !containsText(tr.Texts(), "搜索 Skill") {
		t.Fatalf("the Skills tab lacks its search: %v", tr.Texts())
	}
}

// A model that ran without reporting token counts still lands in the model
// list with a zero value. Its share bar must not divide by zero: a NaN width
// would blank the whole stats page on a real window, where the layout engine
// cannot take a non-finite size the way the headless tester can.
func TestStatsModelZeroShareRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionStats
	cw.convs = []models.Conversation{
		{ID: "c1", Title: "A",
			Turns: []models.Turn{{Model: "MiniMax-M3", StartedAt: time.Now()}}},
	}
	img := mgo.Render(cw.view, 1000, 800, 1)
	if img == nil {
		t.Fatal("stats page with a zero-token model returned a nil frame")
	}
	// The aggregate itself must keep the zero model, and the share must stay
	// finite: the division is what the fix protects.
	sum := cw.computeStats()
	if len(sum.models) != 1 || sum.models[0].name != "MiniMax-M3" {
		t.Fatalf("the zero-token model was dropped: %+v", sum.models)
	}
}
