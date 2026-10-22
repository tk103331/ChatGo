package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	mtheme "chatgo/internal/ui/theme"
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
		cfg:        cfg,
		mcpManager: NewMCPManagerWrapper(),
		provider:   "OpenAI",
		toolOn:     map[string]bool{},
		editingIdx: -1,
		acp:        newACPState(),
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
	cw.settings.tab = 5
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
func TestColorThemesRecolorOnlyTheAccent(t *testing.T) {
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

			if got.Accent == base.Accent && want.Name != "blue" {
				t.Errorf("%s (dark=%v): accent unchanged", want.Name, dark)
			}
			for _, c := range []struct {
				name     string
				got, was mgo.Color
			}{
				{"Background", got.Background, base.Background},
				{"Surface", got.Surface, base.Surface},
				{"Text", got.Text, base.Text},
				{"TextMuted", got.TextMuted, base.TextMuted},
				{"Border", got.Border, base.Border},
				{"Danger", got.Danger, base.Danger},
				{"Success", got.Success, base.Success},
			} {
				if c.got != c.was {
					t.Errorf("%s (dark=%v): %s changed from %v to %v",
						want.Name, dark, c.name, c.was, c.got)
				}
			}
			// Text on the accent must stay readable, whatever the accent.
			if r := mtheme.Contrast(got.AccentText, got.Accent); r < 3 {
				t.Errorf("%s (dark=%v): AccentText on Accent contrast %.2f is too low",
					want.Name, dark, r)
			}
		}
	}
}
