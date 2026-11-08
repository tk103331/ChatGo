package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"

	"github.com/Calcium-Ion/moejs"
)

func TestManifestValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte("export const tools = {};"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"id":"Bad_ID","name":"x","version":"1.0.0"}`)
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("expected invalid id to fail")
	}
	write(`{"id":"ok-plugin","name":"x","version":"1.0.0","events":["nope"]}`)
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("expected unknown event to fail")
	}
	write(`{"id":"ok-plugin","name":"x","version":"1.0.0","tools":[{"name":"a"}]}`)
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("expected missing handler to fail")
	}
	write(`{"id":"ok-plugin","name":"x","version":"1.0.0"}`)
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if m.Main != "main.js" {
		t.Fatalf("main default = %q", m.Main)
	}
}

func TestImporterMapping(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "cursor.json")
	body := `{"mcpServers":{"fs":{"command":"npx","args":["-y","server"],"env":{"A":"1"}},"web":{"url":"https://x.example/mcp"}}}`
	if err := os.WriteFile(conf, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	in := &InstanceState{Manifest: &Manifest{ID: "p"}}
	spec := ImporterSpec{
		ID: "cursor", Kinds: []string{"mcp"}, Format: "json",
		Files: []ImporterFile{{OS: "any", Path: conf}},
		Mapping: map[string]any{"mcp": map[string]any{
			"root": "/mcpServers", "name": "@key", "command": "/command",
			"args": "/args", "env": "/env", "url": "/url",
		}},
	}
	got := in.readImporter(spec, dir, "mcp")
	if len(got) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "fs" || got[0].Command != "npx" || len(got[0].Args) != 2 || got[0].Env["A"] != "1" {
		t.Fatalf("unexpected first candidate: %+v", got[0])
	}
	if got[1].Name != "web" || got[1].URL != "https://x.example/mcp" {
		t.Fatalf("unexpected second candidate: %+v", got[1])
	}
}

func TestImporterRejectsEscape(t *testing.T) {
	in := &InstanceState{Manifest: &Manifest{ID: "p"}}
	spec := ImporterSpec{ID: "x", Files: []ImporterFile{{Path: "../../etc/passwd"}}, Format: "json"}
	if got := in.readImporter(spec, t.TempDir(), "mcp"); len(got) != 0 {
		t.Fatalf("path escape was read: %+v", got)
	}
}

func TestRuntimeCallTool(t *testing.T) {
	src := `export const tools = {
  add(args) { return { sum: args.a + args.b, label: "ok" }; },
};`
	mod, err := moejs.Compile("main.js", src)
	if err != nil {
		t.Fatal(err)
	}
	pool := newRuntimePool(mod, 64, 1)
	hook, err := mod.Hook("tools", "add")
	if err != nil {
		t.Fatal(err)
	}
	ctxValue := map[string]any{"plugin": map[string]any{"id": "p"}}
	out, err := pool.call(context.Background(), time.Second, hook, map[string]any{"a": 2, "b": 3}, ctxValue)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", out)
	}
	if m["label"] != "ok" {
		t.Fatalf("label = %v", m["label"])
	}
	if m["sum"] != int64(5) {
		t.Fatalf("sum = %#v", m["sum"])
	}
}

func TestLoadExamplePlugin(t *testing.T) {
	src := filepath.Join("..", "..", "examples", "plugins", "weather")
	if _, err := os.Stat(src); err != nil {
		t.Skip("example plugin not present")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "example-weather")
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	man, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	m := &Manager{cfg: cfg, root: root, dataRoot: t.TempDir(), instances: map[string]*InstanceState{}}
	inst := &InstanceState{
		Manifest: man, Dir: dir, DataDir: filepath.Join(m.dataRoot, man.ID),
		Ref: config.PluginRef{ID: man.ID, Enabled: true, Permissions: []string{"store"}},
	}
	m.instances[man.ID] = inst
	if err := m.load(inst); err != nil {
		t.Fatalf("load: %v", err)
	}
	defer m.unload(inst)

	if len(cfg.BuiltinTools) != 2 || cfg.BuiltinTools[0].Type != "plugin" {
		t.Fatalf("tool contribution = %+v", cfg.BuiltinTools)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "Weather Helper" {
		t.Fatalf("agent contribution = %+v", cfg.Agents)
	}
	if len(m.Skills()) != 1 {
		t.Fatalf("skills = %+v", m.Skills())
	}
	if _, ok := m.FindTool("example-weather", "forecast"); !ok {
		t.Fatal("forecast tool not registered")
	}
	out, err := m.InvokeTool(context.Background(), "example-weather", "forecast", `{"city":"北京"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "北京") || !strings.Contains(out, "tempC") {
		t.Fatalf("result = %q", out)
	}
	view, err := m.InvokeTool(context.Background(), "example-weather", "forecast_table", `{"cities":["深圳"]}`)
	if err != nil {
		t.Fatalf("invoke table: %v", err)
	}
	if !strings.Contains(view, "$chatgo") || !strings.Contains(view, "深圳") {
		t.Fatalf("view result = %q", view)
	}
	// The disabled state reverts every contribution.
	m.unload(inst)
	if len(cfg.BuiltinTools) != 0 || len(cfg.Agents) != 0 {
		t.Fatalf("contributions not reverted: %+v %+v", cfg.BuiltinTools, cfg.Agents)
	}
}

func TestDiscoverSkillImports(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "weather-guide")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: weather-guide\ndescription: d\n---\n\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := &InstanceState{
		Manifest: &Manifest{ID: "p", Importers: []ImporterSpec{{
			ID: "x", Label: "X", Kinds: []string{"skills"}, Dirs: []string{root},
		}}},
		status: StatusLoaded,
	}
	m := &Manager{cfg: &config.Config{}, instances: map[string]*InstanceState{"p": inst}}
	got := m.DiscoverSkillImports(root)
	if len(got) != 1 || got[0].Name != "weather-guide" || got[0].Path != dir {
		t.Fatalf("candidates = %+v", got)
	}
}
