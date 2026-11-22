// Command mobile is ChatGo's native phone app. It connects to a self-hosted
// relay, reads the same /api as the browser, and keeps Chat, Rooms and Tasks
// in one native MyGo window.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/push"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

const (
	tabChat = iota
	tabRooms
	tabTasks
	tabSettings
)

type screen int

const (
	screenList screen = iota
	screenChat
	screenRooms
	screenRoom
	screenTasks
	screenTask
	screenSettings
	screenNew
)

type server struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RelayURL  string `json:"relay_url"`
	DesktopID string `json:"desktop_id"`
	DeviceID  string `json:"device_id,omitempty"`
	Token     string `json:"-"`
	E2EKey    []byte `json:"-"`
}

type app struct {
	dispatch   func(func())
	storageDir string
	win        *mygo.Window

	servers          []server
	server           int
	client           *mobileapi.Client
	connErr          string
	status           string
	connecting       bool
	connected        bool
	eventsConnected  bool
	connectionCancel context.CancelFunc
	creating         bool

	screen screen
	tab    int

	meta   mobileapi.Meta
	agents []mobileapi.Agent
	// agentIcons holds the icons the desktop configured for its agents, so
	// the list and the picker draw each agent's own mark.
	agentIcons map[string]ui.ImageSource
	sessions   []mobileapi.SessionSummary
	// listQuery narrows the session list to the rows matching it.
	listQuery string
	openConv  string
	conv      *models.Conversation
	running   bool
	draft     string
	// chatScroll follows the conversation's end while a turn runs, unless
	// the reader scrolled up; followEnd brings a just-opened or just-sent
	// conversation into view.
	chatScroll ui.ScrollState
	followEnd  bool

	rooms    []mobileapi.Room
	roomID   string
	roomName string
	roomSess []mobileapi.RoomSession

	tasks    []mobileapi.Task
	taskID   string
	task     *mobileapi.Task
	taskSess []mobileapi.RoomSession

	interaction *mobileapi.Interaction
	askOpen     bool
	answer      string

	pairOpen    bool
	pairRelay   string
	pairDesktop string
	pairCode    string
	pairName    string
	pairErr     string
	pairing     bool
	scanning    bool
	// pairRequired is set when the desktop or the relay refuses the device
	// token: the phone has to pair again before it can be used.
	pairRequired bool

	serverFormOpen bool
	serverName     string

	roomFormOpen bool
	roomEditing  string
	roomFormName string
	roomFormSel  map[string]bool

	taskFormOpen bool
	taskForm     mobileapi.TaskInput
	taskHour     string
	taskMinute   string
	taskWeekday  string
	taskDay      string
	taskEditing  string

	attachments []mobileapi.Attachment
	attachBusy  bool

	themeMode  mcore.Mode
	colorTheme mtheme.Theme

	newDraft     string
	newAgent     string
	newModel     string
	newWorkspace string
	// chatModel is the model the open conversation sends with; modelFor says
	// which composer the model sheet is choosing for.
	chatModel string
	// newACP and chatACP are the ACP runtimes' model lists, read when their
	// model pickers open.
	newACP        acpModels
	chatACP       acpModels
	modelFor      string
	modelFilter   string
	newFiles      []newFile
	workspaces    []string
	workspaceOpen bool
	agentOpen     bool
	modelOpen     bool

	toolOpen    map[string]bool
	thinkOpen   map[string]bool
	foldOpen    map[string]bool
	foldLive    map[string]bool
	watchCancel context.CancelFunc
	eventCancel context.CancelFunc
	eventSeq    atomic.Int64
}

var current *app
var pendingURL string

