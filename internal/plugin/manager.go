package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"chatgo/internal/config"

	"github.com/Calcium-Ion/moejs"
)

// Status values a plugin can be in.
const (
	StatusDisabled = "disabled"
	StatusLoaded   = "loaded"
	StatusError    = "error"
)

// Manager owns every installed plugin: discovery, loading, contribution
// merging and dispatch. It is safe for concurrent use.
type Manager struct {
	mu        sync.RWMutex
	cfg       *config.Config
	root      string
	dataRoot  string
	instances map[string]*InstanceState
}

// InstanceState is one plugin's live state.
type InstanceState struct {
	Manifest *Manifest
	Dir      string
	DataDir  string
	Ref      config.PluginRef

	pool *runtimePool
	mod  *moejs.Module

	storeMu sync.Mutex
	store   map[string]any

	status   string
	errMsg   string
	warnings []string

	eventCh chan eventMsg
	stopCh  chan struct{}

	// skills are the plugin's skill bodies, resolved at load time.
	skills []SkillContribution

	// builtin marks a shipped plugin materialised from the embedded set.
	builtin bool
}

// Info is the UI-facing snapshot of one plugin.
type Info struct {
	ID          string
	Name        string
	Version     string
	Author      string
	Description string
	Dir         string
	DataDir     string
	Status      string
	Error       string
	Warnings    []string
	Permissions []string
	Missing     []string
	Settings    []SettingField
	Values      map[string]string
	Source      config.PluginSource
	Counts      Counts
	// Builtin marks a shipped plugin: its catalog entries and importers
	// come with the app, and the plugins page badges it so a user knows it
	// is not one they installed.
	Builtin bool
}

// Counts is how many items each contribution kind provides.
type Counts struct {
	Tools, Providers, Middleware, MCPServers, ACPAgents, Agents int
	Skills, Themes, Commands, Contexts, Triggers, Channels      int
	Events, Importers                                           int
	// CatalogProviders and CatalogACP count the add-dialog suggestions.
	CatalogProviders, CatalogACP int
}

// NewManager builds a manager over the default plugin directories.
func NewManager(cfg *config.Config) *Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	root := filepath.Join(home, ".chatgo", "plugins")
	dataRoot := filepath.Join(home, ".chatgo", "plugin-data")
	return &Manager{
		cfg:       cfg,
		root:      root,
		dataRoot:  dataRoot,
		instances: map[string]*InstanceState{},
	}
}

// Root is the directory plugins are installed into.
func (m *Manager) Root() string { return m.root }

// Config returns the manager's live configuration pointer.
func (m *Manager) Config() *config.Config { return m.cfg }

// LoadAll discovers installed plugins and loads the enabled ones. It must run
// before MCP servers start, because plugin MCP contributions are merged into
// the configuration here.
func (m *Manager) LoadAll() {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(m.root, e.Name())
		manifest, err := LoadManifest(dir)
		if err != nil {
			// A directory without a valid manifest is not a plugin. Report it
			// only when it was installed by us (a persisted ref exists).
			m.recordBroken(dir, err)
			continue
		}
		m.mu.Lock()
		ref, ok := m.cfg.FindPlugin(manifest.ID)
		if !ok {
			ref = config.PluginRef{ID: manifest.ID, Enabled: false}
			m.cfg.Plugins = append(m.cfg.Plugins, ref)
			_ = config.SaveConfig(m.cfg)
		}
		inst := &InstanceState{
			Manifest: manifest,
			Dir:      dir,
			DataDir:  filepath.Join(m.dataRoot, manifest.ID),
			Ref:      ref,
			status:   StatusDisabled,
		}
		m.instances[manifest.ID] = inst
		m.mu.Unlock()
		if ref.Enabled {
			_ = m.load(inst)
		}
	}
}

// recordBroken keeps a placeholder so the settings page can show why an
// installed plugin failed, without inventing a fake manifest.
func (m *Manager) recordBroken(dir string, cause error) {
	id := filepath.Base(dir)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cfg.FindPlugin(id); !ok {
		return
	}
	m.instances[id] = &InstanceState{
		Manifest: &Manifest{ID: id, Name: id},
		Dir:      dir,
		DataDir:  filepath.Join(m.dataRoot, id),
		status:   StatusError,
		errMsg:   cause.Error(),
	}
}

// load compiles and activates one instance.
func (m *Manager) load(inst *InstanceState) error {
	source, err := os.ReadFile(filepath.Join(inst.Dir, filepath.FromSlash(inst.Manifest.Main)))
	if err != nil {
		return m.setError(inst, err)
	}
	var mod *moejs.Module
	if strings.HasSuffix(strings.ToLower(inst.Manifest.Main), ".ts") {
		mod, err = moejs.CompileTS(inst.Manifest.Main, string(source))
	} else {
		mod, err = moejs.Compile(inst.Manifest.Main, string(source))
	}
	if err != nil {
		return m.setError(inst, err)
	}
	memory := 0
	if inst.Manifest.Limits != nil {
		memory = inst.Manifest.Limits.MemoryMB
	}
	inst.mod = mod
	inst.pool = newRuntimePool(mod, memory, 2)
	// Compile alone does not run the module; one throwaway runtime proves the
	// top level and every hook load.
	probe, err := inst.pool.acquire(context.Background())
	if err != nil {
		return m.setError(inst, err)
	}
	inst.pool.release(probe)
	inst.loadStore()

	m.mu.Lock()
	m.removeContributionsLocked(inst)
	inst.warnings = nil
	if err := m.applyContributionsLocked(inst); err != nil {
		m.removeContributionsLocked(inst)
		inst.status = StatusError
		inst.errMsg = err.Error()
		m.mu.Unlock()
		return err
	}
	inst.status = StatusLoaded
	inst.errMsg = ""
	if inst.stopCh != nil {
		close(inst.stopCh)
		inst.stopCh = nil
		inst.eventCh = nil
	}
	if len(inst.Manifest.Events) > 0 {
		inst.eventCh = make(chan eventMsg, 1024)
		inst.stopCh = make(chan struct{})
		go inst.eventLoop()
	}
	m.mu.Unlock()
	return nil
}

