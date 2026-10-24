package ui

import (
	"testing"

	"chatgo/internal/config"
)

// pairingWindow has two providers whose model names are unmistakable.
func pairingWindow() *ChatWindow {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{
		{Name: "DeepSeek", Type: config.ProviderTypeCustom, APIType: "openai", APIKey: "k",
			BaseURL: "https://api.deepseek.com/v1",
			Model:   "deepseek-v4-flash", Models: []string{"deepseek-v4-flash"}, Enabled: true},
		{Name: "MiniMax", Type: config.ProviderTypeCustom, APIType: "openai", APIKey: "k",
			BaseURL: "https://api.minimaxi.com/v1",
			Model:   "MiniMax-M3", Models: []string{"MiniMax-M3", "MiniMax-Text"}, Enabled: true},
	}
	return cw
}

// TestSwitchingProviderDropsForeignModel guards the bug where selecting
// MiniMax still sent deepseek-v4-flash upstream, which the API rejects with
// "unknown model".
func TestSwitchingProviderDropsForeignModel(t *testing.T) {
	cw := pairingWindow()
	cw.provider, cw.model = "DeepSeek", "deepseek-v4-flash"
	cw.setupProvider("DeepSeek")
	if cw.model != "deepseek-v4-flash" {
		t.Fatalf("the provider's own model should be kept, got %q", cw.model)
	}

	cw.setupProvider("MiniMax")
	if cw.provider != "MiniMax" {
		t.Fatalf("provider not switched: %q", cw.provider)
	}
	if cw.model != "MiniMax-M3" {
		t.Fatalf("a foreign model must be replaced, got %q/%q", cw.provider, cw.model)
	}
}

// TestSwitchingProviderKeepsOwnModelChoice makes sure the fix does not reset a
// deliberate choice among the provider's own models.
func TestSwitchingProviderKeepsOwnModelChoice(t *testing.T) {
	cw := pairingWindow()
	cw.provider, cw.model = "MiniMax", "MiniMax-Text"
	cw.setupProvider("MiniMax")
	if cw.model != "MiniMax-Text" {
		t.Fatalf("the second model of the provider should be kept, got %q", cw.model)
	}
}

// TestApplyModelFromHistoryStaysConsistent covers replaying a turn: a stored
// pair from another provider must not leak into the active one.
func TestApplyModelFromHistoryStaysConsistent(t *testing.T) {
	cw := pairingWindow()
	cw.provider, cw.model = "MiniMax", "MiniMax-M3"
	cw.setupProvider("MiniMax")

	// A turn recorded earlier on DeepSeek.
	cw.applyModel("DeepSeek", "deepseek-v4-flash")
	if cw.provider != "DeepSeek" || cw.model != "deepseek-v4-flash" {
		t.Fatalf("replaying a turn should restore its own pair, got %q/%q", cw.provider, cw.model)
	}

	// A turn whose model no longer exists on that provider falls back.
	cw.applyModel("MiniMax", "deepseek-v4-flash")
	if cw.provider != "MiniMax" || cw.model != "MiniMax-M3" {
		t.Fatalf("a stale pair must be corrected, got %q/%q", cw.provider, cw.model)
	}
}

// TestSetModelRefPairsBothSides checks the model selector writes a matching
// provider and model together.
func TestSetModelRefPairsBothSides(t *testing.T) {
	cw := pairingWindow()
	cw.provider, cw.model = "DeepSeek", "deepseek-v4-flash"
	cw.setModelRef(modelRef("MiniMax", "MiniMax-Text"))
	if cw.provider != "MiniMax" || cw.model != "MiniMax-Text" {
		t.Fatalf("setModelRef got %q/%q", cw.provider, cw.model)
	}
	cw.setupProvider(cw.provider)
	if cw.model != "MiniMax-Text" {
		t.Fatalf("setup should keep the chosen model, got %q", cw.model)
	}
}

// TestApplyModelFallsBackWhenModelRemoved covers replaying an old turn whose
// model was since deleted from the provider: it must fall back to a model the
// provider still has, not send the missing name upstream.
func TestApplyModelFallsBackWhenModelRemoved(t *testing.T) {
	cw := pairingWindow()
	cw.provider, cw.model = "MiniMax", "MiniMax-M3"
	cw.setupProvider("MiniMax")

	cw.applyModel("MiniMax", "MiniMax-Retired")
	if cw.provider != "MiniMax" {
		t.Fatalf("the provider should be kept, got %q", cw.provider)
	}
	if cw.model != "MiniMax-M3" {
		t.Fatalf("a removed model must fall back to a configured one, got %q", cw.model)
	}
}
