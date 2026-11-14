package ui

import (
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/plugin"
)

// The built-in provider catalog plugins carry the shipped preset list
// through the manager, and the shipped rules hold: remote presets carry
// their address, custom ones speak a known API type.
func TestProviderCatalogFromPlugins(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	entries := cw.pluginProviderCatalog()
	if len(entries) < 30 {
		t.Fatalf("the built-in catalogs should hold the shipped list, got %d entries", len(entries))
	}
	knownTypes := map[string]bool{}
	for _, typ := range config.ProviderTypes() {
		knownTypes[typ] = true
	}
	apiKnown := map[string]bool{}
	for _, typ := range config.APITypes() {
		apiKnown[typ] = true
	}
	for _, e := range entries {
		if !knownTypes[e.Type] {
			t.Fatalf("%q: unknown provider type %q", e.Name, e.Type)
		}
		if e.Type == config.ProviderTypeCustom && e.APIType != "" && !apiKnown[e.APIType] {
			t.Fatalf("%q: unknown custom api type %q", e.Name, e.APIType)
		}
		if e.BaseURL == "" && !isLocalPreset(e) {
			t.Fatalf("%q: a remote preset needs its API address", e.Name)
		}
		if strings.TrimSpace(e.Name) == "" {
			t.Fatal("an entry without a name")
		}
	}
}

// isLocalPreset reports whether the entry is one of the local-server
// presets, whose address is the user's own and may be empty.
func isLocalPreset(e plugin.ProviderPresetEntry) bool {
	return strings.Contains(e.Plugin, "ollama") ||
		strings.Contains(e.Plugin, "lm-studio") ||
		strings.Contains(e.Plugin, "vllm") ||
		strings.Contains(e.Plugin, "lm-engine")
}

// Adding from the plugin catalog appends a provider preconfigured with the
// preset's protocol and address, disabled until the key is pasted, and
// opens its form; a taken name gets a suffix.
func TestAddProviderFromPreset(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	entries := cw.pluginProviderCatalog()
	var glm *plugin.ProviderPresetEntry
	for i := range entries {
		if entries[i].Name == "智谱 GLM" {
			glm = &entries[i]
		}
	}
	if glm == nil {
		t.Fatal("the vendors catalog lacks 智谱 GLM")
	}
	cw.cfg.Providers = nil // a bare window: the catalog is the only source
	cw.addProviderFromPreset(*glm)
	if len(cw.cfg.Providers) != 1 {
		t.Fatalf("the provider was not added: %+v", cw.cfg.Providers)
	}
	p := cw.cfg.Providers[0]
	if p.Type != config.ProviderTypeCustom || p.APIType != config.ProviderTypeOpenAI {
		t.Fatalf("the protocol was not carried: %+v", p)
	}
	if p.BaseURL != glm.BaseURL {
		t.Fatalf("the address was not carried: %+v", p)
	}
	if p.Enabled {
		t.Fatal("a fresh preset provider must start disabled until the key is in")
	}
	if cw.settings.prov.editing != 0 || cw.settings.prov.baseURL != glm.BaseURL {
		t.Fatalf("the form should open on the new provider: %+v", cw.settings.prov)
	}

	// Adding the same preset again does not clash with the first.
	cw.presetCatalogOpen = true
	cw.addProviderFromPreset(*glm)
	if len(cw.cfg.Providers) != 2 {
		t.Fatalf("the second add failed: %+v", cw.cfg.Providers)
	}
	if cw.cfg.Providers[1].Name == glm.Name {
		t.Fatalf("the name should have been suffixed: %q", cw.cfg.Providers[1].Name)
	}
}

// The catalog dialog renders its plugin groups, and one click adds.
func TestProviderPresetCatalogDialogRenders(t *testing.T) {
	isolateHome(t)
	cw := catalogTestWindow(t)
	cw.section = sectionSettings
	cw.settings.tab = 0
	cw.presetCatalogOpen = true
	tr := mgo.NewTester(cw.view, 1000, 800)
	tr.Frame()
	for _, want := range []string{"从插件添加模型", "OpenAI", "OpenRouter", "Ollama"} {
		if !tr.HasText(want) {
			t.Fatalf("the catalog lacks %q; texts: %v", want, tr.Texts())
		}
	}
	if err := tr.Click("添加"); err != nil {
		t.Fatalf("add a preset: %v", err)
	}
	tr.Frame()
	if len(cw.cfg.Providers) == 0 {
		t.Fatal("the click added nothing")
	}
}
