package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Providers         []Provider      `yaml:"providers"`
	ProviderGroups    []ProviderGroup `yaml:"provider_groups,omitempty"`
	MCPServers        []MCPServer     `yaml:"mcp_servers"`
	BuiltinTools      []BuiltinTool   `yaml:"builtin_tools"`
	CurrentProvider   string          `yaml:"current_provider"`
	UseReactAgent     bool            `yaml:"use_react_agent"`
	ReactAgentMaxStep int             `yaml:"react_agent_max_step"`
	ThemeMode         string          `yaml:"theme_mode,omitempty"`
	ColorTheme        string          `yaml:"color_theme,omitempty"`
	Density           string          `yaml:"density,omitempty"`
	Locale            string          `yaml:"locale,omitempty"`
	QuickChatShortcut string          `yaml:"quick_chat_shortcut,omitempty"`
	ShowTrayIcon      bool            `yaml:"show_tray_icon"`
	RunInBackground   bool            `yaml:"run_in_background"`
	Agents            []AgentDef      `yaml:"agents,omitempty"`
	ACPAgents         []ACPConnection `yaml:"acp_agents,omitempty"`
	// AgentCheckpoint auto-commits an agent's file changes as checkpoints:
	// "off" (default) disables it, "turn" commits once per completed reply,
	// "batch" once per finished tool-call batch. Experimental.
	AgentCheckpoint string `yaml:"agent_checkpoint,omitempty"`
	// RTKEnabled routes command output through the rtk CLI to shrink what
	// the model reads. Off by default; turning it on needs rtk on PATH.
	RTKEnabled      bool       `yaml:"rtk_enabled,omitempty"`
	Workspaces      []string   `yaml:"workspaces,omitempty"`
	SkillsEnabled   []string   `yaml:"skills_enabled,omitempty"`
	Chat            Selection  `yaml:"chat,omitempty"`
	ChatInitialized bool       `yaml:"chat_initialized,omitempty"`
	AutoTasks       []AutoTask `yaml:"auto_tasks,omitempty"`
	Channels        []Channel  `yaml:"channels,omitempty"`
}

// Channel is an IM channel configuration that can trigger Agent execution.
type Channel struct {
	ID         string        `yaml:"id"`
	Type       string        `yaml:"type"` // "feishu" or "wechat"
	Name       string        `yaml:"name"`
	Enabled    bool          `yaml:"enabled"`
	Agent      string        `yaml:"agent,omitempty"`      // "agent:Name" / "acp:Name" / "" = built-in Chat
	AppID      string        `yaml:"app_id,omitempty"`     // feishu: App ID; wechat: iLink Bot ID
	AppSecret  string        `yaml:"app_secret,omitempty"` // feishu: App Secret; wechat: iLink Bot Token
	BaseURL    string        `yaml:"base_url,omitempty"`   // wechat: iLink base URL (default https://ilinkai.weixin.qq.com)
	Workspace  string        `yaml:"workspace,omitempty"`
	ConvID     string        `yaml:"conversation_id,omitempty"`
	ConvIDs    []string      `yaml:"conversation_ids,omitempty"`
	ActiveConv int           `yaml:"active_conv,omitempty"`
	Tools      []ChannelTool `yaml:"tools,omitempty"`
	// Options carries a platform's extra credential fields, which differ per
	// channel type: Slack's app-level token, Discord's guild allow-list.
	Options map[string]string `yaml:"options,omitempty"`
}

// ChannelTool is a tool available only to messages from a specific channel.
type ChannelTool struct {
	Type   string            `yaml:"type"`
	Enable bool              `yaml:"enable"`
	Config map[string]string `yaml:"config,omitempty"`
}

// Schedule is when an AutoTask runs: daily at Hour:Minute, weekly on Weekday,
// or monthly on Day.
type Schedule struct {
	Kind    string `yaml:"kind"`    // daily | weekly | monthly
	Hour    int    `yaml:"hour"`    // 0-23
	Minute  int    `yaml:"minute"`  // 0-59
	Weekday int    `yaml:"weekday"` // 0=Sunday .. 6=Saturday (weekly)
	Day     int    `yaml:"day"`     // 1-31 (monthly)
}

