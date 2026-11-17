package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/plugin"
)

// catalogTestWindow is a window with the plugin manager and the shipped
// catalog plugins registered from the source tree, the way the app has
// them after a user installed the ones they wanted.
func catalogTestWindow(t *testing.T) *ChatWindow {
	t.Helper()
	cw := testWindow()
	m := plugin.NewManager(cw.cfg)
	m.LoadAll()
	m.RegisterCatalogForTest(catalogSourceDir(t))
	cw.plugins = m
	return cw
}

// catalogSourceDir is the shipped plugin sources inside the repository.
func catalogSourceDir(t *testing.T) string {
	t.Helper()
	// Tests run in internal/ui; the sources sit at the repo root.
	dir := filepath.Join("..", "..", "plugins", "catalog")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("the catalog sources are not present")
	}
	return dir
}

// The plugins page is two tabs: my plugins lists the installed with the
// type filter, and the market lists the release's catalog entries.
func TestPluginsPageTwoTabs(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	cw.section = sectionSettings
	cw.settings.tab = settingsPluginsTab
	tr := mgo.NewTester(cw.view, 1200, 800)
	tr.Frame()
	for _, want := range []string{"我的插件", "插件市场", "模型", "安装插件"} {
		if !tr.HasText(want) {
			t.Fatalf("the plugins page lacks %q; texts: %v", want, tr.Texts())
		}
	}
	// The market tab: its filter, its source badge and manual refresh.
	if err := tr.Click("插件市场"); err != nil {
		t.Fatalf("open the market tab: %v", err)
	}
	tr.Frame()
	for _, want := range []string{"索引 · 内置", "更新索引", "全部类型"} {
		if !tr.HasText(want) {
			t.Fatalf("the market tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

// A window with nothing installed shows the market's install buttons.
func TestMarketTabListsInstallable(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	m := plugin.NewManager(cw.cfg)
	m.LoadAll()
	cw.plugins = m
	cw.section = sectionSettings
	cw.settings.tab = settingsPluginsTab
	cw.pluginUI.marketTab = 1
	tr := mgo.NewTester(cw.view, 1200, 800)
	tr.Frame()
	if !tr.HasText("安装") {
		t.Fatalf("the market rows lack an install button; texts: %v", tr.Texts())
	}
	if !tr.HasText("模型") {
		t.Fatalf("the market rows lack the kind label; texts: %v", tr.Texts())
	}
}

// The 模型 and ACP pages' "从插件添加" buttons hand over to the plugins
// page's market tab, filtered to their catalog kind — the dialogs they
// opened before are gone.
func TestGoPluginMarketJumpsFiltered(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	m := plugin.NewManager(cw.cfg)
	m.LoadAll()
	cw.plugins = m

	cw.goPluginMarket("catalog-providers")
	if cw.section != sectionSettings || cw.pluginUI.marketTab != 1 ||
		cw.pluginUI.marketFilter != "catalog-providers" {
		t.Fatalf("the provider jump missed: section=%d tab=%d filter=%q",
			cw.section, cw.pluginUI.marketTab, cw.pluginUI.marketFilter)
	}
	cw.goPluginMarket("catalog-acp")
	if cw.pluginUI.marketFilter != "catalog-acp" {
		t.Fatalf("the acp jump missed: filter=%q", cw.pluginUI.marketFilter)
	}
}

// The import dialogs carry a "从插件添加" button that closes the dialog
// and hands over to the market filtered to the kind's catalog.
func TestImportDialogJumpsToMarket(t *testing.T) {
	isolateHome(t)
	cw := importTestWindow(t)
	cw.section = sectionSettings
	cw.imp = importState{open: true, kind: importMCP, chosen: map[string]bool{}, scanning: true}
	tr := mgo.NewTester(cw.view, 1000, 800)
	tr.Frame()
	if err := tr.Click("从插件添加"); err != nil {
		t.Fatalf("the import dialog lacks the jump: %v", err)
	}
	tr.Frame()
	if cw.imp.open {
		t.Fatal("the jump must close the import dialog")
	}
	if cw.pluginUI.marketTab != 1 || cw.pluginUI.marketFilter != "importers" {
		t.Fatalf("the jump missed the market: tab=%d filter=%q",
			cw.pluginUI.marketTab, cw.pluginUI.marketFilter)
	}
	// The provider import jumps to the provider catalog instead.
	cw.imp = importState{open: true, kind: importProviders, chosen: map[string]bool{}, scanning: true}
	tr.Frame()
	if err := tr.Click("从插件添加"); err != nil {
		t.Fatalf("the provider dialog lacks the jump: %v", err)
	}
	tr.Frame()
	if cw.pluginUI.marketFilter != "catalog-providers" {
		t.Fatalf("the provider jump landed on %q", cw.pluginUI.marketFilter)
	}
}

// The embedded catalog index the app ships matches the plugins the repo
// carries: no duplicate ids, legal kinds, the key entries in place.
func TestCatalogIndexMatchesShippedPlugins(t *testing.T) {
	entries := plugin.Catalog()
	if len(entries) < 48 {
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
	// The registry covers the official ACP agents, so the shipped catalog
	// keeps only the entries it does not list.
	for _, want := range []string{"provider-openai", "acp-traecli", "importer-cursor-config"} {
		if !seen[want] {
			t.Fatalf("the index lacks %q", want)
		}
	}
	for _, gone := range []string{"acp-claudecode", "acp-codexcli", "acp-geminicli"} {
		if seen[gone] {
			t.Fatalf("the index still ships %q; the ACP Registry covers it", gone)
		}
	}
}

// The market's filter narrows by the catalog's own kind.
func TestMarketKindFilterOptions(t *testing.T) {
	if got := importMarketKind(importMCP); got != "importers" {
		t.Fatalf("MCP imports should jump to importers, got %q", got)
	}
	if got := importMarketKind(importSkills); got != "importers" {
		t.Fatalf("Skill imports should jump to importers, got %q", got)
	}
	if got := importMarketKind(importProviders); got != "catalog-providers" {
		t.Fatalf("model imports should jump to providers, got %q", got)
	}
	_ = strings.TrimSpace
}
