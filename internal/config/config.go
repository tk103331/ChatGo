package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Density           string          `yaml:"density,omitempty"`
	Locale            string          `yaml:"locale,omitempty"`
	Agents            []AgentDef      `yaml:"agents,omitempty"`
	ACPAgents         []ACPConnection `yaml:"acp_agents,omitempty"`
	Workspaces        []string        `yaml:"workspaces,omitempty"`
	SkillsEnabled     []string        `yaml:"skills_enabled,omitempty"`
	Chat              Selection       `yaml:"chat,omitempty"`
	ChatInitialized   bool            `yaml:"chat_initialized,omitempty"`
}

// Selection is the models and tools an agent may use. For the built-in Chat it
// is the persisted choice; for a custom Agent it is the configured whitelist,
// all enabled unless the user turns one off while chatting.
type Selection struct {
	Providers    []string `yaml:"providers,omitempty"`
	Provider     string   `yaml:"provider,omitempty"`
	Model        string   `yaml:"model,omitempty"`
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
	BuiltinTools []string `yaml:"builtin_tools,omitempty"`
	MCPServers   []string `yaml:"mcp_servers,omitempty"`
	Skills       []string `yaml:"skills,omitempty"`
}

// ACPConnection is a local coding CLI spoken to over the Agent Client Protocol.
type ACPConnection struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Cwd     string            `yaml:"cwd,omitempty"`
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
		"sequentialthinking",
	}
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
		"sequentialthinking": "Sequential Thinking - Chain of thought reasoning tool",
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
		{Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1", Model: "gpt-4o", Models: []string{"gpt-4o"}, Enabled: false},
		{Name: "Anthropic", Type: "anthropic", BaseURL: "https://api.anthropic.com", Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false},
		{Name: "Claude", Type: "claude", Model: "claude-3-5-sonnet-20241022", Models: []string{"claude-3-5-sonnet-20241022"}, Enabled: false},
		{Name: "Ollama", Type: "ollama", BaseURL: "http://localhost:11434", Model: "llama3.2", Models: []string{"llama3.2"}, Enabled: false},
		{Name: "Qwen", Type: "qwen", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-max", Models: []string{"qwen-max"}, Enabled: false},
		{Name: "DeepSeek", Type: "deepseek", BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat", Models: []string{"deepseek-chat"}, Enabled: false},
		{Name: "Gemini", Type: "gemini", Model: "gemini-2.0-flash", Models: []string{"gemini-2.0-flash"}, Enabled: false},
		{Name: "Custom", Type: "custom", BaseURL: "", Model: "", Enabled: false},
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
		if migrateLegacyConfig(configPath) {
			return loadFile(configPath)
		}
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

	for i := range config.Providers {
		p := &config.Providers[i]
		if len(p.Models) == 0 && p.Model != "" {
			p.Models = []string{p.Model}
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

// migrateLegacyConfig copies a config from the old OS config dir into the new
// ~/.chatgo location, reporting whether it did.
func migrateLegacyConfig(dst string) bool {
	dir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "chatgo", "config.yaml"))
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return false
	}
	return os.WriteFile(dst, data, 0644) == nil
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

// defaultACPAgents returns the ACP connections shipped by default.
func defaultACPAgents() []ACPConnection {
	return []ACPConnection{
		{Name: "Claude Code", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}},
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}},
	}
}

// ensureDefaultACPAgents adds the shipped ACP connections that are missing.
func ensureDefaultACPAgents(existing []ACPConnection) []ACPConnection {
	have := make(map[string]bool, len(existing))
	for _, a := range existing {
		have[a.Name] = true
	}
	out := append([]ACPConnection(nil), existing...)
	for _, d := range defaultACPAgents() {
		if !have[d.Name] {
			out = append(out, d)
		}
	}
	return out
}

// createDefaultBuiltinTools creates the default list of built-in tools
func createDefaultBuiltinTools() []BuiltinTool {
	tools := GetAvailableBuiltinTools()
	builtinTools := make([]BuiltinTool, len(tools))

	for i, toolType := range tools {
		builtinTools[i] = BuiltinTool{
			Name:    toolType,
			Type:    toolType,
			Enabled: false,
			Config:  make(map[string]string),
		}
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
			// Add missing tool as disabled
			result = append(result, BuiltinTool{
				Name:    toolType,
				Type:    toolType,
				Enabled: false,
				Config:  make(map[string]string),
			})
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

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
