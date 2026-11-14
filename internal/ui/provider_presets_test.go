package ui

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
)

// Every preset names a provider type the app knows and, for a custom one, an
// API type it speaks; and every remote preset carries the address its API
// answers at, so adding one only asks for the key — the point of the
// catalog, as it is of magpie's presets.
func TestProviderPresetsAreComplete(t *testing.T) {
	known := map[string]bool{}
	for _, typ := range config.ProviderTypes() {
		known[typ] = true
	}
	apiKnown := map[string]bool{}
	for _, typ := range config.APITypes() {
		apiKnown[typ] = true
	}
	for _, e := range providerPresets() {
		if !known[e.Type] {
			t.Fatalf("%q: unknown provider type %q", e.Name, e.Type)
		}
		if e.Type == config.ProviderTypeCustom && e.APIType != "" && !apiKnown[e.APIType] {
			t.Fatalf("%q: unknown custom api type %q", e.Name, e.APIType)
		}
		if e.BaseURL == "" && e.Kind != presetLocal {
			t.Fatalf("%q: a remote preset needs its API address", e.Name)
		}
	}
	// The three groups each hold something.
	counts := map[string]int{}
	for _, e := range providerPresets() {
		counts[e.Kind]++
	}
	for _, kind := range providerPresetKinds() {
		if counts[kind] == 0 {
			t.Fatalf("the %q group is empty", kind)
		}
	}
}

// Adding from the catalog appends a provider preconfigured with the
// preset's protocol and address, disabled until the key is pasted, and
// opens its form. A name already taken gets a suffix rather than a clash.
func TestAddProviderFromPreset(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.cfg.Providers = nil // a bare window: the catalog is the only source
	e := providerPresetOf(t, "智谱 GLM")
	cw.addProviderFromPreset(e)
	if len(cw.cfg.Providers) != 1 {
		t.Fatalf("the provider was not added: %+v", cw.cfg.Providers)
	}
	p := cw.cfg.Providers[0]
	if p.Type != config.ProviderTypeCustom || p.APIType != config.ProviderTypeOpenAI {
		t.Fatalf("the protocol was not carried: %+v", p)
	}
	if p.BaseURL != e.BaseURL {
		t.Fatalf("the address was not carried: %+v", p)
	}
	if p.Enabled {
		t.Fatal("a fresh preset provider must start disabled until the key is in")
	}
	if cw.settings.prov.editing != 0 || cw.settings.prov.baseURL != e.BaseURL {
		t.Fatalf("the form should open on the new provider: %+v", cw.settings.prov)
	}

	// Adding the same preset again does not clash with the first.
	cw.presetCatalogOpen = true
	cw.addProviderFromPreset(e)
	if len(cw.cfg.Providers) != 2 {
		t.Fatalf("the second add failed: %+v", cw.cfg.Providers)
	}
	if cw.cfg.Providers[1].Name == e.Name {
		t.Fatalf("the name should have been suffixed: %q", cw.cfg.Providers[1].Name)
	}
}

// The catalog dialog renders: the groups show and one click adds.
func TestProviderPresetCatalogDialogRenders(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = 0
	cw.presetCatalogOpen = true
	tr := mgo.NewTester(cw.view, 1000, 800)
	tr.Frame()
	for _, want := range []string{"从目录添加模型", presetVendor, presetRelay, presetLocal, "OpenAI", "OpenRouter", "Ollama"} {
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

// providerPresetOf finds a preset by name.
func providerPresetOf(t *testing.T, name string) providerPreset {
	t.Helper()
	for _, e := range providerPresets() {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no %q preset", name)
	return providerPreset{}
}