// AutoTask runs an agent on a prompt on a schedule, in its own conversations.
type AutoTask struct {
	ID        string    `yaml:"id"`
	Name      string    `yaml:"name"`
	Agent     string    `yaml:"agent,omitempty"` // "" = built-in Chat, "agent:<name>"
	Prompt    string    `yaml:"prompt"`
	Workspace string    `yaml:"workspace,omitempty"`
	Schedule  Schedule  `yaml:"schedule"`
	Reuse     bool      `yaml:"reuse"`                     // reuse one conversation
	ConvID    string    `yaml:"conversation_id,omitempty"` // reused conversation id
	Enabled   bool      `yaml:"enabled"`
	LastRun   time.Time `yaml:"last_run,omitempty"`
	LastNote  string    `yaml:"last_note,omitempty"`
}

// Selection is the models and tools an agent may use. For the built-in Chat it
// is the persisted choice; for a custom Agent it is the configured whitelist,
// all enabled unless the user turns one off while chatting.
type Selection struct {
	Providers    []string `yaml:"providers,omitempty"`
	Provider     string   `yaml:"provider,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	Workspace    string   `yaml:"workspace,omitempty"`
	BuiltinTools []string `yaml:"builtin_tools,omitempty"`
	MCPServers   []string `yaml:"mcp_servers,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
}

// AgentDef is a named assistant preset usable in a conversation.
type AgentDef struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description,omitempty"`
	Icon         string   `yaml:"icon,omitempty"`
	SystemPrompt string   `yaml:"system_prompt,omitempty"`
	Provider     string   `yaml:"provider,omitempty"`  // deprecated: use Models
	Providers    []string `yaml:"providers,omitempty"` // deprecated: use Models
	Models       []string `yaml:"models,omitempty"`    // "provider::model" or "::model"
	Mode         string   `yaml:"mode,omitempty"`      // chat or agent
	Workspace    string   `yaml:"workspace,omitempty"`
	AvatarSeed   string   `yaml:"avatar_seed,omitempty"`
	BuiltinTools []string `yaml:"builtin_tools,omitempty"`
	MCPServers   []string `yaml:"mcp_servers,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
}

// ACPConnection is a local coding CLI spoken to over the Agent Client Protocol.
type ACPConnection struct {
	Name       string            `yaml:"name"`
	Command    string            `yaml:"command"`
	Args       []string          `yaml:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	Cwd        string            `yaml:"cwd,omitempty"`
	Icon       string            `yaml:"icon,omitempty"`
	AvatarSeed string            `yaml:"avatar_seed,omitempty"`
	Disabled   bool              `yaml:"disabled,omitempty"`
}

// Provider represents an LLM provider configuration
type Provider struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"` // openai, anthropic, ollama, etc.
	APIKey  string   `yaml:"api_key"`
	BaseURL string   `yaml:"base_url,omitempty"`
	Model   string   `yaml:"model"`
	Models  []string `yaml:"models,omitempty"`
	Enabled bool     `yaml:"enabled"`
	// APIType selects the upstream protocol of a custom provider: openai,
	// openai_responses, claude or gemini. Requests are written with the OpenAI
	// Chat format and converted to APIType, and the response is converted back.
	APIType string `yaml:"api_type,omitempty"`
	// Preset marks a shipped example whose name and type are fixed.
	Preset bool `yaml:"preset,omitempty"`
}

// The provider types ChatGo can talk to. A provider's type names the protocol
// its endpoint speaks, which is also what llm.NewClient switches on, so these
// strings are written into config.yaml and must stay stable.
const (
	ProviderTypeOpenAI    = "openai"
	ProviderTypeAnthropic = "anthropic"
	ProviderTypeClaude    = "claude"
	ProviderTypeOllama    = "ollama"
	ProviderTypeQwen      = "qwen"
	ProviderTypeDeepSeek  = "deepseek"
	ProviderTypeGemini    = "gemini"
	// ProviderTypeMagpie is the local magpie gateway (https://usemagpie.ai),
	// which re-exports the models configured in magpie over an
	// OpenAI-compatible API, so it takes no credential of its own.
	ProviderTypeMagpie = "magpie"
	ProviderTypeCustom = "custom"
)

// APITypeOpenAIResponses is the OpenAI Responses protocol. It is reachable
// only as a custom provider's api_type, never as a provider type of its own.
const APITypeOpenAIResponses = "openai_responses"

// MagpieBaseURL is the address the magpie gateway listens on by default.
const MagpieBaseURL = "http://127.0.0.1:3425/v1"

