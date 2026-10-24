package agentimport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// MCPCandidate is one MCP server found in another agent's configuration.
type MCPCandidate struct {
	// From names the agent the entry came from, for the import dialog.
	From    string
	Name    string
	Type    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// ProviderCandidate is one model provider found in another agent's
// configuration.
type ProviderCandidate struct {
	From    string
	Name    string
	BaseURL string
	APIKey  string
	Models  []string
}

// claudeConfig is the part of ~/.claude.json ChatGo reads.
type claudeConfig struct {
	MCPServers map[string]struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"mcpServers"`
}

// opencodeConfig is the part of opencode.json ChatGo reads. OpenCode states a
// local server's command as one argv slice and marks remote ones with a URL.
type opencodeConfig struct {
	MCP map[string]struct {
		Type        string            `json:"type"`
		Command     []string          `json:"command"`
		Environment map[string]string `json:"environment"`
		URL         string            `json:"url"`
		Headers     map[string]string `json:"headers"`
		Enabled     *bool             `json:"enabled"`
	} `json:"mcp"`
	Provider map[string]struct {
		Name    string `json:"name"`
		Options struct {
			APIKey  string `json:"apiKey"`
			BaseURL string `json:"baseURL"`
		} `json:"options"`
		Models map[string]struct {
			Name string `json:"name"`
		} `json:"models"`
	} `json:"provider"`
}

// codexConfig is the part of ~/.codex/config.toml ChatGo reads.
type codexConfig struct {
	MCPServers map[string]struct {
		Command string            `toml:"command"`
		Args    []string          `toml:"args"`
		Env     map[string]string `toml:"env"`
		URL     string            `toml:"url"`
	} `toml:"mcp_servers"`
}

// genericMCPFile is the shape shared by Cursor, Gemini CLI and the Claude
// settings file: a single mcpServers object.
type genericMCPFile struct {
	MCPServers map[string]struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"mcpServers"`
}

// DiscoverMCP scans the agent configurations for MCP servers, keeping the
// first entry of a name so a user-level file wins over a project one.
func DiscoverMCP(workdir string) []MCPCandidate {
	var out []MCPCandidate
	seen := map[string]bool{}
	add := func(c MCPCandidate) {
		if c.Name == "" || seen[strings.ToLower(c.Name)] {
			return
		}
		if c.Command == "" && c.URL == "" {
			return
		}
		seen[strings.ToLower(c.Name)] = true
		out = append(out, c)
	}
	for _, f := range mcpFiles(workdir) {
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		switch f.kind {
		case kindClaude:
			var cfg claudeConfig
			if json.Unmarshal(data, &cfg) != nil {
				continue
			}
			for name, s := range cfg.MCPServers {
				add(MCPCandidate{
					From: f.from, Name: name, Type: mcpType(s.Type, s.URL),
					Command: s.Command, Args: s.Args, Env: s.Env,
					URL: s.URL, Headers: s.Headers,
				})
			}
		case kindGeneric:
			var cfg genericMCPFile
			if json.Unmarshal(data, &cfg) != nil {
				continue
			}
			for name, s := range cfg.MCPServers {
				add(MCPCandidate{
					From: f.from, Name: name, Type: mcpType(s.Type, s.URL),
					Command: s.Command, Args: s.Args, Env: s.Env,
					URL: s.URL, Headers: s.Headers,
				})
			}
		case kindOpenCode:
			var cfg opencodeConfig
			if json.Unmarshal(data, &cfg) != nil {
				continue
			}
			for name, s := range cfg.MCP {
				c := MCPCandidate{
					From: f.from, Name: name, Type: mcpType(s.Type, s.URL),
					Env: s.Environment, URL: s.URL, Headers: s.Headers,
				}
				if len(s.Command) > 0 {
					c.Command, c.Args = s.Command[0], s.Command[1:]
				}
				add(c)
			}
		case kindCodex:
			var cfg codexConfig
			if toml.Unmarshal(data, &cfg) != nil {
				continue
			}
			for name, s := range cfg.MCPServers {
				add(MCPCandidate{
					From: f.from, Name: name, Type: mcpType("", s.URL),
					Command: s.Command, Args: s.Args, Env: s.Env, URL: s.URL,
				})
			}
		}
	}
	return out
}

// DiscoverProviders scans the agent configurations for OpenAI-compatible
// providers, which ChatGo imports as custom providers.
func DiscoverProviders(workdir string) []ProviderCandidate {
	var out []ProviderCandidate
	seen := map[string]bool{}
	for _, f := range mcpFiles(workdir) {
		if f.kind != kindOpenCode {
			continue
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		var cfg opencodeConfig
		if json.Unmarshal(data, &cfg) != nil {
			continue
		}
		for id, p := range cfg.Provider {
			if p.Options.BaseURL == "" || seen[strings.ToLower(id)] {
				continue
			}
			seen[strings.ToLower(id)] = true
			name := p.Name
			if name == "" {
				name = id
			}
			models := make([]string, 0, len(p.Models))
			for m := range p.Models {
				models = append(models, m)
			}
			sort.Strings(models)
			out = append(out, ProviderCandidate{
				From: f.from, Name: name, BaseURL: p.Options.BaseURL,
				APIKey: p.Options.APIKey, Models: models,
			})
		}
	}
	return out
}

// mcpType maps another agent's transport name to ChatGo's. OpenCode says
// local/remote, the rest say stdio/sse/http, and some omit it entirely.
func mcpType(declared, url string) string {
	switch strings.ToLower(strings.TrimSpace(declared)) {
	case "remote", "sse":
		return "sse"
	case "http", "streamable_http", "streamable-http":
		return "streamable_http"
	case "local", "stdio":
		return "stdio"
	}
	if url != "" {
		return "sse"
	}
	return "stdio"
}

type confKind int

const (
	kindClaude confKind = iota
	kindGeneric
	kindOpenCode
	kindCodex
)

type confFile struct {
	from string
	path string
	kind confKind
}

// mcpFiles lists the configuration files to scan, user level first.
func mcpFiles(workdir string) []confFile {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	files := []confFile{
		{"Claude Code", filepath.Join(home, ".claude.json"), kindClaude},
		{"Claude Code", filepath.Join(home, ".claude", "settings.json"), kindGeneric},
		{"OpenCode", filepath.Join(home, ".config", "opencode", "opencode.json"), kindOpenCode},
		{"OpenCode", filepath.Join(home, ".opencode", "opencode.json"), kindOpenCode},
		{"Codex", filepath.Join(home, ".codex", "config.toml"), kindCodex},
		{"Cursor", filepath.Join(home, ".cursor", "mcp.json"), kindGeneric},
		{"Gemini CLI", filepath.Join(home, ".gemini", "settings.json"), kindGeneric},
	}
	if workdir != "" {
		files = append(files,
			confFile{"当前项目 · Claude Code", filepath.Join(workdir, ".mcp.json"), kindGeneric},
			confFile{"当前项目 · OpenCode", filepath.Join(workdir, "opencode.json"), kindOpenCode},
			confFile{"当前项目 · Cursor", filepath.Join(workdir, ".cursor", "mcp.json"), kindGeneric},
		)
	}
	return files
}
