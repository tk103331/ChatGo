package ui

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// The agent picker lists every assistant in one list: Chat, each custom agent,
// each enabled ACP connection. Disabled connections stay out.
func TestAgentPickItems(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "Reviewer"}}
	cw.cfg.ACPAgents = []config.ACPConnection{
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}},
		{Name: "Off", Command: "off", Disabled: true},
	}
	items := cw.agentPickItems()
	want := []string{"Chat", "Reviewer", "OpenCode"}
	if len(items) != len(want) {
		t.Fatalf("items = %d, want %d", len(items), len(want))
	}
	for i, it := range items {
		if it.label != want[i] {
			t.Fatalf("item %d = %+v", i, it)
		}
	}
}

// The agent picker renders its searchable agent list with only a mark and
// the agent name per row.
func TestAgentPickerBodyRenders(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{Name: "Reviewer", Description: "评审"}}
	cw.cur = &models.Conversation{ID: "c", Title: "t"}
	current := "agent:Reviewer"
	query := ""
	tr := mgo.NewTester(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Density: mtheme.Compact})
		cw.agentPickerBody(c, &current, func(string) {}, &query, func() {})
	}, 720, 420)
	tr.Frame()
	for _, want := range []string{"Chat", "Reviewer"} {
		if !tr.HasText(want) {
			t.Fatalf("the picker lacks %q; texts: %v", want, tr.Texts())
		}
	}
	// A query that matches nothing renders the hint.
	query = "不存在的"
	tr.Frame()
	if !tr.HasText("没有匹配的 Agent") {
		t.Fatalf("the query hint is missing; texts: %v", tr.Texts())
	}
}

// The agent picker's ACP row shows only the connection name beside its mark.
func TestAgentPickerACPRowShowsName(t *testing.T) {
	cw := testWindow()
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}}}
	current := "acp:OpenCode"
	query := ""
	tr := mgo.NewTester(func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Density: mtheme.Compact})
		cw.agentPickerBody(c, &current, func(string) {}, &query, func() {})
	}, 720, 420)
	tr.Frame()
	for _, want := range []string{"Chat", "OpenCode"} {
		if !tr.HasText(want) {
			t.Fatalf("the ACP row lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

// The model list is read from the config option categorised as a model, both
// flat and grouped; other categories are ignored.
func TestACPModelsFromConfigs(t *testing.T) {
	model := acp.SessionConfigOptionCategoryModel
	mode := acp.SessionConfigOptionCategoryMode
	current := acp.SessionConfigValueId("m2")
	ungrouped := acp.SessionConfigSelectOptionsUngrouped{
		{Value: "a", Name: "写"}, {Value: "b", Name: "读"},
	}
	grouped := acp.SessionConfigSelectOptionsGrouped{
		{Name: "快", Options: []acp.SessionConfigSelectOption{{Value: "m1", Name: "小模型"}}},
		{Name: "强", Options: []acp.SessionConfigSelectOption{{Value: "m2", Name: "大模型"}}},
	}
	configs := []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Name: "模式", Category: &mode, CurrentValue: "b",
			Options: acp.SessionConfigSelectOptions{Ungrouped: &ungrouped},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Name: "模型", Category: &model, CurrentValue: current,
			Options: acp.SessionConfigSelectOptions{Grouped: &grouped},
		}},
	}
	list := acpModelsFromConfigs(configs)
	if len(list) != 2 || list[0].Name != "小模型" || list[1].Name != "大模型" || !list[1].Current {
		t.Fatalf("models = %+v", list)
	}
	// A config option with neither grouping must not panic.
	bare := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Name: "模型", Category: &model, CurrentValue: "m1",
		Options: acp.SessionConfigSelectOptions{},
	}}
	if got := acpModelsFromConfigs([]acp.SessionConfigOption{bare}); len(got) != 0 {
		t.Fatalf("bare options gave %+v", got)
	}
}
