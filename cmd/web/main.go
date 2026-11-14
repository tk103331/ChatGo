// Command web is the browser client for ChatGo. It is built for
// GOOS=js GOARCH=wasm and served by cmd/webd; the model runs in webd, and
// this side only draws the conversation and sends input.
package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	"chatgo/pkg/models"
)

type app struct {
	win *mygo.Window

	mu       sync.Mutex
	selected string
	running  bool
	section  string
	roomConv string
	taskConv string

	meta            metaInfo
	sessions        []sessionSummary
	cur             *models.Conversation
	status          string
	draft           string
	search          string
	modelRef        string
	sidebarOpen     bool
	contextOpen     bool
	contextFor      string
	contextLoading  bool
	contextErr      string
	contextQuery    string
	contextFiles    []contextItem
	folds           map[string]bool
	foldLive        map[string]bool
	collapsedGroups map[string]bool

	caps       map[string]bool
	agents     []agentInfo
	tools      []toolInfo
	workspaces []string
	skills     []skillInfo
	agentRef   string
	toolsFor   string
	skillsFor  string
	skillsOn   []string
	toolsOpen  bool
	// toolTab selects the tab of the tool picker and the three fields keep
	// one search query per tab, like the desktop's 工具与技能 menu.
	toolTab    int
	toolQuery  string
	mcpQuery   string
	skillQuery string

	pending         *interactionInfo
	askAnswers      map[string]string
	askInputs       map[string]string
	askTab          int
	askFor          string
	interactionDone string

	newSessionOpen    bool
	newSessionClosing bool
	// The 新会话 form mirrors the desktop's create page: agent, project,
	// worktree or branch, model and the first message.
	newAgent     string
	newProject   string // project main path, or newProjectCustom
	newCustomDir string // typed directory when newProject is newProjectCustom
	// fsBrowse is the host directory browser behind 新建工作区: it walks the
	// server's own file system so a phone can pick a workspace.
	fsOpen       bool
	fsDir        string
	fsParent     string
	fsEntries    []fsItem
	fsLoading    bool
	fsErr        string
	newTree      string // worktree path, or newTreeCreate
	newBranch    string
	newNewBranch bool
	newModel     string
	newDraft     string
	newCreating  bool

	projects  []projectInfo
	models    []modelInfo
	modelsFor string
	// modelsByAgent caches each agent reference's model list for the
	// new-session model picker, fetched per chosen agent.
	modelsByAgent map[string]modelsByAgentState
	// acpOptions caches each ACP agent's session config options for the
	// new-session dialog.
	acpOptions map[string]acpOptionsState
	// newACPConfig holds the ACP option choices made in the new-session
	// dialog: a config id to the value picked for it.
	newACPConfig map[string]string
	// sessACP caches the open ACP session's config options, and sessACPFor
	// names the session they belong to.
	sessACP    acpOptionsState
	sessACPFor string

	attachments []attachmentRef
	attaching   bool
	attachOpen  bool

	editingID string
	editDraft string

	menuFor     string
	renameID    string
	renameDraft string
	renameOpen  bool
	deleteID    string
	deleteTitle string
	deleteOpen  bool
	revertID    string
	revertOpen  bool

	rooms             []roomInfo
	roomID            string
	roomSessions      []roomSessionInfo
	roomSessionsOpen  bool
	roomEditorOpen    bool
	roomEditorClosing bool
	roomForm          *roomForm
	roomDeleteOpen    bool

	tasks             []taskInfo
	taskID            string
	taskSessions      []roomSessionInfo
	taskSessionsOpen  bool
	taskEditorOpen    bool
	taskEditorClosing bool
	taskForm          *taskForm
	taskDeleteOpen    bool

	msgList kit.MessageListState
}

func (a *app) start() {
	var m metaInfo
	if err := apiGet("/api/meta", &m); err != nil {
		a.win.Update(func() { a.status = err.Error() })
	} else {
		a.win.Update(func() {
			a.meta = m
			a.caps = m.Capabilities
			if a.modelRef == "" {
				a.modelRef = modelRefOf(m.Provider, m.Model)
			}
		})
		a.loadFeatures()
	}
	a.refreshSessions()
	go a.poll()
}

// poll keeps the open conversation fresh. The running reply is polled every
// tick; idle sessions and the sidebar are refreshed at a slower cadence.
func (a *app) poll() {
	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		switch a.getSection() {
		case secRooms:
			if i%4 == 0 {
				a.refreshRooms()
				a.refreshRoomSessions()
			}
			if id := a.getRoomConv(); id != "" && (a.isRunning() || i%4 == 0) {
				a.loadSession(id, false)
			}
		case secTasks:
			if i%4 == 0 {
				a.refreshTasks()
				a.refreshTaskSessions()
			}
			if id := a.getTaskConv(); id != "" && (a.isRunning() || i%4 == 0) {
				a.loadSession(id, false)
			}
		default:
			if i%4 == 0 {
				a.refreshSessions()
			}
			id, running := a.snapshot()
			if id != "" && (running || i%4 == 0) {
				a.loadSession(id, false)
			}
		}
	}
}