// ProviderTypes lists the selectable provider types, in the order the
// settings dropdown shows them. It is derived from ProviderPresets so a type
// cannot be offered without a shipped example to configure it from.
func ProviderTypes() []string {
	presets := ProviderPresets()
	out := make([]string, 0, len(presets))
	for _, p := range presets {
		if !containsString(out, p.Type) {
			out = append(out, p.Type)
		}
	}
	return out
}

// APITypes lists the upstream protocols a custom provider can speak.
func APITypes() []string {
	return []string{ProviderTypeOpenAI, APITypeOpenAIResponses, ProviderTypeClaude, ProviderTypeGemini}
}

// GetAPITypeDisplayName returns the human-readable name of a custom API type.
func GetAPITypeDisplayName(apiType string) string {
	names := map[string]string{
		ProviderTypeOpenAI:     "OpenAI Chat Completions",
		APITypeOpenAIResponses: "OpenAI Responses",
		ProviderTypeClaude:     "Claude Messages",
		ProviderTypeGemini:     "Gemini generateContent",
	}
	if name, ok := names[apiType]; ok {
		return name
	}
	return apiType
}

// EffectiveAPIType returns the downstream protocol of the provider: a custom
// provider uses its configured APIType, everything else its own type. An empty
// custom APIType falls back to OpenAI Chat Completions.
func (p Provider) EffectiveAPIType() string {
	if p.Type != ProviderTypeCustom {
		return p.Type
	}
	if p.APIType != "" {
		return p.APIType
	}
	return ProviderTypeOpenAI
}

// ProviderGroup routes one logical model name over several provider endpoints.
// Every member must name a custom provider; the members are tried in the order
// the routing strategy picks.
type ProviderGroup struct {
	Name     string        `yaml:"name"`
	Strategy string        `yaml:"strategy,omitempty"`
	Members  []GroupMember `yaml:"members,omitempty"`
	Enabled  bool          `yaml:"enabled"`
}

// GroupMember is one provider of a group together with the upstream models
// selected for it. Every provider in a group speaks a custom API type.
type GroupMember struct {
	Provider string   `yaml:"provider"`
	Models   []string `yaml:"models,omitempty"`
}

// Routing strategies of a provider group.
const (
	StrategyFallback   = "fallback"
	StrategyRoundRobin = "round_robin"
	StrategyRandom     = "random"
)

// GroupStrategies lists the routing strategies a provider group can use.
func GroupStrategies() []string {
	return []string{StrategyFallback, StrategyRoundRobin, StrategyRandom}
}

// GetStrategyDisplayName returns the human-readable name of a routing
// strategy. The names are self-explanatory, so the same one fits both the
// settings dropdown and the model selector.
func GetStrategyDisplayName(strategy string) string {
	names := map[string]string{
		StrategyFallback:   "故障迁移",
		StrategyRoundRobin: "依次轮询",
		StrategyRandom:     "随机访问",
	}
	if name, ok := names[strategy]; ok {
		return name
	}
	return strategy
}

// EffectiveStrategy returns the group's strategy, defaulting to fallback.
func (g ProviderGroup) EffectiveStrategy() string {
	if g.Strategy == "" {
		return StrategyFallback
	}
	return g.Strategy
}

// ModelsList returns the group's model names, one per distinct selected model.
func (g ProviderGroup) ModelsList() []string {
	out := make([]string, 0, len(g.Members))
	for _, m := range g.Members {
		for _, model := range m.Models {
			if model != "" && !containsString(out, model) {
				out = append(out, model)
			}
		}
	}
	return out
}

// MemberCount counts the members that selected at least one model.
func (g ProviderGroup) MemberCount() int {
	n := 0
	for _, m := range g.Members {
		if len(m.Models) > 0 {
			n++
		}
	}
	return n
}

// MatchModels reports whether the group can serve the given model name.
func (g ProviderGroup) MatchModels(model string) bool {
	return containsString(g.ModelsList(), model)
}

// Routes returns the endpoints to try for a model, ordered the way the
// strategy picks them: every selected model of every member, starting at the
// member the strategy chose. A member without the requested model falls back
// to its other selected models.
func (g ProviderGroup) Routes(model string, pick func(n int) int) []GroupMember {
	n := len(g.Members)
	if n == 0 {
		return nil
	}
	start := 0
	if pick != nil && n > 1 {
		start = pick(n)
	}
	out := make([]GroupMember, 0, n)
	for i := 0; i < n; i++ {
		member := g.Members[(start+i)%n]
		if len(member.Models) == 0 {
			continue
		}
		ordered := append([]string(nil), member.Models...)
		if model != "" {
			ordered = append([]string{model}, removeString(ordered, model)...)
		}
		out = append(out, GroupMember{Provider: member.Provider, Models: ordered})
	}
	return out
}

