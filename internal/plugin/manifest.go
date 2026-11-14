// Package plugin loads JavaScript plugins (moejs) and merges what they declare
// into the application: tools, providers, model middleware, MCP servers,
// skills, ACP agents, agents and color themes.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)
	namePattern  = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	themePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
)

// Manifest is the parsed plugin.json. Declarations live here; behaviour lives
// in the JavaScript entry module.
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
	Main        string `json:"main,omitempty"`
	Engines     struct {
		ChatGo string `json:"chatgo,omitempty"`
	} `json:"engines,omitempty"`
	Limits      *Limits          `json:"limits,omitempty"`
	Permissions []string         `json:"permissions,omitempty"`
	Settings    []SettingField   `json:"settings,omitempty"`
	Tools       []ToolSpec       `json:"tools,omitempty"`
	Providers   []ProviderSpec   `json:"providers,omitempty"`
	Middleware  []MiddlewareSpec `json:"middleware,omitempty"`
	MCPServers  []MCPServerSpec  `json:"mcpServers,omitempty"`
	ACPAgents   []ACPSpec        `json:"acpAgents,omitempty"`
	Agents      []AgentSpec      `json:"agents,omitempty"`
	Skills      []SkillSpec      `json:"skills,omitempty"`
	Themes      []ThemeSpec      `json:"themes,omitempty"`
	Events      []string         `json:"events,omitempty"`
	Commands    []CommandSpec    `json:"commands,omitempty"`
	Contexts    []ContextSpec    `json:"contextProviders,omitempty"`
	Importers   []ImporterSpec   `json:"importers,omitempty"`
	Triggers    []TriggerSpec    `json:"triggerTypes,omitempty"`
	Channels    []ChannelSpec    `json:"channels,omitempty"`
	// ToolResult declares that the entry module exports onToolResult, a hook
	// that may rewrite every tool result before the model sees it.
	ToolResult bool `json:"toolResult,omitempty"`
	// Catalog entries are the add-dialog suggestions a plugin contributes:
	// model provider presets and ACP agents listed under the add dialogs,
	// which become real entities only when the user clicks. Disabling the
	// plugin hides its group; entities already added stay.
	Catalog []CatalogSpec `json:"catalog,omitempty"`
}

// Limits bounds the plugin's JavaScript runtime.
type Limits struct {
	MemoryMB int `json:"memoryMB,omitempty"`
}

// SettingField is one entry of the plugin's settings form.
type SettingField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type,omitempty"` // text | secret | number | bool | select
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
}

// ToolSpec is one tool the plugin exposes to the model.
type ToolSpec struct {
	Name        string          `json:"name"`
	Label       string          `json:"label,omitempty"`
	Description string          `json:"description,omitempty"`
	Handler     string          `json:"handler"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ProviderSpec is a model provider preset the plugin contributes.
type ProviderSpec struct {
	Name    string   `json:"name"`
	Type    string   `json:"type,omitempty"`
	APIType string   `json:"apiType,omitempty"`
	BaseURL string   `json:"baseURL,omitempty"`
	Models  []string `json:"models,omitempty"`
	// ModelsURL overrides where the model list is fetched from, for a
	// gateway that serves it somewhere other than the protocol's own
	// {base}/models. The fetched list fills the provider's Models, so a
	// plugin ships few names and the endpoint supplies the rest.
	ModelsURL     string            `json:"modelsURL,omitempty"`
	APIKeySetting string            `json:"apiKeySetting,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Enabled       *bool             `json:"enabled,omitempty"`
}

// MiddlewareSpec is one model middleware.
type MiddlewareSpec struct {
	Name      string          `json:"name"`
	Match     MiddlewareMatch `json:"match,omitempty"`
	Priority  int             `json:"priority,omitempty"`
	TimeoutMs int             `json:"timeoutMs,omitempty"`
	Hooks     []string        `json:"hooks,omitempty"`
}

