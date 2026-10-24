package config

import "testing"

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

func TestUsesConversion(t *testing.T) {
	cases := []struct {
		apiType string
		want    bool
	}{
		{"", false},
		{"openai", false},
		{"claude", true},
		{"gemini", true},
		{"openai_responses", true},
	}
	for _, c := range cases {
		p := Provider{Type: ProviderTypeCustom, APIType: c.apiType}
		if got := p.UsesConversion(); got != c.want {
			t.Fatalf("api type %q: got %v want %v", c.apiType, got, c.want)
		}
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
