package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"
)

// importTestHome points HOME at a temp dir holding one agent configuration of
// each kind, so a scan has something to find without touching the real home.
func importTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".claude.json", `{"mcpServers":{"memory":{"command":"npx","args":["-y","m"]}}}`)
	write(".config/opencode/opencode.json", `{
	  "mcp": {"oc": {"type": "remote", "url": "http://127.0.0.1/mcp"}},
	  "provider": {"deepseek": {"options": {"apiKey": "sk-x", "baseURL": "https://api.deepseek.com/v1"}, "models": {"deepseek-chat": {}}}}
	}`)
	write(".claude/skills/demo/SKILL.md", "---\nname: demo\ndescription: 演示\n---\n\nbody")
	return home
}

func TestScanImportableGroupsBySource(t *testing.T) {
	importTestHome(t)

	mcp := scanImportable(importMCP, "")
	if len(mcp) != 2 {
		t.Fatalf("expected two MCP sources, got %+v", mcp)
	}
	seen := map[string]int{}
	for _, g := range mcp {
		seen[g.From] = len(g.Items)
	}
	if seen["Claude Code"] != 1 || seen["OpenCode"] != 1 {
		t.Fatalf("unexpected grouping %+v", seen)
	}

	providers := scanImportable(importProviders, "")
	if len(providers) != 1 || len(providers[0].Items) != 1 {
		t.Fatalf("expected one provider, got %+v", providers)
	}
	if providers[0].Items[0].Title != "deepseek" {
		t.Fatalf("unexpected provider %+v", providers[0].Items[0])
	}

	skills := scanImportable(importSkills, "")
	if len(skills) != 1 || skills[0].Items[0].Title != "demo" {
		t.Fatalf("expected the claude skill, got %+v", skills)
	}
}

func TestRunMCPImportAddsDisabledServers(t *testing.T) {
	importTestHome(t)
	cw := testWindow()
	cw.imp = importState{open: true, kind: importMCP, chosen: map[string]bool{}}
	cw.imp.groups = scanImportable(importMCP, "")
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			cw.imp.chosen[it.ID] = true
		}
	}
	before := len(cw.cfg.MCPServers)
	cw.runMCPImport()
	added := cw.cfg.MCPServers[before:]
	if len(added) != 2 {
		t.Fatalf("expected 2 servers, got %+v", added)
	}
	for _, s := range added {
		if s.Enabled {
			t.Fatalf("an imported server must start disabled: %+v", s)
		}
		if s.Type == "" {
			t.Fatalf("an imported server needs a type: %+v", s)
		}
	}
}

func TestRunProviderImportCreatesCustomProvider(t *testing.T) {
	importTestHome(t)
	cw := testWindow()
	cw.imp = importState{open: true, kind: importProviders, chosen: map[string]bool{}}
	cw.imp.groups = scanImportable(importProviders, "")
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			cw.imp.chosen[it.ID] = true
		}
	}
	before := len(cw.cfg.Providers)
	cw.runProviderImport()
	added := cw.cfg.Providers[before:]
	if len(added) != 1 {
		t.Fatalf("expected one provider, got %+v", added)
	}
	p := added[0]
	if p.Type != config.ProviderTypeCustom || p.APIType != "openai" {
		t.Fatalf("a provider should be imported as custom/openai, got %+v", p)
	}
	if p.BaseURL == "" || p.APIKey != "sk-x" || p.Model != "deepseek-chat" {
		t.Fatalf("credentials or model missing: %+v", p)
	}
	if p.Enabled {
		t.Fatal("an imported provider must start disabled")
	}
}

// TestImportNeverOverwritesExistingNames guards against an import replacing a
// server or provider the user already configured.
func TestImportNeverOverwritesExistingNames(t *testing.T) {
	importTestHome(t)
	cw := testWindow()
	cw.cfg.MCPServers = []config.MCPServer{{Name: "memory", Command: "mine"}}
	cw.cfg.Providers = []config.Provider{{Name: "deepseek", Type: "deepseek", APIKey: "mine"}}

	cw.imp = importState{open: true, kind: importMCP, chosen: map[string]bool{}}
	cw.imp.groups = scanImportable(importMCP, "")
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			cw.imp.chosen[it.ID] = true
		}
	}
	cw.runMCPImport()
	if cw.cfg.MCPServers[0].Command != "mine" {
		t.Fatalf("the existing server was overwritten: %+v", cw.cfg.MCPServers[0])
	}
	foundRenamed := false
	for _, s := range cw.cfg.MCPServers[1:] {
		if s.Name == "memory-2" {
			foundRenamed = true
		}
	}
	if !foundRenamed {
		t.Fatalf("a clashing name should be suffixed, got %+v", cw.cfg.MCPServers)
	}

	cw.imp = importState{open: true, kind: importProviders, chosen: map[string]bool{}}
	cw.imp.groups = scanImportable(importProviders, "")
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			cw.imp.chosen[it.ID] = true
		}
	}
	cw.runProviderImport()
	if cw.cfg.Providers[0].APIKey != "mine" {
		t.Fatalf("the existing provider was overwritten: %+v", cw.cfg.Providers[0])
	}
	if cw.cfg.Providers[len(cw.cfg.Providers)-1].Name != "deepseek-2" {
		t.Fatalf("a clashing provider name should be suffixed, got %+v", cw.cfg.Providers)
	}
}

