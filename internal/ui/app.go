// Package ui renders ChatGo with MyGo's native UI toolkit and the MujicaUI
// component library. The window's state lives in ChatWindow; its view method
// builds the interface from that state on every frame.
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

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/mcp"
	"chatgo/pkg/models"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	acp "github.com/coder/acp-go-sdk"
	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
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
	appsLoaded bool
	appTabs    map[string][]string
	appTab     map[string]string
	appConv    map[string]string
	currentApp string

	appTermTabs  []*appTermTab
	appFiles     map[string]*appFile
	gitCommitMsg string
	preAppConvID string

	rightPanel bool
	rightTab   int
	leftHidden bool
	leftW      float32

	termOpen    bool
	terms       []*termTab
	termActive  int
	termH       float32
	treeW       float32
	previewW    float32
	chatW       float32
	lastWinW    float32
	lastRightW  float32
	lastPreview bool

	previewTabs []previewTab
	previewSel  int

	gitRepo    bool
	gitBranch  string
	gitChanges []string
	gitLog     []string
	gitLoaded  bool
	gitFor     string

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
	msgList  mchat.MessageListState

	draft        string
	mentionRoot  string
	mentionFiles []mchat.ContextItem
	provider     string
	model        string
	agentMode    bool

	searchQuery string
	editingIdx  int
	editDraft   string

	llmClient   *llm.Client
	reactClient *llm.ReactClient

	streaming    bool
	streamCancel context.CancelFunc

	toolOn  map[string]bool
	skillOn map[string]bool

	status        string
	toastText     string
	toastSeverity mcore.Severity

	themeMode mcore.Mode
	density   mtheme.Density
	locale    mcore.Locale

	settings settingsState
	confirm  confirmState

	agentForm       agentForm
	agentEditorOpen bool
	agentSecModels  bool
	agentSecTools   bool
	agentSecMCP     bool
	agentSecSkills  bool

	autoTaskEditorOpen bool
	autoForm           autoForm
	autoRunning        map[string]bool

	avatarMu  sync.Mutex
	avatarPNG map[string]*mgo.Bitmap

	acpList mgo.ListState
	acpForm acpForm

	mcpTabSel         int
	mcpEditorOpen     bool
	mcpMarket         []mcpMarketServer
	mcpMarketList     mgo.ListState
	mcpMarketLoading  bool
	mcpMarketLoaded   bool
	mcpMarketErr      string
	mcpMarketQuery    string
	mcpMarketCategory string
	mcpDetailOpen     bool
	mcpDetail         mcpMarketServer
	mcpToolsOpen      bool
	mcpToolsServer    string
	mcpTools          []mcp.MCPTool

	skills          []skill
	skillsLoaded    bool
	skillsSel       string
	skillsQuery     string
	skillsListState mgo.ListState
	toolQuery       string
	mcpQuery        string
	skillMenuQuery  string

	skillFiles      []string
	skillFilesFor   string
	skillFileSel    string
	skillFileBody   string
	skillFileBodyOf string

	acpMu         sync.Mutex
	acpConvs      map[string]*acpConv
	acpConvBySess map[acp.SessionId]string
	acpConvObj    map[string]*models.Conversation
	acpStarting   map[string]bool
	channelSess   map[acp.SessionId]bool
	channelReply  map[acp.SessionId]*strings.Builder
	toolStart     map[string]time.Time

	permissionReq  *acp.RequestPermissionRequest
	permissionCh   chan acp.RequestPermissionResponse
	permissionShow bool

	// clientDirty asks the next frame to rebuild the model client.
	clientDirty atomic.Bool

	// toolCache memoizes built tools per selection so a chat switch does not
	// re-list every MCP server, which blocks the UI thread.
	toolCache    []tool.BaseTool
	toolCacheKey string

	quickShortcut string

	channelMu      sync.Mutex
	channelRunning bool
	channelCancel  context.CancelFunc
	channelState   feishuConnState
	channelNote    string

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
		cfg:            cfg,
		convManager:    cm,
		mcpManager:     NewMCPManagerWrapper(),
		provider:       cfg.CurrentProvider,
		agentMode:      cfg.UseReactAgent,
		toolOn:         map[string]bool{},
		skillOn:        map[string]bool{},
		acpConvs:       map[string]*acpConv{},
		acpConvBySess:  map[acp.SessionId]string{},
		acpConvObj:     map[string]*models.Conversation{},
		acpStarting:    map[string]bool{},
		channelSess:    map[acp.SessionId]bool{},
		channelReply:   map[acp.SessionId]*strings.Builder{},
		toolStart:      map[string]time.Time{},
		rightPanel:     false,
		wsCollapsed:    map[string]bool{},
		themeMode:      parseThemeMode(cfg.ThemeMode),
		density:        mtheme.Compact,
		locale:         parseLocale(cfg.Locale),
		editingIdx:     -1,
		agentSecModels: true,
		agentSecTools:  true,
		agentSecMCP:    true,
		agentSecSkills: true,
		avatarPNG:      map[string]*mgo.Bitmap{},
		autoRunning:    map[string]bool{},
	}
	if wd, err := os.Getwd(); err == nil {
		cw.workspace = wd
	}
	cw.settings.prov.editing = -1
	cw.settings.mcp.editing = -1
	cw.settings.builtin.editing = -1
	cw.settings.channel.editing = -1
	cw.settings.channel.typ = "feishu"
	cw.settings.shortcut.value = cw.quickChatShortcut()
	cw.agentForm.editing = -1
	cw.acpForm.editing = -1
	cw.reloadConvs()
	cw.currentAgent = ""
	cw.applyAgent("")
	go cw.startMCP()
	go cw.autoTaskLoop()
	go cw.startFeishuChannel()
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
	_ = w.SetIcon(logoPNG)
	_ = mygo.App.Dock.SetIcon(logoPNG)
	cw.applyQuickShortcut()
}

