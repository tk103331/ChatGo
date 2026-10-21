// Package ui renders ChatGo with MyGo's native UI toolkit and the MujicaUI
// component library. The window's state lives in ChatWindow; its view method
// builds the interface from that state on every frame. Cohesive state clusters
// (ACP runtime, skills browser, MCP page) live in their own structs embedded in
// ChatWindow and are defined alongside the code that drives them.
package ui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"chatgo/internal/apps/browserapp"
	"chatgo/internal/apps/codeeditorapp"
	"chatgo/internal/apps/gitapp"
	"chatgo/internal/apps/dockerapp"
	"chatgo/internal/apps/filemanagerapp"
	"chatgo/internal/apps/logapp"
	"chatgo/internal/apps/redisapp"
	"chatgo/internal/apps/markdownapp"
	"chatgo/internal/apps/terminalapp"
	"chatgo/internal/apps/webapp"
	"chatgo/internal/config"
	"chatgo/internal/llm"
	agentspkg "chatgo/internal/ui/agents"
	"chatgo/internal/ui/assets"
	autotaskpkg "chatgo/internal/ui/autotask"
	"chatgo/internal/ui/chatpanel"
	mcppage "chatgo/internal/ui/mcppage"
	settingspkg "chatgo/internal/ui/settings"
	skillspkg "chatgo/internal/ui/skills"
	"chatgo/internal/ui/stats"
	"chatgo/internal/ui/theme"
	"chatgo/pkg/models"

	"github.com/cloudwego/eino/components/tool"
	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

const defaultTitle = "新会话"

// pageMode is which of the two windows the app shows.

type pageMode int

const (
	modeHome pageMode = iota
	modeChat
)

// ChatWindow is the application state the window draws.

type ChatWindow struct {
	cfg         *config.Config
	convManager *models.ConversationManager
	mcpManager  *MCPManagerWrapper

	winMu  sync.Mutex
	window *mygo.Window

	mode    pageMode
	section navSection

	quickMode  bool
	quickSaved *mygo.Rectangle

	appActive  string
	appLeftW   float32
	appLeftFor string
	appWinW    float32
	appsLoaded bool
	appTabs    map[string][]string
	appTab     map[string]string
	appConv    map[string]string
	appCtxTabs map[string][]string
	appCtxAll  map[string]bool
	appsGrid   mgo.GridState
	appChatHidden bool
	currentApp string

	gitCommitMsg string
	gitApp       *gitapp.Client
	termApp      *terminalapp.Client
	mdApp        *markdownapp.Client
	codeApp      *codeeditorapp.Client
	browserApp   *browserapp.Client
	logApp       *logapp.Client
	dockerApp    *dockerapp.Client
	redisApp     *redisapp.Client
	filesApp     *filemanagerapp.Client
	webApps      map[string]*webapp.Client
	webAppList   []webapp.App
	preAppConvID string

	rightPanel bool
	rightTab   int
	leftHidden bool
	leftW      float32

	webPanel   bool
	webURL     string
	webCurrent string
	webView    *mygo.WebView

	termOpen     bool
	terms        []*termTab
	termTabs     []mcode.TerminalTab
	termActiveID string
	termSeq      int
	termH        float32
	treeW        float32
	previewW     float32
	mainW        float32
	chatW        float32
	lastWinW     float32
	lastRightW   float32
	lastPreview  bool

	previewTabs []previewTab
	previewSel  int

	gitRepo        bool
	gitBranch      string
	gitBranches    []mgit.Branch
	gitChanges     []mgit.ChangedFile
	gitCommits     []mgit.Commit
	gitChangesList mgit.ChangesListState
	gitGraph       mgit.CommitGraphState
	gitCommit      mgit.CommitMessage
	gitCommitBusy  bool
	gitSplitH      float32
	gitStat        [2]int
	gitLoaded      bool
	gitFor         string

	currentAgent string
	systemPrompt string
	homeAgent    string

	convs       []models.Conversation
	taskConvs   []models.Conversation
	cur         *models.Conversation
	workspace   string
	wsCollapsed map[string]bool

	homeList mgo.ListState
	convList mgo.ListState
	panel    *chatpanel.Panel

	provider  string
	model     string
	agentMode bool

	searchQuery string

	llmClient   *llm.Client
	reactClient *llm.ReactClient

	streaming    bool
	streamCancel context.CancelFunc

	toolOn  map[string]bool
	skillOn map[string]bool

	status        string
	toastText     string
	toastSeverity mcore.Severity

	themeMode  mcore.Mode
	colorTheme theme.Theme
	density    mtheme.Density
	locale     mcore.Locale

	settings settingspkg.State
	confirm  confirmState
	input    inputState

	paletteOpen bool

	agents agentspkg.State

	autotasks autotaskpkg.State

	avatarMu  sync.Mutex
	avatarPNG map[string]*mgo.Bitmap

	iconMu    sync.Mutex
	iconCache map[string]iconImage

	acpList         mgo.ListState
	acpForm         acpForm
	acpCatalogOpen  bool
	acpCatalogQuery string

	mcp mcppage.State

	statsPage *stats.Page

	toolQuery string
	mcpQuery  string

	skills skillspkg.State

	acp acpState

	// clientDirty asks the next frame to rebuild the model client.
	clientDirty atomic.Bool

	// toolCache memoizes built tools per selection so a chat switch does not
	// re-list every MCP server, which blocks the UI thread.
	toolCache    []tool.BaseTool
	toolCacheKey string

	quickShortcut string

	tray              *mygo.Tray
	trayMenuKeyCached string
	quitting          bool

	channelMu      sync.Mutex
	channelRunning bool
	channelCancel  context.CancelFunc
	channelState   feishuConnState
	channelNote    string

	wechatMu      sync.Mutex
	wechatRunning bool
	wechatCancel  context.CancelFunc
	wechatState   feishuConnState
	wechatNote    string

	idMu  sync.Mutex
	idSeq uint64
}

