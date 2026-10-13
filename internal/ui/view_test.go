package ui

import (
	"os"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"
)

func testWindow() *ChatWindow {
	cfg := &config.Config{
		Providers:         []config.Provider{{Name: "OpenAI", Type: "openai", Model: "gpt-4", Enabled: true}},
		CurrentProvider:   "OpenAI",
		ReactAgentMaxStep: 20,
	}
	return &ChatWindow{
		cfg:           cfg,
		mcpManager:    NewMCPManagerWrapper(),
		provider:      "OpenAI",
		toolOn:        map[string]bool{},
		editingIdx:    -1,
		acpConvs:      map[string]*acpConv{},
		acpConvBySess: map[acp.SessionId]string{},
		acpConvObj:    map[string]*models.Conversation{},
	}
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
	cw.settings.tab = 3
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
	cw.acpConvs["acp1"] = &acpConv{
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
	cw.mcpMarketLoaded = true
	cw.mcpMarket = []mcpMarketServer{
		{ID: "github", Name: "GitHub", Description: "GitHub MCP", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{ID: "sqlite", Name: "SQLite", Description: "SQLite MCP", Command: "uvx", Args: []string{"mcp-server-sqlite"}},
	}
	cw.mcpTabSel = 1
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp render failed: %v", img)
	}
	cw.mcpTabSel = 0
	img2 := mgo.Render(cw.view, 1280, 820, 1)
	if img2 == nil || img2.Bounds().Dx() != 1280 {
		t.Fatalf("mcp servers render failed: %v", img2)
	}
}

func TestMCPEmptyStateRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.mcpTabSel = 0
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp empty state render failed: %v", img)
	}
}

func TestMCPMarketEmptyResultRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.mcpTabSel = 1
	cw.mcpMarketLoaded = true
	cw.mcpMarket = []mcpMarketServer{
		{ID: "github", Name: "GitHub", Description: "GitHub MCP"},
	}
	cw.mcpMarketQuery = "zzz-no-match"
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("mcp market empty result render failed: %v", img)
	}
}

func TestSkillPageRenders(t *testing.T) {
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
	cw.openPreview(wd + "/chat.go")
	cw.openPreview(wd + "/nav.go")
	tr := mgo.NewTester(cw.view, 1400, 800)
	tr.Frame()
	if err := tr.Click("关闭预览"); err != nil {
		t.Fatalf("click close: %v", err)
	}
	tr.Frame()
	if len(cw.previewTabs) != 1 {
		t.Fatalf("expected 1 tab, got %d", len(cw.previewTabs))
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
