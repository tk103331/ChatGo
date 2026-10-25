package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveAPIType(t *testing.T) {
	custom := Provider{Type: ProviderTypeCustom}
	if got := custom.EffectiveAPIType(); got != "openai" {
		t.Fatalf("empty custom api type should fall back to openai, got %s", got)
	}
	custom.APIType = "claude"
	if got := custom.EffectiveAPIType(); got != "claude" {
		t.Fatalf("got %s", got)
	}
	openai := Provider{Type: "openai", APIType: "gemini"}
	if got := openai.EffectiveAPIType(); got != "openai" {
		t.Fatalf("non-custom provider must ignore api_type, got %s", got)
	}
}

func TestProviderPresetsIncludeCustom(t *testing.T) {
	for _, p := range ProviderPresets() {
		if p.Type == ProviderTypeCustom {
			if p.APIType != "openai" {
				t.Fatalf("custom preset should default to openai api type, got %q", p.APIType)
			}
			return
		}
	}
	t.Fatal("custom preset missing")
}

func TestProviderGroupModelsAndStrategy(t *testing.T) {
	g := ProviderGroup{
		Name: "g",
		Members: []GroupMember{
			{Provider: "a", Models: []string{"claude-sonnet-4-5", "gpt-4o"}},
			{Provider: "b", Models: []string{"claude-sonnet-4-5"}},
		},
	}
	if got := g.EffectiveStrategy(); got != StrategyFallback {
		t.Fatalf("default strategy should be fallback, got %s", got)
	}
	models := g.ModelsList()
	if len(models) != 2 || models[0] != "claude-sonnet-4-5" || models[1] != "gpt-4o" {
		t.Fatalf("unexpected group models %v", models)
	}
	if !g.MatchModels("gpt-4o") || g.MatchModels("unknown") {
		t.Fatalf("MatchModels mismatch")
	}
	if got := g.MemberCount(); got != 2 {
		t.Fatalf("MemberCount got %d", got)
	}
	routes := g.Routes("gpt-4o", nil)
	if len(routes) != 2 || routes[0].Provider != "a" {
		t.Fatalf("unexpected routes %+v", routes)
	}
	if routes[0].Models[0] != "gpt-4o" || routes[0].Models[1] != "claude-sonnet-4-5" {
		t.Fatalf("requested model should be tried first, got %v", routes[0].Models)
	}
	if routes[1].Models[0] != "gpt-4o" {
		t.Fatalf("a member without the model should fall back to the requested one, got %v", routes[1].Models)
	}
	if last := g.Routes("", func(int) int { return 1 }); last[0].Provider != "b" {
		t.Fatalf("pick should choose the start member, got %+v", last[0])
	}
}

func TestEnabledGroups(t *testing.T) {
	c := &Config{ProviderGroups: []ProviderGroup{
		{Name: "on", Enabled: true, Members: []GroupMember{{Provider: "a", Models: []string{"m"}}}},
		{Name: "off", Enabled: false, Members: []GroupMember{{Provider: "a", Models: []string{"m"}}}},
		{Name: "empty", Enabled: true},
	}}
	groups := c.EnabledGroups()
	if len(groups) != 1 || groups[0].Name != "on" {
		t.Fatalf("unexpected enabled groups %+v", groups)
	}
	if _, ok := c.FindProviderGroup("off"); !ok {
		t.Fatal("FindProviderGroup should find disabled groups")
	}
}

func TestValidateExplainsMissingConfigInChinese(t *testing.T) {
	// A provider type that authenticates must say the API key is missing
	// rather than leak the SDK's environment-variable wording.
	for _, typ := range []string{"openai", "deepseek", "qwen", "claude", "anthropic", "gemini"} {
		p := Provider{Name: "线上", Type: typ, Model: "m"}
		err := p.Validate()
		if err == nil {
			t.Fatalf("%s without an API key must be rejected", typ)
		}
		if !strings.Contains(err.Error(), "API Key") || !strings.Contains(err.Error(), "线上") {
			t.Fatalf("%s message should name the provider and the API key, got %q", typ, err)
		}
	}
	// Ollama runs locally and needs no key.
	if err := (Provider{Name: "本地", Type: "ollama", Model: "llama3"}).Validate(); err != nil {
		t.Fatalf("ollama should not require an API key: %v", err)
	}
	// A custom provider may target an unauthenticated endpoint, and an empty
	// base URL falls back to the protocol's official host, so neither is
	// required.
	if err := (Provider{Name: "代理", Type: ProviderTypeCustom, Model: "m"}).Validate(); err != nil {
		t.Fatalf("a custom provider should not require a key or base URL: %v", err)
	}
	// A missing model name is reported before anything else.
	err := (Provider{Name: "空", Type: "openai", APIKey: "k"}).Validate()
	if err == nil || !strings.Contains(err.Error(), "模型名") {
		t.Fatalf("a provider without a model should say so, got %v", err)
	}
}

