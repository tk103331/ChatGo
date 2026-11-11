package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

func TestWebRuntimeAgentsAndTools(t *testing.T) {
	cw := testWindow()
	cw.cfg.Agents = []config.AgentDef{{
		Name: "Reviewer", Description: "审查代码",
		BuiltinTools: []string{"filesystem"}, MCPServers: []string{"github"},
	}}
	cw.cfg.ACPAgents = []config.ACPConnection{{Name: "Claude", Command: "claude"}}
	cw.cfg.BuiltinTools = []config.BuiltinTool{
		{Name: "filesystem", Enabled: true},
		{Name: "wikipedia", Enabled: true},
	}
	cw.cfg.MCPServers = []config.MCPServer{{Name: "github", Enabled: true}}

	rt := cw.WebRuntime()
	agents := rt.Agents()
	if len(agents) != 2 {
		t.Fatalf("agents = %#v", agents)
	}
	if agents[0].Ref != "agent:Reviewer" || agents[0].Kind != "agent" {
		t.Fatalf("agent = %#v", agents[0])
	}
	if agents[1].Ref != "acp:Claude" || agents[1].Kind != "acp" {
		t.Fatalf("acp = %#v", agents[1])
	}

	tools := rt.Tools("agent:Reviewer")
	on := map[string]bool{}
	for _, tool := range tools {
		on[tool.ID] = tool.Enabled
	}
	if !on["builtin:filesystem"] || !on["mcp:github"] {
		t.Fatalf("agent defaults = %#v", tools)
	}
	if on["builtin:wikipedia"] {
		t.Fatalf("wikipedia should not be enabled for the agent: %#v", tools)
	}
	global := rt.Tools("")
	on = map[string]bool{}
	for _, tool := range global {
		on[tool.ID] = tool.Enabled
	}
	if !on["builtin:wikipedia"] {
		t.Fatalf("global defaults = %#v", global)
	}
}

func TestWebRuntimeRevert(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}

	writeFile(t, dir, "a.txt", "first\n")
	cw.autoCommitCheckpoint(conv, "m1", nil)
	if gitOut(dir, "log", "-1", "--format=%s") == "" {
		t.Fatal("checkpoint commit missing")
	}

	writeFile(t, dir, "a.txt", "second\n")
	if _, err := gitRun(dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitRun(dir, "-c", "user.name=Test", "-c", "user.email=test@local", "commit", "-m", "later"); err != nil {
		t.Fatal(err)
	}

	rt := cw.WebRuntime()
	if err := rt.Revert(context.Background(), conv, "m1"); err != nil {
		t.Fatalf("revert: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first\n" {
		t.Fatalf("content = %q, want the checkpoint state", data)
	}
}

func TestWebRuntimeRevertWithoutCheckpoint(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	rt := cw.WebRuntime()
	err := rt.Revert(context.Background(), &models.Conversation{ID: "c1", Workspace: dir}, "missing")
	if err == nil {
		t.Fatal("expected an error when the reply has no checkpoint")
	}
}

type fakeLive struct {
	conv  *models.Conversation
	asks  int
	perms int
}

func (l *fakeLive) Update(fn func(*models.Conversation)) { fn(l.conv) }
func (l *fakeLive) Ask(ctx context.Context, items []webui.AskItem) (string, error) {
	l.asks++
	return "是", nil
}
func (l *fakeLive) Permission(ctx context.Context, req webui.PermissionRequest) (webui.PermissionDecision, error) {
	l.perms++
	return webui.PermissionDecision{OptionID: "allow"}, nil
}

func TestWebRunLLMRecordsTurnModel(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{{
		Name: "Local", Type: config.ProviderTypeOllama,
		Model: "llama3", Models: []string{"llama3"}, Enabled: true,
		// A closed port fails the request immediately, so the test never
		// reaches a real model.
		BaseURL: "http://127.0.0.1:1",
	}}
	cw.cfg.Chat.Provider = "Local"
	cw.cfg.Chat.Model = "llama3"
	now := time.Now()
	conv := &models.Conversation{
		ID: "c1", Provider: "Local", Model: "llama3",
		Tools:    []string{"builtin:filesystem"},
		ToolsSet: true,
		Messages: []models.Message{
			conv0Human("u1", "你好", now),
			conv0Agent("r1", now),
		},
		Turns: []models.Turn{{ID: "t1", UserMsgID: "u1", ReplyMsgID: "r1", StartedAt: now}},
	}
	live := &fakeLive{conv: conv}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := cw.WebRuntime().Run(ctx, conv, 1, webui.RunRequest{Provider: "Local", Model: "llama3"}, live)
	if err == nil {
		t.Fatal("expected the closed-port model call to fail")
	}
	if conv.Provider != "Local" || conv.Model != "llama3" {
		t.Fatalf("turn model = %s/%s", conv.Provider, conv.Model)
	}
	if turn := conv.Turns[0]; turn.Provider != "Local" || turn.Model != "llama3" {
		t.Fatalf("turn = %#v", turn)
	}
}

func conv0Human(id, content string, at time.Time) models.Message {
	return models.Message{
		ID: id, Role: "user", Content: content, Timestamp: at,
		AuthorID: models.AuthorHuman, AuthorKind: models.AuthorKindHuman,
	}
}

func conv0Agent(id string, at time.Time) models.Message {
	return models.Message{
		ID: id, Role: "assistant", Timestamp: at,
		AuthorKind: models.AuthorKindAgent,
	}
}