// MiddlewareMatch limits the providers and models a middleware applies to.
type MiddlewareMatch struct {
	Providers []string `json:"providers,omitempty"`
	Models    []string `json:"models,omitempty"`
}

// MCPServerSpec is one MCP server the plugin adds.
type MCPServerSpec struct {
	Name           string            `json:"name"`
	Type           string            `json:"type,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	Enabled        *bool             `json:"enabled,omitempty"`
}

// ACPSpec is one ACP connection the plugin adds.
type ACPSpec struct {
	Name       string            `json:"name"`
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Icon       string            `json:"icon,omitempty"`
	Disabled   *bool             `json:"disabled,omitempty"`
	AvatarSeed string            `json:"avatarSeed,omitempty"`
}

// AgentSpec is one preset agent the plugin adds.
type AgentSpec struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Icon         string   `json:"icon,omitempty"`
	SystemPrompt string   `json:"systemPrompt,omitempty"`
	Models       []string `json:"models,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	MCPServers   []string `json:"mcpServers,omitempty"`
	Skills       []string `json:"skills,omitempty"`
}

// SkillSpec is one skill the plugin adds; Path is relative to the package.
type SkillSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
}

// ThemeSpec is one color theme the plugin adds.
type ThemeSpec struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Light        string `json:"light"`
	LightHover   string `json:"lightHover,omitempty"`
	LightPressed string `json:"lightPressed,omitempty"`
	Dark         string `json:"dark"`
	DarkHover    string `json:"darkHover,omitempty"`
	DarkPressed  string `json:"darkPressed,omitempty"`
	LightBG      string `json:"lightBg,omitempty"`
	DarkBG       string `json:"darkBg,omitempty"`
	LightSurface string `json:"lightSurface,omitempty"`
	DarkSurface  string `json:"darkSurface,omitempty"`
}

// CommandSpec is one slash command.
type CommandSpec struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"` // prompt | action
	Template    string `json:"template,omitempty"`
	Handler     string `json:"handler,omitempty"`
	AutoSend    bool   `json:"autoSend,omitempty"`
}

