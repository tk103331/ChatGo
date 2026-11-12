package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func webTestWindow(t *testing.T) *ChatWindow {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatalf("conversation manager: %v", err)
	}
	cw.convManager = cm
	return cw
}

func TestWebRuntimeRoomCRUD(t *testing.T) {
	cw := webTestWindow(t)
	rt := cw.WebRuntime().(webui.RoomRunner)
	room, err := rt.CreateRoom(webui.RoomInput{
		Name: "评审群", Activation: "all",
		Members: []webui.RoomMemberInfo{{Agent: "agent:Reviewer", Name: "审查者"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if room.ID == "" || len(rt.Rooms()) != 1 {
		t.Fatalf("rooms = %#v", rt.Rooms())
	}
	room, err = rt.UpdateRoom(room.ID, webui.RoomInput{
		Name: "改名群", Activation: "natural",
		Members: []webui.RoomMemberInfo{{Agent: "agent:Reviewer"}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if room.Name != "改名群" || room.Activation != "natural" {
		t.Fatalf("updated = %#v", room)
	}
	session, err := rt.CreateRoomSession(room.ID)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if sessions := rt.RoomSessions(room.ID); len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("sessions = %#v", sessions)
	}
	if err := rt.DeleteRoom(room.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(rt.Rooms()) != 0 {
		t.Fatalf("rooms after delete = %#v", rt.Rooms())
	}
	all, _ := cw.convManager.ListConversations()
	for _, conv := range all {
		if conv.RoomID == room.ID {
			t.Fatalf("room conversation %s was not deleted", conv.ID)
		}
	}
}

func TestWebRuntimeTaskCRUD(t *testing.T) {
	cw := webTestWindow(t)
	rt := cw.WebRuntime().(webui.TaskRunner)
	task, err := rt.CreateTask(webui.TaskInput{
		Name: "日报", Prompt: "写日报", Enabled: true,
		Schedule: webui.ScheduleInfo{Kind: "daily", Hour: 9},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(rt.Tasks()) != 1 {
		t.Fatalf("tasks = %#v", rt.Tasks())
	}
	task, err = rt.UpdateTask(task.ID, webui.TaskInput{
		Name: "周报", Prompt: "写周报", Enabled: false,
		Schedule: webui.ScheduleInfo{Kind: "weekly", Weekday: 1, Hour: 10},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if task.Name != "周报" || task.Enabled {
		t.Fatalf("updated = %#v", task)
	}
	rt.SetTaskConv(task.ID, "conv-1")
	rt.StartTask(task.ID)
	rt.MarkTask(task.ID, true, "成功")
	tasks := rt.Tasks()
	if len(tasks) != 1 || tasks[0].ConvID != "conv-1" || tasks[0].LastNote != "成功" || tasks[0].LastRun.IsZero() {
		t.Fatalf("marked = %#v", tasks)
	}
	if cw.autoRunning[task.ID] {
		t.Fatal("task still marked running")
	}
	if err := rt.DeleteTask(task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(rt.Tasks()) != 0 {
		t.Fatalf("tasks after delete = %#v", rt.Tasks())
	}
}

func TestWebRuntimeRunRoom(t *testing.T) {
	cw := webTestWindow(t)
	cw.cfg.Rooms = []config.Room{{
		ID: "r1", Name: "评审群", Activation: "all", KBID: "test-kb",
		Members: []config.RoomMember{{Agent: "agent:Reviewer"}},
	}}
	cw.cfg.Agents = []config.AgentDef{{Name: "Reviewer"}}
	now := time.Now()
	conv := &models.Conversation{ID: "c1", RoomID: "r1"}
	conv.Messages = []models.Message{conv.NewHumanMessage("u1", "大家好", now)}
	live := &fakeLive{conv: conv}
	err := cw.WebRuntime().(webui.RoomRunner).RunRoom(context.Background(), "r1", conv, "大家好", live)
	if err != nil {
		t.Fatalf("run room: %v", err)
	}
	if len(conv.Messages) != 2 {
		t.Fatalf("messages = %#v", conv.Messages)
	}
	reply := conv.Messages[1]
	if reply.AuthorName != "Reviewer" || !strings.HasPrefix(reply.Content, "⚠️") {
		t.Fatalf("reply = %#v", reply)
	}
}