// TestProviderTypesMatchPresets locks the single source of the type list: the
// settings dropdown is built from ProviderTypes, so a type offered there
// without a shipped preset would open an empty form, and a preset whose type
// is missing from the list would make the dropdown blank out an existing
// provider and persist the wrong type on the next edit.
func TestProviderTypesMatchPresets(t *testing.T) {
	types := ProviderTypes()
	seen := map[string]bool{}
	for _, typ := range types {
		if typ == "" {
			t.Fatal("an empty provider type must not be offered")
		}
		if seen[typ] {
			t.Fatalf("provider type %q is listed twice", typ)
		}
		seen[typ] = true
	}
	for _, p := range ProviderPresets() {
		if !seen[p.Type] {
			t.Fatalf("preset %q has type %q, which the dropdown does not offer", p.Name, p.Type)
		}
	}
	// APITypeOpenAIResponses is reachable only as a custom api_type, so
	// offering it as a provider type would build a client that no preset
	// documents.
	if seen[APITypeOpenAIResponses] {
		t.Fatalf("%q is an api_type, not a provider type", APITypeOpenAIResponses)
	}
	if !containsString(APITypes(), APITypeOpenAIResponses) {
		t.Fatalf("%q must stay selectable as a custom api_type", APITypeOpenAIResponses)
	}
}

// TestProviderTypeConstantsKeepTheirWireValues pins the constants to the
// strings already written into every user's config.yaml: renaming one would
// silently orphan their configured providers.
func TestProviderTypeConstantsKeepTheirWireValues(t *testing.T) {
	for want, got := range map[string]string{
		"openai":           ProviderTypeOpenAI,
		"anthropic":        ProviderTypeAnthropic,
		"claude":           ProviderTypeClaude,
		"ollama":           ProviderTypeOllama,
		"qwen":             ProviderTypeQwen,
		"deepseek":         ProviderTypeDeepSeek,
		"gemini":           ProviderTypeGemini,
		"magpie":           ProviderTypeMagpie,
		"custom":           ProviderTypeCustom,
		"openai_responses": APITypeOpenAIResponses,
	} {
		if got != want {
			t.Fatalf("constant changed wire value: got %q, want %q", got, want)
		}
	}
}

func TestNeedsAPIKey(t *testing.T) {
	for _, typ := range []string{"openai", "deepseek", "claude", "gemini", "qwen", "anthropic"} {
		if !(Provider{Type: typ}).NeedsAPIKey() {
			t.Fatalf("%s should need an API key", typ)
		}
	}
	for _, typ := range []string{"ollama", ProviderTypeMagpie, ProviderTypeCustom} {
		if (Provider{Type: typ}).NeedsAPIKey() {
			t.Fatalf("%s should not need an API key", typ)
		}
	}
}

// TestMagpiePreset records that the magpie preset is usable as shipped: the
// gateway runs on this machine, so the preset carries its address and the
// provider passes validation with no key, only a model to pick.
func TestMagpiePreset(t *testing.T) {
	var preset Provider
	for _, p := range ProviderPresets() {
		if p.Type == ProviderTypeMagpie {
			preset = p
			break
		}
	}
	if preset.Type == "" {
		t.Fatal("magpie preset missing")
	}
	if preset.BaseURL != MagpieBaseURL {
		t.Fatalf("magpie preset should point at the local gateway, got %q", preset.BaseURL)
	}
	if preset.APIType != "" {
		t.Fatalf("magpie is a type of its own and needs no api_type, got %q", preset.APIType)
	}
	if preset.Enabled {
		t.Fatal("a shipped preset must stay disabled")
	}
	preset.Model = "deepseek/deepseek-v4-pro"
	if err := preset.Validate(); err != nil {
		t.Fatalf("magpie with a model and no key should pass: %v", err)
	}
}

// TestMagpieStaysEnabledWithoutKey guards the load-time rule that disables an
// enabled provider with no API key: the local gateway takes none, so a user
// who enabled magpie must find it still enabled on the next start.
func TestMagpieStaysEnabledWithoutKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "providers:\n" +
		"  - name: magpie\n" +
		"    type: magpie\n" +
		"    base_url: " + MagpieBaseURL + "\n" +
		"    model: group/joycode\n" +
		"    enabled: true\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	for _, p := range cfg.Providers {
		if p.Type != ProviderTypeMagpie || p.Name != "magpie" {
			continue
		}
		if !p.Enabled {
			t.Fatal("magpie without an API key must stay enabled")
		}
		if len(p.Models) != 1 || p.Models[0] != "group/joycode" {
			t.Fatalf("the single model should seed the model list, got %v", p.Models)
		}
		return
	}
	t.Fatal("the magpie provider was dropped")
}

// TestCustomProviderMayOmitAPIKey records a deliberate choice: a custom
// provider can point at a local, unauthenticated endpoint such as vLLM or LM
// Studio, so an empty API key is accepted and any rejection is left to the
// upstream. Requiring a key here would break those setups.
func TestCustomProviderMayOmitAPIKey(t *testing.T) {
	for _, apiType := range APITypes() {
		p := Provider{
			Name: "本地", Type: ProviderTypeCustom, APIType: apiType,
			Model: "m", BaseURL: "http://127.0.0.1:8000/v1",
		}
		if p.NeedsAPIKey() {
			t.Fatalf("custom/%s must not require an API key", apiType)
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("custom/%s without a key should pass: %v", apiType, err)
		}
	}
	// A native type still requires one.
	if !(Provider{Type: "openai"}).NeedsAPIKey() {
		t.Fatal("a native provider type must require an API key")
	}
}