func main() {
	mygo.App.OnOpenURL(func(raw string) {
		if current != nil {
			current.handleURL(raw)
		} else {
			pendingURL = raw
		}
	})
	mygo.App.WhenReady(func() {
		a := &app{
			tab:         tabChat,
			themeMode:   mcore.System,
			colorTheme:  mtheme.ByName(""),
			toolOpen:    map[string]bool{},
			thinkOpen:   map[string]bool{},
			foldOpen:    map[string]bool{},
			foldLive:    map[string]bool{},
			roomFormSel: map[string]bool{},
		}
		current = a
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "ChatGo",
			Width:   420,
			Height:  860,
			Content: ui.View(a.view),
		})
		a.screen = screenList
		a.start()
		if pendingURL != "" {
			raw := pendingURL
			pendingURL = ""
			a.handleURL(raw)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) start() {
	a.loadServers()
	if len(a.servers) == 0 {
		// Show the empty state rather than opening a sheet before the first
		// frame; the sheet's entrance animation needs a running window.
		a.pairOpen = false
		a.invalidate()
		return
	}
	if a.server < 0 || a.server >= len(a.servers) {
		a.server = 0
	}
	a.connect(a.servers[a.server])
	a.startPush()
}

func (a *app) connect(s server) {
	a.disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	a.connectionCancel = cancel
	a.stopEvents()
	a.eventSeq.Store(0)
	a.client = mobileapi.New(apiBase(s.RelayURL)+"/d/"+s.DesktopID, s.Token, s.DesktopID)
	a.client.E2EKey = s.E2EKey
	a.client.DeviceID = s.DeviceID
	a.connErr = ""
	a.connecting = true
	a.status = "正在连接桌面…"
	client := a.client
	client.OnAuthError = func(error) { a.onAuthError(client) }
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		meta, err := client.Meta(ctx)
		if err != nil {
			a.update(func() {
				if a.client != client {
					return
				}
				a.connecting = false
				a.connErr = err.Error()
				a.status = "连接失败"
			})
			return
		}
		a.update(func() {
			if a.client != client {
				return
			}
			a.connecting, a.connected = false, true
			a.status = ""
			a.pairRequired = false
			a.meta = meta
			a.themeMode = mcore.ParseMode(meta.Theme.Mode)
			a.colorTheme = mtheme.ByName(meta.Theme.Color)
			a.ensureNewChoices()
			a.startEvents()
			if a.openConv != "" {
				a.loadChat(a.openConv)
			}
		})
		if agents, err := client.Agents(ctx); err == nil {
			a.update(func() {
				if a.client == client {
					a.agents = agents
					a.ensureNewChoices()
				}
			})
			a.loadAgentIcons(client, agents)
		}
		if workspaces, err := client.Workspaces(ctx); err == nil {
			a.update(func() {
				if a.client == client {
					a.workspaces = workspaces
					if a.newWorkspace != "" && !slices.Contains(workspaces, a.newWorkspace) {
						a.newWorkspace = ""
					}
				}
			})
		}
		a.update(func() {
			if a.client != client {
				return
			}
			a.refreshSessions()
			a.refreshRooms()
			a.refreshTasks()
		})
	}()
}

// loadAgentIcons fetches the icons the desktop configured for its agents.
// Agents without one keep the mark of their kind.
func (a *app) loadAgentIcons(client *mobileapi.Client, agents []mobileapi.Agent) {
	icons := map[string]ui.ImageSource{}
	for _, agent := range agents {
		if agent.Ref == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		data, err := client.AgentIcon(ctx, agent.Ref)
		cancel()
		if err != nil || len(data) == 0 {
			continue
		}
		if src := decodeIcon(data); src != nil {
			icons[agent.Ref] = src
		}
	}
	if len(icons) == 0 {
		return
	}
	a.update(func() {
		if a.client != client {
			return
		}
		if a.agentIcons == nil {
			a.agentIcons = map[string]ui.ImageSource{}
		}
		for ref, src := range icons {
			a.agentIcons[ref] = src
		}
	})
}

// decodeIcon reads an icon file as an SVG or a bitmap.
func decodeIcon(data []byte) ui.ImageSource {
	if svg, err := ui.ParseSVG(data); err == nil && svg != nil {
		return svg
	}
	if bmp, err := ui.DecodeBitmap(data); err == nil && bmp != nil {
		return bmp
	}
	return nil
}

// agentMark draws an agent's configured icon, or the mark of its kind.
func (a *app) agentMark(c *ui.Context, ref string, size float32) {
	a.agentMarkColor(c, ref, size, c.Theme().TextMuted)
}

// agentMarkColor draws an agent's mark in the given color.
func (a *app) agentMarkColor(c *ui.Context, ref string, size float32, color ui.Color) {
	if src := a.agentIcons[ref]; src != nil {
		ui.Image(c, src).Size(size, size).Shrink(0)
		return
	}
	ui.Icon(c, icons.Must(sessionIcon(ref))).Size(size, size).TextColor(color).Shrink(0)
}