// ProviderGroups returns the enabled, non-empty groups of a config.
func (c *Config) EnabledGroups() []ProviderGroup {
	out := make([]ProviderGroup, 0, len(c.ProviderGroups))
	for _, g := range c.ProviderGroups {
		if g.Enabled && g.Name != "" && len(g.Members) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// FindProviderGroup looks a group up by name.
func (c *Config) FindProviderGroup(name string) (ProviderGroup, bool) {
	for _, g := range c.ProviderGroups {
		if g.Name == name {
			return g, true
		}
	}
	return ProviderGroup{}, false
}

func removeString(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// NeedsAPIKey reports whether a provider type cannot work without an API key.
// Ollama and magpie are local endpoints, and a custom provider may point at an
// unauthenticated one, so none of them requires a key.
func (p Provider) NeedsAPIKey() bool {
	switch p.Type {
	case ProviderTypeOllama, ProviderTypeMagpie, ProviderTypeCustom:
		return false
	}
	return true
}

// Validate reports why a provider cannot be used yet, in a message meant for
// the user rather than the upstream SDK's wording.
func (p Provider) Validate() error {
	if p.Model == "" {
		return fmt.Errorf("模型「%s」尚未选择具体模型，请在设置中添加模型名", p.Name)
	}
	if p.NeedsAPIKey() && strings.TrimSpace(p.APIKey) == "" {
		return fmt.Errorf("模型「%s」尚未填写 API Key，请在设置 → 模型中填写", p.Name)
	}
	return nil
}

// ModelsList returns the provider's models, falling back to the single Model.
func (p Provider) ModelsList() []string {
	if len(p.Models) > 0 {
		return p.Models
	}
	if p.Model != "" {
		return []string{p.Model}
	}
	return nil
}

// MCPServerType represents the type of MCP server connection
type MCPServerType string

const (
	MCPServerTypeStdIO          MCPServerType = "stdio"
	MCPServerTypeSSE            MCPServerType = "sse"
	MCPServerTypeStreamableHTTP MCPServerType = "streamable_http"
)

// MCPServer represents an MCP server configuration
type MCPServer struct {
	Name           string            `yaml:"name"`
	Type           MCPServerType     `yaml:"type"`
	Enabled        bool              `yaml:"enabled"`
	Command        string            `yaml:"command,omitempty"`
	Args           []string          `yaml:"args,omitempty"`
	Env            map[string]string `yaml:"env,omitempty"`
	URL            string            `yaml:"url,omitempty"`             // For SSE and StreamableHTTP
	Headers        map[string]string `yaml:"headers,omitempty"`         // For SSE and StreamableHTTP
	TimeoutSeconds int               `yaml:"timeout_seconds,omitempty"` // For SSE and StreamableHTTP
}

// BuiltinTool represents a built-in tool configuration from Eino framework
type BuiltinTool struct {
	Name    string            `yaml:"name"`
	Type    string            `yaml:"type"` // bingsearch, googlesearch, wikipedia, duckduckgosearch, httprequest, browseruse, commandline, sequentialthinking
	Enabled bool              `yaml:"enabled"`
	Config  map[string]string `yaml:"config,omitempty"` // Tool-specific configuration
}

// GetAvailableBuiltinTools returns a list of all available built-in tool types
func GetAvailableBuiltinTools() []string {
	return []string{
		"bingsearch",
		"googlesearch",
		"wikipedia",
		"duckduckgosearch",
		"httprequest",
		"browseruse",
		"commandline",
		"filesystem",
		"sequentialthinking",
		"jssandbox",
		"webview",
		"knowledge_search",
	}
}

// GetBuiltinToolDisplayName returns a human-readable display name for the tool type.
func GetBuiltinToolDisplayName(toolType string) string {
	names := map[string]string{
		"bingsearch":         "Bing 搜索",
		"googlesearch":       "Google 搜索",
		"wikipedia":          "Wikipedia",
		"duckduckgosearch":   "DuckDuckGo 搜索",
		"httprequest":        "HTTP 请求",
		"browseruse":         "浏览器自动化",
		"commandline":        "命令行",
		"filesystem":         "文件系统",
		"sequentialthinking": "链式思考",
		"jssandbox":          "JS 沙箱",
		"webview":            "WebView 浏览器",
		"knowledge_search":   "知识库检索",
	}
	if name, ok := names[toolType]; ok {
		return name
	}
	return toolType
}

// GetBuiltinToolDescription returns a description for the given tool type
func GetBuiltinToolDescription(toolType string) string {
	descriptions := map[string]string{
		"bingsearch":         "Bing Search - Search the web using Bing search engine",
		"googlesearch":       "Google Search - Search the web using Google search engine",
		"wikipedia":          "Wikipedia - Search and retrieve information from Wikipedia",
		"duckduckgosearch":   "DuckDuckGo Search - Private search using DuckDuckGo",
		"httprequest":        "HTTP Request - Make HTTP requests to web services",
		"browseruse":         "Browser Use - Automate browser interactions",
		"commandline":        "Command Line - Execute shell commands (use with caution)",
		"filesystem":         "Filesystem - Read, write or list files under the working directory",
		"sequentialthinking": "Sequential Thinking - Chain of thought reasoning tool",
		"jssandbox":          "JS Sandbox - Run JavaScript/TypeScript code in a secure sandbox (powered by moejs)",
		"webview":            "WebView - Open a URL or local HTML in the app's web view, read the page, click and type in it",
		"knowledge_search":   "Knowledge Search - Search the local knowledge bases built from indexed text documents, returning the matching passages with their source",
	}
	if desc, ok := descriptions[toolType]; ok {
		return desc
	}
	return "Unknown tool type"
}

// GetBuiltinToolConfigFields returns configurable fields for the given tool type
func GetBuiltinToolConfigFields(toolType string) []string {
	switch toolType {
	case "bingsearch":
		return []string{"api_key"}
	case "googlesearch":
		return []string{"api_key", "search_engine_id"}
	case "wikipedia":
		return []string{"language"}
	case "duckduckgosearch":
		return []string{}
	case "httprequest":
		return []string{"timeout", "max_redirects"}
	case "browseruse":
		return []string{"headless", "timeout"}
	case "commandline":
		return []string{"allowed_commands"}
	case "sequentialthinking":
		return []string{"max_iterations"}
	case "jssandbox":
		return []string{"memory_limit_mb", "timeout_seconds", "typescript"}
	case "webview":
		return []string{"allow_remote"}
	case "knowledge_search":
		return []string{"knowledge_base", "top_k"}
	default:
		return []string{}
	}
}

// GetRequiredConfigFields returns required (non-optional) fields for the given tool type
func GetRequiredConfigFields(toolType string) []string {
	switch toolType {
	case "bingsearch":
		return []string{"api_key"}
	case "googlesearch":
		return []string{"api_key", "search_engine_id"}
	case "wikipedia":
		return []string{} // language is optional
	case "duckduckgosearch":
		return []string{}
	case "httprequest":
		return []string{} // timeout and max_redirects have defaults
	case "browseruse":
		return []string{} // headless and timeout have defaults
	case "commandline":
		return []string{"allowed_commands"} // security requirement
	case "sequentialthinking":
		return []string{} // max_iterations has default
	case "jssandbox":
		return []string{} // all fields have defaults
	case "webview":
		return []string{} // allow_remote has a default
	case "knowledge_search":
		return []string{} // searches every knowledge base by default
	default:
		return []string{}
	}
}

// ValidateBuiltinToolConfig checks if all required fields are configured for a tool
func ValidateBuiltinToolConfig(tool BuiltinTool) error {
	requiredFields := GetRequiredConfigFields(tool.Type)

	for _, field := range requiredFields {
		value, exists := tool.Config[field]
		if !exists || strings.TrimSpace(value) == "" {
			return fmt.Errorf("required field '%s' is missing for tool '%s'", field, tool.Name)
		}
	}

	return nil
}

// configFile returns the configuration path under the user's ~/.chatgo.
func configFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".chatgo", "config.yaml"), nil
}

// ensureProviderPresets adds a disabled example for every type not yet present.
func ensureProviderPresets(existing []Provider) []Provider {
	have := make(map[string]bool, len(existing))
	for _, p := range existing {
		have[p.Type] = true
	}
	out := existing
	for _, p := range ProviderPresets() {
		if !have[p.Type] {
			out = append(out, p)
			have[p.Type] = true
		}
	}
	return out
}

// ProviderPresets returns one disabled example provider for every supported
// type, so a fresh configuration shows a template of each.
func ProviderPresets() []Provider {
	return []Provider{
		{Name: "OpenAI", Type: ProviderTypeOpenAI, BaseURL: "https://api.openai.com/v1", Model: "gpt-4o", Models: []string{"gpt-4o"}, Enabled: false, Preset: true},
		{Name: "Anthropic", Type: ProviderTypeAnthropic, BaseURL: "https://api.anthropic.com", Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false, Preset: true},
		{Name: "Claude", Type: ProviderTypeClaude, Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false, Preset: true},
		{Name: "Ollama", Type: ProviderTypeOllama, BaseURL: "http://localhost:11434", Model: "llama3.2", Models: []string{"llama3.2"}, Enabled: false, Preset: true},
		{Name: "Qwen", Type: ProviderTypeQwen, BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-max", Models: []string{"qwen-max"}, Enabled: false, Preset: true},
		{Name: "DeepSeek", Type: ProviderTypeDeepSeek, BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", Models: []string{"deepseek-chat"}, Enabled: false, Preset: true},
		{Name: "Gemini", Type: ProviderTypeGemini, Model: "gemini-2.0-flash", Models: []string{"gemini-2.0-flash"}, Enabled: false, Preset: true},
		{Name: "magpie", Type: ProviderTypeMagpie, BaseURL: MagpieBaseURL, Enabled: false, Preset: true},
		{Name: "Custom", Type: ProviderTypeCustom, BaseURL: "https://api.example.com/v1", Model: "gpt-4o", Models: []string{"gpt-4o"}, APIType: ProviderTypeOpenAI, Enabled: false, Preset: true},
	}
}

// LoadConfig loads the configuration from the default location
func LoadConfig() (*Config, error) {
	configPath, err := configFile()
	if err != nil {
		return nil, err
	}

	// Create default config if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return nil, err
		}

		// Create default built-in tools
		builtinTools := createDefaultBuiltinTools()

		defaultConfig := &Config{
			Providers: ProviderPresets(),
			MCPServers: []MCPServer{
				{
					Name:    "filesystem",
					Type:    MCPServerTypeStdIO,
					Enabled: true,
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", fmt.Sprintf("%s", os.Getenv("HOME"))},
					Env:     map[string]string{},
				},
			},
			BuiltinTools:      builtinTools,
			ACPAgents:         defaultACPAgents(),
			Channels:          defaultChannels(),
			CurrentProvider:   "",
			UseReactAgent:     false,
			ReactAgentMaxStep: 40,
			AgentCheckpoint:   "off",
			ThemeMode:         "system",
			Density:           "compact",
			Locale:            "zh-CN",
		}

		data, err := yaml.Marshal(defaultConfig)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return nil, err
		}

		return defaultConfig, nil
	}

	return loadFile(configPath)
}

