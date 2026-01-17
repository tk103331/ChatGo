package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Providers       []Provider  `yaml:"providers"`
	MCPServers      []MCPServer `yaml:"mcp_servers"`
	CurrentProvider string      `yaml:"current_provider"`
}

// Provider represents an LLM provider configuration
type Provider struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"` // openai, anthropic, ollama, etc.
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url,omitempty"`
	Model   string `yaml:"model"`
}

// MCPServer represents an MCP server configuration
type MCPServer struct {
	Name    string            `yaml:"name"`
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
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

		defaultConfig := &Config{
			Providers: []Provider{
				{
					Name:    "OpenAI",
					Type:    "openai",
					APIKey:  "",
					BaseURL: "https://api.openai.com/v1",
					Model:   "gpt-4",
				},
				{
					Name:   "Claude",
					Type:   "claude",
					APIKey: "",
					Model:  "claude-3-5-sonnet-20241022",
				},
				{
					Name:    "Ollama",
					Type:    "ollama",
					BaseURL: "http://localhost:11434",
					Model:   "llama3.2",
				},
				{
					Name:   "Qwen",
					Type:   "qwen",
					APIKey: "",
					Model:  "qwen-max",
				},
				{
					Name:   "DeepSeek",
					Type:   "deepseek",
					APIKey: "",
					Model:  "deepseek-chat",
				},
				{
					Name:   "Gemini",
					Type:   "gemini",
					APIKey: "",
					Model:  "gemini-2.0-flash-exp",
				},
			},
			MCPServers:      []MCPServer{},
			CurrentProvider: "OpenAI",
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

	return &config, nil
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
