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
	Workspaces        []string        `yaml:"workspaces,omitempty"`
	SkillsEnabled     []string        `yaml:"skills_enabled,omitempty"`
	Chat              Selection       `yaml:"chat,omitempty"`
	ChatInitialized   bool            `yaml:"chat_initialized,omitempty"`
	AutoTasks         []AutoTask      `yaml:"auto_tasks,omitempty"`
	Channels          []Channel       `yaml:"channels,omitempty"`
}

// Channel is an IM channel configuration that can trigger Agent execution.
type Channel struct {
	ID         string   `yaml:"id"`
	Type       string   `yaml:"type"` // "feishu" or "wechat"
	Name       string   `yaml:"name"`
	Enabled    bool     `yaml:"enabled"`
	Agent      string   `yaml:"agent,omitempty"`      // "agent:Name" / "acp:Name" / "" = built-in Chat
	AppID      string   `yaml:"app_id,omitempty"`     // feishu: App ID; wechat: iLink Bot ID
	AppSecret  string   `yaml:"app_secret,omitempty"` // feishu: App Secret; wechat: iLink Bot Token
	BaseURL    string   `yaml:"base_url,omitempty"`   // wechat: iLink base URL (default https://ilinkai.weixin.qq.com)
	Workspace  string   `yaml:"workspace,omitempty"`
	ConvID     string   `yaml:"conversation_id,omitempty"`
	ConvIDs    []string `yaml:"conversation_ids,omitempty"`
	ActiveConv int      `yaml:"active_conv,omitempty"`
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
	// Preset marks a shipped example whose name and type are fixed.
	Preset bool `yaml:"preset,omitempty"`
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
		{Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o", Models: []string{"gpt-4o"}, Enabled: false, Preset: true},
		{Name: "Anthropic", Type: "anthropic", BaseURL: "https://api.anthropic.com", Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false, Preset: true},
		{Name: "Claude", Type: "claude", Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false, Preset: true},
		{Name: "Ollama", Type: "ollama", BaseURL: "http://localhost:11434", Model: "llama3.2", Models: []string{"llama3.2"}, Enabled: false, Preset: true},
		{Name: "Qwen", Type: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-max", Models: []string{"qwen-max"}, Enabled: false, Preset: true},
		{Name: "DeepSeek", Type: "deepseek", BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", Models: []string{"deepseek-chat"}, Enabled: false, Preset: true},
		{Name: "Gemini", Type: "gemini", Model: "gemini-2.0-flash", Models: []string{"gemini-2.0-flash"}, Enabled: false, Preset: true},
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
		if p.Enabled && p.APIKey == "" && p.Type != "ollama" && p.Type != "custom" {
			p.Enabled = false
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

// defaultChannels returns the shipped channel presets: feishu and wechat,
// both disabled until the user configures credentials.
func defaultChannels() []Channel {
	return []Channel{
		{ID: "feishu-default", Type: "feishu", Name: "飞书", Enabled: false},
		{ID: "wechat-default", Type: "wechat", Name: "微信", Enabled: false},
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
	return out
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
