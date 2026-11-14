package ui

// Provider presets from the plugin catalog: the model page's "从插件添加"
// button hands over to the plugins page's market tab; the entries here are
// what a plugin's catalog contributes once installed.

import (
	"chatgo/internal/plugin"
)

// providerNameTaken reports whether a provider of that name exists.
func (cw *ChatWindow) providerNameTaken(name string) bool {
	for _, p := range cw.cfg.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// pluginProviderCatalog reads the provider suggestions from the plugin
// manager, nil when no manager is up (tests without plugins).
func (cw *ChatWindow) pluginProviderCatalog() []plugin.ProviderPresetEntry {
	if cw.plugins == nil {
		return nil
	}
	return cw.plugins.ProviderCatalog()
}