// applyQuickShortcut registers the configured accelerator, replacing any
// previous one, and reports a failure through the status line.
func (cw *ChatWindow) applyQuickShortcut() {
	acc := cw.quickChatShortcut()
	if acc == cw.quickShortcut {
		return
	}
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if err := mygo.GlobalShortcut.Register(acc, func() {
		cw.toggleQuickMode()
		if w != nil {
			w.Focus()
		}
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
	mcore.Use(c, mcore.Settings{Mode: cw.themeMode, Density: cw.density, Locale: cw.locale})
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
	cw.confirmDialog(c)
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
			cw.messageArea(c)
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
		mgo.Image(c, brandBitmap()).Size(16, 16)
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
func (cw *ChatWindow) toast(msg string, sev mcore.Severity) {
	cw.toastText = msg
	cw.toastSeverity = sev
	cw.invalidate()
}

func (cw *ChatWindow) flushToast(c *mgo.Context) {
	if cw.toastText == "" {
		return
	}
	mfeedback.ShowToast(c, mfeedback.ToastMessage{Message: cw.toastText, Severity: cw.toastSeverity})
	cw.toastText = ""
}

// titleBar draws the self-drawn title bar under the window controls.
func (cw *ChatWindow) titleBar(c *mgo.Context) {
	bar := c.TitleBar()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	h := navItemSize
	mgo.Row(c).FillWidth().Height(h).Shrink(0).
		Padding(0, bar.Right+u*2, 0, bar.Left+u*2).DragWindow().
		AlignItems(mgo.Center).Gap(u).Background(k.Background).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		mgo.Image(c, brandBitmap()).Size(16, 16)
		mgo.Text(c, cw.windowTitle()).FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		if cw.section == sectionChat {
			if mcore.IconAction(c, icons.Must("panel-left"), "会话列表").Clicked() {
				cw.leftHidden = !cw.leftHidden
			}
			if mcore.IconAction(c, icons.Must("terminal"), "终端").Clicked() {
				cw.toggleTerminal()
			}
			if mcore.IconAction(c, panelRight(), "文件树").Clicked() {
				cw.rightPanel = !cw.rightPanel
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
}

func convLast(conv models.Conversation) time.Time {
	if len(conv.Messages) == 0 {
		return conv.CreatedAt
	}
	return conv.Messages[len(conv.Messages)-1].Timestamp
}

func (cw *ChatWindow) findProvider(name string) (config.Provider, bool) {
	for _, p := range cw.cfg.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return config.Provider{}, false
}

func (cw *ChatWindow) firstEnabled() (config.Provider, bool) {
	for _, p := range cw.cfg.Providers {
		if p.Enabled {
			return p, true
		}
	}
	return config.Provider{}, false
}

// setupProvider builds the client for provider name, or the first enabled one.
// With agent mode on it builds a React agent carrying the selected tools.
func (cw *ChatWindow) setupProvider(name string) {
	p, ok := cw.findProvider(name)
	if !ok {
		p, ok = cw.firstEnabled()
	}
	if !ok {
		cw.llmClient, cw.reactClient = nil, nil
		cw.status = "尚未配置模型，请在设置中添加 Provider"
		return
	}
	cw.provider = p.Name
	cw.cfg.CurrentProvider = p.Name
	if cw.model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			cw.model = ms[0]
		}
	}
	p.Model = cw.model
	if p.Model == "" {
		cw.llmClient, cw.reactClient = nil, nil
		cw.status = "Provider「" + p.Name + "」尚未配置模型"
		return
	}
	if cw.agentMode || len(cw.selectedToolIDs()) > 0 {
		if err := cw.setupReactAgent(p); err == nil {
			cw.status = ""
			return
		}
	}
	client, err := llm.NewClient(p)
	if err != nil {
		cw.llmClient, cw.reactClient = nil, nil
		cw.status = "模型初始化失败: " + err.Error()
		return
	}
	cw.llmClient, cw.reactClient = client, nil
	cw.status = ""
}

// setupReactAgent builds the tool-enabled React agent from cfg and tool state.
// An App conversation carries only that App's tools; every other conversation
// carries the selected builtin and MCP tools.
func (cw *ChatWindow) setupReactAgent(provider config.Provider) error {
	var einoTools []tool.BaseTool
	if cw.currentApp != "" {
		einoTools = cw.buildAppTools(cw.currentApp)
	} else {
		einoTools = cw.buildEinoTools(cw.selectedToolIDs(), cw.fileRoot())
	}
	systemPrompt := cw.systemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	agentConfig := &llm.ReactAgentConfig{
		MaxStep:      cw.cfg.ReactAgentMaxStep,
		SystemPrompt: systemPrompt,
	}
	reactClient, err := llm.NewReactClientWithEinoTools(provider, einoTools, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to create React client: %w", err)
	}
	cw.reactClient = reactClient
	cw.llmClient = nil
	return nil
}

// defaultSystemPrompt is used when an agent has no system prompt.
const defaultSystemPrompt = "You are a helpful AI assistant with access to various tools. Use tools when appropriate. When you use a tool, provide accurate parameters."

// buildEinoTools builds the Eino tools for the given builtin/MCP ids, running
// file tools under root. The result is memoized per selection so switching
// conversations does not re-list MCP tools on the UI thread.
func (cw *ChatWindow) buildEinoTools(toolIDs []string, root string) []tool.BaseTool {
	key := cw.toolCacheSignature(toolIDs, root)
	if key == cw.toolCacheKey && cw.toolCache != nil {
		return cw.toolCache
	}
	ctx := context.Background()
	einoTools := make([]tool.BaseTool, 0)
	mcpServers := make(map[string]bool)
	for _, toolID := range toolIDs {
		switch {
		case strings.HasPrefix(toolID, "builtin:"):
			name := strings.TrimPrefix(toolID, "builtin:")
			if bt, ok := cw.findBuiltinTool(name); ok && isEinoBuiltinTool(bt.Type) {
				if t, delegated, err := einoBuiltinTool(bt.Type, bt.Config); delegated {
					if err == nil && t != nil {
						einoTools = append(einoTools, llm.ObserveTool(t))
					}
					continue
				}
			}
			def, err := cw.createBuiltinToolDefinition(name, root)
			if err != nil {
				continue
			}
			einoTools = append(einoTools, llm.ObserveTool(newBuiltinToolWrapper(def)))
		case strings.HasPrefix(toolID, "mcp:"):
			mcpServers[strings.TrimPrefix(toolID, "mcp:")] = true
		}
	}
	for serverName := range mcpServers {
		status, ok := cw.mcpManager.GetServerStatus(serverName)
		if !ok || status.Status != "initialized" {
			continue
		}
		names := make([]string, 0, len(status.Tools))
		for _, tl := range status.Tools {
			names = append(names, tl.Name)
		}
		// Listing tools talks to the server: bound it so a hung server cannot
		// block the caller, which is the UI thread when switching chats.
		callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		mcpTools, err := einomcp.GetTools(callCtx, &einomcp.Config{Cli: status.Client, ToolNameList: names})
		cancel()
		if err != nil {
			continue
		}
		for _, t := range mcpTools {
			einoTools = append(einoTools, llm.ObserveTool(t))
		}
	}
	cw.toolCache, cw.toolCacheKey = einoTools, key
	return einoTools
}

// toolCacheSignature keys the built tools by the selection, the file root and
// each selected MCP server's tool set, so cache hits are safe.
func (cw *ChatWindow) toolCacheSignature(toolIDs []string, root string) string {
	ids := append([]string(nil), toolIDs...)
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString(root)
	for _, id := range ids {
		b.WriteByte('\n')
		b.WriteString(id)
		switch {
		case strings.HasPrefix(id, "mcp:"):
			name := strings.TrimPrefix(id, "mcp:")
			if status, ok := cw.mcpManager.GetServerStatus(name); ok {
				names := make([]string, 0, len(status.Tools))
				for _, tl := range status.Tools {
					names = append(names, tl.Name)
				}
				sort.Strings(names)
				fmt.Fprintf(&b, ":%p:%s", status.Client, strings.Join(names, ","))
			}
		default:
			if name, ok := strings.CutPrefix(id, "builtin:"); ok {
				if bt, ok := cw.findBuiltinTool(name); ok {
					fmt.Fprintf(&b, ":%s:%s", bt.Type, builtinConfigSig(bt.Config))
				}
			}
		}
	}
	return b.String()
}

// builtinConfigSig is a stable digest of a builtin tool's config values.
func builtinConfigSig(cfg map[string]string) string {
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s;", k, cfg[k])
	}
	return b.String()
}

// findBuiltinTool returns the enabled builtin tool with the given name.
func (cw *ChatWindow) findBuiltinTool(name string) (config.BuiltinTool, bool) {
	for i := range cw.cfg.BuiltinTools {
		if cw.cfg.BuiltinTools[i].Name == name {
			return cw.cfg.BuiltinTools[i], true
		}
	}
	return config.BuiltinTool{}, false
}

// createBuiltinToolDefinition builds an Eino tool definition for a builtin tool.
func (cw *ChatWindow) createBuiltinToolDefinition(toolName, root string) (llm.ToolDefinition, error) {
	var builtin *config.BuiltinTool
	for i := range cw.cfg.BuiltinTools {
		if cw.cfg.BuiltinTools[i].Name == toolName && cw.cfg.BuiltinTools[i].Enabled {
			builtin = &cw.cfg.BuiltinTools[i]
			break
		}
	}
	if builtin == nil {
		return llm.ToolDefinition{}, fmt.Errorf("builtin tool %s not found or disabled", toolName)
	}
	return llm.ToolDefinition{
		Name:        builtin.Name,
		Description: config.GetBuiltinToolDescription(builtin.Type),
		Parameters:  builtinToolParams(builtin.Type),
		Handler:     cw.builtinToolHandler(builtin, root),
	}, nil
}

// postMain runs fn on the main thread and redraws, if the window exists yet.
func (cw *ChatWindow) postMain(fn func()) {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w != nil {
		w.Update(fn)
	}
}

// builtinToolWrapper adapts a builtin tool definition to Eino's tool interface.
type builtinToolWrapper struct {
	info    *schema.ToolInfo
	handler func(ctx context.Context, arguments string) (string, error)
}

func newBuiltinToolWrapper(def llm.ToolDefinition) tool.BaseTool {
	return &builtinToolWrapper{
		info: &schema.ToolInfo{
			Name:        def.Name,
			Desc:        def.Description,
			ParamsOneOf: schema.NewParamsOneOfByParams(def.Parameters),
		},
		handler: def.Handler,
	}
}

func (w *builtinToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) { return w.info, nil }

func (w *builtinToolWrapper) InvokableRun(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
	return w.handler(ctx, arguments)
}

func (w *builtinToolWrapper) StreamableRun(ctx context.Context, arguments string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported for this tool")
}

// MCPManagerWrapper wraps the MCP manager for the UI.
type MCPManagerWrapper struct {
	manager *mcp.Manager
}

func NewMCPManagerWrapper() *MCPManagerWrapper {
	return &MCPManagerWrapper{manager: mcp.NewManager()}
}

func (m *MCPManagerWrapper) GetServerStatus(name string) (*mcp.MCPServerStatus, bool) {
	return m.manager.GetServerStatus(name)
}

func (m *MCPManagerWrapper) DisconnectServer(name string) error {
	return m.manager.DisconnectServer(name)
}

// startMCP initializes configured MCP servers, then rebuilds the model client.
func (cw *ChatWindow) startMCP() {
	cw.initMCPServers()
	for {
		cw.winMu.Lock()
		w := cw.window
		cw.winMu.Unlock()
		if w != nil {
			w.Update(func() { cw.clientDirty.Store(true) })
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// initMCPServers initializes every enabled MCP server in parallel.
func (cw *ChatWindow) initMCPServers() {
	var wg sync.WaitGroup
	for _, server := range cw.cfg.MCPServers {
		if !server.Enabled {
			continue
		}
		wg.Add(1)
		go func(srv config.MCPServer) {
			defer wg.Done()
			_, _ = cw.mcpManager.manager.InitializeServer(srv)
		}(server)
	}
	wg.Wait()
}

// workspaces lists the configured workspaces, the current one first.
func (cw *ChatWindow) workspaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(w string) {
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	add(cw.workspace)
	for _, w := range cw.cfg.Workspaces {
		add(w)
	}
	for _, conv := range cw.convs {
		add(conv.Workspace)
	}
	return out
}

func (cw *ChatWindow) addWorkspace(dir string) {
	if dir == "" || containsStr(cw.cfg.Workspaces, dir) {
		cw.workspace = dir
		return
	}
	cw.cfg.Workspaces = append(cw.cfg.Workspaces, dir)
	_ = config.SaveConfig(cw.cfg)
	cw.workspace = dir
}

func (cw *ChatWindow) removeWorkspace(dir string) {
	for i, w := range cw.cfg.Workspaces {
		if w == dir {
			cw.cfg.Workspaces = append(cw.cfg.Workspaces[:i], cw.cfg.Workspaces[i+1:]...)
			_ = config.SaveConfig(cw.cfg)
			return
		}
	}
}

// newConversation creates a conversation in the current workspace.
func (cw *ChatWindow) newConversation() {
	cw.newConversationIn(cw.workspace, "")
}

// newConversationIn creates a conversation in ws, run by the chosen agent.
func (cw *ChatWindow) newConversationIn(ws, agent string) {
	if aw := cw.agentWorkspace(agent); aw != "" {
		ws = aw
	}
	name := cw.provider
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		}
	}
	model := cw.model
	if p, ok := cw.findProvider(name); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(defaultTitle, name, model)
	if err != nil {
		cw.status = "创建会话失败: " + err.Error()
		return
	}
	conv.Workspace = ws
	conv.Agent = agent
	_ = cw.convManager.SaveConversation(conv)
	cw.cur = conv
	cw.provider = name
	if ws != "" {
		cw.workspace = ws
	}
	cw.mode = modeChat
	cw.msgList.ScrollToEnd()
	cw.applyConversationAgent(conv)
	cw.reloadConvs()
}

// selectConversation loads a conversation and shows it in chat mode.
func (cw *ChatWindow) selectConversation(id string) {
	conv, err := cw.convManager.LoadConversation(id)
	if err != nil {
		return
	}
	cw.cur = conv
	cw.mode = modeChat
	if conv.Workspace != "" {
		cw.workspace = conv.Workspace
	}
	cw.applyConversationAgent(conv)
	cw.msgList.ScrollToEnd()
}

// sendDraft appends the draft as a user turn and streams the assistant reply.
func (cw *ChatWindow) sendDraft() {
	text := strings.TrimSpace(cw.draft)
	if text == "" {
		return
	}
	if cw.cur == nil {
		cw.newConversation()
	}
	if cw.cur == nil {
		return
	}
	conv := cw.cur
	user := models.Message{ID: cw.newID(), Role: "user", Content: text, Timestamp: time.Now()}
	conv.Messages = append(conv.Messages, user)
	conv.UpdatedAt = time.Now()
	idx := len(conv.Messages)
	conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Timestamp: time.Now()})
	cw.draft = ""
	if strings.HasPrefix(conv.Agent, "acp:") && conv.Title == defaultTitle {
		conv.Title = titleFromText(text)
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.msgList.ScrollToEnd()
	cw.streaming = true
	if strings.HasPrefix(conv.Agent, "acp:") {
		cw.reloadConvs()
		cw.acpChatStart(conv, strings.TrimPrefix(conv.Agent, "acp:"), idx, text)
		return
	}
	msgs := cw.chatMessages(conv, idx)
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, idx, msgs)
}

// applyConversationAgent restores the client state for a conversation's agent.
func (cw *ChatWindow) applyConversationAgent(conv *models.Conversation) {
	cw.currentAgent = ""
	cw.currentApp = ""
	cw.systemPrompt = ""
	switch {
	case strings.HasPrefix(conv.Agent, "agent:"):
		cw.currentAgent = strings.TrimPrefix(conv.Agent, "agent:")
		cw.applyAgent(cw.currentAgent)
	case strings.HasPrefix(conv.Agent, "app:"):
		cw.applyApp(strings.TrimPrefix(conv.Agent, "app:"))
	case strings.HasPrefix(conv.Agent, "acp:"):
		cw.llmClient, cw.reactClient = nil, nil
		cw.status = ""
		cw.acpWarmup(conv)
	default:
		cw.applyAgent("")
	}
}

// startAppSession loads or creates the app's own conversation and makes it
// current, so the right pane chats against the app.
func (cw *ChatWindow) startAppSession(id string) {
	cw.ensureAppState()
	if convID := cw.appConv[id]; convID != "" {
		if conv, err := cw.convManager.LoadConversation(convID); err == nil {
			cw.cur = conv
			cw.mode = modeChat
			cw.applyApp(id)
			cw.msgList.ScrollToEnd()
			return
		}
	}
	cw.newAppConversation(id)
}

// newAppConversation creates a fresh conversation bound to an app.
func (cw *ChatWindow) newAppConversation(id string) {
	a, ok := appByID(id)
	if !ok {
		return
	}
	name := cw.provider
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		}
	}
	model := cw.model
	if p, ok := cw.findProvider(name); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(a.Title, name, model)
	if err != nil {
		cw.status = "创建应用会话失败: " + err.Error()
		return
	}
	conv.Agent = "app:" + id
	if cw.workspace != "" {
		conv.Workspace = cw.workspace
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.appConv[id] = conv.ID
	cw.cur = conv
	cw.mode = modeChat
	cw.applyApp(id)
	cw.msgList.ScrollToEnd()
}

