package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/theme"

	"gopkg.in/yaml.v3"
)

// Contribution types the UI queries directly. Everything that fits an
// existing config struct is merged into config instead.
type (
	// ToolContribution is one plugin tool, addressed by plugin:<id>:<name>.
	ToolContribution struct {
		ID          string
		Name        string // model-facing name, <plugin>__<tool>
		Label       string
		Description string
		PluginID    string
		ToolName    string
		Params      json.RawMessage
	}

	// SkillContribution is one plugin skill.
	SkillContribution struct {
		ID          string
		Name        string
		Description string
		Body        string
		Path        string
		Source      string
	}

	// CommandContribution is one slash command.
	CommandContribution struct {
		ID       string
		Token    string
		PluginID string
		Spec     CommandSpec
	}

	// ContextContribution is one @ mention provider.
	ContextContribution struct {
		ID       string
		PluginID string
		Spec     ContextSpec
	}

	// TriggerContribution is one task trigger type.
	TriggerContribution struct {
		ID       string
		PluginID string
		Spec     TriggerSpec
	}

	// ChannelContribution is one messaging channel type.
	ChannelContribution struct {
		ID       string
		PluginID string
		Spec     ChannelSpec
	}

	// ImporterContribution is one declarative importer.
	ImporterContribution struct {
		ID       string
		PluginID string
		Spec     ImporterSpec
	}
)

// sourceName is the Source marker written into config entries.
func sourceName(id string) string { return "plugin:" + id }

// applyContributionsLocked merges everything the plugin declares. The manager
// lock must be held.
func (m *Manager) applyContributionsLocked(inst *InstanceState) error {
	id := inst.Manifest.ID
	source := sourceName(id)
	var warnings []string

	for _, spec := range inst.Manifest.Tools {
		name := id + "__" + spec.Name
		if m.builtinToolExistsLocked(name) {
			warnings = append(warnings, "工具名冲突，已跳过: "+name)
			continue
		}
		m.cfg.BuiltinTools = append(m.cfg.BuiltinTools, config.BuiltinTool{
			Name: name, Type: "plugin", Enabled: true, Source: source,
			Config: map[string]string{"plugin": id, "tool": spec.Name},
		})
	}

	for _, spec := range inst.Manifest.Providers {
		if m.providerExistsLocked(spec.Name) {
			warnings = append(warnings, "Provider 名称冲突，已跳过: "+spec.Name)
			continue
		}
		p := config.Provider{
			Name: spec.Name, Type: config.ProviderTypeCustom, APIType: spec.APIType,
			BaseURL: spec.BaseURL, Models: spec.Models, Headers: spec.Headers,
			Enabled: true, Source: source,
		}
		if p.APIType == "" {
			p.APIType = config.ProviderTypeOpenAI
		}
		if len(p.Models) > 0 {
			p.Model = p.Models[0]
		}
		if spec.Enabled != nil {
			p.Enabled = *spec.Enabled
		}
		if spec.APIKeySetting != "" {
			p.APIKey = inst.Ref.Settings[spec.APIKeySetting]
			if strings.TrimSpace(p.APIKey) == "" {
				p.Enabled = false
			}
		}
		m.cfg.Providers = append(m.cfg.Providers, p)
	}

	for _, spec := range inst.Manifest.MCPServers {
		if m.mcpExistsLocked(spec.Name) {
			warnings = append(warnings, "MCP Server 名称冲突，已跳过: "+spec.Name)
			continue
		}
		typ := spec.Type
		if typ == "" {
			typ = "stdio"
		}
		enabled := true
		if spec.Enabled != nil {
			enabled = *spec.Enabled
		}
		m.cfg.MCPServers = append(m.cfg.MCPServers, config.MCPServer{
			Name: spec.Name, Type: config.MCPServerType(typ), Enabled: enabled,
			Command: spec.Command, Args: spec.Args, Env: spec.Env,
			URL: spec.URL, Headers: spec.Headers, TimeoutSeconds: spec.TimeoutSeconds,
			Source: source,
		})
	}

	for _, spec := range inst.Manifest.ACPAgents {
		if m.acpExistsLocked(spec.Name) {
			warnings = append(warnings, "ACP Agent 名称冲突，已跳过: "+spec.Name)
			continue
		}
		disabled := true
		if spec.Disabled != nil {
			disabled = *spec.Disabled
		}
		m.cfg.ACPAgents = append(m.cfg.ACPAgents, config.ACPConnection{
			Name: spec.Name, Command: spec.Command, Args: spec.Args, Env: spec.Env,
			Cwd: spec.Cwd, Icon: spec.Icon, AvatarSeed: spec.AvatarSeed,
			Disabled: disabled, Source: source,
		})
	}

	for _, spec := range inst.Manifest.Agents {
		if m.agentExistsLocked(spec.Name) {
			warnings = append(warnings, "Agent 名称冲突，已跳过: "+spec.Name)
			continue
		}
		mode := spec.Mode
		if mode == "" {
			mode = "chat"
		}
		m.cfg.Agents = append(m.cfg.Agents, config.AgentDef{
			Name: spec.Name, Description: spec.Description, Icon: spec.Icon,
			SystemPrompt: spec.SystemPrompt, Models: spec.Models, Mode: mode,
			Workspace: spec.Workspace, BuiltinTools: spec.Tools,
			MCPServers: spec.MCPServers, Skills: spec.Skills, Source: source,
		})
	}

	inst.skills = m.collectSkills(inst)
	if len(inst.Manifest.Themes) > 0 {
		theme.RegisterThemes(source, themeSet(inst.Manifest.Themes))
	}
	if len(warnings) > 0 {
		inst.warnings = warnings
	}
	return nil
}