// defaultQuickShortcut triggers the focused interface when unset.

const defaultQuickShortcut = "CmdOrCtrl+Shift+Space"

// quickChatShortcut is the configured accelerator for the focused interface.

func (cw *ChatWindow) quickChatShortcut() string {
	if s := strings.TrimSpace(cw.cfg.QuickChatShortcut); s != "" {
		return s
	}
	return defaultQuickShortcut
}

// NewChatWindow loads configuration and prepares the LLM and MCP clients.

func NewChatWindow(cfg *config.Config) (*ChatWindow, error) {
	cm, err := models.NewConversationManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation manager: %w", err)
	}
	cw := &ChatWindow{
		cfg:         cfg,
		convManager: cm,
		mcpManager:  NewMCPManagerWrapper(),
		provider:    cfg.CurrentProvider,
		agentMode:   cfg.UseReactAgent,
		toolOn:      map[string]bool{},
		skillOn:     map[string]bool{},
		acp:         newACPState(),
		mcp:         mcppage.State{},
		rightPanel:  false,
		wsCollapsed: map[string]bool{},
		themeMode:   theme.ParseMode(cfg.ThemeMode),
		colorTheme:  theme.ByName(cfg.ColorTheme),
		density:     mtheme.Compact,
		locale:      theme.ParseLocale(cfg.Locale),
		panel:       chatpanel.New(),
		statsPage:   stats.New(),
		agents:      agentspkg.State{SecModels: true, SecTools: true, SecMCP: true, SecSkills: true, Form: agentspkg.Form{Editing: -1}},
		autotasks:   autotaskpkg.State{Running: map[string]bool{}},
		avatarPNG:   map[string]*mgo.Bitmap{},
		iconCache:   map[string]iconImage{},
	}
	if wd, err := os.Getwd(); err == nil {
		cw.workspace = wd
	}
	cw.settings.Prov.Editing = -1
	cw.settings.Builtin.Editing = -1
	cw.settings.Channel.Editing = -1
	cw.settings.Channel.Typ = "feishu"
	cw.settings.Shortcut.Value = cw.quickChatShortcut()
	cw.acpForm.editing = -1
	cw.reloadConvs()
	cw.currentAgent = ""
	cw.applyAgent("")
	cw.loadWebApps()
	go cw.startMCP()
	go cw.autoTaskLoop()
	go cw.startFeishuChannel()
	go cw.startWechatChannel()
	return cw, nil
}

// Show creates the MyGo window; call it from mygo.App.WhenReady.

func (cw *ChatWindow) Show() {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:         "ChatGo",
		Width:         1000,
		Height:        700,
		MinWidth:      720,
		MinHeight:     520,
		TitleBarStyle: mygo.TitleBarHiddenInset,
		Content:       mgo.View(cw.view),
	})
	cw.winMu.Lock()
	cw.window = w
	cw.winMu.Unlock()
	_ = w.SetIcon(assets.LogoPNG())
	_ = mygo.App.Dock.SetIcon(assets.LogoPNG())
	cw.applyQuickShortcut()
	cw.syncTray()
	w.OnClose(func(e *mygo.CloseEvent) {
		if !cw.cfg.RunInBackground {
			return
		}
		if cw.quitting {
			cw.quitting = false
			return
		}
		e.PreventDefault()
		w.Hide()
	})
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) { cw.quitting = true })
	mygo.App.OnWillQuit(func(e *mygo.QuitEvent) { cw.quitting = false })
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			cw.openNormal()
		}
	})
}