// setError marks an instance failed and records why.
func (m *Manager) setError(inst *InstanceState, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeContributionsLocked(inst)
	inst.status = StatusError
	inst.errMsg = err.Error()
	return err
}

// Reload rebuilds one plugin from disk.
func (m *Manager) Reload(id string) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("插件不存在: %s", id)
	}
	if !inst.Ref.Enabled {
		return fmt.Errorf("插件未启用: %s", id)
	}
	return m.load(inst)
}

// SetEnabled turns a plugin on or off, applying or reverting its
// contributions immediately.
func (m *Manager) SetEnabled(id string, enabled bool, permissions []string, settings map[string]string) error {
	m.mu.RLock()
	inst, ok := m.instances[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("插件不存在: %s", id)
	}
	m.mu.Lock()
	inst.Ref.Enabled = enabled
	if permissions != nil {
		inst.Ref.Permissions = permissions
	}
	if settings != nil {
		inst.Ref.Settings = settings
	}
	for i := range m.cfg.Plugins {
		if m.cfg.Plugins[i].ID == id {
			m.cfg.Plugins[i] = inst.Ref
		}
	}
	m.mu.Unlock()
	_ = config.SaveConfig(m.cfg)

	if !enabled {
		m.unload(inst)
		m.Emit("plugin.disabled", map[string]any{"pluginId": id})
		return nil
	}
	if err := m.load(inst); err != nil {
		return err
	}
	m.Emit("plugin.enabled", map[string]any{"pluginId": id})
	return nil
}

// unload reverts a plugin's contributions and stops its event worker.
func (m *Manager) unload(inst *InstanceState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeContributionsLocked(inst)
	if inst.stopCh != nil {
		close(inst.stopCh)
		inst.stopCh = nil
		inst.eventCh = nil
	}
	inst.status = StatusDisabled
	inst.errMsg = ""
}

// Info returns one plugin's snapshot.
func (m *Manager) Info(id string) (Info, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[id]
	if !ok {
		return Info{}, false
	}
	return inst.info(), true
}

// List returns every installed plugin, sorted by name.
func (m *Manager) List() []Info {
	m.mu.RLock()
	out := make([]Info, 0, len(m.instances))
	for _, inst := range m.instances {
		out = append(out, inst.info())
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (inst *InstanceState) info() Info {
	counts := Counts{
		Tools: len(inst.Manifest.Tools), Providers: len(inst.Manifest.Providers),
		Middleware: len(inst.Manifest.Middleware), MCPServers: len(inst.Manifest.MCPServers),
		ACPAgents: len(inst.Manifest.ACPAgents), Agents: len(inst.Manifest.Agents),
		Skills: len(inst.Manifest.Skills), Themes: len(inst.Manifest.Themes),
		Commands: len(inst.Manifest.Commands), Contexts: len(inst.Manifest.Contexts),
		Triggers: len(inst.Manifest.Triggers), Channels: len(inst.Manifest.Channels),
		Events: len(inst.Manifest.Events), Importers: len(inst.Manifest.Importers),
	}
	for _, e := range inst.Manifest.Catalog {
		switch e.Kind {
		case CatalogProviders:
			counts.CatalogProviders++
		case CatalogACP:
			counts.CatalogACP++
		}
	}
	return Info{
		ID: inst.Manifest.ID, Name: inst.Manifest.Name, Version: inst.Manifest.Version,
		Author: inst.Manifest.Author, Description: inst.Manifest.Description,
		Dir: inst.Dir, DataDir: inst.DataDir, Status: inst.status, Error: inst.errMsg,
		Warnings: inst.warnings, Permissions: inst.Ref.Permissions,
		Missing:  MissingPermissions(inst.Manifest, inst.Ref.Permissions),
		Settings: inst.Manifest.Settings, Values: cloneSettings(inst.Ref.Settings),
		Source: inst.Ref.Source, Counts: counts,
		Builtin: inst.builtin,
	}
}

// FindTool returns the tool contribution with the given plugin and local name.
func (m *Manager) FindTool(pluginID, toolName string) (ToolContribution, bool) {
	for _, t := range m.Tools() {
		if t.PluginID == pluginID && t.ToolName == toolName {
			return t, true
		}
	}
	return ToolContribution{}, false
}

// DefaultPermissions grants the store capability, which only reaches the
// plugin's own data directory, and nothing else.
func DefaultPermissions() []string { return []string{"store"} }

// MissingPermissions lists the manifest permissions the user has not granted.
func MissingPermissions(manifest *Manifest, granted []string) []string {
	have := map[string]bool{}
	for _, p := range granted {
		have[p] = true
	}
	var out []string
	for _, p := range manifest.Permissions {
		if p == "store" {
			continue
		}
		if !have[p] {
			out = append(out, p)
		}
	}
	return out
}

// cloneSettings copies a settings map for the UI snapshot.
func cloneSettings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// jsonString renders a plugin hook result as a tool result string.
func jsonString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		data, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(data)
	}
}