// disconnect keeps pairing credentials and drafts so reconnecting needs no
// new pairing code, while cancelling work owned by the previous connection.
func (a *app) disconnect() {
	if a.connectionCancel != nil {
		a.connectionCancel()
		a.connectionCancel = nil
	}
	a.stopEvents()
	a.stopWatch()
	a.client = nil
	a.connected, a.connecting, a.creating = false, false, false
	a.eventsConnected = false
	a.connErr = ""
	a.status = "已断开连接"
}

func (a *app) refreshSessions() {
	if a.client == nil {
		return
	}
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sessions, err := client.Sessions(ctx)
		a.update(func() {
			if a.client != client {
				return
			}
			if err != nil {
				a.status = "会话: " + err.Error()
				return
			}
			a.sessions = sessions
		})
	}()
}

func (a *app) refreshRooms() {
	if a.client == nil {
		return
	}
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		rooms, err := client.Rooms(ctx)
		a.update(func() {
			if a.client != client {
				return
			}
			if err != nil {
				a.status = "群聊: " + err.Error()
				return
			}
			a.rooms = rooms
		})
	}()
}

func (a *app) refreshTasks() {
	if a.client == nil {
		return
	}
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		tasks, err := client.Tasks(ctx)
		a.update(func() {
			if a.client != client {
				return
			}
			if err != nil {
				a.status = "任务: " + err.Error()
				return
			}
			a.tasks = tasks
		})
	}()
}

func (a *app) startEvents() {
	if a.client == nil {
		return
	}
	client := a.client
	ctx, cancel := context.WithCancel(context.Background())
	a.eventCancel = cancel
	go func() {
		for ctx.Err() == nil {
			conn, err := client.DialEvents(ctx, a.eventSeq.Load(), "")
			if err != nil {
				a.update(func() {
					if a.client == client {
						a.eventsConnected = false
					}
				})
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}
			a.update(func() {
				if a.client == client {
					a.eventsConnected = true
					if a.openConv != "" {
						a.refreshCurrent(a.openConv)
					}
				}
			})
			closed := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					_ = conn.Close()
				case <-closed:
				}
			}()
			for ctx.Err() == nil {
				event, err := mobileapi.ReadEvent(conn, client.E2EKey, client.DesktopID)
				if err != nil {
					break
				}
				a.update(func() {
					if a.client == client && ctx.Err() == nil {
						if event.Seq > a.eventSeq.Load() {
							a.eventSeq.Store(event.Seq)
						}
						a.handleEvent(event)
					}
				})
			}
			close(closed)
			_ = conn.Close()
			a.update(func() {
				if a.client == client {
					a.eventsConnected = false
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
}

func (a *app) stopEvents() {
	if a.eventCancel != nil {
		a.eventCancel()
		a.eventCancel = nil
	}
}

func (a *app) handleEvent(e mobileapi.Event) {
	switch e.Type {
	case "session.updated", "turn.started", "turn.finished":
		if e.SessionID != "" && e.SessionID == a.openConv {
			a.refreshCurrent(e.SessionID)
		}
		a.refreshSessions()
	case "session.created", "session.deleted":
		a.refreshSessions()
	case "room.updated", "room.deleted":
		a.refreshRooms()
		if e.SessionID != "" && e.SessionID == a.roomID {
			a.loadRoomSessions(a.roomID)
		}
	case "task.updated", "task.deleted":
		a.refreshTasks()
	case "resync":
		a.refreshSessions()
		a.refreshRooms()
		a.refreshTasks()
		if a.openConv != "" {
			a.refreshCurrent(a.openConv)
		}
	}
}

func (a *app) update(fn func()) {
	if a.dispatch != nil {
		a.dispatch(fn)
	} else if a.win != nil {
		a.win.Update(fn)
	} else {
		fn()
	}
}

func (a *app) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) view(c *ui.Context) {
	mcore.Use(c, mcore.Settings{
		Mode:    a.themeMode,
		Density: mtheme.Comfortable,
		Locale:  mcore.ZhCN,
		Theme:   a.colorTheme,
	})
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.topBar(c)
		switch a.screen {
		case screenList:
			a.chatList(c)
		case screenChat:
			a.chatView(c)
		case screenRooms:
			a.roomList(c)
		case screenRoom:
			a.roomView(c)
		case screenTasks:
			a.taskList(c)
		case screenTask:
			a.taskView(c)
		case screenSettings:
			a.settingsView(c)
		case screenNew:
			a.newSessionPage(c)
		}
		if a.tabBarVisible() {
			a.bottomBar(c)
		}
		a.pairSheet(c)
		a.askSheet(c)
		a.workspaceSheet(c)
		a.agentSheet(c)
		a.modelSheet(c)
		a.roomFormSheet(c)
		a.taskFormSheet(c)
	})
}

