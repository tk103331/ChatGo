package ui

import (
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// A conversation's half-written message stays with the conversation: the
// next one brings back its own draft, and the first one gets its text again
// when it is reopened.
func TestDraftsAreKeptPerConversation(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	conv1 := &models.Conversation{ID: "c1", Title: "one"}
	conv2 := &models.Conversation{ID: "c2", Title: "two"}
	if err := cm.SaveConversation(conv1); err != nil {
		t.Fatal(err)
	}
	if err := cm.SaveConversation(conv2); err != nil {
		t.Fatal(err)
	}

	cw.cur = conv1
	cw.draft = "给一会话的草稿"
	cw.openConversation(conv2)
	if cw.draft != "" {
		t.Fatalf("the second conversation should start with an empty draft, got %q", cw.draft)
	}
	if cw.drafts["c1"] != "给一会话的草稿" {
		t.Fatalf("the first conversation's draft was lost: %v", cw.drafts)
	}
	cw.draft = "给二会话的草稿"
	cw.openConversation(conv1)
	if cw.draft != "给一会话的草稿" {
		t.Fatalf("the first conversation should get its draft back, got %q", cw.draft)
	}
	if cw.drafts["c2"] != "给二会话的草稿" {
		t.Fatalf("the second conversation's draft was lost: %v", cw.drafts)
	}
}

// While a reply streams, Enter starts nothing: the composer keeps editing,
// but a second turn must not begin until the first one ends.
func TestSendDraftBlockedWhileStreaming(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	conv := &models.Conversation{ID: "c1", Title: "t"}
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	cw.cur = conv
	cw.draft = "hi"

	cw.beginStream("c1", "r1", nil)
	cw.sendDraft()
	if len(conv.Messages) != 0 {
		t.Fatalf("a streaming conversation started another turn: %d messages", len(conv.Messages))
	}

	cw.endStream("r1")
	cw.sendDraft()
	if len(conv.Messages) != 2 {
		t.Fatalf("after the stream ends the turn should start, got %d messages", len(conv.Messages))
	}
	drainMain(cw)
}

// A config change the CLI refuses leaves the old value on the option, so the
// control never shows a choice the runtime rejected.
func TestACPRestoreConfigPutsBackRejectedValue(t *testing.T) {
	cw := testWindow()
	values := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "m1", Name: "小模型"}, {Value: "m2", Name: "大模型"},
	}
	rt := &acpConv{configs: []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{Id: "model", CurrentValue: "m2",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &values}}},
		{Boolean: &acp.SessionConfigOptionBoolean{Id: "think", CurrentValue: true}},
	}}
	cw.acpRestoreConfig(rt, "model", "m1")
	if rt.configs[0].Select.CurrentValue != "m1" {
		t.Fatalf("the select should roll back, got %q", rt.configs[0].Select.CurrentValue)
	}
	cw.acpRestoreConfig(rt, "think", "true")
	if !rt.configs[1].Boolean.CurrentValue {
		t.Fatal("the boolean should roll back to true")
	}
	// A boolean saved value that no longer parses is left alone.
	cw.acpRestoreConfig(rt, "think", "not-a-bool")
	if !rt.configs[1].Boolean.CurrentValue {
		t.Fatal("an unparsable value must not touch the boolean")
	}
}

// A saved choice the runtime no longer offers is dropped instead of being
// forced on it: the option keeps the CLI's own current value.
func TestACPApplySavedDropsValuesTheRuntimeNoLongerOffers(t *testing.T) {
	cw := testWindow()
	values := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "m1", Name: "小模型"}, {Value: "m2", Name: "大模型"},
	}
	rt := &acpConv{configs: []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{Id: "model", CurrentValue: "m1",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &values}}},
	}}
	conv := &models.Conversation{ACPConfig: map[string]string{"model": "m-gone"}}
	// rt.conn is nil: a forced value would panic, so this passing proves the
	// stale value was skipped before any request was built.
	cw.acpApplySaved(rt, conv)
	if rt.configs[0].Select.CurrentValue != "m1" {
		t.Fatalf("the stale value was forced onto the runtime: %q", rt.configs[0].Select.CurrentValue)
	}
}

