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

// The built-in ACP catalog plugins carry the shipped agent list through the
// manager: every entry names a command, and the paseo-checked rules hold —
// Cursor runs cursor-agent, Gemini and Qwen answer --acp, Cline speaks ACP
// natively.
func TestACPCatalogFromPlugins(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	ensureBuiltinCatalog(t, cw, "acp-catalog-mainstream")
	ensureBuiltinCatalog(t, cw, "acp-catalog-extended")
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
	// Groups: the two plugins' groups both show.
	groups := plugin.CatalogGroupOrder(entries, func(e plugin.ACPPresetEntry) string { return e.Group })
	if len(groups) != 2 {
		t.Fatalf("expected the mainstream and extended groups, got %v", groups)
	}
}

// Disabling a catalog plugin removes its entries from the dialog's list —
// the settings page is where a user hides a whole group of suggestions.
func TestACPCatalogDisabledPluginHidden(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	ensureBuiltinCatalog(t, cw, "acp-catalog-mainstream")
	ensureBuiltinCatalog(t, cw, "acp-catalog-extended")
	if err := cw.plugins.SetEnabled("acp-catalog-extended", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	entries := cw.pluginACPCatalog()
	for _, e := range entries {
		if e.Group == "扩展 Agent" {
			t.Fatalf("a disabled plugin's group must be absent, found %q", e.Name)
		}
	}
	// The mainstream group survives.
	found := false
	for _, e := range entries {
		if e.Name == "ClaudeCode" {
			found = true
		}
	}
	if !found {
		t.Fatal("the mainstream group vanished with the extended plugin off")
	}
}

// addACPFromCatalog still lands the entry: enabled when the command can
// run, disabled with a hint when it cannot.
func TestAddACPFromCatalogLandsTheEntry(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	ensureBuiltinCatalog(t, cw, "acp-catalog-mainstream")
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