// loadFile reads and normalizes a configuration file.
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Ensure all built-in tools exist (for backwards compatibility)
	if config.BuiltinTools == nil {
		config.BuiltinTools = createDefaultBuiltinTools()
	} else {
		config.BuiltinTools = ensureAllBuiltinTools(config.BuiltinTools)
	}
	config.ACPAgents = ensureDefaultACPAgents(config.ACPAgents)
	config.Providers = ensureProviderPresets(config.Providers)
	config.Channels = ensureDefaultChannels(config.Channels)

	for i := range config.Providers {
		p := &config.Providers[i]
		if len(p.Models) == 0 && p.Model != "" {
			p.Models = []string{p.Model}
		}
		if p.Enabled && p.APIKey == "" && p.NeedsAPIKey() {
			p.Enabled = false
		}
		if p.Type == ProviderTypeCustom && p.APIType == "" {
			p.APIType = ProviderTypeOpenAI
		}
	}
	for i := range config.Agents {
		a := &config.Agents[i]
		if len(a.Providers) == 0 && a.Provider != "" {
			a.Providers = []string{a.Provider}
		}
		if len(a.Models) == 0 {
			for _, pn := range a.Providers {
				model := firstModel(config.Providers, pn)
				a.Models = append(a.Models, pn+"::"+model)
			}
		}
	}

	return &config, nil
}