// applyApp switches the client to an app's session: its system prompt and its
// own tool set, with no builtin or MCP tools.
func (cw *ChatWindow) applyApp(id string) {
	if _, ok := appByID(id); !ok {
		cw.applyAgent("")
		return
	}
	cw.currentAgent = ""
	cw.currentApp = id
	cw.agentMode = true
	cw.systemPrompt = cw.appSystemPrompt(id)
	cw.setupProvider(cw.provider)
}

// runStream drives one assistant reply, updating state on the main thread.
func (cw *ChatWindow) runStream(ctx context.Context, conv *models.Conversation, idx int, msgs []llm.ChatMessage) {
	var (
		resp *llm.ChatResponse
		err  error
	)
	onChunk := func(chunk string) {
		cw.postMain(func() { conv.Messages[idx].Content += chunk })
	}
	onTool := func(info llm.ToolCallInfo) {
		cw.postMain(func() { applyToolCall(&conv.Messages[idx], info) })
	}
	switch {
	case cw.reactClient != nil:
		resp, err = cw.reactClient.Chat(ctx, msgs, onChunk, onTool)
	case cw.llmClient != nil:
		resp, err = cw.llmClient.Chat(ctx, msgs, onChunk)
	default:
		err = fmt.Errorf("尚未配置模型")
	}
	cw.postMain(func() {
		switch {
		case err != nil && conv.Messages[idx].Content == "":
			conv.Messages[idx].Content = "⚠️ " + err.Error()
		case resp != nil && conv.Messages[idx].Content == "":
			conv.Messages[idx].Content = resp.Content
		}
		cw.streaming = false
		cw.streamCancel = nil
		conv.UpdatedAt = time.Now()
		_ = cw.convManager.SaveConversation(conv)
		cw.reloadConvs()
		cw.generateTitle(conv)
	})
}

