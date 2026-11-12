package main

import (
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// testApp is the web client with one finished exchange that carries thinking,
// a tool call, the answer, usage and a plan.
func testApp() *app {
	now := time.Now()
	conv := &models.Conversation{
		ID:        "c1",
		Title:     "测试会话",
		CreatedAt: now,
		UpdatedAt: now,
		Provider:  "Local",
		Model:     "llama3",
		Messages: []models.Message{
			{
				ID: "u1", Role: "user", Content: "第一问\n带换行", Timestamp: now,
				AuthorID: models.AuthorHuman, AuthorKind: models.AuthorKindHuman,
			},
			{
				ID: "r1", Role: "assistant", Timestamp: now,
				AuthorID: "", AuthorName: "助手", AuthorKind: models.AuthorKindAgent,
				Segments: []models.Segment{
					{Kind: "reasoning", Text: "先想一想"},
					{Kind: "tool", ToolCall: &models.ToolCall{
						ID: "t1", Name: "search", Arguments: `{"query":"ChatGo"}`,
						Result: "找到 2 条结果", DurationMs: 12,
					}},
					{Kind: "text", Text: "**答案**在这里"},
				},
			},
		},
		Turns: []models.Turn{{
			ID: "turn1", UserMsgID: "u1", ReplyMsgID: "r1",
			User: "第一问", Assistant: "答案", Provider: "Local", Model: "llama3",
			Input: 1200, Output: 3400, Total: 4600,
			StartedAt: now.Add(-2 * time.Second), EndedAt: now,
		}},
		Plan: []models.PlanEntry{
			{Content: "读取文件", Status: models.PlanCompleted},
			{Content: "修改代码", Status: models.PlanInProgress, Priority: "high"},
		},
	}
	return &app{
		folds:           map[string]bool{},
		foldLive:        map[string]bool{},
		collapsedGroups: map[string]bool{},
		askAnswers:      map[string]string{},
		askInputs:       map[string]string{},
		caps: map[string]bool{
			capAgents: true, capTools: true, capACP: true, capAsk: true,
			capPermission: true, capCheckpoints: true, capAttachments: true,
			capSkills: true, "rooms": true, "tasks": true,
		},
		agents:     []agentInfo{{Ref: "agent:Reviewer", Name: "Reviewer", Kind: "agent"}},
		tools:      []toolInfo{{ID: "builtin:filesystem", Name: "filesystem", Kind: "builtin", Enabled: true}},
		workspaces: []string{"/tmp/project"},
		skills:     []skillInfo{{ID: "s1", Name: "Review", Token: "Review"}},
		meta:       metaInfo{Ready: true, Provider: "Local", Model: "llama3"},
		sessions:   []sessionSummary{{ID: "c1", Title: "测试会话", Workspace: "/tmp/project", UpdatedAt: now, Messages: 2}},
		cur:        conv,
		selected:   "c1",
		modelRef:   "Local::llama3",
	}
}

func renderApp(t *testing.T, a *app, w, h int) {
	t.Helper()
	img := mgo.Render(a.view, w, h, 1)
	if img == nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		t.Fatalf("render %dx%d failed: %v", w, h, img)
	}
}

func TestWebViewRenders(t *testing.T) {
	renderApp(t, testApp(), 1180, 820)
}

