package plugin

// Migration from the embedded built-in plugins to the installable catalog:
// releases before the catalog shipped every plugin materialised into
// ~/.chatgo/plugins/.builtin/. Those directories are gone from the app
// now — the plugins ship as release zips installed on demand — so the
// first run after the upgrade removes the materialised copies. The
// enabled flags stay in the configuration: a plugin the user had on comes
// back on the moment it is installed again, and the plugins page lists it
// among the installable entries either way.

import (
	"os"
	"path/filepath"

	"chatgo/internal/config"
)

// builtinRoot is where the old releases materialised the built-ins.
func (m *Manager) builtinRoot() string {
	return filepath.Join(m.root, ".builtin")
}

// RegisterCatalogForTest registers and loads every plugin of a catalog
// source directory, enabled, without downloading anything. Only tests use
// it; the app installs catalog entries through InstallCatalog.
func (m *Manager) RegisterCatalogForTest(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		manifest, err := LoadManifest(full)
		if err != nil {
			continue
		}
		m.mu.Lock()
		ref, ok := m.cfg.FindPlugin(manifest.ID)
		if !ok {
			ref = config.PluginRef{ID: manifest.ID, Enabled: true}
			m.cfg.Plugins = append(m.cfg.Plugins, ref)
		}
		inst := &InstanceState{
			Manifest: manifest, Dir: full,
			DataDir: filepath.Join(m.dataRoot, manifest.ID),
			Ref:     ref, status: StatusDisabled,
		}
		m.instances[manifest.ID] = inst
		m.mu.Unlock()
		_ = m.load(inst)
	}
}

// RemoveLegacyBuiltin deletes the materialised built-in plugins of the
// previous releases. Call it once at startup; it is a no-op when the
// directory is already gone. Registered instances of removed directories
// are dropped from the manager, their enabled flags stay in the config.
func (m *Manager) RemoveLegacyBuiltin() {
	root := m.builtinRoot()
	if _, err := os.Stat(root); err != nil {
		return
	}
	// Collect the plugin ids under the legacy root before deleting them.
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if manifest, err := LoadManifest(dir); err == nil {
			ids = append(ids, manifest.ID)
		}
	}
	if err := os.RemoveAll(root); err != nil {
		return
	}
	// Drop the instances of the removed directories. A plugin the user
	// installs from the catalog re-registers through Install.
	m.mu.Lock()
	for _, id := range ids {
		if inst, ok := m.instances[id]; ok && inst.Dir != "" && withinPath(inst.Dir, root) {
			if inst.status == StatusLoaded {
				m.removeContributionsLocked(inst)
			}
			delete(m.instances, id)
		}
	}
	m.mu.Unlock()
}
