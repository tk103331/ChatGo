package main

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// The agent picker lists every assistant in one list, in the runner's order:
// Chat, the custom agents, then the ACP connections.
func TestAgentPickItems(t *testing.T) {
	agents := []agentInfo{
		{Ref: "agent:Reviewer", Name: "Reviewer", Kind: "agent"},
		{Ref: "acp:OpenCode", Name: "OpenCode", Kind: "acp"},
	}
	items := agentPickItems(agents)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	want := []string{"", "agent:Reviewer", "acp:OpenCode"}
	labels := []string{"Chat", "Reviewer", "OpenCode"}
	for i, it := range items {
		if it.Ref != want[i] || it.Name != labels[i] {
			t.Fatalf("item %d = %+v", i, it)
		}
	}
}

// The button labels the chosen agent.
func TestAgentPickLabel(t *testing.T) {
	agents := []agentInfo{{Ref: "agent:Reviewer", Name: "Reviewer", Kind: "agent"}}
	if got := agentPickLabel(agents, ""); got != "Chat" {
		t.Fatalf("label = %q", got)
	}
	if got := agentPickLabel(agents, "agent:Reviewer"); got != "Reviewer" {
		t.Fatalf("label = %q", got)
	}
	if got := agentPickLabel(agents, "acp:Unknown"); got != "Unknown" {
		t.Fatalf("label = %q", got)
	}
}

// The picker body renders its agent list, and an empty search shows the hint.
func TestWebAgentPickerBodyRenders(t *testing.T) {
	a := testApp()
	a.agents = []agentInfo{
		{Ref: "agent:Reviewer", Name: "Reviewer", Kind: "agent"},
		{Ref: "acp:OpenCode", Name: "OpenCode", Kind: "acp", Description: "opencode acp"},
	}
	current := "acp:OpenCode"
	query := ""
	img := mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.agentPickerBody(c, c.Theme(), &current, func(string) {}, &query, func() {})
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("the picker did not render")
	}
	query = "不存在的"
	img = mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.agentPickerBody(c, c.Theme(), &current, func(string) {}, &query, func() {})
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("the empty picker did not render")
	}
}