func (a *app) refreshSessions() {
	var resp sessionsResponse
	if err := apiGet("/api/sessions", &resp); err != nil {
		a.win.Update(func() { a.status = "会话列表: " + err.Error() })
		return
	}
	sort.Slice(resp.Sessions, func(i, j int) bool {
		return resp.Sessions[i].UpdatedAt.After(resp.Sessions[j].UpdatedAt)
	})
	a.win.Update(func() { a.sessions = resp.Sessions })
	if a.getSection() != secChat {
		return
	}
	if a.getSelected() == "" && len(resp.Sessions) > 0 {
		id := resp.Sessions[0].ID
		a.setSelected(id)
		go a.loadSession(id, true)
	}
}

func (a *app) loadSession(id string, showLoading bool) {
	if showLoading {
		a.win.Update(func() {
			if a.activeID() == id {
				a.status = "加载中…"
			}
		})
	}
	var resp sessionResponse
	if err := apiGet(sessionPath(id), &resp); err != nil {
		a.win.Update(func() {
			if a.activeID() == id {
				a.setRunning(false)
				a.status = err.Error()
			}
		})
		return
	}
	a.win.Update(func() {
		if a.activeID() != id {
			return
		}
		oldID := ""
		if a.cur != nil {
			oldID = a.cur.ID
		}
		a.cur = &resp.Session
		if oldID != resp.Session.ID && resp.Session.Provider != "" {
			a.modelRef = modelRefOf(resp.Session.Provider, resp.Session.Model)
		}
		switch {
		case resp.Interaction == nil:
			a.pending = nil
			a.interactionDone = ""
		case resp.Interaction.ID == a.interactionDone:
			a.pending = nil
		default:
			a.pending = resp.Interaction
		}
		a.agentRef = resp.Session.Agent
		a.syncAgentFeatures()
		a.setRunning(resp.Running)
		if !resp.Running && (a.status == "加载中…" || a.status == "正在生成…" || a.status == "正在停止…") {
			a.status = ""
		}
		if showLoading {
			a.msgList.ScrollToEnd()
		}
	})
}

func (a *app) openSession(id string) {
	if id == "" {
		return
	}
	a.setSection(secChat)
	a.setSelected(id)
	a.menuFor = ""
	a.editingID = ""
	a.editDraft = ""
	a.pending = nil
	a.attachments = nil
	a.skillsOn = nil
	a.sidebarOpen = false
	a.contextOpen = false
	a.contextFor = ""
	a.contextFiles = nil
	a.contextErr = ""
	a.msgList.ScrollToEnd()
	go a.loadSession(id, true)
}

func (a *app) send() {
	id, running := a.snapshot()
	text := strings.TrimSpace(a.draft)
	attachments := append([]attachmentRef{}, a.attachments...)
	skills := append([]string{}, a.skillsOn...)
	if id == "" || running || a.attaching || a.getSection() != secChat || (text == "" && len(attachments) == 0) {
		return
	}
	a.draft = ""
	a.attachments = nil
	a.skillsOn = nil
	a.setRunning(true)
	a.status = "正在生成…"
	localID := fmt.Sprintf("local-%d", time.Now().UnixNano())
	if a.cur != nil && a.cur.ID == id {
		content := text
		for _, att := range attachments {
			content += "\n\n[附件 " + att.Name + "]"
		}
		a.cur.Messages = append(a.cur.Messages, models.Message{
			ID: localID, Role: "user", Content: content,
			AuthorID: models.AuthorHuman, AuthorKind: models.AuthorKindHuman,
			Timestamp: time.Now(),
		})
	}
	a.msgList.ScrollToEnd()
	a.win.Update(func() {})
	provider, model := a.selectedModel()
	body := map[string]any{"text": text}
	if provider != "" {
		body["provider"] = provider
		body["model"] = model
	}
	if len(skills) > 0 {
		body["skills"] = skills
	}
	if len(attachments) > 0 {
		body["attachments"] = attachments
	}
	go func() {
		var out struct {
			OK bool `json:"ok"`
		}
		if err := apiPost(sessionPath(id)+"/messages", body, &out); err != nil {
			a.win.Update(func() {
				if a.cur != nil && a.cur.ID == id {
					for i := len(a.cur.Messages) - 1; i >= 0; i-- {
						if a.cur.Messages[i].ID == localID {
							a.cur.Messages = append(a.cur.Messages[:i], a.cur.Messages[i+1:]...)
							break
						}
					}
				}
				// The unsent message comes back only when the same session
				// is still on screen, and only when nothing newer was typed,
				// so the failure never clobbers another conversation's draft.
				if a.activeID() == id && strings.TrimSpace(a.draft) == "" {
					a.draft = text
				}
				if a.activeID() == id {
					a.attachments = attachments
					a.skillsOn = skills
				}
				a.setRunning(false)
				a.status = err.Error()
			})
			return
		}
		a.loadSession(id, false)
	}()
}