// firstModel returns the first model of the named provider.
func firstModel(providers []Provider, name string) string {
	for _, p := range providers {
		if p.Name == name {
			if ms := p.ModelsList(); len(ms) > 0 {
				return ms[0]
			}
			return ""
		}
	}
	return ""
}

// defaultACPAgents returns the ACP connections shipped by default, shown in
// the ACP settings page and disabled until the matching CLI is installed.
func defaultACPAgents() []ACPConnection {
	return []ACPConnection{
		{Name: "ClaudeCode", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}, Disabled: true},
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}, Disabled: true},
		{Name: "CodexCLI", Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp"}, Disabled: true},
		{Name: "PiAgent", Command: "npx", Args: []string{"-y", "pi-acp"}, Disabled: true},
	}
}

// ensureDefaultACPAgents adds the shipped ACP connections that are missing,
// migrating the legacy spaced name "Claude Code" to "ClaudeCode".
func ensureDefaultACPAgents(existing []ACPConnection) []ACPConnection {
	have := make(map[string]bool, len(existing))
	out := make([]ACPConnection, 0, len(existing))
	for _, a := range existing {
		key := a.Name
		if key == "Claude Code" {
			a.Name = "ClaudeCode"
			key = "ClaudeCode"
		}
		if have[key] {
			continue
		}
		have[key] = true
		out = append(out, a)
	}
	for _, d := range defaultACPAgents() {
		if !have[d.Name] {
			have[d.Name] = true
			out = append(out, d)
		}
	}
	return out
}

