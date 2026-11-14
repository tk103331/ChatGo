package plugin

// Built-in plugins: the catalogs the app ships — provider presets, ACP
// agents, the agent-configuration importers — packaged as plugins so a
// user can disable a whole group of suggestions from the settings page.
// Their manifests are embedded and materialised into the built-in root on
// first use; from there they load like any installed plugin.

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"

	"chatgo/internal/config"
)

// builtinFS holds the shipped plugin manifests, one directory per plugin
// with a plugin.json inside.
//
//go:embed builtin/*
var builtinFS embed.FS

// builtinRoot is where built-in plugins materialise, under their own
// subdirectory of the plugin root so a user's plugin can never shadow a
// built-in one or vice versa.
func (m *Manager) builtinRoot() string {
	return filepath.Join(m.root, ".builtin")
}

// EnsureBuiltin materialises the embedded built-in plugins (the shipped
// catalogs and importers) and registers them, then loads the enabled ones.
func (m *Manager) EnsureBuiltin() { m.ensureBuiltin() }

// ensureBuiltin materialises every embedded built-in plugin and registers
// it, the way LoadAll registers a discovered one. An embedded manifest
// whose version differs from the materialised copy is rewritten, so a
// built-in plugin updates with the app. Call it after LoadAll: the new
// directories are registered here, the existing ones were already found by
// LoadAll's walk.
func (m *Manager) ensureBuiltin() {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return
	}
	root := m.builtinRoot()
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := builtinFS.ReadFile("builtin/" + name + "/plugin.json")
		if err != nil {
			continue
		}
		dir := filepath.Join(root, name)
		if !builtinStale(data, filepath.Join(dir, "plugin.json")) {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		if err := materialiseBuiltinDir(name, dir); err != nil {
			continue
		}
	}
	// Register every built-in that is not on the books yet. Ones already
	// registered keep their persisted enabled choice.
	manifests := m.builtinManifests(root)
	if len(manifests) == 0 {
		return
	}
	m.mu.Lock()
	newRefs := false
	for _, manifest := range manifests {
		if _, exists := m.instances[manifest.ID]; exists {
			continue
		}
		ref, ok := m.cfg.FindPlugin(manifest.ID)
		if !ok {
			// Built-ins come on with the app the first time; the user's
			// disable choice persists from then on.
			ref = config.PluginRef{ID: manifest.ID, Enabled: true}
			m.cfg.Plugins = append(m.cfg.Plugins, ref)
			newRefs = true
		}
		m.instances[manifest.ID] = &InstanceState{
			Manifest: manifest,
			Dir:      filepath.Join(root, manifest.ID),
			DataDir:  filepath.Join(m.dataRoot, manifest.ID),
			Ref:      ref,
			status:   StatusDisabled,
			builtin:  true,
		}
	}
	m.mu.Unlock()
	if newRefs {
		_ = config.SaveConfig(m.cfg)
	}
	// Load the enabled built-ins that are not loaded yet — a built-in that
	// materialised this run, or one whose directory LoadAll's walk skipped
	// because .builtin starts with a dot.
	for _, manifest := range manifests {
		inst := m.instances[manifest.ID]
		if inst == nil || !inst.Ref.Enabled {
			continue
		}
		m.mu.RLock()
		loaded := inst.status != StatusDisabled
		m.mu.RUnlock()
		if loaded {
			continue
		}
		_ = m.load(inst)
	}
}

// materialiseBuiltinDir copies every file of an embedded built-in plugin
// into its materialised directory: the manifest, the data-only entry stub
// and anything else it ships.
func materialiseBuiltinDir(name, dir string) error {
	entries, err := builtinFS.ReadDir("builtin/" + name)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile("builtin/" + name + "/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// builtinStale reports whether the embedded manifest should be (re)written
// to dest: no copy exists, or its version differs from the embedded one.
func builtinStale(embedded []byte, dest string) bool {
	cur, err := os.ReadFile(dest)
	if err != nil {
		return true
	}
	var a, b struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(embedded, &a) != nil || json.Unmarshal(cur, &b) != nil {
		return true
	}
	return a.Version != b.Version
}

// builtinManifests loads the manifests of the materialised built-ins.
func (m *Manager) builtinManifests(root string) []*Manifest {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []*Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		manifest, err := LoadManifest(dir)
		if err != nil {
			continue
		}
		out = append(out, manifest)
	}
	return out
}
