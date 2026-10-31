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
	collapsedGroups map[string]bool

	msgList kit.MessageListState
}

func (a *app) start() {
	var m metaInfo
	if err := apiGet("/api/meta", &m); err != nil {
		a.win.Update(func() { a.status = err.Error() })
	} else {
		a.win.Update(func() {
			a.meta = m
			if a.modelRef == "" {
				a.modelRef = modelRefOf(m.Provider, m.Model)
			}
		})
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
		if i%4 == 0 {
			a.refreshSessions()
		}
		id, running := a.snapshot()
		if id != "" && (running || i%4 == 0) {
			a.loadSession(id, false)
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
	if a.getSelected() == "" && len(resp.Sessions) > 0 {
		id := resp.Sessions[0].ID
		a.setSelected(id)
		go a.loadSession(id, true)
	}
}

func (a *app) loadSession(id string, showLoading bool) {
	if showLoading {
		a.win.Update(func() {
			if a.getSelected() == id {
				a.status = "加载中…"
			}
		})
	}
	var resp sessionResponse
	if err := apiGet(sessionPath(id), &resp); err != nil {
		a.win.Update(func() {
			if a.getSelected() == id {
				a.setRunning(false)
				a.status = err.Error()
			}
		})
		return
	}
	a.win.Update(func() {
		if a.getSelected() != id {
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
		a.setRunning(resp.Running)
		if !resp.Running && (a.status == "加载中…" || a.status == "正在生成…" || a.status == "正在停止…") {
			a.status = ""
		}
		if showLoading {
			a.msgList.ScrollToEnd()
		}
	})
}

func (a *app) newSession() {
	a.win.Update(func() { a.status = "创建中…" })
	provider, model := a.selectedModel()
	body := map[string]string{}
	if provider != "" {
		body["provider"] = provider
		body["model"] = model
	}
	go func() {
		var resp sessionResponse
		if err := apiPost("/api/sessions", body, &resp); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.setSelected(resp.Session.ID)
		a.win.Update(func() {
			a.cur = &resp.Session
			a.modelRef = modelRefOf(resp.Session.Provider, resp.Session.Model)
			a.setRunning(false)
			a.status = ""
			a.sidebarOpen = false
			a.contextOpen = false
			a.contextFor = ""
			a.contextFiles = nil
			a.contextErr = ""
			a.msgList.ScrollToEnd()
		})
		a.refreshSessions()
	}()
}

func (a *app) openSession(id string) {
	if id == "" {
		return
	}
	a.setSelected(id)
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
	if id == "" || text == "" || running {
		return
	}
	a.draft = ""
	a.setRunning(true)
	a.status = "正在生成…"
	localID := fmt.Sprintf("local-%d", time.Now().UnixNano())
	if a.cur != nil && a.cur.ID == id {
		a.cur.Messages = append(a.cur.Messages, models.Message{
			ID: localID, Role: "user", Content: text,
			AuthorID: models.AuthorHuman, AuthorKind: models.AuthorKindHuman,
			Timestamp: time.Now(),
		})
	}
	a.msgList.ScrollToEnd()
	a.win.Update(func() {})
	provider, model := a.selectedModel()
	body := map[string]string{"text": text}
	if provider != "" {
		body["provider"] = provider
		body["model"] = model
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
		a := &app{folds: map[string]bool{}, collapsedGroups: map[string]bool{}}
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