// DefaultChannelsForTest exposes the shipped presets to tests that check every
// platform has a driver.
func DefaultChannelsForTest() []Channel { return defaultChannels() }

// defaultChannels returns the shipped channel presets, all disabled until the
// user configures credentials. Each preset names the platform it speaks so the
// settings card can show the fields that platform needs.
func defaultChannels() []Channel {
	return []Channel{
		{ID: "feishu-default", Type: "feishu", Name: "飞书", Enabled: false},
		{ID: "wechat-default", Type: "wechat", Name: "微信", Enabled: false},
		{ID: "telegram-default", Type: "telegram", Name: "Telegram", Enabled: false},
		{ID: "dingtalk-default", Type: "dingtalk", Name: "钉钉", Enabled: false},
		{ID: "slack-default", Type: "slack", Name: "Slack", Enabled: false},
		{ID: "discord-default", Type: "discord", Name: "Discord", Enabled: false},
		{ID: "wecom-default", Type: "wecom", Name: "企业微信", Enabled: false},
	}
}

// ensureDefaultChannels adds the shipped channel presets that are missing.
func ensureDefaultChannels(existing []Channel) []Channel {
	have := make(map[string]bool, len(existing))
	for _, ch := range existing {
		have[ch.Type] = true
	}
	out := append([]Channel(nil), existing...)
	for _, d := range defaultChannels() {
		if !have[d.Type] {
			out = append(out, d)
		}
	}
	for i := range out {
		EnsureChannelTools(&out[i])
	}
	return out
}

func EnsureChannelTools(ch *Channel) {
	for _, t := range GetChannelToolTypes(ch.Type) {
		found := false
		for _, existing := range ch.Tools {
			if existing.Type == t {
				found = true
				break
			}
		}
		if !found {
			ch.Tools = append(ch.Tools, ChannelTool{Type: t, Config: map[string]string{}})
		}
	}
}

func GetChannelToolTypes(channelType string) []string {
	switch channelType {
	case "feishu":
		return []string{"feishu_send_message", "feishu_send_card"}
	case "wechat":
		return []string{"wechat_send_message"}
	case "telegram":
		return []string{"telegram_send_message"}
	case "dingtalk":
		return []string{"dingtalk_send_message"}
	case "slack":
		return []string{"slack_send_message"}
	case "discord":
		return []string{"discord_send_message"}
	case "wecom":
		return []string{"wecom_send_message"}
	}
	return nil
}

func GetChannelToolDisplayName(toolType string) string {
	switch toolType {
	case "feishu_send_message":
		return "飞书发消息"
	case "feishu_send_card":
		return "飞书发卡片"
	case "wechat_send_message":
		return "微信发消息"
	case "telegram_send_message":
		return "Telegram 发消息"
	case "dingtalk_send_message":
		return "钉钉发消息"
	case "slack_send_message":
		return "Slack 发消息"
	case "discord_send_message":
		return "Discord 发消息"
	case "wecom_send_message":
		return "企业微信发消息"
	}
	return toolType
}

