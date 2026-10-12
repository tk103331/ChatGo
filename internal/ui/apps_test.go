package ui

import (
	"testing"

	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"
)

func TestAppsRegistered(t *testing.T) {
	got := apps()
	if len(got) != 3 {
		t.Fatalf("want 3 apps, got %d", len(got))
	}
	want := map[string]bool{"terminal": false, "markdown": false, "git": false}
	for _, a := range got {
		if a.ID == "" || a.Title == "" || a.Icon == "" {
			t.Fatalf("app %+v missing fields", a)
		}
		if _, ok := want[a.ID]; !ok {
			t.Fatalf("unexpected app id %q", a.ID)
		}
		want[a.ID] = true
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("app %q not registered", id)
		}
	}
}

func TestAppsPageRenders(t *testing.T) {
	cw := testWindow()
	cw.section = sectionApps
	img := mgo.Render(cw.view, 1000, 700, 1)
	if img == nil || img.Bounds().Dx() != 1000 {
		t.Fatalf("apps page render failed: %v", img)
	}
}

func TestAppWorkspacesRender(t *testing.T) {
	for _, a := range apps() {
		a := a
		t.Run(a.ID, func(t *testing.T) {
			cw := testWindow()
			cw.section = sectionApps
			cw.ensureAppState()
			cw.appActive = a.ID
			cw.appTabList(a.ID)
			img := mgo.Render(cw.view, 1000, 700, 1)
			if img == nil || img.Bounds().Dx() != 1000 {
				t.Fatalf("app %s render failed: %v", a.ID, img)
			}
		})
	}
}

func TestAppToolsBuild(t *testing.T) {
	cw := testWindow()
	for _, a := range apps() {
		cw.currentApp = a.ID
		tools := cw.buildAppTools(a.ID)
		if len(tools) == 0 {
			t.Fatalf("app %s exposes no tools", a.ID)
		}
		for _, tl := range tools {
			info, err := tl.Info(t.Context())
			if err != nil {
				t.Fatalf("app %s tool Info: %v", a.ID, err)
			}
			if info.Name == "" {
				t.Fatalf("app %s tool has empty name", a.ID)
			}
		}
		t.Logf("app %s tools: %d", a.ID, len(tools))
	}
}

func TestOpenAppCreatesSession(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.section = sectionApps
	cw.openApp("markdown")
	if cw.cur == nil {
		t.Fatal("openApp did not create a conversation")
	}
	if cw.cur.Agent != "app:markdown" {
		t.Fatalf("want app:markdown agent, got %q", cw.cur.Agent)
	}
	if cw.currentApp != "markdown" {
		t.Fatalf("want currentApp markdown, got %q", cw.currentApp)
	}
	if cw.appConv["markdown"] != cw.cur.ID {
		t.Fatalf("appConv not recorded: %q vs %q", cw.appConv["markdown"], cw.cur.ID)
	}
	cw2 := testWindow()
	cm2, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw2.convManager = cm2
	cw2.section = sectionApps
	cw2.openApp("markdown")
	cw2.openApp("git")
	if cw2.appConv["markdown"] == cw2.appConv["git"] {
		t.Fatal("apps should have separate conversations")
	}
}
