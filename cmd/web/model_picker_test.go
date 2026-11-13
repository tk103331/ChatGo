package main

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// The dialog's fields share one style: the agent and model triggers are the
// same size as the project picker between them.
func TestWebDialogFieldsShareOneStyle(t *testing.T) {
	a := testApp()
	a.newSessionOpen = true
	a.projects = []projectInfo{{Main: "/tmp/project", Branches: []string{"main"}}}
	a.newProject = "/tmp/project"
	a.newTree = "/tmp/project"
	a.modelsByAgent = map[string]modelsByAgentState{
		"": {models: []modelInfo{
			{Ref: "magpie::glm", Label: "GLM-5.3-Flash", Provider: "magpie", ProviderType: "magpie"},
		}},
	}
	tr := mgo.NewTester(a.view, 1180, 820)
	for range 4 {
		tr.Frame()
	}
	size := func(name string) mgo.Rect {
		t.Helper()
		info, ok := tr.AccessByRole(mgo.RoleButton, name)
		if !ok {
			t.Fatalf("the dialog has no %q trigger; texts: %v", name, tr.Texts())
		}
		return info.Bounds
	}
	agent := size("Chat")
	model := size("默认模型")
	if agent.W != model.W || agent.H != model.H {
		t.Fatalf("the agent and model fields differ in size: agent %v, model %v", agent, model)
	}
	if !tr.HasText("项目") {
		t.Fatalf("the dialog has no project field; texts: %v", tr.Texts())
	}
}

// The picker groups the flat model list by provider, keeping first-appearance
// order and filling each group's type from its models.
func TestGroupModels(t *testing.T) {
	models := []modelInfo{
		{Ref: "magpie::a", Label: "a", Provider: "magpie", ProviderType: "magpie"},
		{Ref: "Local::b", Label: "b", Provider: "Local", ProviderType: "ollama"},
		{Ref: "magpie::c", Label: "c", Provider: "magpie", ProviderType: "magpie"},
	}
	groups := groupModels(models)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].name != "magpie" || groups[0].typ != "magpie" || len(groups[0].models) != 2 {
		t.Fatalf("group 0 = %+v", groups[0])
	}
	if groups[1].name != "Local" || groups[1].typ != "ollama" || len(groups[1].models) != 1 {
		t.Fatalf("group 1 = %+v", groups[1])
	}
	// Provider-less references land in the anonymous group.
	groups = groupModels([]modelInfo{{Ref: "d", Label: "d"}})
	if len(groups) != 1 || groups[0].name != "" {
		t.Fatalf("anonymous group = %+v", groups)
	}
	if groups[0].logo() == nil {
		t.Fatal("the anonymous group needs a generic mark")
	}
}

// The button labels the current model: the list entry's name, the ref's model
// part, or 默认模型 when nothing is chosen.
func TestCurrentModelLabel(t *testing.T) {
	models := []modelInfo{{Ref: "magpie::glm", Label: "GLM-5.3-Flash", Provider: "magpie"}}
	if got := currentModelLabel(models, "magpie::glm"); got != "GLM-5.3-Flash" {
		t.Fatalf("label = %q", got)
	}
	if got := currentModelLabel(models, "Other::deep"); got != "deep" {
		t.Fatalf("unknown label = %q", got)
	}
	if got := currentModelLabel(models, ""); got != "默认模型" {
		t.Fatalf("empty label = %q", got)
	}
}

// The picker body renders on its own: rail, list and details, with a rail
// filter and a query active.
func TestWebModelPickerBodyRenders(t *testing.T) {
	a := testApp()
	models := []modelInfo{
		{Ref: "magpie::glm", Label: "GLM-5.3-Flash", Provider: "magpie", ProviderType: "magpie"},
		{Ref: "Local::llama3", Label: "llama3", Provider: "Local", ProviderType: "ollama"},
	}
	current := "magpie::glm"
	query := ""
	img := mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.modelPickerBody(c, c.Theme(), models, &current, func(string) {}, &query, func() {})
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("picker body render failed")
	}
	// A query with no hits draws the empty hint.
	query = "不存在的"
	img = mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.modelPickerBody(c, c.Theme(), models, &current, func(string) {}, &query, func() {})
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("empty picker render failed")
	}
}

// The composer draws the picker's anchor button instead of the plain select
// once the enriched model list is loaded.
func TestWebComposerModelPickerRenders(t *testing.T) {
	a := testApp()
	a.models = []modelInfo{
		{Ref: "magpie::glm", Label: "GLM-5.3-Flash", Provider: "magpie", ProviderType: "magpie"},
		{Ref: "Local::llama3", Label: "llama3", Provider: "Local", ProviderType: "ollama"},
	}
	renderApp(t, a, 1180, 820)
}

// The new-session dialog carries two separate selectors: the agent picker,
// then a provider-classified model picker with 默认模型 as a row.
func TestWebNewSessionModelPickerRenders(t *testing.T) {
	a := testApp()
	a.newSessionOpen = true
	a.modelsByAgent = map[string]modelsByAgentState{
		"": {models: []modelInfo{
			{Ref: "magpie::glm", Label: "GLM-5.3-Flash", Provider: "magpie", ProviderType: "magpie"},
		}},
	}
	if got := currentModelLabel(a.modelsByAgent[""].models, ""); got != "默认模型" {
		t.Fatalf("dialog default label = %q", got)
	}
	tr := mgo.NewTester(a.view, 1180, 820)
	for range 4 {
		tr.Frame()
	}
	// Agent and model stay separate controls.
	for _, want := range []string{"Agent", "默认模型"} {
		if !tr.HasText(want) {
			t.Fatalf("the dialog lacks %q: %v", want, tr.Texts())
		}
	}
	if err := tr.Click("默认模型"); err != nil {
		t.Fatalf("open the model picker: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	for _, want := range []string{"选择模型", "GLM-5.3-Flash"} {
		if !tr.HasText(want) {
			t.Fatalf("the model picker lacks %q: %v", want, tr.Texts())
		}
	}
}