// removeContributionsLocked reverts every contribution of a plugin.
func (m *Manager) removeContributionsLocked(inst *InstanceState) {
	source := sourceName(inst.Manifest.ID)
	if m.cfg != nil {
		m.cfg.BuiltinTools = dropSource(m.cfg.BuiltinTools, func(t config.BuiltinTool) string { return t.Source }, source)
		m.cfg.Providers = dropSource(m.cfg.Providers, func(p config.Provider) string { return p.Source }, source)
		m.cfg.MCPServers = dropSource(m.cfg.MCPServers, func(s config.MCPServer) string { return s.Source }, source)
		m.cfg.ACPAgents = dropSource(m.cfg.ACPAgents, func(a config.ACPConnection) string { return a.Source }, source)
		m.cfg.Agents = dropSource(m.cfg.Agents, func(a config.AgentDef) string { return a.Source }, source)
	}
	theme.UnregisterThemes(source)
}

func dropSource[T any](items []T, source func(T) string, want string) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		if source(it) != want {
			out = append(out, it)
		}
	}
	return out
}

func (m *Manager) builtinToolExistsLocked(name string) bool {
	for _, t := range m.cfg.BuiltinTools {
		if t.Name == name && t.Source != sourceName("") {
			return true
		}
	}
	return false
}

