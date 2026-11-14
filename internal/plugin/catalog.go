package plugin

// Catalog contribution: add-dialog suggestions from enabled plugins. A
// catalog entry is data, not a live entity — the dialog lists it, the user
// clicks, and only then does the caller create the provider or connection.

import (
	"sort"
	"strings"
)

// CatalogKinds the add dialogs ask for.
const (
	CatalogProviders = "providers"
	CatalogACP       = "acp"
)

// ProviderPresetEntry is one model-provider suggestion.
type ProviderPresetEntry struct {
	// Plugin names the plugin that offers the group, so the dialog can say
	// where a suggestion came from and the settings page can point at the
	// plugin to disable it.
	Plugin  string
	Group   string
	Name    string
	Type    string
	APIType string
	BaseURL string
	Note    string
	KeysURL string
}

// ACPPresetEntry is one ACP-connection suggestion.
type ACPPresetEntry struct {
	Plugin  string
	Group   string
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// ProviderCatalog lists the model-provider suggestions of every enabled
// plugin, in plugin load order and manifest order within each.
func (m *Manager) ProviderCatalog() []ProviderPresetEntry {
	m.mu.RLock()
	insts := m.loadedInstancesLocked()
	m.mu.RUnlock()
	var out []ProviderPresetEntry
	for _, inst := range insts {
		for _, spec := range inst.Manifest.Catalog {
			if spec.Kind != CatalogProviders || strings.TrimSpace(spec.Name) == "" {
				continue
			}
			out = append(out, ProviderPresetEntry{
				Plugin: inst.Manifest.Name,
				Group:  firstNonEmpty(spec.Group, inst.Manifest.Name),
				Name:   spec.Name, Type: spec.Type, APIType: spec.APIType,
				BaseURL: spec.BaseURL, Note: spec.Note, KeysURL: spec.KeysURL,
			})
		}
	}
	return out
}

// ACPCatalog lists the ACP-connection suggestions of every enabled plugin.
func (m *Manager) ACPCatalog() []ACPPresetEntry {
	m.mu.RLock()
	insts := m.loadedInstancesLocked()
	m.mu.RUnlock()
	var out []ACPPresetEntry
	for _, inst := range insts {
		for _, spec := range inst.Manifest.Catalog {
			if spec.Kind != CatalogACP || strings.TrimSpace(spec.Name) == "" ||
				strings.TrimSpace(spec.Command) == "" {
				continue
			}
			out = append(out, ACPPresetEntry{
				Plugin: inst.Manifest.Name,
				Group:  firstNonEmpty(spec.Group, inst.Manifest.Name),
				Name:   spec.Name, Command: spec.Command, Args: spec.Args, Env: spec.Env,
			})
		}
	}
	return out
}

// CatalogGroupOrder lists the group names of a kind in first-seen order, so
// the dialog keeps each plugin's section together.
func CatalogGroupOrder[T any](entries []T, groupOf func(T) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		g := groupOf(e)
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		out = append(out, g)
	}
	// Alphabetical groups read better than load order when plugins vary.
	sort.Strings(out)
	return out
}
