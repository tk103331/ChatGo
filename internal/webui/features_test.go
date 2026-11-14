package webui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"chatgo/pkg/models"
)

type fakeRunner struct {
	mu sync.Mutex

	lastReq    RunRequest
	lastUser   string
	reply      string
	ask        bool
	permission bool
	agents     []AgentInfo
	tools      []ToolInfo
	workspaces []string
	skills     []SkillInfo
	revertErr  error
	revertedID string

	rooms        []RoomInfo
	roomSessions []RoomSessionInfo
	roomText     string
	tasks        []TaskInfo
	taskSessions []RoomSessionInfo
	startedTask  string
	markedTask   string
	markedOK     bool
	markedNote   string
}

func (f *fakeRunner) Capabilities() map[string]bool {
	return map[string]bool{
		CapAgents: true, CapTools: true, CapACP: true, CapAsk: true,
		CapPermission: true, CapCheckpoints: true, CapSkills: true,
	}
}

func (f *fakeRunner) Run(ctx context.Context, conv *models.Conversation, replyIdx int, req RunRequest, live Live) error {
	f.mu.Lock()
	f.lastReq = req
	f.lastUser = conv.Messages[replyIdx-1].Content
	reply := f.reply
	ask, permission := f.ask, f.permission
	f.mu.Unlock()

	if reply == "" {
		reply = "reply"
	}
	live.Update(func(c *models.Conversation) {
		c.Messages[replyIdx].Content = reply
		appendSegmentText(&c.Messages[replyIdx], "text", reply)
	})
	if ask {
		answer, err := live.Ask(ctx, []AskItem{{Question: "继续？", Options: []string{"是", "否"}}})
		if err != nil {
			return err
		}
		live.Update(func(c *models.Conversation) {
			c.Messages[replyIdx].Content += " " + answer
			appendSegmentText(&c.Messages[replyIdx], "text", " "+answer)
		})
	}
	if permission {
		decision, err := live.Permission(ctx, PermissionRequest{
			Title: "运行命令",
			Options: []PermissionOption{
				{ID: "allow", Name: "允许", Kind: "allow_once"},
				{ID: "deny", Name: "拒绝", Kind: "reject_once"},
			},
		})
		if err != nil {
			return err
		}
		live.Update(func(c *models.Conversation) {
			c.Messages[replyIdx].Content += " " + decision.OptionID
		})
	}
	return nil
}

func (f *fakeRunner) Agents() []AgentInfo             { return f.agents }
func (f *fakeRunner) Tools(agent string) []ToolInfo   { return f.tools }
func (f *fakeRunner) Workspaces() []string            { return f.workspaces }
func (f *fakeRunner) Skills(agent string) []SkillInfo { return f.skills }

func (f *fakeRunner) Projects() []ProjectInfo { return nil }

func (f *fakeRunner) Models(agent string) []ModelInfo { return nil }

func (f *fakeRunner) ACPOptions(agent string) []ACPConfigOptionInfo { return nil }

func (f *fakeRunner) SessionACPOptions(convID string) []ACPConfigOptionInfo { return nil }

func (f *fakeRunner) SetSessionConfig(convID, configID, value string) error {
	return nil
}

func (f *fakeRunner) CreateWorktree(project, branch string, newBranch bool) (string, error) {
	return "", nil
}

func (f *fakeRunner) Revert(ctx context.Context, conv *models.Conversation, messageID string) error {
	if f.revertErr != nil {
		return f.revertErr
	}
	f.mu.Lock()
	f.revertedID = messageID
	f.mu.Unlock()
	return nil
}

func runnerServer(t *testing.T, runner Runner) (*Server, *models.ConversationManager) {
	t.Helper()
	s, cm := testServer(t)
	s.runner = runner
	return s, cm
}