func (a *app) tabBarVisible() bool {
	switch a.screen {
	case screenList, screenRooms, screenTasks, screenSettings, screenNew:
		return !a.pairOpen
	}
	return false
}

func (a *app) topBar(c *ui.Context) {
	title := "会话"
	leading := func() {}
	trailing := func() {}
	switch a.screen {
	case screenList:
		// The list's own actions: refresh here, a new conversation from the
		// button floating over the rows.
		if !a.offline() {
			trailing = func() {
				if smallIconButton(c, "refresh-cw", "刷新").Clicked() {
					a.refreshSessions()
				}
			}
		}
	case screenChat:
		title = "对话"
		if a.conv != nil && strings.TrimSpace(a.conv.Title) != "" {
			title = a.conv.Title
		}
		leading = func() {
			if smallIconButton(c, "chevron-left", "返回").Clicked() {
				a.backFromChat()
			}
		}
	case screenRooms:
		title = "群聊"
		trailing = func() {
			if !a.offline() && smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.refreshRooms()
			}
		}
	case screenRoom:
		title = a.roomName
		leading = func() {
			if smallIconButton(c, "chevron-left", "返回").Clicked() {
				a.backToRooms()
			}
		}
		trailing = func() {
			if !a.offline() && smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.loadRoomSessions(a.roomID)
			}
		}
	case screenTasks:
		title = "自动任务"
		trailing = func() {
			if !a.offline() && smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.refreshTasks()
			}
		}
	case screenTask:
		title = "任务"
		leading = func() {
			if smallIconButton(c, "chevron-left", "返回").Clicked() {
				a.backToTasks()
			}
		}
		trailing = func() {
			if !a.offline() && smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.loadTaskSessions(a.taskID)
			}
		}
	case screenSettings:
		title = "设置"
	case screenNew:
		title = "新会话"
		trailing = func() {
			if smallIconButton(c, "history", "历史会话").Clicked() {
				a.screen = screenList
				a.refreshSessions()
			}
		}
	}
	mobile.TopBar(c, mobile.TopBarOptions{
		Title:    title,
		Leading:  leading,
		Trailing: trailing,
	})
	// The status strip only appears when there is something to act on. A
	// healthy link needs no line on every page; settings has the live state
	// in its own rows, so it does not repeat it here.
	if len(a.servers) > 0 && !(a.connected && a.eventsConnected) && a.screen != screenSettings {
		t := c.Theme()
		label, color := "已断开", t.TextMuted
		switch {
		case a.connecting:
			label = "正在连接…"
		case a.connErr != "":
			// The failure detail lives in the offline page and in settings.
			// The strip stays a calm status line.
			label = "未连接"
		case a.connected:
			label, color = "实时连接恢复中…", t.Warning
		}
		ui.Row(c).FillWidth().Padding(2, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(color).Shrink(0)
			ui.Text(c, firstNonEmpty(a.currentServer().Name, "桌面")+" · "+label).
				FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		})
	}
	if a.status != "" && a.connErr == "" {
		ui.Text(c, a.status).FontSize(12).TextColor(c.Theme().TextMuted).Padding(4, 16).MaxLines(2)
	}
}