const titleSystemPrompt = "你是会话标题生成助手。根据用户与助手的对话，输出一个不超过 16 个字的中文简短标题，只输出标题本身，不要引号、标点或解释。"

// generateTitle asks the model for a short title after the first exchange.
func (cw *ChatWindow) generateTitle(conv *models.Conversation) {
	if conv.Title != defaultTitle || len(conv.Messages) < 2 {
		return
	}
	user, assistant := firstExchange(conv)
	if user == "" || assistant == "" || strings.HasPrefix(assistant, "⚠️") {
		return
	}
	provider, ok := cw.findProvider(conv.Provider)
	if !ok {
		provider, ok = cw.firstEnabled()
	}
	if !ok {
		return
	}
	client, err := llm.NewClient(provider)
	if err != nil {
		return
	}
	go func() {
		resp, err := client.Chat(context.Background(), []llm.ChatMessage{
			{Role: "system", Content: titleSystemPrompt},
			{Role: "user", Content: "用户：" + user + "\n助手：" + assistant},
		}, nil)
		if err != nil || resp == nil {
			return
		}
		title := cleanTitle(resp.Content)
		if title == "" {
			return
		}
		cw.postMain(func() {
			if conv.Title != defaultTitle {
				return
			}
			conv.Title = title
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
		})
	}()
}