func (a *app) stop() {
	id := a.getSelected()
	if id == "" {
		return
	}
	a.status = "正在停止…"
	a.win.Update(func() {})
	go func() {
		if err := apiPost(sessionPath(id)+"/cancel", nil, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
		}
	}()
}

func (a *app) getSelected() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.selected
}

func (a *app) setSelected(id string) {
	a.mu.Lock()
	a.selected = id
	a.mu.Unlock()
}

func (a *app) snapshot() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.selected, a.running
}

func (a *app) setRunning(v bool) {
	a.mu.Lock()
	a.running = v
	a.mu.Unlock()
}

func (a *app) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *app) current() *models.Conversation { return a.cur }

// Sections of the web client: one conversation list, group chats and the
// scheduled tasks.
const (
	secChat  = "chat"
	secRooms = "rooms"
	secTasks = "tasks"
)

func (a *app) getSection() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.section == "" {
		return secChat
	}
	return a.section
}

func (a *app) setSection(section string) {
	a.mu.Lock()
	a.section = section
	a.mu.Unlock()
}

// selectSection switches the visible list and conversation, reloading what
// the new section shows.
func (a *app) selectSection(section string) {
	if a.getSection() == section {
		return
	}
	a.setSection(section)
	a.roomSessionsOpen = false
	a.taskSessionsOpen = false
	a.pending = nil
	a.attachments = nil
	a.skillsOn = nil
	a.editingID = ""
	a.editDraft = ""
	a.draft = ""
	a.cur = nil
	a.setRunning(false)
	a.win.Update(func() {})
	go func() {
		switch section {
		case secRooms:
			a.refreshRooms()
		case secTasks:
			a.refreshTasks()
		default:
			a.refreshSessions()
		}
		switch section {
		case secRooms:
			if id := a.getRoomConv(); id != "" {
				a.loadSession(id, true)
			}
		case secTasks:
			if id := a.getTaskConv(); id != "" {
				a.loadSession(id, true)
			}
		default:
			if id := a.getSelected(); id != "" {
				a.loadSession(id, true)
			}
		}
	}()
}

func (a *app) getRoomID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.roomID
}

func (a *app) setRoomID(id string) {
	a.mu.Lock()
	a.roomID = id
	a.mu.Unlock()
}

func (a *app) getTaskID() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.taskID
}

func (a *app) setTaskID(id string) {
	a.mu.Lock()
	a.taskID = id
	a.mu.Unlock()
}

func (a *app) getRoomConv() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.roomConv
}

func (a *app) setRoomConv(id string) {
	a.mu.Lock()
	a.roomConv = id
	a.mu.Unlock()
}

func (a *app) getTaskConv() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.taskConv
}

func (a *app) setTaskConv(id string) {
	a.mu.Lock()
	a.taskConv = id
	a.mu.Unlock()
}

// activeID is the conversation the current section shows, the one polling
// keeps fresh.
func (a *app) activeID() string {
	switch a.getSection() {
	case secRooms:
		return a.getRoomConv()
	case secTasks:
		return a.getTaskConv()
	default:
		return a.getSelected()
	}
}

// sessionAgent is the conversation's desktop agent reference. The web client
// only runs a plain chat model, so an agent session is read-only there.
func (a *app) sessionAgent() string {
	if a.cur == nil {
		return ""
	}
	return strings.TrimSpace(a.cur.Agent)
}

func (a *app) selectedModel() (string, string) {
	provider, model := splitModelRef(a.modelRef)
	if provider == "" && model == "" {
		return a.meta.Provider, a.meta.Model
	}
	if provider == "" {
		provider = a.meta.Provider
	}
	return provider, model
}

func modelRefOf(provider, model string) string {
	if provider == "" && model == "" {
		return ""
	}
	return provider + "::" + model
}

func splitModelRef(ref string) (string, string) {
	if i := strings.Index(ref, "::"); i >= 0 {
		return ref[:i], ref[i+2:]
	}
	return "", ref
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func main() {
	mygo.App.WhenReady(func() {
		a := &app{
			folds:           map[string]bool{},
			foldLive:        map[string]bool{},
			collapsedGroups: map[string]bool{},
			askAnswers:      map[string]string{},
			askInputs:       map[string]string{},
			toolsFor:        "\x00",
			skillsFor:       "\x00",
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "ChatGo",
			Width:   1180,
			Height:  820,
			Content: ui.View(a.view),
		})
		go a.start()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