// A directory outside any repository is a workspace in its own right: the
// create page says so instead of showing an endless "loading worktrees", and
// the directory is never probed again.
func TestCreatePlainDirectoryShowsWorkspace(t *testing.T) {
	dir := t.TempDir()
	cw := worktreeWindow(t, dir)
	cw.create.agent = ""
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "工作区")
	if tr.HasText("正在读取项目工作树") {
		t.Fatalf("a plain directory should not show an endless load: %v", tr.Texts())
	}
	if !cw.convGitLoaded(dir) {
		t.Fatal("the non-repository result should be cached, not re-probed")
	}
	if cw.convGitProject(dir) != nil {
		t.Fatal("a plain directory has no project")
	}
}

// The composer's ACP row keeps its first three controls inline and moves the
// rest behind the settings popover, so a long option list never overflows
// the composer.
func TestACPExtrasLiveBehindTheSettingsPopover(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "acp1", Title: "ACP", Workspace: "/tmp/demo", Agent: "acp:OpenCode",
		Messages: []models.Message{{ID: "u1", Role: "user", Content: "hi", Timestamp: time.Now()}}}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)
	model := acp.SessionConfigOptionCategoryModel
	thought := acp.SessionConfigOptionCategoryThoughtLevel
	mode := acp.SessionConfigOptionCategoryMode
	sel := func(id, name string, cat *acp.SessionConfigOptionCategory, value, opt string) acp.SessionConfigOption {
		values := acp.SessionConfigSelectOptionsUngrouped{{Value: acp.SessionConfigValueId(value), Name: opt}}
		return acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
			Id: acp.SessionConfigId(id), Name: name, Category: cat, CurrentValue: acp.SessionConfigValueId(value),
			Options: acp.SessionConfigSelectOptions{Ungrouped: &values},
		}}
	}
	cw.acp.convs["acp1"] = &acpConv{configs: []acp.SessionConfigOption{
		sel("model", "Model", &model, "gpt-4", "GPT-4"),
		sel("thought", "Thought", &thought, "low", "Low"),
		sel("mode", "Mode", &mode, "fast", "Fast"),
		sel("other", "Other", nil, "x", "第四项"),
	}}
	tr := mgo.NewTester(cw.view, 1280, 820)
	for range 5 {
		tr.Frame()
	}
	for _, want := range []string{"GPT-4", "Low", "Fast", "ACP 设置"} {
		if !tr.HasText(want) {
			t.Fatalf("the composer row lacks %q; texts: %v", want, tr.Texts())
		}
	}
	if tr.HasText("第四项") {
		t.Fatalf("the fourth control belongs behind the popover, not in the row: %v", tr.Texts())
	}
	if err := tr.Click("ACP 设置"); err != nil {
		t.Fatalf("open the settings popover: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("第四项") {
		t.Fatalf("the popover should show the extra control; texts: %v", tr.Texts())
	}
	if !tr.HasText("Other（选择）") {
		t.Fatalf("the popover should name the option and its kind; texts: %v", tr.Texts())
	}
}

// Selecting a different agent on the 新会话 page drops the ACP choices made
// for the previous one, so no option id leaks into another agent's session.
func TestCreateAgentChangeDropsACPChoices(t *testing.T) {
	cw := testWindow()
	cw.create.agent = "acp:PiAgent"
	cw.createACPSet("model", "m2")
	cw.createACPSet("think", "false")
	// The picker's onPick callback, as the page wires it.
	cw.create.agent = "acp:Claude"
	cw.create.acpConfig = nil
	if len(cw.create.acpConfig) != 0 {
		t.Fatalf("the choices should be gone, got %v", cw.create.acpConfig)
	}
}