func (a *app) bottomBar(c *ui.Context) {
	// The new-session page is a step inside 会话 rather than a tab of its
	// own, so no tab is marked while it shows: tapping 会话 there goes back
	// to the list.
	selected := a.tab
	if a.screen == screenNew {
		selected = -1
	}
	bar := mobile.BottomBar(c, &selected,
		mobile.BarButton{Label: "会话", Icon: iconChat},
		mobile.BarButton{Label: "群聊", Icon: iconRooms},
		mobile.BarButton{Label: "任务", Icon: iconTasks},
		mobile.BarButton{Label: "设置", Icon: iconSettings},
	)
	if bar.Changed() {
		a.tab = selected
		a.stopWatch()
		switch a.tab {
		case tabChat:
			// The tab opens the history; a new conversation starts from the
			// button floating over it.
			a.screen = screenList
			a.stopWatch()
			a.openConv = ""
			a.refreshSessions()
		case tabRooms:
			a.screen = screenRooms
			a.refreshRooms()
		case tabTasks:
			a.screen = screenTasks
			a.refreshTasks()
		case tabSettings:
			a.screen = screenSettings
		}
	}
}

func (a *app) backFromChat() {
	a.stopWatch()
	if a.roomID != "" {
		a.screen = screenRoom
		a.openConv = ""
		a.loadRoomSessions(a.roomID)
		return
	}
	a.backToList()
}

func (a *app) backToList() {
	a.stopWatch()
	a.screen = screenList
	a.openConv = ""
	a.refreshSessions()
}

func (a *app) backToRooms() {
	a.stopWatch()
	a.screen = screenRooms
	a.refreshRooms()
}

func (a *app) backToTasks() {
	a.stopWatch()
	a.screen = screenTasks
	a.refreshTasks()
}

func (a *app) stopWatch() {
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
}

func (a *app) handleURL(raw string) {
	u, err := parseDeepLink(raw)
	if err != nil {
		return
	}
	switch u.kind {
	case "pair":
		// A scanned code pairs straight away, as the sheet promises: the
		// empty state shows the progress, and a sheet opened from a posted
		// change would stay below the screen.
		a.update(func() {
			a.pairRelay = firstNonEmpty(u.relay, a.pairRelay)
			a.pairDesktop = firstNonEmpty(u.desktop, a.pairDesktop)
			a.pairCode = firstNonEmpty(u.code, a.pairCode)
			a.pairErr = ""
			a.pair()
		})
	case "session":
		if u.session != "" {
			a.openChat(u.session, "", "")
		}
	case "room":
		if u.room != "" {
			a.openRoom(u.room)
		}
	case "task":
		if u.task != "" {
			a.openTask(u.task)
		}
	}
}

func (a *app) startPush() {
	go func() {
		ctx := context.Background()
		token, err := push.Register(ctx)
		if err != nil || token == "" {
			return
		}
		provider := ""
		switch runtime.GOOS {
		case "ios":
			provider = "apns"
		case "android":
			provider = "fcm"
		default:
			return
		}
		if client := a.client; client != nil {
			_ = client.RegisterPush(ctx, apiBase(a.currentServer().RelayURL), provider, token, "prod")
		}
	}()
	push.OnOpened(func(data []byte) {
		var payload struct {
			SessionID string `json:"session_id"`
			Kind      string `json:"kind"`
		}
		if json.Unmarshal(data, &payload) == nil && payload.SessionID != "" {
			a.openChat(payload.SessionID, "", "")
		}
	})
}

func (a *app) currentServer() server {
	if a.server >= 0 && a.server < len(a.servers) {
		return a.servers[a.server]
	}
	return server{}
}

func apiBase(relay string) string {
	relay = strings.TrimSpace(relay)
	relay = strings.TrimSuffix(relay, "/")
	relay = strings.Replace(relay, "wss://", "https://", 1)
	relay = strings.Replace(relay, "ws://", "http://", 1)
	return relay
}

func (a *app) dataDir() string {
	if a.storageDir != "" {
		return a.storageDir
	}
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".chatgo", "mobile")
}

func (a *app) serversPath() string {
	return filepath.Join(a.dataDir(), "servers.json")
}

func (a *app) loadServers() {
	data, err := os.ReadFile(a.serversPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &a.servers)
	for i := range a.servers {
		a.servers[i].Token = loadToken(a.servers[i].DesktopID)
		a.servers[i].E2EKey = loadE2EKey(a.servers[i].DesktopID)
	}
}

