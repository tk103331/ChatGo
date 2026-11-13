package main

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// The workspace browser renders its states: the directory listing with the
// parent row, and the error wording when the host refuses.
func TestWebFSBrowserRenders(t *testing.T) {
	a := testApp()
	a.newSessionOpen = true
	a.newProject = newProjectCustom
	a.fsDir = "/tmp/project"
	a.fsParent = "/tmp"
	a.fsEntries = []fsItem{{Name: "sub", Path: "/tmp/project/sub"}}
	a.fsOpen = true
	img := mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.fsBrowserButton(c, c.Theme())
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("the fs browser did not render")
	}
	// A failed listing shows the reason and a retry.
	a.fsErr = "目录不存在：/tmp/gone"
	a.fsEntries = nil
	img = mgo.Render(func(c *mgo.Context) {
		uikit.Use(c, uikit.Settings{Mode: uikit.System, Density: theme.DefaultDensity, Locale: uikit.ZhCN})
		a.fsBrowserButton(c, c.Theme())
	}, 720, 420, 1)
	if img == nil {
		t.Fatal("the fs browser error state did not render")
	}
}

// The load walks one directory and posts the listing back to the app state.
func TestLoadFSNavigates(t *testing.T) {
	a := testApp()
	if a.fsDir != "" || a.fsLoading {
		t.Fatalf("initial state = %q loading=%v", a.fsDir, a.fsLoading)
	}
	// The request itself needs the server; here only the state reset shows.
	a.loadFS("")
	if !a.fsLoading {
		t.Fatal("the walk should mark the browser as loading")
	}
}
