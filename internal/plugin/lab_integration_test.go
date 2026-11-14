package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/ui/theme"
)

// installLab copies the lab example into a temp home and enables it, so the
// test exercises discovery, permission grant, compilation and contribution
// merging exactly as the app does.
func installLab(t *testing.T) (*Manager, *config.Config, string) {
	t.Helper()
	src := filepath.Join("..", "..", "examples", "plugins", "lab")
	if _, err := os.Stat(src); err != nil {
		t.Skip("lab example not present")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(home, ".chatgo", "plugins", "lab")
	if err := copyTree(src, target); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	m := NewManager(cfg)
	m.LoadAll()
	if err := m.SetEnabled("lab", true, nil, nil); err != nil {
		t.Fatalf("enable lab: %v", err)
	}
	t.Cleanup(func() {
		_ = m.SetEnabled("lab", false, nil, nil)
	})
	return m, cfg, home
}

func TestLabPluginCapabilities(t *testing.T) {
	m, cfg, home := installLab(t)
	ctx := context.Background()

	// Contributions: every declarative kind must be merged.
	if len(cfg.BuiltinTools) != 3 {
		t.Fatalf("tools = %d, want 3", len(cfg.BuiltinTools))
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "Lab AI" ||
		cfg.Providers[0].Headers["X-Lab-Plugin"] != "1" || cfg.Providers[0].Enabled {
		t.Fatalf("provider contribution = %+v", cfg.Providers)
	}
	if cfg.Providers[0].ModelsURL != "https://lab.invalid/llm/catalog" {
		t.Fatalf("the plugin's modelsURL was lost: %+v", cfg.Providers[0])
	}
	if len(cfg.MCPServers) != 1 || cfg.MCPServers[0].Enabled {
		t.Fatalf("mcp contribution = %+v", cfg.MCPServers)
	}
	if len(cfg.ACPAgents) != 1 || !cfg.ACPAgents[0].Disabled {
		t.Fatalf("acp contribution = %+v", cfg.ACPAgents)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "Plugin Lab Helper" {
		t.Fatalf("agent contribution = %+v", cfg.Agents)
	}
	if len(m.Skills()) != 1 || m.Skills()[0].Name != "lab-guide" {
		t.Fatalf("skills = %+v", m.Skills())
	}
	if got := theme.ByName("lab-neon"); got.Name != "lab-neon" {
		t.Fatalf("theme not registered: %q", got.Name)
	}
	if len(m.Middlewares()) != 1 {
		t.Fatalf("middleware = %d", len(m.Middlewares()))
	}
	if len(m.Commands()) != 2 || len(m.Contexts()) != 1 || len(m.Triggers()) != 1 ||
		len(m.Channels()) != 1 || len(m.Importers()) != 1 {
		t.Fatalf("secondary contributions: cmd=%d ctx=%d trig=%d ch=%d imp=%d",
			len(m.Commands()), len(m.Contexts()), len(m.Triggers()), len(m.Channels()), len(m.Importers()))
	}
	info, _ := m.Info("lab")
	if info.Status != StatusLoaded || info.Counts.Events != 4 {
		t.Fatalf("info = %+v", info)
	}

	// Tools: success, declarative views, and the failure path.
	echo, err := m.InvokeTool(ctx, "lab", "echo", `{"text":"hello"}`)
	if err != nil || !strings.Contains(echo, "hello") || !strings.Contains(echo, "friendly") {
		t.Fatalf("echo = %q, %v", echo, err)
	}
	for _, view := range []string{"table", "chart", "keyvalue", "list", "code", "diff", "links", "markdown", "image"} {
		out, err := m.InvokeTool(ctx, "lab", "render", `{"view":"`+view+`"}`)
		if err != nil {
			t.Fatalf("render %s: %v", view, err)
		}
		var env struct {
			Chatgo *struct {
				V    int            `json:"v"`
				View map[string]any `json:"view"`
			} `json:"$chatgo"`
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil || env.Chatgo == nil || env.Chatgo.V != 1 {
			t.Fatalf("render %s envelope = %s", view, out)
		}
	}
	if _, err := m.InvokeTool(ctx, "lab", "fail", `{}`); err == nil || !strings.Contains(err.Error(), "故意") {
		t.Fatalf("fail error = %v", err)
	}

	// Tool-result hook masks secrets by default.
	masked := m.ApplyToolResult(ctx, "lab__echo", "", "token my secret sk-abcdef123")
	if strings.Contains(masked, "secret") || strings.Contains(masked, "sk-abcdef") {
		t.Fatalf("tool result not masked: %q", masked)
	}

	// Middleware only matches the plugin provider and rewrites all four stages.
	mws := m.Middlewares()
	if !mws[0].Matches("Lab AI", "lab-1") || mws[0].Matches("OpenAI", "gpt-4o") {
		t.Fatal("middleware match is wrong")
	}
	req := &llm.MiddlewareRequest{Provider: "Lab AI", Model: "lab-1", System: "base"}
	if err := mws[0].BeforeRequest(ctx, req); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.System, "[lab]") {
		t.Fatalf("system = %q", req.System)
	}
	chunk, _ := mws[0].OnChunk(ctx, llm.MiddlewareChunk{Text: "LAB_OK done"})
	if chunk.Text != "✅ done" {
		t.Fatalf("chunk = %q", chunk.Text)
	}
	resp := &llm.ChatResponse{Content: "  reply  "}
	if err := mws[0].AfterResponse(ctx, resp); err != nil || resp.Content != "reply" {
		t.Fatalf("afterResponse = %q, %v", resp.Content, err)
	}
	if res, ok := mws[0].OnError(ctx, errFake{}); !ok || !strings.Contains(res.Message, "Lab") {
		t.Fatalf("onError = %+v %v", res, ok)
	}

	// Commands: template and action.
	if res, err := m.RunCommand(ctx, "lab", "brief", "", "选中内容", ""); err != nil ||
		!strings.Contains(res.Draft, "选中内容") {
		t.Fatalf("brief = %+v, %v", res, err)
	}
	if res, err := m.RunCommand(ctx, "lab", "draft", "", "发布", ""); err != nil ||
		!strings.Contains(res.Draft, "发布") {
		t.Fatalf("draft = %+v, %v", res, err)
	}

	// Context provider: search and resolve from the seeded notes.
	items, err := m.SearchContext(ctx, "lab", "notes", "发布", "")
	if err != nil || len(items) == 0 || !strings.Contains(items[0].Label, "发布") {
		t.Fatalf("search context = %+v, %v", items, err)
	}
	text, err := m.ResolveContext(ctx, "lab", "notes", items[0].ID, "")
	if err != nil || !strings.Contains(text, "发布清单") {
		t.Fatalf("resolve context = %q, %v", text, err)
	}

	// Trigger evaluator returns a future time.
	now := time.Now()
	next, err := m.EvaluateTrigger(ctx, "lab", "interval", map[string]string{"seconds": "15"}, time.Time{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if d := next.Sub(now); d < 14*time.Second || d > 16*time.Second {
		t.Fatalf("next = %v (delta %v)", next, d)
	}

	// Channel: signature check, inbound normalisation and outbound bridge.
	in, err := m.VerifyChannel(ctx, "lab", "labchat", map[string]string{"app_secret": ""},
		ChannelRequest{Body: `{"text":"hi","from":"u","chatId":"c"}`})
	if err != nil || in.Text != "hi" || in.ChatID != "c" || in.FromUser != "u" {
		t.Fatalf("verify fallback = %+v, %v", in, err)
	}
	if _, err := m.VerifyChannel(ctx, "lab", "labchat", map[string]string{"app_secret": "s3"},
		ChannelRequest{Body: `{"text":"hi","secret":"bad"}`}); err == nil {
		t.Fatal("wrong signature accepted")
	}
	if err := m.SendChannel(ctx, "lab", "labchat", map[string]string{}, "c", "ok"); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Importer reads the sample config shipped inside the plugin directory.
	got := m.DiscoverImports(home, "mcp")
	if len(got) != 1 || got[0].Name != "lab-sample" || got[0].Command != "npx" {
		t.Fatalf("imports = %+v", got)
	}

	// Events: the plugin records counters in its own state file.
	m.Emit("turn.completed", map[string]any{"conversationId": "c1"})
	statePath := filepath.Join(home, ".chatgo", "plugin-data", "lab", "state.json")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(statePath); err == nil && strings.Contains(string(data), "turn.completed") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(statePath)
	if err != nil || !strings.Contains(string(data), "turn.completed") {
		t.Fatalf("event not recorded: %s, %v", data, err)
	}
}

type errFake struct{}

func (errFake) Error() string { return "boom" }
