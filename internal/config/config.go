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
	Providers         []Provider         `yaml:"providers"`
	MCPServers        []MCPServer        `yaml:"mcp_servers"`
	BuiltinTools      []BuiltinTool      `yaml:"builtin_tools"`
	CurrentProvider   string             `yaml:"current_provider"`
	UseReactAgent     bool               `yaml:"use_react_agent"`
	ReactAgentMaxStep int                `yaml:"react_agent_max_step"`
}

// Provider represents an LLM provider configuration
type Provider struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"` // openai, anthropic, ollama, etc.
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
	Model   string `yaml:"model"`
	Enabled bool   `yaml:"enabled"`
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
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"` // bingsearch, googlesearch, wikipedia, duckduckgosearch, httprequest, browseruse, commandline, sequentialthinking
	Enabled     bool              `yaml:"enabled"`
	Config      map[string]string `yaml:"config,omitempty"` // Tool-specific configuration
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
		"bingsearch":          "Bing Search - Search the web using Bing search engine",
		"googlesearch":        "Google Search - Search the web using Google search engine",
		"wikipedia":           "Wikipedia - Search and retrieve information from Wikipedia",
		"duckduckgosearch":    "DuckDuckGo Search - Private search using DuckDuckGo",
		"httprequest":         "HTTP Request - Make HTTP requests to web services",
		"browseruse":          "Browser Use - Automate browser interactions",
		"commandline":         "Command Line - Execute shell commands (use with caution)",
		"sequentialthinking":  "Sequential Thinking - Chain of thought reasoning tool",
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

// LoadConfig loads the configuration from the default location
func LoadConfig() (*Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(configDir, "chatgo", "config.yaml")

	// Create default config if it doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return nil, err
		}

		// Create default built-in tools
		builtinTools := createDefaultBuiltinTools()

		defaultConfig := &Config{
			Providers: []Provider{
				{
					Name:    "OpenAI",
					Type:    "openai",
					APIKey:  "",
					BaseURL: "https://api.openai.com/v1",
					Model:   "gpt-4",
					Enabled: true,
				},
				{
					Name:    "Claude",
					Type:    "claude",
					APIKey:  "",
					Model:   "claude-3-5-sonnet-20241022",
					Enabled: true,
				},
				{
					Name:    "Ollama",
					Type:    "ollama",
					BaseURL: "http://localhost:11434",
					Model:   "llama3.2",
					Enabled: true,
				},
				{
					Name:    "Qwen",
					Type:    "qwen",
					APIKey:  "",
					Model:   "qwen-max",
					Enabled: false,
				},
				{
					Name:    "DeepSeek",
					Type:    "deepseek",
					APIKey:  "",
					Model:   "deepseek-chat",
					Enabled: false,
				},
				{
					Name:    "Gemini",
					Type:    "gemini",
					APIKey:  "",
					Model:   "gemini-2.0-flash-exp",
					Enabled: false,
				},
			},
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
			CurrentProvider:   "OpenAI",
			UseReactAgent:     false,
			ReactAgentMaxStep: 40,
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

	data, err := os.ReadFile(configPath)
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

	return &config, nil
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
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, "chatgo", "config.yaml")

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