func (a *app) saveServers() {
	_ = os.MkdirAll(a.dataDir(), 0o700)
	var out []server
	for _, s := range a.servers {
		s.Token = ""
		out = append(out, s)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(a.serversPath(), data, 0o600)
}

func (a *app) addServer(s server) {
	replaced := false
	for i := range a.servers {
		if a.servers[i].DesktopID == s.DesktopID {
			a.servers[i] = s
			a.server = i
			replaced = true
			break
		}
	}
	if !replaced {
		a.servers = append(a.servers, s)
		a.server = len(a.servers) - 1
	}
	a.saveServers()
}

func (a *app) removeServer(i int) {
	if i < 0 || i >= len(a.servers) {
		return
	}
	// Removing a desktop the phone is not talking to must leave the live
	// connection alone; removing the current one moves to a neighbour.
	current := i == a.server
	deleteToken(a.servers[i].DesktopID)
	deleteE2EKey(a.servers[i].DesktopID)
	a.servers = append(a.servers[:i], a.servers[i+1:]...)
	if current {
		if a.server >= len(a.servers) {
			a.server = len(a.servers) - 1
		}
		if a.server < 0 {
			a.server = 0
		}
	} else if i < a.server {
		a.server--
	}
	a.saveServers()
	if !current {
		return
	}
	if len(a.servers) > 0 {
		a.connect(a.servers[a.server])
	} else {
		a.disconnect()
		a.pairOpen = true
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (a *app) toast(text string) {
	a.update(func() { a.status = text })
}

func (a *app) errorf(format string, args ...any) {
	a.update(func() { a.status = fmt.Sprintf(format, args...) })
}

// ensureNewChoices drops an agent, model or project the newly connected
// desktop does not offer, so switching desktops never sends stale refs.
func (a *app) ensureNewChoices() {
	if a.newAgent != "" {
		found := false
		for _, agent := range a.agents {
			if agent.Ref == a.newAgent {
				found = true
				break
			}
		}
		if !found {
			a.newAgent = ""
		}
	}
	if len(a.meta.Models) == 0 {
		a.newModel = ""
		a.chatModel = ""
		return
	}
	valid := func(ref string) bool {
		for _, m := range a.meta.Models {
			if m.Provider+"::"+m.Model == ref {
				return true
			}
		}
		return false
	}
	if !valid(a.newModel) {
		m := a.meta.Models[0]
		a.newModel = m.Provider + "::" + m.Model
	}
	if !valid(a.chatModel) {
		a.chatModel = a.newModel
	}
}

// offline reports a missing desktop or a link that failed to come up.
func (a *app) offline() bool { return a.client == nil || a.connErr != "" }

func (a *app) connectionError(client *mobileapi.Client, format string, args ...any) {
	a.update(func() {
		if a.client == client {
			a.status = fmt.Sprintf(format, args...)
		}
	})
}

// onAuthError handles a refused device token: the desktop dropped this
// phone, so the saved token is dead and the app asks for a new pairing
// instead of retrying a request that can never succeed.
func (a *app) onAuthError(client *mobileapi.Client) {
	a.update(func() {
		if a.client != client {
			return
		}
		srv := a.currentServer()
		if srv.DesktopID != "" {
			deleteToken(srv.DesktopID)
			deleteE2EKey(srv.DesktopID)
		}
		for i := range a.servers {
			if a.servers[i].DesktopID == srv.DesktopID {
				a.servers[i].Token = ""
				a.servers[i].E2EKey = nil
			}
		}
		a.saveServers()
		a.stopEvents()
		a.client = nil
		a.connected, a.connecting = false, false
		a.connErr = ""
		a.status = ""
		a.pairRequired = true
	})
}

// openPairSheet opens the pairing sheet. It runs on the main thread, from a
// tap, so it sets the fields directly: a sheet opened by a posted change
// never gets its entrance frame and stays below the screen.
//
// rePair keeps the desktop the phone was paired with, so only a new code is
// needed; adding another desktop starts from empty fields.
func (a *app) openPairSheet(rePair bool) {
	a.pairOpen = true
	a.pairErr = ""
	a.pairCode = ""
	if !rePair {
		a.pairRelay, a.pairDesktop = "", ""
		return
	}
	srv := a.currentServer()
	if srv.RelayURL != "" {
		a.pairRelay = apiBase(srv.RelayURL)
	}
	if srv.DesktopID != "" {
		a.pairDesktop = srv.DesktopID
	}
}