func firstExchange(conv *models.Conversation) (user, assistant string) {
	for _, m := range conv.Messages {
		switch m.Role {
		case "user":
			if user == "" {
				user = m.Content
			}
		case "assistant":
			if assistant == "" {
				assistant = m.Content
			}
		}
		if user != "" && assistant != "" {
			break
		}
	}
	return user, assistant
}

// titleFromText makes a short title from the user's first message.
func titleFromText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return s
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "「」\"'“”‘’《》#*`")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 20 {
		r = r[:20]
	}
	return strings.TrimSpace(string(r))
}

// stopStreaming cancels the reply being generated.
func (cw *ChatWindow) stopStreaming() {
	if cw.streamCancel != nil {
		cw.streamCancel()
		cw.streamCancel = nil
	}
}

// applyToolCall records a tool call start or result on an assistant message.
func applyToolCall(m *models.Message, info llm.ToolCallInfo) {
	for i := range m.ToolCalls {
		if m.ToolCalls[i].ID == info.ID {
			if info.Arguments != "" {
				m.ToolCalls[i].Arguments = info.Arguments
			}
			if info.Error != nil {
				m.ToolCalls[i].Error = info.Error.Error()
			}
			if info.Result != "" {
				m.ToolCalls[i].Result = info.Result
			}
			if info.Duration > 0 {
				m.ToolCalls[i].DurationMs = info.Duration.Milliseconds()
			}
			return
		}
	}
	tc := models.ToolCall{ID: info.ID, Name: info.Name, Arguments: info.Arguments}
	if info.Error != nil {
		tc.Error = info.Error.Error()
	}
	if info.Result != "" {
		tc.Result = info.Result
	}
	if info.Duration > 0 {
		tc.DurationMs = info.Duration.Milliseconds()
	}
	m.ToolCalls = append(m.ToolCalls, tc)
}