// applyQuickShortcut registers the configured accelerator, replacing any
// previous one, and reports a failure through the status line.

func (cw *ChatWindow) applyQuickShortcut() {
	acc := cw.quickChatShortcut()
	if acc == cw.quickShortcut {
		return
	}
	if err := mygo.GlobalShortcut.Register(acc, func() {
		cw.openQuick()
	}); err != nil {
		cw.status = "快捷对话快捷键注册失败: " + err.Error()
		return
	}
	if cw.quickShortcut != "" {
		mygo.GlobalShortcut.Unregister(cw.quickShortcut)
	}
	cw.quickShortcut = acc
	cw.status = ""
}

// saveQuickShortcut persists the accelerator and re-registers it.

func (cw *ChatWindow) saveQuickShortcut() {
	_ = config.SaveConfig(cw.cfg)
	cw.applyQuickShortcut()
	cw.invalidate()
}

// view builds the whole interface for the frame.

func (cw *ChatWindow) view(c *mgo.Context) {
	light, dark := cw.colorTheme.Tokens()
	mcore.Use(c, mcore.Settings{Mode: cw.themeMode, Density: cw.density, Locale: cw.locale, Light: light, Dark: dark})
	if cw.clientDirty.Swap(false) {
		cw.setupProvider(cw.provider)
	}
	if cw.quickMode {
		cw.quickView(c)
	} else {
		mgo.Column(c).Fill().AlignItems(mgo.Stretch).Children(func() {
			cw.titleBar(c)
			mgo.Row(c).Grow(1).MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
				cw.navRail(c)
				mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
					cw.sectionView(c)
				})
			})
		})
	}
	cw.commandPalette(c)
	cw.confirmDialog(c)
	cw.inputDialog(c)
	cw.acpPermissionDialog(c)
	cw.flushToast(c)
	mfeedback.Toaster(c)
}

// quickView is the focused chat: only the message area and composer, with a
// slim title bar carrying the button back to the full interface.

func (cw *ChatWindow) quickView(c *mgo.Context) {
	if cw.mode != modeChat {
		cw.newConversation()
	}
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Children(func() {
		cw.quickBar(c)
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			cw.panel.View(c, cw.chatPanelOptions())
		})
	})
}

// quickBar is the quick interface's title bar: draggable, with the button
// that returns to the full interface at its right.

func (cw *ChatWindow) quickBar(c *mgo.Context) {
	bar := c.TitleBar()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Height(navItemSize).Shrink(0).
		Padding(0, bar.Right+u*2, 0, bar.Left+u*2).DragWindow().
		AlignItems(mgo.Center).Gap(u).Background(k.Background).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		mgo.Image(c, assets.BrandBitmap()).Size(16, 16)
		mgo.Text(c, cw.windowTitle()).FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if mcore.IconAction(c, icons.Must("maximize"), "切换到常规界面").Clicked() {
			cw.toggleQuickMode()
		}
	})
}

// toggleQuickMode switches between the focused and the full interface,
// narrowing the window for the focused one and restoring its width after.

func (cw *ChatWindow) toggleQuickMode() {
	cw.quickMode = !cw.quickMode
	if cw.quickMode {
		cw.section = sectionChat
		cw.shrinkForQuick()
	} else {
		cw.restoreFromQuick()
	}
	cw.invalidate()
}

// openQuick shows the window in the focused quick interface.
func (cw *ChatWindow) openQuick() {
	if !cw.quickMode {
		cw.toggleQuickMode()
	}
	cw.showMainWindow()
}

// openNormal shows the window in the full interface.
func (cw *ChatWindow) openNormal() {
	if cw.quickMode {
		cw.toggleQuickMode()
	}
	cw.showMainWindow()
}

// quickMinWidth is the least width the focused interface allows.

const quickMinWidth = 360

// shrinkForQuick remembers the window's size and narrows it for the focused
// interface.

func (cw *ChatWindow) shrinkForQuick() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil {
		return
	}
	b := w.Bounds()
	cw.quickSaved = &b
	target := min(b.Width, 720)
	w.SetMinimumSize(quickMinWidth, 0)
	w.SetSize(target, b.Height)
}

