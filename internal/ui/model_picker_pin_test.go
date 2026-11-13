package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// The model's search field is pinned above the list: scrolling the models
// leaves it where it was.
func TestModelPickerSearchStaysWhileTheListScrolls(t *testing.T) {
	cw := testWindow()
	cw.provider = "OpenAI"
	cw.model = "gpt-4"
	refs := []string{"OpenAI::gpt-4"}
	for i := range 60 {
		refs = append(refs, "OpenAI::model-"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	view := func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Density: mtheme.Compact})
		mgo.Column(c).Fill().Justify(mgo.Start).Children(func() {
			cw.modelPickerButton(c, refs)
		})
	}
	tr := mgo.NewTester(view, 600, 600)
	tr.Frame()
	if err := tr.Click("gpt-4"); err != nil {
		t.Fatalf("open the model picker: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	before, ok := tr.AccessByRole(mgo.RoleTextField, "搜索模型")
	if !ok {
		t.Fatalf("the picker has no search field; texts: %v", tr.Texts())
	}
	// Scroll the list down; the pinned field must not move.
	tr.Scroll(240, 110, 0, 300)
	for range 3 {
		tr.Frame()
	}
	after, ok := tr.AccessByRole(mgo.RoleTextField, "搜索模型")
	if !ok {
		t.Fatalf("the search field is gone after scrolling")
	}
	if after.Bounds.Y != before.Bounds.Y {
		t.Fatalf("the search field moved with the list: before %v, after %v",
			before.Bounds, after.Bounds)
	}
}

// The agent picker's search field is pinned above the list too.
func TestAgentPickerSearchStaysWhileTheListScrolls(t *testing.T) {
	cw := testWindow()
	for i := range 60 {
		cw.cfg.Agents = append(cw.cfg.Agents, config.AgentDef{
			Name: "Agent-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
		})
	}
	current := ""
	view := func(c *mgo.Context) {
		mcore.Use(c, mcore.Settings{Density: mtheme.Compact})
		mgo.Column(c).Fill().Justify(mgo.Start).Children(func() {
			cw.agentPickerButton(c, &current, func(string) {})
		})
	}
	tr := mgo.NewTester(view, 600, 600)
	tr.Frame()
	if err := tr.Click(chatAgentName); err != nil {
		t.Fatalf("open the agent picker: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	before, ok := tr.AccessByRole(mgo.RoleTextField, "搜索 Agent")
	if !ok {
		t.Fatalf("the picker has no search field; texts: %v", tr.Texts())
	}
	tr.Scroll(240, 110, 0, 300)
	for range 3 {
		tr.Frame()
	}
	after, ok := tr.AccessByRole(mgo.RoleTextField, "搜索 Agent")
	if !ok {
		t.Fatalf("the search field is gone after scrolling")
	}
	if after.Bounds.Y != before.Bounds.Y {
		t.Fatalf("the search field moved with the list: before %v, after %v",
			before.Bounds, after.Bounds)
	}
}
