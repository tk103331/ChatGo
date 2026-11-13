package main

import (
	"reflect"
	"testing"
)

// The picker splits the tool list into built-in tools and MCP servers and
// appends the skills tab, matching the desktop's 工具与技能 menu.
func TestToolPanesSplitToolsMCPSkills(t *testing.T) {
	a := testApp()
	a.tools = []toolInfo{
		{ID: "builtin:filesystem", Name: "filesystem", Kind: "builtin", Enabled: true},
		{ID: "mcp:docs", Name: "docs", Kind: "mcp", Enabled: true},
	}
	a.skills = []skillInfo{{ID: "s1", Name: "Review", Token: "Review"}}
	panes := a.toolPanes()
	if len(panes) != 3 {
		t.Fatalf("panes = %d, want 3", len(panes))
	}
	want := []string{"工具", "MCP", "Skills"}
	for i, p := range panes {
		if p.name != want[i] {
			t.Errorf("pane %d = %q, want %q", i, p.name, want[i])
		}
	}
	if ids := paneIDs(panes[0]); !reflect.DeepEqual(ids, []string{"builtin:filesystem"}) {
		t.Errorf("工具 pane = %v", ids)
	}
	if ids := paneIDs(panes[1]); !reflect.DeepEqual(ids, []string{"mcp:docs"}) {
		t.Errorf("MCP pane = %v", ids)
	}
	if ids := paneIDs(panes[2]); !reflect.DeepEqual(ids, []string{"s1"}) {
		t.Errorf("Skills pane = %v", ids)
	}
}

func paneIDs(p webToolPane) []string {
	var out []string
	for _, it := range p.items {
		out = append(out, it.id)
	}
	return out
}

// The picker is built per capability: without the runtime's tool list only
// the skills tab remains.
func TestToolPanesFollowCapabilities(t *testing.T) {
	a := testApp()
	a.caps = map[string]bool{capSkills: true}
	if panes := a.toolPanes(); len(panes) != 1 || panes[0].name != "Skills" {
		t.Fatalf("panes = %+v", panes)
	}
	// A standalone server has neither; the button disappears.
	a.caps = map[string]bool{}
	if panes := a.toolPanes(); len(panes) != 0 {
		t.Fatalf("panes = %+v", panes)
	}
}

// Skill switches drive the same selection the chips show, so a skill picked
// in the menu is carried by the next message.
func TestToggleSkillUpdatesSelection(t *testing.T) {
	a := testApp()
	a.skills = []skillInfo{{ID: "s1", Name: "Review", Token: "Review"}}
	a.toggleSkill("s1", true)
	if !a.skillOn("s1") {
		t.Fatal("skill not selected after toggle on")
	}
	// Toggling on twice keeps one entry.
	a.toggleSkill("s1", true)
	a.toggleSkill("s1", false)
	if a.skillOn("s1") {
		t.Fatal("skill still selected after toggle off")
	}
	if len(a.skillsOn) != 0 {
		t.Fatalf("skillsOn = %v", a.skillsOn)
	}
}

// The tab index follows the pane list: losing the skills tab (an ACP
// session) must not leave the index out of range.
func TestToolTabStaysInRange(t *testing.T) {
	a := testApp()
	a.caps = map[string]bool{capSkills: true}
	a.toolTab = 5
	a.clampToolTab(len(a.toolPanes()))
	if a.toolTab != 0 {
		t.Fatalf("toolTab = %d, want 0", a.toolTab)
	}
	a.caps = map[string]bool{capTools: true, capSkills: true}
	a.toolTab = 1
	a.clampToolTab(len(a.toolPanes()))
	if a.toolTab != 1 {
		t.Fatalf("toolTab = %d, want 1", a.toolTab)
	}
}

// The create form resolves the directory a session opens in: a picked
// worktree wins, then the project's main tree, then a typed directory, then
// the server's default workspace.
func TestWorkspaceForCreate(t *testing.T) {
	cases := []struct {
		name          string
		project, tree string
		customDir     string
		want          string
	}{
		{"default", "", "", "", ""},
		{"project main", "/tmp/p", "/tmp/p", "", "/tmp/p"},
		{"worktree", "/tmp/p", "/tmp/wt/feat", "", "/tmp/wt/feat"},
		{"pending new tree", "/tmp/p", newTreeCreate, "", "/tmp/p"},
		{"custom dir", newProjectCustom, "", "/tmp/other", "/tmp/other"},
	}
	for _, tc := range cases {
		a := testApp()
		a.newProject, a.newTree, a.newCustomDir = tc.project, tc.tree, tc.customDir
		if got := a.workspaceForCreate(); got != tc.want {
			t.Errorf("%s: workspace = %q, want %q", tc.name, got, tc.want)
		}
	}
}