// restoreFromQuick returns the window to the size it had before the focused
// interface.

func (cw *ChatWindow) restoreFromQuick() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil || cw.quickSaved == nil {
		return
	}
	w.SetMinimumSize(720, 520)
	w.SetBounds(*cw.quickSaved)
	cw.quickSaved = nil
}

// toast queues a toast for the next frame, from any goroutine.

func (cw *ChatWindow) titleBar(c *mgo.Context) {
	bar := c.TitleBar()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	h := navItemSize
	mgo.Row(c).FillWidth().Height(h).Shrink(0).
		Padding(0, bar.Right+u*2, 0, bar.Left+u*2).DragWindow().
		AlignItems(mgo.Center).Gap(u).Background(k.Background).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		mgo.Image(c, assets.BrandBitmap()).Size(16, 16)
		mgo.Text(c, cw.windowTitle()).FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if mcore.IconAction(c, icons.Must("command"), "命令面板").Clicked() {
			cw.paletteOpen = true
		}
		if cw.section == sectionChat {
			if mcore.IconAction(c, icons.Must("panel-left"), "会话列表").Clicked() {
				cw.leftHidden = !cw.leftHidden
			}
			if mcore.IconAction(c, icons.Must("terminal"), "终端").Clicked() {
				cw.toggleTerminal()
			}
			if mcore.IconAction(c, assets.PanelRight(), "文件树").Clicked() {
				cw.rightPanel = !cw.rightPanel
			}
			if mcore.IconAction(c, icons.Must("globe"), "网页预览").Clicked() {
				cw.toggleWebPanel()
			}
		}
	})
}

func (cw *ChatWindow) windowTitle() string {
	if cw.section == sectionChat && cw.mode == modeChat && cw.cur != nil && cw.cur.Title != "" {
		return "ChatGo · " + cw.cur.Title
	}
	return "ChatGo"
}

func (cw *ChatWindow) invalidate() {
	if cw.window != nil {
		cw.window.Invalidate()
	}
}

func (cw *ChatWindow) newID() string {
	cw.idMu.Lock()
	cw.idSeq++
	n := cw.idSeq
	cw.idMu.Unlock()
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), n)
}

// reloadConvs lists saved conversations, newest activity first, split into
// normal and task conversations.

func (cw *ChatWindow) reloadConvs() {
	all, err := cw.convManager.ListConversations()
	if err != nil {
		return
	}
	sort.SliceStable(all, func(i, j int) bool {
		return convLast(all[i]).After(convLast(all[j]))
	})
	var normal, tasks []models.Conversation
	for _, conv := range all {
		if conv.TaskID == "" {
			normal = append(normal, conv)
		} else {
			tasks = append(tasks, conv)
		}
	}
	cw.convs = normal
	cw.taskConvs = tasks
	cw.updateTrayMenu()
}

func convLast(conv models.Conversation) time.Time {
	if len(conv.Messages) == 0 {
		return conv.CreatedAt
	}
	return conv.Messages[len(conv.Messages)-1].Timestamp
}

func (cw *ChatWindow) allConversations() []models.Conversation {
	all := make([]models.Conversation, 0, len(cw.convs)+len(cw.taskConvs))
	all = append(all, cw.convs...)
	all = append(all, cw.taskConvs...)
	return all
}

const defaultSystemPrompt = "You are a helpful AI assistant with access to various tools. Use tools when appropriate. When you use a tool, provide accurate parameters."

// buildEinoTools builds the Eino tools for the given builtin/MCP ids, running
// file tools under root. The result is memoized per selection so switching
// conversations does not re-list MCP tools on the UI thread.

// nativeWindow returns the window the UI draws into, for embedded WebViews.
func (cw *ChatWindow) nativeWindow() *mygo.Window {
	cw.winMu.Lock()
	defer cw.winMu.Unlock()
	return cw.window
}

func (cw *ChatWindow) postMain(fn func()) {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w != nil {
		w.Update(fn)
	}
}

// builtinToolWrapper adapts a builtin tool definition to Eino's tool interface.

const titleSystemPrompt = "你是会话标题生成助手。根据用户与助手的对话，输出一个不超过 16 个字的中文简短标题，只输出标题本身，不要引号、标点或解释。"

// generateTitle asks the model for a short title after the first exchange.

func (cw *ChatWindow) switchProvider(name string) {
	cw.provider = name
	cw.cfg.CurrentProvider = name
	_ = config.SaveConfig(cw.cfg)
	cw.setupProvider(name)
}