// chatMessages are the conversation's turns with the active system prompt.
func (cw *ChatWindow) chatMessages(conv *models.Conversation, upto int) []llm.ChatMessage {
	out := buildMessages(conv, upto)
	sys := cw.systemPrompt
	if skills := cw.skillPrompt(); skills != "" {
		if sys != "" {
			sys += "\n\n"
		}
		sys += skills
	}
	if sys != "" {
		out = append([]llm.ChatMessage{{Role: "system", Content: sys}}, out...)
	}
	return out
}

// buildMessages converts stored messages up to upto into LLM chat messages.
func buildMessages(conv *models.Conversation, upto int) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, upto)
	for i := 0; i < upto && i < len(conv.Messages); i++ {
		m := conv.Messages[i]
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// switchProvider changes the active model and reconfigures the client.
func (cw *ChatWindow) switchProvider(name string) {
	cw.provider = name
	cw.cfg.CurrentProvider = name
	if cw.cur != nil {
		cw.cur.Provider = name
		cw.cur.Model = cw.model
		if cw.cur.Model == "" {
			if p, ok := cw.findProvider(name); ok {
				if ms := p.ModelsList(); len(ms) > 0 {
					cw.cur.Model = ms[0]
				}
			}
		}
		_ = cw.convManager.SaveConversation(cw.cur)
	}
	_ = config.SaveConfig(cw.cfg)
	cw.setupProvider(name)
}
