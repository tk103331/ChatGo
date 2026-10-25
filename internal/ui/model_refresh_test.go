package ui

import (
	"testing"

	"chatgo/internal/config"
)

// TestModelRefreshMergeKeepsExistingModels covers the merge rule: ticked
// models are added, and a model the user configured by hand survives even
// when the endpoint did not list it. Dropping it would change which model a
// saved conversation runs on.
func TestModelRefreshMergeKeepsExistingModels(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{{
		Name: "p", Type: config.ProviderTypeOpenAI, APIKey: "k",
		Model: "hand-written", Models: []string{"hand-written"}, Enabled: true,
	}}
	cw.settings.prov = providerForm{
		editing: 0, name: "p", typ: config.ProviderTypeOpenAI, apiKey: "k",
		models: []string{"hand-written"},
	}
	cw.modelRef = modelRefreshState{
		open: true, provider: 0, name: "p",
		found:      []string{"gpt-4o", "o1-mini"},
		chosen:     map[string]bool{"gpt-4o": true},
		configured: map[string]bool{"hand-written": true},
	}

	cw.applyModelRefresh()

	got := cw.settings.prov.models
	if !containsStr(got, "hand-written") {
		t.Fatalf("a hand-written model must survive a refresh, got %v", got)
	}
	if !containsStr(got, "gpt-4o") {
		t.Fatalf("the ticked model should be added, got %v", got)
	}
	if containsStr(got, "o1-mini") {
		t.Fatalf("an unticked model must not be added, got %v", got)
	}
	if cw.modelRef.open {
		t.Fatal("applying should close the dialog")
	}
}

// TestModelRefreshIgnoresAStaleResult guards the race the index check exists
// for: a slow endpoint answering after the user moved to another provider
// must not write models into that other row.
func TestModelRefreshIgnoresAStaleResult(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{
		{Name: "a", Type: config.ProviderTypeOpenAI, APIKey: "k", Models: []string{"a-1"}},
		{Name: "b", Type: config.ProviderTypeOpenAI, APIKey: "k", Models: []string{"b-1"}},
	}
	// The form now edits provider 1, while the run was started for 0.
	cw.settings.prov = providerForm{
		editing: 1, name: "b", typ: config.ProviderTypeOpenAI, apiKey: "k",
		models: []string{"b-1"},
	}
	cw.modelRef = modelRefreshState{
		open: true, provider: 0, name: "a",
		found:  []string{"leaked"},
		chosen: map[string]bool{"leaked": true},
	}

	cw.applyModelRefresh()

	if containsStr(cw.settings.prov.models, "leaked") {
		t.Fatalf("a stale result leaked into another provider: %v", cw.settings.prov.models)
	}
	if cw.modelRef.open {
		t.Fatal("a stale run should be closed")
	}
}

// TestModelRefreshPreselectsOnlyConfigured records the default that keeps a
// gateway's hundreds of models from flooding the config: the picker starts
// with the provider's current models ticked and nothing else.
func TestModelRefreshPreselectsOnlyConfigured(t *testing.T) {
	cw := testWindow()
	cw.modelRef = modelRefreshState{
		open: true, name: "p",
		found:      []string{"keep-me", "new-1", "new-2"},
		chosen:     map[string]bool{},
		configured: map[string]bool{"keep-me": true},
	}
	for _, m := range cw.modelRef.found {
		if cw.modelRef.configured[m] {
			cw.modelRef.chosen[m] = true
		}
	}
	if n := cw.modelRefreshChosenCount(); n != 1 {
		t.Fatalf("only the configured model should start ticked, got %d", n)
	}
	// The configured entries sort first so they are not lost in a long list.
	items := cw.modelRefreshItems()
	if len(items) != 3 || items[0].ID != "keep-me" {
		t.Fatalf("configured models should lead the list, got %+v", items)
	}
	if len(items[0].Tags) == 0 {
		t.Fatal("a configured model should be tagged as such")
	}
	if len(items[1].Tags) != 0 {
		t.Fatalf("a newly found model needs no tag, got %v", items[1].Tags)
	}
}

// TestProviderFormCanListFollowsTheType keeps the refresh button disabled for
// a protocol with no listing endpoint.
func TestProviderFormCanListFollowsTheType(t *testing.T) {
	cw := testWindow()
	for _, typ := range config.ProviderTypes() {
		cw.settings.prov = providerForm{editing: 0, name: "p", typ: typ}
		if !cw.providerFormCanList() {
			t.Fatalf("type %q should allow refreshing", typ)
		}
	}
	cw.settings.prov = providerForm{
		editing: 0, name: "p", typ: config.ProviderTypeCustom, apiType: "no-such-protocol",
	}
	if cw.providerFormCanList() {
		t.Fatal("an unknown api_type must not offer a refresh")
	}
}
