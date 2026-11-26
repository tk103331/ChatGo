package a2ui

import "testing"

func TestRenderPreferenceRoundTrip(t *testing.T) {
	meta := map[string]any{"extensions": map[string]any{"org/other": map[string]any{"x": 1.0}}}
	meta = WithRenderPreference(meta, RenderPreference{Mode: RenderWeb, Bundle: "basic"})
	pref := RenderPreferenceOf(meta)
	if pref.Mode != RenderWeb || pref.Bundle != "basic" {
		t.Fatalf("pref = %#v", pref)
	}
	ext := meta["extensions"].(map[string]any)
	if ext["org/other"] == nil {
		t.Fatal("other extension lost")
	}
	if pref := RenderPreferenceOf(nil); pref.Mode != "" {
		t.Fatalf("nil pref = %#v", pref)
	}
}
