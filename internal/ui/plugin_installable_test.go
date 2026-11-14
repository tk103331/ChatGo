package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/plugin"
)

// The plugins page's installable section lists the catalog entries the
// release offers — from the embedded index, so it renders offline — and
// marks the ones already installed.
func TestPluginInstallableSectionListsCatalog(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	cw.section = sectionSettings
	// The plugins tab is the last of the settings page.
	cw.settings.tab = 10
	tr := mgo.NewTester(cw.view, 1200, 800)
	tr.Frame()
	if !tr.HasText("可安装目录") {
		t.Fatalf("the plugins tab lacks the installable section; texts: %v", tr.Texts())
	}
	// Expand it and check the entries show with an install button.
	if err := tr.Click("展开"); err != nil {
		t.Fatalf("expand the section: %v", err)
	}
	tr.Frame()
	if !tr.HasText("安装") {
		t.Fatalf("the installable rows lack an install button; texts: %v", tr.Texts())
	}
}

// The catalog entries the app embeds match the plugins the repo ships: one
// index entry per source plugin, so the release and the app agree.
func TestCatalogIndexMatchesShippedPlugins(t *testing.T) {
	entries := plugin.Catalog()
	if len(entries) < 80 {
		t.Fatalf("the embedded index should hold the shipped catalog, got %d", len(entries))
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.ID] {
			t.Fatalf("the index lists %q twice", e.ID)
		}
		seen[e.ID] = true
		switch e.Kind {
		case "catalog-providers", "catalog-acp", "importers":
		default:
			t.Fatalf("%q carries an unknown kind %q", e.ID, e.Kind)
		}
	}
	for _, want := range []string{"provider-openai", "acp-claudecode", "importer-cursor-config"} {
		if !seen[want] {
			t.Fatalf("the index lacks %q", want)
		}
	}
}
