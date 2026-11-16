package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/acpregistry"
	"chatgo/internal/config"
)

// registryAgents is a fixture slice of the index: an npx entry, a uvx entry
// and a binary-only one.
func registryAgents() []acpregistry.Agent {
	return []acpregistry.Agent{
		{ID: "codex-acp", Name: "Codex", Version: "2.6.0", License: "Apache-2.0",
			Description: "ACP adapter for OpenAI's coding assistant",
			Distribution: acpregistry.Distribution{Npx: &acpregistry.NpxDist{
				Package: "@agentclientprotocol/codex-acp@2.6.0"}}},
		{ID: "fast-agent", Name: "fast-agent", Version: "0.10.1",
			Distribution: acpregistry.Distribution{Uvx: &acpregistry.UvxDist{
				Package: "fast-agent-acp==0.10.1", Args: []string{"-x"},
				Env: map[string]string{"FAST_AGENT_MODEL": "codexplan"}}}},
		{ID: "binary-only", Name: "Binary Only", Version: "1.0.0",
			Distribution: acpregistry.Distribution{Binary: map[string]acpregistry.BinaryDist{
				acpregistry.PlatformKey("darwin", "arm64"): {Archive: "https://example.com/b.tar.gz", Cmd: "./b"}}}},
	}
}

// acpRegistryWindow is a settings window with the ACP tab up and the
// Registry dialog holding the fixture index.
func acpRegistryWindow(t *testing.T) *ChatWindow {
	t.Helper()
	isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsACPTab
	cw.acpReg.open = true
	cw.acpReg.checked = true // the background refresh is started by hand
	cw.acpReg.agents = registryAgents()
	return cw
}

// The dialog lists the index: each entry with its name, version and
// description; a binary-only entry is named but not installable, and an
// entry already running as a connection reads as added — the shipped
// CodexCLI preset runs the same npm package as the registry's Codex.
func TestACPRegistryDialogListsAndMarks(t *testing.T) {
	cw := acpRegistryWindow(t)
	cw.cfg.ACPAgents = []config.ACPConnection{{
		Name: "CodexCLI", Command: "npx",
		Args: []string{"-y", "@agentclientprotocol/codex-acp"},
	}}
	tr := mgo.NewTester(cw.view, 1100, 800)
	for range 6 {
		tr.Frame()
	}
	for _, want := range []string{"从 ACP Registry 添加", "Codex", "v2.6.0", "Apache-2.0",
		"fast-agent", "Binary Only", "仅二进制发行", "已添加"} {
		if !tr.HasText(want) {
			t.Fatalf("the dialog should show %q; texts: %v", want, tr.Texts())
		}
	}

	// The search narrows the list to the matching entry.
	cw.acpReg.query = "fast"
	tr.Frame()
	if tr.HasText("v2.6.0") {
		t.Fatalf("the search should hide non-matching entries; texts: %v", tr.Texts())
	}
	if !tr.HasText("v0.10.1") {
		t.Fatalf("the matching entry is missing; texts: %v", tr.Texts())
	}
}

// Adding an entry creates the connection with the registry's pinned package,
// its environment and its recorded ID, then saves; a name that is taken gets
// a counter, and the row turns into 已添加.
func TestACPRegistryAddCreatesTheConnection(t *testing.T) {
	cw := acpRegistryWindow(t)
	cw.cfg.ACPAgents = []config.ACPConnection{{
		Name: "fast-agent", Command: "uvx", Args: []string{"fast-agent-acp==0.9.0"},
	}}
	tr := mgo.NewTester(cw.view, 1100, 800)
	for range 6 {
		tr.Frame()
	}

	// The uvx entry counts as added through its package, whatever pin each
	// side carries.
	if !tr.HasText("已添加") {
		t.Fatalf("an existing same-package connection should read as added; texts: %v", tr.Texts())
	}

	// The Codex entry is not installed yet.
	if err := tr.Click("添加"); err != nil {
		t.Fatalf("add the Codex entry: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if len(cw.cfg.ACPAgents) != 2 {
		t.Fatalf("the entry should have been added, got %+v", cw.cfg.ACPAgents)
	}
	conn := cw.cfg.ACPAgents[1]
	if conn.Name != "Codex" || conn.Command != "npx" {
		t.Fatalf("the connection should run npx as Codex, got %+v", conn)
	}
	if got := conn.Args; len(got) != 2 || got[0] != "-y" || got[1] != "@agentclientprotocol/codex-acp@2.6.0" {
		t.Fatalf("the pinned package should carry over, got %v", got)
	}
	if conn.RegistryID != "codex-acp" {
		t.Fatalf("the registry ID should be recorded, got %q", conn.RegistryID)
	}
	// The connection was saved, not only shown.
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".chatgo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "@agentclientprotocol/codex-acp@2.6.0") ||
		!strings.Contains(string(data), "registry_id: codex-acp") {
		t.Fatalf("the connection should be persisted: %s", data)
	}

	// Adding again takes the free name; a taken one gets a counter.
	cw.acpReg.agents[0].ID = "codex-acp-2"
	cw.addACPRegistryAgent(cw.acpReg.agents[0])
	if got := cw.cfg.ACPAgents[len(cw.cfg.ACPAgents)-1].Name; got != "Codex 2" {
		t.Fatalf("a taken name should get a counter, got %q", got)
	}
}

// Opening the dialog with no fetched index reads the disk cache, so the
// list shows without waiting for the network.
func TestACPRegistryOpensFromCache(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsACPTab
	// A previous launch cached the index document.
	reg := acpregistry.Registry{Version: "1.0.0", Agents: registryAgents()}
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".chatgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".chatgo", "acp-registry.json"),
		append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	cw.acpReg.open = true
	cw.acpReg.checked = true // no background refresh: this test reads the cache only
	loadACPRegistryCache(&cw.acpReg)
	if len(cw.acpReg.agents) != len(reg.Agents) {
		t.Fatalf("the cache should have loaded, got %d agents", len(cw.acpReg.agents))
	}
	// The cached list shows right away; the background refresh improving on
	// it never blanks the dialog.
	tr := mgo.NewTester(cw.view, 1100, 800)
	for range 6 {
		tr.Frame()
	}
	if !tr.HasText("Codex") || !tr.HasText("Binary Only") {
		t.Fatalf("the cached index should list; texts: %v", tr.Texts())
	}
}