func GetChannelToolDescription(toolType string) string {
	switch toolType {
	case "feishu_send_message":
		return "向当前飞书会话发送文本消息（自动使用消息来源会话）。需要渠道配置的飞书应用具有 im:message:send_as_bot 权限。"
	case "feishu_send_card":
		return "向当前飞书会话发送交互卡片（JSON 格式，自动使用消息来源会话）。需要渠道配置的飞书应用具有 im:message:send_as_bot 权限。"
	case "wechat_send_message":
		return "向当前微信用户发送文本消息（自动使用消息来源用户）。使用渠道配置的 iLink Bot 凭证。"
	case "telegram_send_message":
		return "向当前 Telegram 会话发送文本消息（自动使用消息来源会话）。使用渠道配置的 Bot Token。"
	case "dingtalk_send_message":
		return "向当前钉钉会话发送 Markdown 消息（自动使用消息来源会话的 sessionWebhook）。"
	case "slack_send_message":
		return "向当前 Slack 频道发送消息（自动使用消息来源频道）。需要 chat:write 权限。"
	case "discord_send_message":
		return "向当前 Discord 频道发送消息（自动使用消息来源频道）。"
	case "wecom_send_message":
		return "向当前企业微信会话发送 Markdown 消息（自动使用消息来源会话）。"
	}
	return toolType
}

func GetChannelToolConfigFields(toolType string) []string {
	switch toolType {
	case "feishu_send_message", "feishu_send_card":
		return []string{"default_chat_id"}
	case "wechat_send_message":
		return []string{"default_to_user"}
	case "telegram_send_message":
		return []string{"default_chat_id"}
	case "dingtalk_send_message":
		return []string{"default_chat_id"}
	case "slack_send_message":
		return []string{"default_chat_id"}
	case "discord_send_message":
		return []string{"default_chat_id"}
	case "wecom_send_message":
		return []string{"default_chat_id"}
	}
	return nil
}

// defaultBuiltinTool is the shipped state of one built-in tool: httprequest and
// commandline start enabled with sensible defaults, the rest disabled.
func defaultBuiltinTool(toolType string) BuiltinTool {
	t := BuiltinTool{Name: toolType, Type: toolType, Enabled: false, Config: map[string]string{}}
	switch toolType {
	case "httprequest":
		t.Enabled = true
		t.Config = map[string]string{"timeout": "30", "max_redirects": "5"}
	case "commandline":
		t.Enabled = true
		t.Config = map[string]string{"allowed_commands": "ls, pwd, cat, echo, grep, find, head, tail, wc"}
	case "filesystem":
		t.Enabled = true
	case "webview":
		t.Enabled = true
		t.Config = map[string]string{"allow_remote": "true"}
	case "knowledge_search":
		t.Enabled = true
		t.Config = map[string]string{"top_k": "5"}
	}
	return t
}

// createDefaultBuiltinTools creates the default list of built-in tools
func createDefaultBuiltinTools() []BuiltinTool {
	tools := GetAvailableBuiltinTools()
	builtinTools := make([]BuiltinTool, len(tools))
	for i, toolType := range tools {
		builtinTools[i] = defaultBuiltinTool(toolType)
	}
	return builtinTools
}

// ensureAllBuiltinTools ensures all available tools are in the list
func ensureAllBuiltinTools(existing []BuiltinTool) []BuiltinTool {
	availableTools := GetAvailableBuiltinTools()
	existingMap := make(map[string]BuiltinTool)

	// Create map of existing tools
	for _, tool := range existing {
		existingMap[tool.Type] = tool
	}

	// Build new list ensuring all tools are present
	result := make([]BuiltinTool, 0, len(availableTools))
	for _, toolType := range availableTools {
		if tool, exists := existingMap[toolType]; exists {
			result = append(result, tool)
		} else {
			result = append(result, defaultBuiltinTool(toolType))
		}
	}

	return result
}

// SaveConfig saves the configuration to the default location
func SaveConfig(config *Config) error {
	configPath, err := configFile()
	if err != nil {
		return err
	}
	return writeConfig(configPath, config)
}

// writeConfig marshals and writes a configuration to path, creating its dir.
func writeConfig(path string, config *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
