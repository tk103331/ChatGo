package agentimport

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverMCPReadsEveryAgentFormat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Claude Code keeps an mcpServers object with a string command.
	writeFile(t, filepath.Join(home, ".claude.json"), `{
	  "mcpServers": {
	    "memory": {"type": "stdio", "command": "npx", "args": ["-y", "server-memory"], "env": {"K": "v"}},
	    "exa": {"type": "http", "url": "https://mcp.exa.ai/mcp"}
	  }
	}`)
	// OpenCode states the command as one argv slice and says local/remote.
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{
	  "mcp": {
	    "oc-local": {"type": "local", "command": ["bun", "x", "srv"], "enabled": true},
	    "oc-remote": {"type": "remote", "url": "http://127.0.0.1:3210/mcp"}
	  }
	}`)
	// Codex uses TOML.
	writeFile(t, filepath.Join(home, ".codex", "config.toml"), `
[mcp_servers.cx]
command = "/bin/tool"
args = ["mcp"]
`)
	// Cursor uses the generic shape.
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{
	  "mcpServers": {"cur": {"url": "http://localhost:8421/sse"}}
	}`)

	got := DiscoverMCP("")
	byName := map[string]MCPCandidate{}
	for _, c := range got {
		byName[c.Name] = c
	}
	if len(byName) != 6 {
		t.Fatalf("expected 6 servers, got %d: %+v", len(byName), got)
	}
	if c := byName["memory"]; c.From != "Claude Code" || c.Type != "stdio" ||
		c.Command != "npx" || len(c.Args) != 2 || c.Env["K"] != "v" {
		t.Fatalf("claude stdio parsed as %+v", c)
	}
	if c := byName["exa"]; c.Type != "streamable_http" || c.URL == "" {
		t.Fatalf("claude http parsed as %+v", c)
	}
	if c := byName["oc-local"]; c.From != "OpenCode" || c.Type != "stdio" ||
		c.Command != "bun" || len(c.Args) != 2 || c.Args[0] != "x" {
		t.Fatalf("opencode argv slice parsed as %+v", c)
	}
	if c := byName["oc-remote"]; c.Type != "sse" || c.URL == "" {
		t.Fatalf("opencode remote parsed as %+v", c)
	}
	if c := byName["cx"]; c.From != "Codex" || c.Command != "/bin/tool" || c.Type != "stdio" {
		t.Fatalf("codex toml parsed as %+v", c)
	}
	// A URL without a declared type is treated as SSE.
	if c := byName["cur"]; c.Type != "sse" {
		t.Fatalf("a bare URL should be sse, got %+v", c)
	}
}

func TestDiscoverMCPSkipsIncompleteAndDuplicates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// An entry with neither a command nor a URL cannot be used.
	writeFile(t, filepath.Join(home, ".claude.json"), `{
	  "mcpServers": {"ok": {"command": "a"}, "broken": {"type": "stdio"}}
	}`)
	// The same name in a lower-priority file must not be added twice.
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{
	  "mcpServers": {"ok": {"command": "b"}}
	}`)
	got := DiscoverMCP("")
	if len(got) != 1 || got[0].Name != "ok" || got[0].Command != "a" {
		t.Fatalf("unexpected result %+v", got)
	}
}

func TestDiscoverMCPIgnoresBrokenJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".claude.json"), `{ not json`)
	writeFile(t, filepath.Join(home, ".cursor", "mcp.json"), `{"mcpServers":{"ok":{"command":"a"}}}`)
	got := DiscoverMCP("")
	if len(got) != 1 || got[0].Name != "ok" {
		t.Fatalf("a broken file must not stop the scan, got %+v", got)
	}
}

func TestDiscoverProvidersReadsOpenAICompatible(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeFile(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{
	  "provider": {
	    "deepseek": {
	      "options": {"apiKey": "sk-test", "baseURL": "https://api.deepseek.com/v1"},
	      "models": {"deepseek-chat": {}, "deepseek-reasoner": {}}
	    },
	    "named": {
	      "name": "MiniMax",
	      "options": {"apiKey": "k", "baseURL": "https://api.minimaxi.com/v1"},
	      "models": {"m1": {}}
	    },
	    "no-base-url": {"options": {"apiKey": "x"}, "models": {"m": {}}}
	  }
	}`)
	got := DiscoverProviders("")
	if len(got) != 2 {
		t.Fatalf("a provider without a base URL must be skipped, got %+v", got)
	}
	byName := map[string]ProviderCandidate{}
	for _, p := range got {
		byName[p.Name] = p
	}
	ds, ok := byName["deepseek"]
	if !ok {
		t.Fatalf("deepseek missing from %+v", got)
	}
	if ds.BaseURL != "https://api.deepseek.com/v1" || ds.APIKey != "sk-test" {
		t.Fatalf("credentials not read: %+v", ds)
	}
	if len(ds.Models) != 2 || ds.Models[0] != "deepseek-chat" {
		t.Fatalf("models should be sorted, got %v", ds.Models)
	}
	if _, ok := byName["MiniMax"]; !ok {
		t.Fatalf("the display name should be preferred, got %+v", got)
	}
}

func TestMCPTypeMapping(t *testing.T) {
	cases := map[string]string{
		"local": "stdio", "stdio": "stdio",
		"remote": "sse", "sse": "sse",
		"http": "streamable_http", "streamable-http": "streamable_http",
		"": "stdio",
	}
	for in, want := range cases {
		if got := mcpType(in, ""); got != want {
			t.Fatalf("mcpType(%q) = %q, want %q", in, got, want)
		}
	}
	if got := mcpType("", "http://x"); got != "sse" {
		t.Fatalf("a URL without a type should be sse, got %q", got)
	}
}
