package ui

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// acpCreateOptions is the runtime's own set of options: a model, a mode and a
// switch, the way a coding CLI reports them.
func acpCreateOptions() []acp.SessionConfigOption {
	model := acp.SessionConfigOptionCategoryModel
	mode := acp.SessionConfigOptionCategoryMode
	models := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "m1", Name: "小模型"},
		{Value: "m2", Name: "大模型"},
	}
	modes := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "fast", Name: "快速"},
	}
	return []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Id: "model", Name: "Model", Category: &model, CurrentValue: "m1",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &models},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Id: "mode", Name: "Mode", Category: &mode, CurrentValue: "fast",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &modes},
		}},
		{Boolean: &acp.SessionConfigOptionBoolean{
			Id: "think", Name: "Think", CurrentValue: true,
		}},
	}
}

// Picking an ACP agent on the 新会话 page shows the runtime's model and its
// config options in the composer's row, the way a conversation's composer
// shows them.
func TestCreateACPChoicesShowModelAndOptions(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "PiAgent", Command: "pi"}}
	cw.acp.configs["PiAgent"] = acpCreateOptions()
	cw.create.agent = "acp:PiAgent"
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	// The controls show the runtime's current choices, as in a conversation.
	for _, want := range []string{"小模型", "快速"} {
		if !tr.HasText(want) {
			t.Fatalf("the composer lacks the ACP control %q; texts: %v", want, tr.Texts())
		}
	}
}

// The choices made on the page ride the new conversation, so its session
// starts with them.
func TestCreateACPChoicesRideTheConversation(t *testing.T) {
	cw := testWindow()
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "PiAgent", Command: "pi"}}
	cw.create.agent = "acp:PiAgent"
	cw.createACPSet("model", "m2")
	cw.createACPSet("think", "false")
	cw.cur = &models.Conversation{ID: "c", Title: "t"}
	cw.applyCreateACPConfig()
	if cw.cur.ACPConfig["model"] != "m2" {
		t.Fatalf("the model's choice was lost: %v", cw.cur.ACPConfig)
	}
	if cw.cur.ACPConfig["think"] != "false" {
		t.Fatalf("the switch's choice was lost: %v", cw.cur.ACPConfig)
	}
}

// A connection whose warm-up failed stays out of the composer and becomes
// disabled in the Agent picker instead.
func TestCreateACPChoicesReportWarmFailure(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "PiAgent", Command: "pi"}}
	cw.acp.modelErrs["PiAgent"] = "启动超时"
	cw.create.agent = "acp:PiAgent"
	cw.create.ws = dir
	cw.openCreateTab()
	tr := cw.tester(1000, 700, "新会话")
	if tr.HasText("预热失败") || tr.HasText("启动超时") {
		t.Fatalf("the composer should not show the warm-up failure; texts: %v", tr.Texts())
	}
	items := cw.agentPickItems()
	for _, item := range items {
		if item.ref == "acp:PiAgent" && !item.disabled {
			t.Fatal("the failed ACP Agent should be disabled in the picker")
		}
	}
}
