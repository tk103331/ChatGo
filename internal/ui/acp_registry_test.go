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
			Authors:     []string{"OpenAI", "JetBrains s.r.o", "Zed Industries"},
			Repository:  "https://github.com/agentclientprotocol/codex-acp",
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

// A row expands on click into its details — the full description, the
// authors, the links and the exact command it runs — and folds back on a
// second click. The 添加 action sits outside the expandable header, so
// adding never toggles the details.
func TestACPRegistryRowExpands(t *testing.T) {
	cw := acpRegistryWindow(t)
	tr := mgo.NewTester(cw.view, 1100, 800)
	for range 6 {
		tr.Frame()
	}
	if tr.HasText("Registry ID：") {
		t.Fatalf("the details should start folded; texts: %v", tr.Texts())
	}
	if err := tr.Click("Codex"); err != nil {
		t.Fatalf("expand the Codex row: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	for _, want := range []string{
		"作者", "OpenAI · JetBrains s.r.o · Zed Industries",
		"许可证", "仓库", "github.com/agentclientprotocol/codex-acp",
		"npx -y @agentclientprotocol/codex-acp@2.6.0",
		"Registry ID：codex-acp",
	} {
		if !tr.HasText(want) {
			t.Fatalf("the expanded details lack %q; texts: %v", want, tr.Texts())
		}
	}
	// A binary-only entry names its archive targets instead of a command.
	if err := tr.Click("Binary Only"); err != nil {
		t.Fatalf("expand the binary-only row: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if !tr.HasText("二进制") || !tr.HasText("macOS · Apple Silicon") {
		t.Fatalf("the binary targets should be named; texts: %v", tr.Texts())
	}
	// The second click folds the details away.
	if err := tr.Click("Codex"); err != nil {
		t.Fatalf("fold the Codex row: %v", err)
	}
	for range 3 {
		tr.Frame()
	}
	if tr.HasText("Registry ID：codex-acp") {
		t.Fatalf("the details should have folded; texts: %v", tr.Texts())
	}
}

// The entry's brand icon loads from the disk cache without the network: the
// fetch reads the cached SVG into memory, and the row then draws it instead
// of the bundled fallback. The registry icon overrides the local asset, so
// every entry can carry its own mark even when ChatGo bundles none.
func TestACPRegistryIconFromDiskCache(t *testing.T) {
	isolateHome(t)
	resetACPRegistryIcons()
	agents := registryAgents()
	agents[0].Icon = "https://cdn.example/codex-acp.svg"
	// A previous launch cached the icon.
	path := acpRegistryIconFile(agents[0].ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`)
	if err := os.WriteFile(path, svg, 0o644); err != nil {
		t.Fatal(err)
	}

	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsACPTab
	cw.acpReg.open = true
	cw.acpReg.checked = true
	cw.acpReg.agents = agents

	// The synchronous load fills the memory cache from disk, the way the
	// background fetch does in the app.
	cw.fetchACPRegistryIcon(agents[0])
	acpRegIconMu.Lock()
	_, loaded := acpRegIconCache[agents[0].Icon]
	acpRegIconMu.Unlock()
	if !loaded {
		t.Fatal("the cached icon should have loaded from disk")
	}

	tr := mgo.NewTester(cw.view, 1100, 800)
	for range 6 {
		tr.Frame()
	}
	if !tr.HasText("Codex") {
		t.Fatalf("the row should render; texts: %v", tr.Texts())
	}
	// The icon was served from cache: no fetch was started for it.
	acpRegIconMu.Lock()
	tried := acpRegIconTried[agents[0].Icon]
	acpRegIconMu.Unlock()
	if tried {
		t.Fatal("a served icon should not start another fetch")
	}
}

// resetACPRegistryIcons empties the icon caches between tests.
func resetACPRegistryIcons() {
	acpRegIconMu.Lock()
	acpRegIconCache = map[string]*mgo.SVG{}
	acpRegIconTried = map[string]bool{}
	acpRegIconMu.Unlock()
}
