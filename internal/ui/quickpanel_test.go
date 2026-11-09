package ui

import (
	"os"
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

// A captured selection lands in the field as a quote block, and submitting it
// unchanged does not attach the same text a second time.
func TestQuickPanelPrefillsSelection(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)
	qp.hasSelection = true
	qp.selection = capture.Selection{Text: "func main() {\n\tprintln(1)\n}"}
	qp.prefillSelection()
	want := "> func main() {\n> \tprintln(1)\n> }"
	if qp.draft != want {
		t.Fatalf("draft = %q, want %q", qp.draft, want)
	}
	if !qp.selectionInDraft {
		t.Fatal("the draft is not marked as carrying the selection")
	}
	if got := qp.prompt(); got != want {
		t.Fatalf("prompt duplicates the selection: %q", got)
	}

	// A draft the user already typed is left alone.
	qp.draft = "解释一下"
	if qp.prefillSelection(); qp.draft != "解释一下" {
		t.Fatalf("prefill overwrote the draft: %q", qp.draft)
	}
}

// A selection captured by the Services menu opens the composer with the text
// quoted, without reading the selection again.
func TestQuickPanelOpensServiceSelection(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)
	qp.showSelection(capture.Selection{Text: "hello", Source: capture.SourceService})
	if qp.draft != "> hello" {
		t.Fatalf("draft = %q, want the quoted selection", qp.draft)
	}
	if !qp.hasSelection || !qp.selectionInDraft {
		t.Fatalf("selection state: has=%v inDraft=%v", qp.hasSelection, qp.selectionInDraft)
	}
}

// A preset replaces the quote block, so submit attaches the selection again.
func TestQuickPanelPresetReattachesSelection(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)
	qp.hasSelection = true
	qp.selection = capture.Selection{Text: "hello"}
	qp.prefillSelection()
	qp.draft = "解释这段内容"
	qp.selectionInDraft = false
	if got := qp.prompt(); got != "解释这段内容\n\n> hello" {
		t.Fatalf("prompt = %q", got)
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
	conv, idx, replyID, err := cw.quickConversation("agent:Review", "", dir, "解释这段代码")
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

	// A model chosen in the panel overrides the agent's first model.
	conv2, _, _, err := cw.quickConversation("agent:Review", "OpenAI::gpt-4", dir, "再解释一次")
	if err != nil {
		t.Fatal(err)
	}
	if conv2.Provider != "OpenAI" || conv2.Model != "gpt-4" {
		t.Errorf("override target = %s::%s", conv2.Provider, conv2.Model)
	}
	if len(conv2.Turns) != 1 || conv2.Turns[0].Model != "gpt-4" {
		t.Errorf("override turn = %+v", conv2.Turns)
	}
}