func (m *Manager) providerExistsLocked(name string) bool {
	for _, p := range m.cfg.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (m *Manager) mcpExistsLocked(name string) bool {
	for _, s := range m.cfg.MCPServers {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (m *Manager) acpExistsLocked(name string) bool {
	for _, a := range m.cfg.ACPAgents {
		if a.Name == name {
			return true
		}
	}
	return false
}

func (m *Manager) agentExistsLocked(name string) bool {
	for _, a := range m.cfg.Agents {
		if a.Name == name {
			return true
		}
	}
	return false
}

// themeSet converts manifest themes into the UI's theme type.
func themeSet(specs []ThemeSpec) []theme.Theme {
	out := make([]theme.Theme, 0, len(specs))
	for _, s := range specs {
		hover := func(v, fallback string) string {
			if v == "" {
				return fallback
			}
			return v
		}
		out = append(out, theme.Theme{
			Name: s.Name, Label: s.Label,
			Accent: theme.Accent{
				Light: s.Light, LightHover: hover(s.LightHover, s.Light), LightPressed: hover(s.LightPressed, s.Light),
				Dark: s.Dark, DarkHover: hover(s.DarkHover, s.Dark), DarkPressed: hover(s.DarkPressed, s.Dark),
			},
			LightBG: s.LightBG, DarkBG: s.DarkBG,
			LightSurface: s.LightSurface, DarkSurface: s.DarkSurface,
		})
	}
	return out
}

// collectSkills reads the bodies of declared and auto-discovered skills.
func (m *Manager) collectSkills(inst *InstanceState) []SkillContribution {
	out := make([]SkillContribution, 0, len(inst.Manifest.Skills))
	seen := map[string]bool{}
	add := func(name, desc, path, body string) {
		key := strings.ToLower(name)
		if name == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, SkillContribution{
			ID: "plugin:" + inst.Manifest.ID + ":" + name, Name: name,
			Description: desc, Body: body, Path: path, Source: sourceName(inst.Manifest.ID),
		})
	}
	for _, spec := range inst.Manifest.Skills {
		path := ""
		body := ""
		if spec.Path != "" {
			if p, err := safeJoin(inst.Dir, spec.Path); err == nil {
				path = p
				if data, err := os.ReadFile(p); err == nil {
					body = string(data)
				}
			}
		}
		if body == "" && spec.Description != "" {
			body = spec.Description
		}
		add(spec.Name, spec.Description, path, stripFrontmatter(body))
	}
	// Auto-discover skills/<name>/SKILL.md.
	entries, err := os.ReadDir(filepath.Join(inst.Dir, "skills"))
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := filepath.Join(inst.Dir, "skills", e.Name(), "SKILL.md")
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name, desc, body := parseSkillFile(e.Name(), data)
			add(name, desc, path, body)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// parseSkillFile reads the frontmatter ChatGo skills use.
func parseSkillFile(fallback string, data []byte) (string, string, string) {
	raw := strings.TrimSpace(string(data))
	name, desc := fallback, ""
	if strings.HasPrefix(raw, "---") {
		rest := raw[3:]
		if i := strings.Index(rest, "\n---"); i >= 0 {
			var fm struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if yaml.Unmarshal([]byte(rest[:i]), &fm) == nil {
				if fm.Name != "" {
					name = fm.Name
				}
				desc = fm.Description
			}
			raw = strings.TrimSpace(rest[i+4:])
		}
	}
	return name, desc, raw
}

// stripFrontmatter removes a leading YAML block from a skill body.
func stripFrontmatter(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {
		return raw
	}
	rest := raw[3:]
	if i := strings.Index(rest, "\n---"); i >= 0 {
		return strings.TrimSpace(rest[i+4:])
	}
	return raw
}

// Skills returns every loaded plugin skill.
func (m *Manager) Skills() []SkillContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []SkillContribution
	for _, inst := range m.loadedInstancesLocked() {
		out = append(out, inst.skills...)
	}
	return out
}

// Tools returns every tool an enabled plugin contributes.
func (m *Manager) Tools() []ToolContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ToolContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Tools {
			out = append(out, ToolContribution{
				ID:          "plugin:" + inst.Manifest.ID + ":" + spec.Name,
				Name:        inst.Manifest.ID + "__" + spec.Name,
				Label:       firstNonEmpty(spec.Label, spec.Name),
				Description: spec.Description, PluginID: inst.Manifest.ID,
				ToolName: spec.Name, Params: spec.Parameters,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Commands returns every slash command of enabled plugins.
func (m *Manager) Commands() []CommandContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []CommandContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Commands {
			out = append(out, CommandContribution{
				ID:       "plugin:" + inst.Manifest.ID + ":" + spec.Name,
				Token:    inst.Manifest.ID + "-" + spec.Name,
				PluginID: inst.Manifest.ID, Spec: spec,
			})
		}
	}
	return out
}

// Contexts returns every @ provider of enabled plugins.
func (m *Manager) Contexts() []ContextContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ContextContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Contexts {
			out = append(out, ContextContribution{
				ID:       "plugin:" + inst.Manifest.ID + ":" + spec.ID,
				PluginID: inst.Manifest.ID, Spec: spec,
			})
		}
	}
	return out
}

// Triggers returns every trigger type of enabled plugins.
func (m *Manager) Triggers() []TriggerContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []TriggerContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Triggers {
			out = append(out, TriggerContribution{
				ID: inst.Manifest.ID + ":" + spec.Type, PluginID: inst.Manifest.ID, Spec: spec,
			})
		}
	}
	return out
}

// Channels returns every channel type of enabled plugins.
func (m *Manager) Channels() []ChannelContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ChannelContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Channels {
			out = append(out, ChannelContribution{
				ID: inst.Manifest.ID + ":" + spec.Type, PluginID: inst.Manifest.ID, Spec: spec,
			})
		}
	}
	return out
}

// Importers returns every importer of enabled plugins.
func (m *Manager) Importers() []ImporterContribution {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []ImporterContribution
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Importers {
			out = append(out, ImporterContribution{
				ID: inst.Manifest.ID + ":" + spec.ID, PluginID: inst.Manifest.ID, Spec: spec,
			})
		}
	}
	return out
}

// loadedInstancesLocked lists loaded instances. The manager lock must be held.
func (m *Manager) loadedInstancesLocked() []*InstanceState {
	out := make([]*InstanceState, 0, len(m.instances))
	for _, inst := range m.instances {
		if inst.status == StatusLoaded {
			out = append(out, inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out
}

// Find returns a loaded instance by plugin id.
func (m *Manager) Find(id string) (*InstanceState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[id]
	if !ok || inst.status != StatusLoaded {
		return nil, false
	}
	return inst, true
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
