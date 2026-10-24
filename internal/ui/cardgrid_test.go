package ui

import (
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"
)

// assertNoOverlap fails when any two card cells intersect, which is what a
// grid-based layout did: a width of 100% inside a grid cell resolves against
// the whole grid, so neighbouring cards were drawn on top of each other.
func assertNoOverlap(t *testing.T, name string, rects []mgo.Rect, want int) {
	t.Helper()
	if len(rects) != want {
		t.Fatalf("%s: got %d card cells, want %d: %+v", name, len(rects), want, rects)
	}
	for i, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("%s: card %d is empty: %+v", name, i, r)
		}
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			a, b := rects[i], rects[j]
			if a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H {
				t.Fatalf("%s: cards %d %+v and %d %+v overlap", name, i, a, j, b)
			}
		}
	}
}

func TestKnowledgeCardsDoNotOverlap(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	for _, name := range []string{"ChatGo", "sandbox-web-server", "第三个知识库", "第四个"} {
		if _, err := cw.knowledge.db.CreateKnowledgeBase(name, ""); err != nil {
			t.Fatal(err)
		}
	}
	cw.reloadKnowledgeBases()
	drain()
	if len(cw.knowledge.bases) != 4 {
		t.Fatalf("expected 4 knowledge bases, got %d", len(cw.knowledge.bases))
	}
	assertNoOverlap(t, "knowledge", measureCardCells(cw.view, 1280, 820), 4)
}

func TestAgentCardsDoNotOverlap(t *testing.T) {
	cw := testWindow()
	cw.section = sectionAgent
	cw.cfg.Agents = []config.AgentDef{
		{Name: "test", Mode: "agent"},
		{Name: "test2", Mode: "agent"},
		{Name: "test3", Mode: "chat"},
		{Name: "test4", Mode: "chat"},
	}
	assertNoOverlap(t, "agents", measureCardCells(cw.view, 1280, 820), 4)
}

func TestAppCardsDoNotOverlap(t *testing.T) {
	cw := testWindow()
	cw.section = sectionApps
	want := len(apps())
	if want == 0 {
		t.Skip("no apps are registered")
	}
	assertNoOverlap(t, "apps", measureCardCells(cw.view, 1280, 820), want)
}

func TestAutoTaskCardsDoNotOverlap(t *testing.T) {
	cw := testWindow()
	cw.section = sectionAuto
	cw.cfg.AutoTasks = []config.AutoTask{
		{ID: "1", Name: "每日报告", Schedule: config.Schedule{Kind: "daily", Hour: 9}, Enabled: true},
		{ID: "2", Name: "周报", Schedule: config.Schedule{Kind: "weekly", Hour: 18, Weekday: 5}},
		{ID: "3", Name: "月度汇总", Schedule: config.Schedule{Kind: "monthly", Day: 1}},
	}
	assertNoOverlap(t, "autotask", measureCardCells(cw.view, 1280, 820), 3)
}

func TestMCPCardsDoNotOverlap(t *testing.T) {
	cw := testWindow()
	cw.section = sectionMCP
	cw.cfg.MCPServers = []config.MCPServer{
		{Name: "filesystem", Type: "stdio", Command: "npx"},
		{Name: "github", Type: "stdio", Command: "npx"},
		{Name: "sqlite", Type: "stdio", Command: "uvx"},
	}
	assertNoOverlap(t, "mcp", measureCardCells(cw.view, 1280, 820), 3)
}

// TestEmptyStatesRender covers the shared placeholder on every card page.
func TestEmptyStatesRender(t *testing.T) {
	cases := []struct {
		name    string
		section navSection
	}{
		{"agent", sectionAgent},
		{"autotask", sectionAuto},
		{"mcp", sectionMCP},
	}
	for _, tc := range cases {
		cw := testWindow()
		cw.section = tc.section
		img := mgo.Render(cw.view, 1280, 820, 1)
		if img == nil || img.Bounds().Dx() != 1280 {
			t.Fatalf("%s empty state render failed: %v", tc.name, img)
		}
	}
	cw, drain := knowledgeTestWindow(t)
	cw.ensureKnowledgeBases()
	drain()
	if len(cw.knowledge.bases) != 0 {
		t.Fatalf("expected an empty knowledge store, got %d", len(cw.knowledge.bases))
	}
	if img := mgo.Render(cw.view, 1280, 820, 1); img == nil {
		t.Fatal("knowledge empty state render failed")
	}
}