// ContextSpec is one @ mention provider.
type ContextSpec struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Icon         string `json:"icon,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Search       string `json:"search"`
	Resolve      string `json:"resolve"`
	MaxChars     int    `json:"maxChars,omitempty"`
	CacheSeconds int    `json:"cacheSeconds,omitempty"`
}

// ImporterSpec is one declarative importer of another agent's configuration.
type ImporterSpec struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Kinds []string `json:"kinds,omitempty"`
	// Dirs lists skill roots for kinds:["skills"]; each subdirectory with a
	// SKILL.md is one candidate.
	Dirs    []string       `json:"dirs,omitempty"`
	Files   []ImporterFile `json:"files,omitempty"`
	Format  string         `json:"format,omitempty"`
	Mapping map[string]any `json:"mapping,omitempty"`
}

// CatalogSpec is one add-dialog suggestion a plugin contributes: a model
// provider preset or an ACP agent. Unlike Providers or ACPAgents — which a
// plugin injects as live entities on load — a catalog entry is inert until
// the user picks it, so a catalog plugin can offer dozens of vendors
// without filling the settings page on its own.
type CatalogSpec struct {
	// Kind is "providers" or "acp".
	Kind string `json:"kind"`
	// Group names the section the entries list under in the dialog.
	Group string `json:"group,omitempty"`

	// Provider fields, for kind "providers".
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	APIType string `json:"apiType,omitempty"`
	BaseURL string `json:"baseURL,omitempty"`
	Note    string `json:"note,omitempty"`
	KeysURL string `json:"keysURL,omitempty"`

	// ACP fields, for kind "acp": one entry per ACP connection. Env rides
	// the connection the way ACPSpec's does.
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ImporterFile is one configuration file an importer reads.
type ImporterFile struct {
	OS   string `json:"os,omitempty"`
	Path string `json:"path"`
}

// TriggerSpec is one task trigger type.
type TriggerSpec struct {
	Type     string         `json:"type"`
	Label    string         `json:"label"`
	External bool           `json:"external,omitempty"`
	Fields   []SettingField `json:"fields,omitempty"`
	Evaluate string         `json:"evaluate,omitempty"`
}

// ChannelSpec is one messaging channel the plugin adds.
type ChannelSpec struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Mode   string         `json:"mode,omitempty"` // webhook | poll
	Fields []SettingField `json:"fields,omitempty"`
	Verify string         `json:"verify,omitempty"`
	Send   string         `json:"send"`
	Poll   string         `json:"poll,omitempty"`
	// PollSeconds is the poll interval, default 10 and clamped to 2..300.
	PollSeconds int `json:"pollSeconds,omitempty"`
}

// LoadManifest reads and validates plugin.json from dir.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return nil, fmt.Errorf("读取 plugin.json 失败: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析 plugin.json 失败: %w", err)
	}
	if m.Main == "" {
		m.Main = "main.js"
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	main := filepath.Join(dir, filepath.FromSlash(m.Main))
	if info, err := os.Stat(main); err != nil || info.IsDir() {
		return nil, fmt.Errorf("入口文件不存在: %s", m.Main)
	}
	for _, s := range m.Skills {
		if s.Path == "" {
			continue
		}
		p, err := safeJoin(dir, s.Path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("Skill 文件不存在: %s", s.Path)
		}
	}
	return &m, nil
}

var settingTypes = map[string]bool{"": true, "text": true, "secret": true, "number": true, "bool": true, "select": true}

var knownEvents = map[string]bool{
	"turn.completed": true, "turn.failed": true, "turn.started": true,
	"message.received": true, "tool.afterCall": true, "plan.updated": true,
	"task.started": true, "task.finished": true,
	"channel.message.received": true, "channel.reply.sent": true,
	"conversation.created": true, "conversation.deleted": true,
	"app.opened": true, "knowledge.imported": true,
	"plugin.enabled": true, "plugin.disabled": true,
}

// Validate checks the manifest's required fields and naming rules.
func (m *Manifest) Validate() error {
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("插件 id 非法（需匹配 %s）", idPattern.String())
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("插件缺少 name")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("插件缺少 version")
	}
	for _, p := range m.Permissions {
		if err := validPermission(p); err != nil {
			return err
		}
	}
	for _, s := range m.Settings {
		if strings.TrimSpace(s.Key) == "" {
			return fmt.Errorf("设置项缺少 key")
		}
		if !settingTypes[s.Type] {
			return fmt.Errorf("设置项 %s 的类型 %q 不支持", s.Key, s.Type)
		}
	}
	seen := map[string]bool{}
	for _, t := range m.Tools {
		if !namePattern.MatchString(t.Name) {
			return fmt.Errorf("工具名 %q 非法", t.Name)
		}
		if t.Handler == "" {
			return fmt.Errorf("工具 %s 缺少 handler", t.Name)
		}
		if seen["tool:"+t.Name] {
			return fmt.Errorf("工具名重复: %s", t.Name)
		}
		seen["tool:"+t.Name] = true
	}
	for _, p := range m.Providers {
		if strings.TrimSpace(p.Name) == "" || len(p.Models) == 0 {
			return fmt.Errorf("Provider 必须声明 name 与 models")
		}
		switch p.APIType {
		case "", "openai", "openai_responses", "claude", "gemini":
		default:
			return fmt.Errorf("Provider %s 的 apiType %q 不支持", p.Name, p.APIType)
		}
		if p.APIKeySetting != "" && !m.hasSetting(p.APIKeySetting) {
			return fmt.Errorf("Provider %s 引用了不存在的设置项 %s", p.Name, p.APIKeySetting)
		}
	}
	for _, mw := range m.Middleware {
		if !namePattern.MatchString(mw.Name) {
			return fmt.Errorf("中间件名 %q 非法", mw.Name)
		}
		for _, h := range mw.Hooks {
			switch h {
			case "beforeRequest", "onChunk", "afterResponse", "onError":
			default:
				return fmt.Errorf("中间件 %s 的钩子 %q 不支持", mw.Name, h)
			}
		}
	}
	for _, srv := range m.MCPServers {
		if strings.TrimSpace(srv.Name) == "" {
			return fmt.Errorf("MCP Server 缺少 name")
		}
		if srv.Type == "" {
			srv.Type = "stdio"
		}
		if srv.Type == "stdio" && srv.Command == "" {
			return fmt.Errorf("MCP Server %s 缺少 command", srv.Name)
		}
	}
	for _, a := range m.ACPAgents {
		if strings.TrimSpace(a.Name) == "" || a.Command == "" {
			return fmt.Errorf("ACP Agent 必须声明 name 与 command")
		}
	}
	for _, a := range m.Agents {
		if strings.TrimSpace(a.Name) == "" {
			return fmt.Errorf("Agent 缺少 name")
		}
	}
	for _, th := range m.Themes {
		if !themePattern.MatchString(th.Name) || th.Label == "" {
			return fmt.Errorf("主题 %q 非法", th.Name)
		}
		if th.Light == "" || th.Dark == "" {
			return fmt.Errorf("主题 %s 必须声明 light 与 dark", th.Name)
		}
	}
	for _, ev := range m.Events {
		if !knownEvents[ev] {
			return fmt.Errorf("未知事件 %q", ev)
		}
	}
	for _, c := range m.Commands {
		if !namePattern.MatchString(c.Name) {
			return fmt.Errorf("命令名 %q 非法", c.Name)
		}
		switch c.Kind {
		case "", "prompt":
			if c.Template == "" {
				return fmt.Errorf("命令 %s 缺少 template", c.Name)
			}
		case "action":
			if c.Handler == "" {
				return fmt.Errorf("命令 %s 缺少 handler", c.Name)
			}
		default:
			return fmt.Errorf("命令 %s 的类型 %q 不支持", c.Name, c.Kind)
		}
	}
	for _, cp := range m.Contexts {
		if cp.ID == "" || cp.Search == "" || cp.Resolve == "" {
			return fmt.Errorf("上下文提供者必须声明 id / search / resolve")
		}
	}
	for _, ch := range m.Channels {
		if !namePattern.MatchString(ch.Type) || ch.Send == "" {
			return fmt.Errorf("渠道声明非法: %s", ch.Type)
		}
		switch ch.Mode {
		case "", "webhook", "poll":
		default:
			return fmt.Errorf("渠道 %s 的 mode %q 不支持", ch.Type, ch.Mode)
		}
		if ch.Mode == "poll" && ch.Poll == "" {
			return fmt.Errorf("渠道 %s 使用 poll 模式但未声明 poll", ch.Type)
		}
	}
	return nil
}

func (m *Manifest) hasSetting(key string) bool {
	for _, s := range m.Settings {
		if s.Key == key {
			return true
		}
	}
	return false
}

func validPermission(p string) error {
	switch {
	case p == "exec" || p == "store" || p == "events" || p == "context":
		return nil
	case strings.HasPrefix(p, "http:") && len(p) > len("http:"):
		return nil
	case strings.HasPrefix(p, "env:") && len(p) > len("env:"):
		return nil
	}
	return fmt.Errorf("权限 %q 非法（支持 http:<host> / env:<KEY> / exec / store / events / context）", p)
}

// safeJoin resolves a relative path inside root, rejecting escapes.
func safeJoin(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("不允许绝对路径: %s", rel)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(absRoot, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	if abs != absRoot && !strings.HasPrefix(abs, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越界: %s", rel)
	}
	return abs, nil
}

// sortedKeys returns map keys in a stable order, for messages and tests.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