func TestWebViewRendersExpandedProcess(t *testing.T) {
	a := testApp()
	a.folds["round:r1:0"] = true
	a.folds["round:r1:0:r0"] = true
	a.folds["round:r1:0:t1:t1"] = true
	a.folds["tool:r1:t1"] = true
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersNarrow(t *testing.T) {
	a := testApp()
	renderApp(t, a, 520, 800)
	a.sidebarOpen = true
	renderApp(t, a, 520, 800)
}

func TestWebViewRendersEditingMessage(t *testing.T) {
	a := testApp()
	a.editingID = "u1"
	a.editDraft = "改过的第一问"
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersDialogs(t *testing.T) {
	a := testApp()
	a.renameID = "c1"
	a.renameDraft = "测试会话"
	a.renameOpen = true
	renderApp(t, a, 1180, 820)

	a = testApp()
	a.deleteID = "c1"
	a.deleteTitle = "测试会话"
	a.deleteOpen = true
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersSessionMenu(t *testing.T) {
	a := testApp()
	a.menuFor = "c1"
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersAgentSessionReadOnly(t *testing.T) {
	a := testApp()
	a.cur.Agent = "agent:Reviewer"
	renderApp(t, a, 1180, 820)
	renderApp(t, a, 520, 800)
}

func TestWebViewRendersFeatureSelectors(t *testing.T) {
	renderApp(t, testApp(), 1180, 820)
}

func TestWebViewRendersAskPanel(t *testing.T) {
	a := testApp()
	a.pending = &interactionInfo{
		ID: "ask1", Kind: "ask",
		Items: []askItem{
			{Question: "继续？", Options: []string{"是", "否"}},
			{Question: "用哪个模型？", Options: []string{"本地", "云端"}},
		},
	}
	renderApp(t, a, 1180, 820)
	a.askTab = 1
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersPermissionPanel(t *testing.T) {
	a := testApp()
	a.pending = &interactionInfo{
		ID: "perm1", Kind: "permission",
		Permission: &permissionRequest{
			Title:  "运行命令",
			Detail: "npm install",
			Options: []permissionOption{
				{ID: "allow", Name: "允许", Kind: "allow_once"},
				{ID: "deny", Name: "拒绝", Kind: "reject_once"},
			},
		},
	}
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersNewSessionDialog(t *testing.T) {
	a := testApp()
	a.newSessionOpen = true
	a.newAgent = "agent:Reviewer"
	a.newWorkspace = "/tmp/project"
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersAttachmentChips(t *testing.T) {
	a := testApp()
	a.attachments = []attachmentRef{{Name: "note.txt", Path: "/tmp/note.txt"}}
	a.attaching = true
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersRevertDialog(t *testing.T) {
	a := testApp()
	a.revertID = "r1"
	a.revertOpen = true
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersRooms(t *testing.T) {
	a := testApp()
	a.section = secRooms
	a.rooms = []roomInfo{{
		ID: "r1", Name: "评审群", Activation: "master",
		Members:  []roomMemberInfo{{Agent: "agent:A", Name: "A"}, {Agent: "agent:B", Name: "B"}},
		Sessions: 2, Running: true,
	}}
	a.roomID = "r1"
	a.roomSessions = []roomSessionInfo{{ID: "c1", Label: "第一次讨论"}}
	a.cur.RoomID = "r1"
	a.roomSessionsOpen = true
	renderApp(t, a, 1180, 820)
	renderApp(t, a, 520, 800)
}

func TestWebViewRendersRoomEditor(t *testing.T) {
	a := testApp()
	a.section = secRooms
	a.roomForm = &roomForm{
		id: "r1", name: "评审群", activation: "all", workspace: "/tmp/project",
		members: []roomMemberInfo{{Agent: "agent:Reviewer", Name: "审查者"}},
	}
	a.roomEditorOpen = true
	renderApp(t, a, 1180, 820)
}

func TestWebViewRendersTasks(t *testing.T) {
	a := testApp()
	a.section = secTasks
	now := time.Now()
	a.tasks = []taskInfo{{
		ID: "t1", Name: "每日日报", Agent: "agent:Reviewer", Prompt: "汇总今天的提交并写日报",
		Schedule: scheduleInfo{Kind: "daily", Hour: 9, Minute: 30},
		Enabled:  true, LastRun: now, LastNote: "成功", Sessions: 3,
	}}
	a.taskID = "t1"
	a.taskSessions = []roomSessionInfo{{ID: "c1", Label: "今天的日报", Updated: now}}
	a.cur.TaskID = "t1"
	a.taskSessionsOpen = true
	renderApp(t, a, 1180, 820)
	renderApp(t, a, 520, 800)
}

func TestWebViewRendersTaskEditor(t *testing.T) {
	a := testApp()
	a.section = secTasks
	a.taskForm = &taskForm{
		id: "t1", name: "每日日报", prompt: "汇总提交并写日报",
		agent: "agent:Reviewer", workspace: "/tmp/project",
		kind: "weekly", hour: "09", minute: "30", weekday: 1, day: "1",
		enabled: true,
	}
	a.taskEditorOpen = true
	renderApp(t, a, 1180, 820)
}
