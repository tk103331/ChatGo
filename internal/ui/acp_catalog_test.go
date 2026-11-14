package ui

import (
	"strings"
	"testing"

	"chatgo/internal/plugin"
)

// catalogTestWindow is a window with the plugin manager and the built-in
// catalogs enabled, the way the app runs.
func catalogTestWindow(t *testing.T) *ChatWindow {
	t.Helper()
	cw := testWindow()
	m := plugin.NewManager(cw.cfg)
	m.LoadAll()
	m.EnsureBuiltin()
	cw.plugins = m
	return cw
}

// ensureBuiltinCatalog turns a built-in catalog plugin on, so a test sees
// its entries even if an earlier test disabled it.
func ensureBuiltinCatalog(t *testing.T, cw *ChatWindow, id string) {
	t.Helper()
	if inst, ok := cw.plugins.Info(id); !ok || inst.Status == plugin.StatusDisabled {
		if err := cw.plugins.SetEnabled(id, true, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
}

// The built-in per-agent catalog plugins carry the shipped ACP list: every
// entry names a command, and the paseo-checked rules hold — Cursor runs
// cursor-agent, Gemini and Qwen answer --acp, Cline speaks ACP natively.
func TestACPCatalogFromPlugins(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	entries := cw.pluginACPCatalog()
	if len(entries) < 40 {
		t.Fatalf("the built-in catalogs should hold the shipped list, got %d entries", len(entries))
	}
	find := func(name string) plugin.ACPPresetEntry {
		for _, e := range entries {
			if e.Name == name {
				return e
			}
		}
		t.Fatalf("no %q entry", name)
		return plugin.ACPPresetEntry{}
	}
	if e := find("Cursor"); e.Command != "cursor-agent" || !containsStr(e.Args, "acp") {
		t.Fatalf("Cursor should run cursor-agent acp, got %s %v", e.Command, e.Args)
	}
	if e := find("GeminiCLI"); !containsStr(e.Args, "--acp") {
		t.Fatalf("Gemini needs --acp, got %s %v", e.Command, e.Args)
	}
	if e := find("Cline"); e.Command != "npx" || !containsStr(e.Args, "cline") || containsStr(e.Args, "cline-acp") {
		t.Fatalf("Cline should run npx cline --acp, got %s %v", e.Command, e.Args)
	}
	if e := find("FactoryDroid"); !containsStr(e.Args, "acp-daemon") || e.Env["DROID_DISABLE_AUTO_UPDATE"] != "true" {
		t.Fatalf("Factory Droid's command or env was lost: %s %v %v", e.Command, e.Args, e.Env)
	}
	for _, e := range entries {
		if strings.TrimSpace(e.Command) == "" {
			t.Fatalf("%q has no command", e.Name)
		}
	}
	// One plugin per agent: the group of every entry is its own plugin.
	for _, e := range entries {
		if e.Group != e.Plugin {
			t.Fatalf("%q's group should be its plugin name: %q vs %q", e.Name, e.Group, e.Plugin)
		}
	}
}

// Disabling one agent's catalog plugin removes exactly that agent from the
// dialog's list — the per-plugin granularity the plugins page promises.
func TestACPCatalogPerPluginToggle(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	if err := cw.plugins.SetEnabled("acp-cursor", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	entries := cw.pluginACPCatalog()
	for _, e := range entries {
		if e.Name == "Cursor" {
			t.Fatal("the disabled plugin's entry must be absent")
		}
	}
	found := false
	for _, e := range entries {
		if e.Name == "GeminiCLI" {
			found = true
		}
	}
	if !found {
		t.Fatal("disabling one plugin must leave the others in place")
	}
}

// addACPFromCatalog still lands the entry: enabled when the command can
// run, disabled with a hint when it cannot.
func TestAddACPFromCatalogLandsTheEntry(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	e := plugin.ACPPresetEntry{Name: "Fake", Command: "npx", Args: []string{"-y", "fake-acp"}}
	cw.addACPFromCatalog(e)
	if len(cw.cfg.ACPAgents) != 1 {
		t.Fatalf("the entry was not added: %+v", cw.cfg.ACPAgents)
	}
	conn := cw.cfg.ACPAgents[0]
	if conn.Command != "npx" || conn.Disabled {
		t.Fatalf("an npx entry should be added enabled: %+v", conn)
	}
}

// The plugins page filters: the category narrows by contribution kind, and
// the query by name.
func TestPluginFilters(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	infos := cw.plugins.List()
	if len(infos) < 70 {
		t.Fatalf("the per-entry built-ins should number the catalog, got %d", len(infos))
	}
	provs := filterPlugins(infos, "catalog-providers", "")
	for _, i := range provs {
		if i.Counts.CatalogProviders == 0 {
			t.Fatalf("%q landed in the provider filter without a provider entry", i.Name)
		}
	}
	if len(provs) < 30 {
		t.Fatalf("the provider filter should hold the vendors and relays, got %d", len(provs))
	}
	byQuery := filterPlugins(infos, "", "gemini")
	if len(byQuery) == 0 {
		t.Fatal("the name query should find the Gemini plugins")
	}
	// The query searches the name, the id and the description, so it finds
	// both the Gemini preset plugins and the importer that reads Gemini
	// CLI's configuration.
	anyGeminiPreset := false
	for _, i := range byQuery {
		if strings.Contains(strings.ToLower(i.Name), "gemini") {
			anyGeminiPreset = true
		}
	}
	if !anyGeminiPreset {
		t.Fatalf("no preset matched the name: %v", byQuery)
	}
	// The query also searches the description, where the importer names its
	// sources: a search for an agent's name finds the importer that reads
	// its configuration.
	byDesc := filterPlugins(infos, "", "cursor")
	found := false
	for _, i := range byDesc {
		if i.ID == "importer-cursor-config" {
			found = true
		}
	}
	if !found {
		t.Fatal("a search naming a source agent should find the importer")
	}
	// Built-in vs installed.
	builtin := filterPlugins(infos, "builtin", "")
	for _, i := range builtin {
		if !i.Builtin {
			t.Fatalf("%q is filtered as built-in but is not", i.Name)
		}
	}
	// Disabled entries stay listed (a catalog off is still on the books).
	if err := cw.plugins.SetEnabled("acp-cursor", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	infos = cw.plugins.List()
	stillListed := false
	for _, i := range infos {
		if i.ID == "acp-cursor" {
			stillListed = true
		}
	}
	if !stillListed {
		t.Fatal("a disabled plugin must stay listed, ready to enable again")
	}
}
