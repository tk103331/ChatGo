package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"
)

// The system tab is taller than a normal window, so it must scroll: the
// about block at the bottom has to be reachable.
func TestSystemTabScrolls(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsSystemTab
	tr := mgo.NewTester(cw.view, 1200, 360)
	for range 8 {
		tr.Frame()
	}
	if r, ok := tr.Find("关于"); ok && r.H > 0 {
		t.Fatalf("the about block is already visible; the window is not short enough: %+v", r)
	}
	tr.Scroll(600, 300, 0, 2000)
	tr.Frame()
	r, ok := tr.Find("关于")
	if !ok || r.H == 0 {
		t.Fatalf("the system tab does not scroll to its bottom; rect %+v, texts: %v", r, tr.Texts())
	}
}
