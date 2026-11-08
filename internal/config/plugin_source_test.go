package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWriteConfigDropsPluginContributions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &Config{
		Providers: []Provider{
			{Name: "user", Type: ProviderTypeCustom, Model: "m", Models: []string{"m"}},
			{Name: "plug", Type: ProviderTypeCustom, Model: "m", Models: []string{"m"}, Source: "plugin:x"},
		},
		MCPServers: []MCPServer{
			{Name: "user-mcp", Type: MCPServerTypeStdIO, Command: "x"},
			{Name: "plug-mcp", Type: MCPServerTypeStdIO, Command: "x", Source: "plugin:x"},
		},
		BuiltinTools: []BuiltinTool{
			{Name: "httprequest", Type: "httprequest"},
			{Name: "plug__tool", Type: "plugin", Source: "plugin:x"},
		},
		Agents: []AgentDef{
			{Name: "User Agent"},
			{Name: "Plugin Agent", Source: "plugin:x"},
		},
		ACPAgents: []ACPConnection{
			{Name: "User CLI", Command: "x"},
			{Name: "Plugin CLI", Command: "x", Source: "plugin:x"},
		},
		Plugins: []PluginRef{{ID: "x", Enabled: true}},
	}
	if err := writeConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"name: plug", "name: plug-mcp", "name: plug__tool", "name: Plugin Agent", "name: Plugin CLI"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("plugin contribution %q was persisted:\n%s", leaked, data)
		}
	}
	var back Config
	if err := yaml.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Providers) != 1 || back.Providers[0].Name != "user" {
		t.Fatalf("providers = %+v", back.Providers)
	}
	if len(back.BuiltinTools) != 1 || back.BuiltinTools[0].Name != "httprequest" {
		t.Fatalf("builtin tools = %+v", back.BuiltinTools)
	}
	if len(back.Plugins) != 1 || back.Plugins[0].ID != "x" {
		t.Fatalf("plugin refs = %+v", back.Plugins)
	}
	// The live config must keep the contributions for this run.
	if len(cfg.Providers) != 2 || len(cfg.BuiltinTools) != 2 {
		t.Fatalf("writeConfig mutated the live config")
	}
}

func TestPluginPermissionGranted(t *testing.T) {
	ref := PluginRef{Permissions: []string{"store", "http:example.com"}}
	if !ref.PermissionGranted("http:example.com") {
		t.Fatal("granted permission not found")
	}
	if ref.PermissionGranted("http:other.com") {
		t.Fatal("unexpected permission")
	}
}