func TestMetaReportsRunnerCapabilities(t *testing.T) {
	s, _ := runnerServer(t, &fakeRunner{})
	w := serve(t, s, http.MethodGet, "/api/meta", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var resp struct {
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Capabilities[CapAgents] || !resp.Capabilities[CapAsk] || !resp.Capabilities[CapAttachments] {
		t.Fatalf("capabilities = %#v", resp.Capabilities)
	}
}

func TestFeatureLists(t *testing.T) {
	f := &fakeRunner{
		agents:     []AgentInfo{{Ref: "agent:Reviewer", Name: "Reviewer", Kind: "agent"}},
		tools:      []ToolInfo{{ID: "builtin:filesystem", Name: "filesystem", Kind: "builtin", Enabled: true}},
		workspaces: []string{"/tmp/project"},
		skills:     []SkillInfo{{ID: "s1", Name: "Review", Token: "Review"}},
	}
	s, _ := runnerServer(t, f)
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/api/agents", "Reviewer"},
		{"/api/tools?agent=agent:Reviewer", "builtin:filesystem"},
		{"/api/workspaces", "/tmp/project"},
		{"/api/skills?agent=agent:Reviewer", "\"token\":\"Review\""},
	} {
		w := serve(t, s, http.MethodGet, tc.path, "")
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
}

func TestCreateAndPatchSessionSettings(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	ws := t.TempDir()
	body := fmt.Sprintf(`{"agent":"agent:Reviewer","workspace":%q,"tools":["builtin:filesystem","bad:x"]}`, ws)
	w := serve(t, s, http.MethodPost, "/api/sessions", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	var created struct {
		Session models.Conversation `json:"session"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	conv := created.Session
	if conv.Agent != "agent:Reviewer" || conv.Workspace != ws || len(conv.Tools) != 1 || !conv.ToolsSet {
		t.Fatalf("session = %#v", conv)
	}

	w = serve(t, s, http.MethodPatch, "/api/sessions/"+conv.ID, `{"agent":"acp:Claude","tools":["mcp:github"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch %d: %s", w.Code, w.Body)
	}
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Agent != "acp:Claude" || len(loaded.Tools) != 1 || loaded.Tools[0] != "mcp:github" || !loaded.ToolsSet {
		t.Fatalf("patched = %#v", loaded)
	}
}

// The ACP options picked in the new-session form ride the conversation, so
// its session starts with them.
func TestCreateSessionCarriesACPConfig(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	w := serve(t, s, http.MethodPost, "/api/sessions",
		`{"agent":"acp:Claude","acp_config":{"model":"m2","think":"false"}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	var created struct {
		Session models.Conversation `json:"session"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	loaded, err := cm.LoadConversation(created.Session.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ACPConfig["model"] != "m2" || loaded.ACPConfig["think"] != "false" {
		t.Fatalf("the ACP choices were lost: %#v", loaded.ACPConfig)
	}
}

func TestRevertCallsRunner(t *testing.T) {
	f := &fakeRunner{}
	s, cm := runnerServer(t, f)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/r2/revert", "")
	if w.Code != http.StatusOK {
		t.Fatalf("revert %d: %s", w.Code, w.Body)
	}
	if f.revertedID != "r2" {
		t.Fatalf("reverted %q", f.revertedID)
	}
}

func TestSendUsesRunnerAndTools(t *testing.T) {
	f := &fakeRunner{reply: "from runner"}
	s, cm := runnerServer(t, f)
	conv := newConversation(t, cm, "会话")
	conv.Agent = "agent:Reviewer"
	conv.Tools = []string{"builtin:filesystem"}
	conv.ToolsSet = true
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages", `{"text":"你好","skills":["s1"]}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	f.mu.Lock()
	req := f.lastReq
	f.mu.Unlock()
	if req.Agent != "agent:Reviewer" || len(req.Tools) != 1 || len(req.Skills) != 1 {
		t.Fatalf("run request = %#v", req)
	}
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := loaded.Messages[len(loaded.Messages)-1].Content; got != "from runner" {
		t.Fatalf("reply = %q", got)
	}
}

func TestAskInteraction(t *testing.T) {
	f := &fakeRunner{ask: true, reply: "需要确认"}
	s, cm := runnerServer(t, f)
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages", `{"text":"开始"}`); w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	info := waitInteraction(t, s, conv.ID)
	if info.Kind != "ask" || len(info.Items) != 1 || info.Items[0].Question != "继续？" {
		t.Fatalf("interaction = %#v", info)
	}
	body := fmt.Sprintf(`{"id":%q,"answer":"是"}`, info.ID)
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/interaction", body); w.Code != http.StatusOK {
		t.Fatalf("answer %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	last := loaded.Messages[len(loaded.Messages)-1]
	if !strings.HasSuffix(last.Content, "是") {
		t.Fatalf("reply = %q", last.Content)
	}
}

func TestPermissionInteraction(t *testing.T) {
	f := &fakeRunner{permission: true, reply: "等待权限"}
	s, cm := runnerServer(t, f)
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages", `{"text":"执行"}`); w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	info := waitInteraction(t, s, conv.ID)
	if info.Kind != "permission" || info.Permission == nil || len(info.Permission.Options) != 2 {
		t.Fatalf("interaction = %#v", info)
	}
	body := fmt.Sprintf(`{"id":%q,"option_id":"allow"}`, info.ID)
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/interaction", body); w.Code != http.StatusOK {
		t.Fatalf("choose %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	last := loaded.Messages[len(loaded.Messages)-1]
	if !strings.HasSuffix(last.Content, "allow") {
		t.Fatalf("reply = %q", last.Content)
	}
}

func TestAttachmentUploadAndEmbed(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	data := base64.StdEncoding.EncodeToString([]byte("附件内容"))
	body := fmt.Sprintf(`{"name":"note.txt","data":%q}`, data)
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/attachments", body)
	if w.Code != http.StatusOK {
		t.Fatalf("upload %d: %s", w.Code, w.Body)
	}
	var up struct {
		Attachment attachmentRef `json:"attachment"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &up); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if up.Attachment.Path == "" || up.Attachment.Name != "note.txt" {
		t.Fatalf("attachment = %#v", up.Attachment)
	}
	sendBody := fmt.Sprintf(`{"text":"看看","attachments":[{"name":"note.txt","path":%q}]}`, up.Attachment.Path)
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages", sendBody); w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	user := loaded.Messages[len(loaded.Messages)-2]
	if !strings.Contains(user.Content, "附件内容") || !strings.Contains(user.Content, "note.txt") {
		t.Fatalf("user message = %q", user.Content)
	}
}

func waitInteraction(t *testing.T, s *Server, id string) *InteractionInfo {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if info := s.getRun(id); info != nil {
			if pi := info.interaction(); pi != nil {
				return pi
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("interaction did not appear")
	return nil
}

// jsonUnmarshal keeps the test file independent of encoding/json imports used
// elsewhere.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func TestAttachmentNameIsSanitized(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	data := base64.StdEncoding.EncodeToString([]byte("x"))
	body := fmt.Sprintf(`{"name":"../../evil.txt","data":%q}`, data)
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/attachments", body)
	if w.Code != http.StatusOK {
		t.Fatalf("upload %d: %s", w.Code, w.Body)
	}
	var up struct {
		Attachment attachmentRef `json:"attachment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &up); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if up.Attachment.Name != "evil.txt" || strings.Contains(up.Attachment.Path, "..") {
		t.Fatalf("attachment = %#v", up.Attachment)
	}
}

func TestAttachmentOutsideUploadDirIsNotEmbedded(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	foreign := t.TempDir() + "/secret.txt"
	if err := os.WriteFile(foreign, []byte("SECRET"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	body := fmt.Sprintf(`{"text":"看","attachments":[{"name":"secret.txt","path":%q}]}`, foreign)
	if w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages", body); w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	user := loaded.Messages[len(loaded.Messages)-2]
	if strings.Contains(user.Content, "SECRET") {
		t.Fatalf("foreign file was embedded: %q", user.Content)
	}
}

func TestAttachmentLargerThanReadLimit(t *testing.T) {
	s, cm := runnerServer(t, &fakeRunner{})
	conv := newConversation(t, cm, "会话")
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	// The attachment endpoint must accept bodies larger than an ordinary JSON
	// request; a 1 MB file becomes ~1.4 MB of base64.
	payload := strings.Repeat("a", 1<<20)
	data := base64.StdEncoding.EncodeToString([]byte(payload))
	body := fmt.Sprintf(`{"name":"big.txt","data":%q}`, data)
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/attachments", body)
	if w.Code != http.StatusOK {
		t.Fatalf("upload %d: %s", w.Code, w.Body)
	}
}

// --- Rooms and scheduled tasks ---

func (f *fakeRunner) Rooms() []RoomInfo { return f.rooms }
func (f *fakeRunner) CreateRoom(in RoomInput) (RoomInfo, error) {
	room := RoomInfo{ID: "room-new", Name: in.Name, Workspace: in.Workspace, Activation: in.Activation}
	for _, m := range in.Members {
		room.Members = append(room.Members, RoomMemberInfo{Agent: m.Agent, Name: m.Name})
	}
	f.rooms = append(f.rooms, room)
	return room, nil
}
func (f *fakeRunner) UpdateRoom(id string, in RoomInput) (RoomInfo, error) {
	for i := range f.rooms {
		if f.rooms[i].ID == id {
			f.rooms[i].Name = in.Name
			return f.rooms[i], nil
		}
	}
	return RoomInfo{}, errors.New("room not found")
}
func (f *fakeRunner) DeleteRoom(id string) error {
	for i := range f.rooms {
		if f.rooms[i].ID == id {
			f.rooms = append(f.rooms[:i], f.rooms[i+1:]...)
			return nil
		}
	}
	return errors.New("room not found")
}
func (f *fakeRunner) RoomSessions(id string) []RoomSessionInfo { return f.roomSessions }
func (f *fakeRunner) CreateRoomSession(id string) (RoomSessionInfo, error) {
	return RoomSessionInfo{ID: "room-conv-new", Label: "新会话"}, nil
}
func (f *fakeRunner) RunRoom(ctx context.Context, roomID string, conv *models.Conversation, text string, live Live) error {
	f.mu.Lock()
	f.roomText = text
	f.mu.Unlock()
	live.Update(func(c *models.Conversation) {
		m := c.NewAgentMessage("room-reply", "agent:A", "A", time.Now())
		m.Content = "room reply"
		c.Messages = append(c.Messages, m)
	})
	return nil
}

func (f *fakeRunner) Tasks() []TaskInfo { return f.tasks }
func (f *fakeRunner) CreateTask(in TaskInput) (TaskInfo, error) {
	t := TaskInfo{ID: "task-new", Name: in.Name, Prompt: in.Prompt, Enabled: in.Enabled, Schedule: in.Schedule}
	f.tasks = append(f.tasks, t)
	return t, nil
}
func (f *fakeRunner) UpdateTask(id string, in TaskInput) (TaskInfo, error) {
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			f.tasks[i].Name = in.Name
			f.tasks[i].Enabled = in.Enabled
			return f.tasks[i], nil
		}
	}
	return TaskInfo{}, errors.New("task not found")
}
func (f *fakeRunner) DeleteTask(id string) error {
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			f.tasks = append(f.tasks[:i], f.tasks[i+1:]...)
			return nil
		}
	}
	return errors.New("task not found")
}
func (f *fakeRunner) TaskSessions(id string) []RoomSessionInfo { return f.taskSessions }
func (f *fakeRunner) SetTaskConv(id, convID string) {
	for i := range f.tasks {
		if f.tasks[i].ID == id {
			f.tasks[i].ConvID = convID
		}
	}
}
func (f *fakeRunner) StartTask(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startedTask = id
	return true
}
func (f *fakeRunner) MarkTask(id string, ok bool, note string) {
	f.mu.Lock()
	f.markedTask = id
	f.markedOK = ok
	f.markedNote = note
	f.mu.Unlock()
}

func TestRoomsAPI(t *testing.T) {
	f := &fakeRunner{rooms: []RoomInfo{{
		ID: "room1", Name: "评审群", Activation: "all",
		Members: []RoomMemberInfo{{Agent: "agent:A", Name: "A"}},
	}}}
	s, _ := runnerServer(t, f)
	w := serve(t, s, http.MethodGet, "/api/rooms", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "评审群") {
		t.Fatalf("list %d: %s", w.Code, w.Body)
	}
	body := `{"name":"新群","members":[{"agent":"agent:A"}]}`
	if w := serve(t, s, http.MethodPost, "/api/rooms", body); w.Code != http.StatusCreated {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	if w := serve(t, s, http.MethodPatch, "/api/rooms/room1", body); w.Code != http.StatusOK {
		t.Fatalf("patch %d: %s", w.Code, w.Body)
	}
	if w := serve(t, s, http.MethodDelete, "/api/rooms/room1", ""); w.Code != http.StatusOK {
		t.Fatalf("delete %d: %s", w.Code, w.Body)
	}
}

func TestRoomSendRunsTurn(t *testing.T) {
	f := &fakeRunner{}
	s, cm := runnerServer(t, f)
	conv := newConversation(t, cm, "群聊会话")
	conv.RoomID = "room1"
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/rooms/room1/sessions/"+conv.ID+"/messages", `{"text":"大家好"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("send %d: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)
	f.mu.Lock()
	text := f.roomText
	f.mu.Unlock()
	if text != "大家好" {
		t.Fatalf("run text = %q", text)
	}
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 2 || loaded.Messages[0].Content != "大家好" || loaded.Messages[1].Content != "room reply" {
		t.Fatalf("messages = %#v", loaded.Messages)
	}
}

func TestTasksAPIAndRun(t *testing.T) {
	f := &fakeRunner{
		reply: "task reply",
		tasks: []TaskInfo{{
			ID: "t1", Name: "日报", Prompt: "写日报", Enabled: true,
			Schedule: ScheduleInfo{Kind: "daily", Hour: 9},
		}},
	}
	s, cm := runnerServer(t, f)
	w := serve(t, s, http.MethodGet, "/api/tasks", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "日报") {
		t.Fatalf("list %d: %s", w.Code, w.Body)
	}
	if w := serve(t, s, http.MethodPost, "/api/tasks", `{"name":"周报","prompt":"写周报","schedule":{"kind":"weekly","weekday":1}}`); w.Code != http.StatusCreated {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	if w := serve(t, s, http.MethodPatch, "/api/tasks/t1", `{"name":"日报2","prompt":"写日报","enabled":false,"schedule":{"kind":"daily","hour":9}}`); w.Code != http.StatusOK {
		t.Fatalf("patch %d: %s", w.Code, w.Body)
	}

	w = serve(t, s, http.MethodPost, "/api/tasks/t1/run", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("run %d: %s", w.Code, w.Body)
	}
	var runResp struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &runResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	waitIdle(t, s, runResp.SessionID)
	f.mu.Lock()
	started, marked, note := f.startedTask, f.markedTask, f.markedNote
	f.mu.Unlock()
	if started != "t1" || marked != "t1" || note != "成功" {
		t.Fatalf("task hooks: started=%q marked=%q note=%q", started, marked, note)
	}
	loaded, err := cm.LoadConversation(runResp.SessionID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.TaskID != "t1" || loaded.TaskStatus != "success" {
		t.Fatalf("conv = %#v", loaded)
	}
	if len(loaded.Messages) != 2 || loaded.Messages[0].Content != "写日报" || loaded.Messages[1].Content != "task reply" {
		t.Fatalf("messages = %#v", loaded.Messages)
	}
}