func TestImportDialogRenders(t *testing.T) {
	importTestHome(t)
	for _, kind := range []importKind{importSkills, importMCP, importProviders} {
		cw := testWindow()
		cw.section = sectionSettings
		cw.imp = importState{open: true, kind: kind, chosen: map[string]bool{}}
		cw.imp.groups = scanImportable(kind, "")
		cw.imp.scanned = true
		for _, g := range cw.imp.groups {
			for _, it := range g.Items {
				cw.imp.chosen[it.ID] = true
			}
		}
		if img := mgo.Render(cw.view, 1100, 760, 1); img == nil {
			t.Fatalf("import dialog for kind %v failed to render", kind)
		}
	}
}

func TestUniqueNameSuffixes(t *testing.T) {
	taken := map[string]bool{"a": true, "a-2": true}
	got := uniqueName("a", func(n string) bool { return taken[n] })
	if got != "a-3" {
		t.Fatalf("uniqueName returned %q", got)
	}
	if got := uniqueName("free", func(string) bool { return false }); got != "free" {
		t.Fatalf("a free name should be kept, got %q", got)
	}
}

func TestModelTagsListModelsAndCap(t *testing.T) {
	if got := modelTags(nil); got != nil {
		t.Fatalf("no models should produce no tags, got %v", got)
	}
	short := []string{"a", "b", "c"}
	if got := modelTags(short); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("a short list should be shown in full, got %v", got)
	}

	// More models than fit as chips, but still all imported: the trailing
	// count must add up to what is imported.
	mid := make([]string, 0, maxImportedModels)
	for i := 0; i < maxImportedModels; i++ {
		mid = append(mid, fmt.Sprintf("m%02d", i))
	}
	got := modelTags(mid)
	if len(got) != maxModelTags+1 {
		t.Fatalf("expected %d chips, got %v", maxModelTags+1, got)
	}
	if want := fmt.Sprintf("+%d", maxImportedModels-maxModelTags); got[len(got)-1] != want {
		t.Fatalf("the overflow chip should count the imported rest, got %q want %q", got[len(got)-1], want)
	}

	// More models than are imported: the chips must say so, so they never
	// promise models the import drops.
	many := make([]string, 0, 61)
	for i := 0; i < 61; i++ {
		many = append(many, fmt.Sprintf("m%02d", i))
	}
	got = modelTags(many)
	last := got[len(got)-1]
	if !strings.Contains(last, fmt.Sprint(maxImportedModels)) {
		t.Fatalf("the chips should state the import limit, got %q", last)
	}
	sum := maxModelTags
	if n, err := parseOverflow(got[maxModelTags]); err == nil {
		sum += n
	} else {
		t.Fatalf("expected an overflow chip, got %q", got[maxModelTags])
	}
	if sum != maxImportedModels {
		t.Fatalf("the chips account for %d models but %d are imported", sum, maxImportedModels)
	}
	if len(importedModels(many)) != maxImportedModels {
		t.Fatalf("importedModels should cap at %d", maxImportedModels)
	}
}

// parseOverflow reads the "+N" chip.
func parseOverflow(tag string) (int, error) {
	var n int
	_, err := fmt.Sscanf(tag, "+%d", &n)
	return n, err
}

// TestProviderCandidatesCarryModelTags checks the import list shows a
// provider's models as chips instead of only a count.
func TestProviderCandidatesCarryModelTags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"provider": {"deepseek": {
	  "options": {"apiKey": "k", "baseURL": "https://api.deepseek.com/v1"},
	  "models": {"deepseek-chat": {}, "deepseek-reasoner": {}}
	}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	groups := scanImportable(importProviders, "")
	if len(groups) != 1 || len(groups[0].Items) != 1 {
		t.Fatalf("expected one provider, got %+v", groups)
	}
	it := groups[0].Items[0]
	if len(it.Tags) != 2 {
		t.Fatalf("the models should become tags, got %v", it.Tags)
	}
	if it.Tags[0] != "deepseek-chat" || it.Tags[1] != "deepseek-reasoner" {
		t.Fatalf("unexpected tags %v", it.Tags)
	}
	// The detail keeps the endpoint but no longer repeats the model count.
	if !strings.Contains(it.Desc, "api.deepseek.com") {
		t.Fatalf("the detail should name the endpoint, got %q", it.Desc)
	}
	if strings.Contains(it.Desc, "个模型") {
		t.Fatalf("the count is replaced by the tags, got %q", it.Desc)
	}
}