func TestQuickPanelModelSelection(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{
		{Name: "Review", Models: []string{"OpenAI::gpt-4o"}},
		{Name: "Code", Models: []string{"OpenAI::gpt-4"}},
	}
	qp := newQuickPanel(cw)
	qp.setAgent("agent:Review")
	if qp.model != "OpenAI::gpt-4o" {
		t.Errorf("review model = %q", qp.model)
	}
	qp.setAgent("agent:Code")
	if qp.model != "OpenAI::gpt-4" || qp.effectiveModel() != "OpenAI::gpt-4" {
		t.Errorf("code model = %q effective %q", qp.model, qp.effectiveModel())
	}
	// ACP agents choose their own model, so the panel offers none.
	if refs := cw.modelRefsForAgent("acp:Claude"); len(refs) != 0 {
		t.Errorf("acp refs = %v", refs)
	}
	// The presets exist and carry an instruction.
	if len(quickPresets) < 3 {
		t.Fatalf("presets = %+v", quickPresets)
	}
	for _, p := range quickPresets {
		if p.Label == "" || p.Prompt == "" {
			t.Errorf("empty preset: %+v", p)
		}
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

// The agent chips and the model choice share one row, so the panel does not
// stack two half-empty lines above the composer.
func TestQuickPanelAgentAndModelShareRow(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)
	qp.mode = quickPanelCompose
	tr := mgo.NewTester(qp.view, quickPanelWidth, quickPanelHeight)
	tr.Frame()
	agent, ok := tr.Find("Chat")
	if !ok {
		t.Fatalf("the agent chip is missing; texts: %v", tr.Texts())
	}
	model, ok := tr.Find("OpenAI · gpt-4")
	if !ok {
		t.Fatalf("the model selector is missing; texts: %v", tr.Texts())
	}
	if agent.Y+agent.H/2 != model.Y+model.H/2 {
		t.Fatalf("the agent and model sit on different rows: agent %+v, model %+v", agent, model)
	}
}

// The panel names its worktree toggle instead of leaving a bare switch.
func TestQuickPanelNamesItsWorktreeSwitch(t *testing.T) {
	cw := testWindow()
	qp := newQuickPanel(cw)
	qp.mode = quickPanelCompose
	tr := mgo.NewTester(qp.view, quickPanelWidth, quickPanelHeight)
	tr.Frame()
	r, ok := tr.Find("新建 worktree")
	if !ok || r.H == 0 {
		t.Fatalf("the worktree switch is clipped or missing: %+v, texts: %v", r, tr.Texts())
	}
	if r.Y+r.H > quickPanelHeight {
		t.Fatalf("the worktree switch overflows the panel: %+v", r)
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

func TestQuickRunningCounter(t *testing.T) {
	cw := testWindow()
	cw.markQuickRunning("r1")
	cw.markQuickRunning("r1")
	if cw.quickRunning != 1 {
		t.Fatalf("running = %d, want 1", cw.quickRunning)
	}
	cw.unmarkQuickReply("r1")
	cw.unmarkQuickReply("r1")
	if cw.quickRunning != 0 {
		t.Fatalf("running = %d, want 0", cw.quickRunning)
	}
}

func TestQuickStartTaskRefusesAtCap(t *testing.T) {
	cw := testWindow()
	cw.quickRunning = quickMaxRunning
	if task := cw.quickStartTask("解释一下", "", "/tmp", "", false); task != nil {
		t.Fatalf("started a task at the cap: %+v", task)
	}
}

func TestAgentSkillPrompt(t *testing.T) {
	home := os.Getenv("HOME")
	dir := filepath.Join(home, ".chatgo", "skills", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Demo\n\nDo the demo thing."), 0o644); err != nil {
		t.Fatal(err)
	}

	cw := testWindow()
	cw.loadSkills()
	if !containsStr(cw.cfg.SkillsEnabled, dir+string(os.PathSeparator)+"SKILL.md") {
		t.Fatalf("loadSkills did not enable the skill: %v", cw.cfg.SkillsEnabled)
	}

	// The built-in chat gets every enabled skill.
	if got := cw.agentSkillPrompt(""); !strings.Contains(got, "Do the demo thing") {
		t.Errorf("chat skills = %q", got)
	}
	// A custom agent without it gets nothing.
	cw.cfg.Agents = []config.AgentDef{{Name: "NoSkills"}}
	if got := cw.agentSkillPrompt("agent:NoSkills"); got != "" {
		t.Errorf("agent without skills = %q", got)
	}
	// A custom agent that lists it gets it.
	cw.cfg.Agents = []config.AgentDef{{Name: "WithSkills", Skills: []string{"demo"}}}
	if got := cw.agentSkillPrompt("agent:WithSkills"); !strings.Contains(got, "Do the demo thing") {
		t.Errorf("agent with skills = %q", got)
	}
	// ACP manages its own skills.
	if got := cw.agentSkillPrompt("acp:Claude"); got != "" {
		t.Errorf("acp skills = %q", got)
	}
}

func TestSplitBundleList(t *testing.T) {
	got := splitBundleList(" com.apple.Terminal, com.tencent.xinWeChat;com.apple.finder ")
	want := []string{"com.apple.Terminal", "com.tencent.xinWeChat", "com.apple.finder"}
	if !equalStrings(got, want) {
		t.Errorf("split = %v, want %v", got, want)
	}
	if len(splitBundleList("  ")) != 0 {
		t.Errorf("blank split not empty")
	}
}

func TestQuickBarRendersManyAgents(t *testing.T) {
	cw := testWindow()
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		cw.cfg.Agents = append(cw.cfg.Agents, config.AgentDef{Name: name})
	}
	qp := newQuickPanel(cw)
	qp.mode = quickPanelBar
	qp.hasSelection = true
	qp.selection = capture.Selection{Text: "hello"}
	if img := mgo.Render(qp.view, quickBarWidth, quickBarHeight, 1); img == nil {
		t.Fatal("bar with six agents did not render")
	}
}
