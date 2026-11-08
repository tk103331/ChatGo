package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/capture"
	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"
)

func TestQuickTaskSummary(t *testing.T) {
	if got := quickTaskSummary("  a\n b  "); got != "a b" {
		t.Errorf("summary = %q", got)
	}
	long := strings.Repeat("字", 60)
	got := quickTaskSummary(long)
	if r := []rune(got); len(r) != 49 || !strings.HasSuffix(got, "…") {
		t.Errorf("summary len = %d, %q", len(r), got)
	}
}

func TestQuickPanelAgents(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "Review"}, {Name: "Code"}}
	cw.cfg.ACPAgents = []config.ACPConnection{
		{Name: "Claude Code"},
		{Name: "Off", Disabled: true},
	}
	qp := newQuickPanel(cw)

	refs := func() []string {
		var out []string
		for _, a := range qp.agents() {
			out = append(out, a.Ref)
		}
		return out
	}

	// Without favorites: the built-in chat first, then every enabled agent.
	want := []string{"", "agent:Review", "agent:Code", "acp:Claude Code"}
	if got := refs(); !equalStrings(got, want) {
		t.Errorf("agents = %v, want %v", got, want)
	}

	// Favorites order the chips, and the default agent always comes first.
	cw.cfg.QuickPanel.Favorites = []string{"acp:Claude Code", "agent:Review"}
	cw.cfg.QuickPanel.DefaultAgent = "agent:Code"
	want = []string{"agent:Code", "acp:Claude Code", "agent:Review"}
	if got := refs(); !equalStrings(got, want) {
		t.Errorf("favorite agents = %v, want %v", got, want)
	}
}

func TestQuickPanelReset(t *testing.T) {
	cw := testWindow()
	on := true
	onCopy := on
	cw.cfg.QuickPanel = config.QuickPanelConfig{
		Enabled: &on, Background: &on, ClipboardFallback: &onCopy,
		DefaultAgent: "agent:Review", DefaultWorktree: true,
	}
	cw.workspace = "/tmp/demo"
	qp := newQuickPanel(cw)
	qp.selection = capture.Selection{Text: "x"}
	qp.hasSelection = true
	qp.draft = "y"

	qp.reset()

	if qp.hasSelection || qp.draft != "" || len(qp.selection.Text) != 0 {
		t.Errorf("reset kept captured state: %+v", qp)
	}
	if qp.agent != "agent:Review" || !qp.worktree {
		t.Errorf("reset defaults: agent=%q worktree=%v", qp.agent, qp.worktree)
	}
	if qp.workspace != "/tmp/demo" {
		t.Errorf("reset workspace = %q", qp.workspace)
	}
	if !qp.focusInput {
		t.Errorf("reset did not ask for input focus")
	}
}

func TestQuickTaskStopWhilePreparing(t *testing.T) {
	cw := testWindow()
	task := &quickTask{preparing: true, note: "正在创建 worktree…"}
	task.stop(cw)
	if !task.canceled || task.preparing || task.note != "已取消" {
		t.Errorf("stop while preparing: canceled=%v preparing=%v note=%q",
			task.canceled, task.preparing, task.note)
	}
}

func TestQuickConversation(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "Review", Models: []string{"OpenAI::gpt-4o"}}}
	cm, err := models.NewConversationManager()
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm

	dir := t.TempDir()
	conv, idx, replyID, err := cw.quickConversation("agent:Review", dir, "解释这段代码")
	if err != nil {
		t.Fatal(err)
	}
	if conv.Workspace != dir || conv.Agent != "agent:Review" {
		t.Errorf("conversation: workspace=%q agent=%q", conv.Workspace, conv.Agent)
	}
	if conv.Provider != "OpenAI" || conv.Model != "gpt-4o" {
		t.Errorf("target: provider=%q model=%q", conv.Provider, conv.Model)
	}
	if len(conv.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(conv.Messages))
	}
	if conv.Messages[0].Role != "user" || conv.Messages[0].Content != "解释这段代码" {
		t.Errorf("user message = %+v", conv.Messages[0])
	}
	if idx != 1 || conv.Messages[idx].ID != replyID || conv.Messages[idx].Role != "assistant" {
		t.Errorf("reply at %d: %+v", idx, conv.Messages[idx])
	}
	if len(conv.Turns) != 1 || conv.Turns[0].ReplyMsgID != replyID || conv.Turns[0].Model != "gpt-4o" {
		t.Errorf("turns = %+v", conv.Turns)
	}
}

func TestExistingDir(t *testing.T) {
	dir := t.TempDir()
	if got := existingDir(".", dir); got != filepath.Clean(dir) {
		t.Errorf("existingDir(.) = %q, want %q", got, dir)
	}
	if got := existingDir("missing-child", dir); got != "" {
		t.Errorf("existingDir(missing) = %q", got)
	}
}

func TestQuickPanelViewsRender(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)

	qp.mode = quickPanelBar
	qp.hasSelection = true
	qp.selection = capture.Selection{
		Text: "hello world", App: "Safari",
		Rect: capture.Rect{X: 100, Y: 100, Width: 90, Height: 18},
	}
	if img := mgo.Render(qp.view, quickBarWidth, quickBarHeight, 1); img == nil {
		t.Fatal("bar view did not render")
	}

	qp.mode = quickPanelCompose
	qp.workspace = t.TempDir()
	if img := mgo.Render(qp.view, quickPanelWidth, quickPanelHeight, 1); img == nil {
		t.Fatal("compose view did not render")
	}

	qp.mode = quickPanelRunning
	qp.task = &quickTask{
		Prompt:    "解释这段代码",
		Agent:     "agent:Review",
		Workspace: qp.workspace,
		Conv: &models.Conversation{
			ID:    "c1",
			Title: "解释这段代码",
			Messages: []models.Message{
				{ID: "u1", Role: "user", Content: "解释这段代码"},
				{ID: "r1", Role: "assistant", Content: "好的，这段代码…"},
			},
		},
		ReplyIdx: 1,
		ReplyID:  "r1",
	}
	if img := mgo.Render(qp.view, quickPanelWidth, quickRunningHeight, 1); img == nil {
		t.Fatal("running view did not render")
	}
}

func TestToggleRef(t *testing.T) {
	if got := toggleRef(nil, "agent:A"); len(got) != 1 || got[0] != "agent:A" {
		t.Errorf("toggleRef on empty = %v", got)
	}
	got := toggleRef([]string{"agent:A", "acp:B"}, "agent:A")
	if len(got) != 1 || got[0] != "acp:B" {
		t.Errorf("toggleRef remove = %v", got)
	}
}

func TestQuickPanelShortcut(t *testing.T) {
	cw := testWindow()
	if got := cw.quickPanelShortcut(); got != defaultQuickPanelShortcut {
		t.Errorf("default panel shortcut = %q, want %q", got, defaultQuickPanelShortcut)
	}
	if defaultQuickPanelShortcut == defaultQuickShortcut {
		t.Error("the panel and the compact window must not share a default shortcut")
	}
	cw.cfg.QuickPanel.Shortcut = "Alt+Space"
	if got := cw.quickPanelShortcut(); got != "Alt+Space" {
		t.Errorf("configured panel shortcut = %q", got)
	}
}
