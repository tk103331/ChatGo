package config

import (
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

func TestNeedsAPIKey(t *testing.T) {
	for _, typ := range []string{"openai", "deepseek", "claude", "gemini", "qwen", "anthropic"} {
		if !(Provider{Type: typ}).NeedsAPIKey() {
			t.Fatalf("%s should need an API key", typ)
		}
	}
	for _, typ := range []string{"ollama", ProviderTypeCustom} {
		if (Provider{Type: typ}).NeedsAPIKey() {
			t.Fatalf("%s should not need an API key", typ)
		}
	}
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
