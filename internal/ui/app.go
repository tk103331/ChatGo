// Package ui renders ChatGo with MyGo's native UI toolkit and the MujicaUI
// component library. The window's state lives in ChatWindow; its view method
// builds the interface from that state on every frame.
package ui

import (
	"context"
	"fmt"
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
	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "github.com/ZacharyZhang-NY/MujicaUI/feedback"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

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

	mode pageMode

	convs []models.Conversation
	cur   *models.Conversation

	homeList mgo.ListState
	convList mgo.ListState
	msgList  mchat.MessageListState

	draft     string
	provider  string
	agentMode bool

	searchQuery string
	editingIdx  int
	editDraft   string

	llmClient   *llm.Client
	reactClient *llm.ReactClient

	streaming    bool
	streamCancel context.CancelFunc

	toolOn map[string]bool

	status        string
	toastText     string
	toastSeverity mcore.Severity

	themeMode mcore.Mode
	density   mtheme.Density
	locale    mcore.Locale

	settings settingsState
	confirm  confirmState

	// clientDirty asks the next frame to rebuild the model client.
	clientDirty atomic.Bool

	idMu  sync.Mutex
	idSeq uint64
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
		themeMode:   parseThemeMode(cfg.ThemeMode),
		density:     parseDensity(cfg.Density),
		locale:      parseLocale(cfg.Locale),
		editingIdx:  -1,
	}
	cw.settings.prov.editing = -1
	cw.settings.mcp.editing = -1
	cw.settings.builtin.editing = -1
	cw.reloadConvs()
	cw.setupProvider(cw.provider)
	go cw.startMCP()
	return cw, nil
}

// Show creates the MyGo window; call it from mygo.App.WhenReady.
func (cw *ChatWindow) Show() {
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:     "ChatGo",
		Width:     1000,
		Height:    700,
		MinWidth:  720,
		MinHeight: 520,
		Content:   mgo.View(cw.view),
	})
	cw.winMu.Lock()
	cw.window = w
	cw.winMu.Unlock()
}

// view builds the whole interface for the frame.
func (cw *ChatWindow) view(c *mgo.Context) {
	mcore.Use(c, mcore.Settings{Mode: cw.themeMode, Density: cw.density, Locale: cw.locale})
	if cw.clientDirty.Swap(false) {
		cw.setupProvider(cw.provider)
	}
	if cw.mode == modeHome {
		cw.homeView(c)
	} else {
		cw.chatView(c)
	}
	cw.settingsDialog(c)
	cw.confirmDialog(c)
	cw.flushToast(c)
	mfeedback.Toaster(c)
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

// reloadConvs lists saved conversations, newest activity first.
func (cw *ChatWindow) reloadConvs() {
	convs, err := cw.convManager.ListConversations()
	if err != nil {
		return
	}
	sort.SliceStable(convs, func(i, j int) bool {
		return convLast(convs[i]).After(convLast(convs[j]))
	})
	cw.convs = convs
}

func convLast(conv models.Conversation) time.Time {
	if len(conv.Messages) == 0 {
		return conv.UpdatedAt
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
	if cw.agentMode {
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
func (cw *ChatWindow) setupReactAgent(provider config.Provider) error {
	ctx := context.Background()
	selected := cw.selectedToolIDs()

	einoTools := make([]tool.BaseTool, 0)
	mcpByServer := make(map[string][]string)

	for _, toolID := range selected {
		switch {
		case strings.HasPrefix(toolID, "builtin:"):
			name := strings.TrimPrefix(toolID, "builtin:")
			def, err := cw.createBuiltinToolDefinition(name)
			if err != nil {
				continue
			}
			einoTools = append(einoTools, newBuiltinToolWrapper(def))
		case strings.HasPrefix(toolID, "mcp:"):
			parts := strings.Split(toolID, ":")
			if len(parts) >= 3 {
				mcpByServer[parts[1]] = append(mcpByServer[parts[1]], parts[2])
			}
		}
	}

	for serverName, names := range mcpByServer {
		status, ok := cw.mcpManager.GetServerStatus(serverName)
		if !ok || status.Status != "initialized" {
			continue
		}
		mcpTools, err := einomcp.GetTools(ctx, &einomcp.Config{Cli: status.Client, ToolNameList: names})
		if err != nil {
			continue
		}
		einoTools = append(einoTools, mcpTools...)
	}

	agentConfig := &llm.ReactAgentConfig{
		MaxStep:      cw.cfg.ReactAgentMaxStep,
		SystemPrompt: "You are a helpful AI assistant with access to various tools. Use tools when appropriate. When you use a tool, provide accurate parameters.",
	}
	reactClient, err := llm.NewReactClientWithEinoTools(provider, einoTools, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to create React client: %w", err)
	}
	cw.reactClient = reactClient
	cw.llmClient = nil
	return nil
}

// createBuiltinToolDefinition builds an Eino tool definition for a builtin tool.
func (cw *ChatWindow) createBuiltinToolDefinition(toolName string) (llm.ToolDefinition, error) {
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
	def := llm.ToolDefinition{
		Name:        builtin.Name,
		Description: config.GetBuiltinToolDescription(builtin.Type),
		Parameters:  make(map[string]*schema.ParameterInfo),
	}
	required := config.GetRequiredConfigFields(builtin.Type)
	for _, field := range config.GetBuiltinToolConfigFields(builtin.Type) {
		req := false
		for _, r := range required {
			if r == field {
				req = true
				break
			}
		}
		def.Parameters[field] = &schema.ParameterInfo{Type: schema.String, Desc: field + " parameter", Required: req}
	}
	name := toolName
	def.Handler = func(ctx context.Context, arguments string) (string, error) {
		return fmt.Sprintf("Tool %s executed with args: %s\n\n(Actual execution not implemented)", name, arguments), nil
	}
	return def, nil
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

// newConversation creates an empty conversation with the current provider.
func (cw *ChatWindow) newConversation() {
	name := cw.provider
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		}
	}
	model := ""
	if p, ok := cw.findProvider(name); ok {
		model = p.Model
	}
	conv, err := cw.convManager.CreateConversation(fmt.Sprintf("Chat-%s", time.Now().Format("20060102150405")), name, model)
	if err != nil {
		cw.status = "创建会话失败: " + err.Error()
		return
	}
	cw.cur = conv
	cw.provider = name
	cw.mode = modeChat
	cw.msgList.ScrollToEnd()
	cw.setupProvider(name)
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
	if conv.Provider != "" {
		cw.provider = conv.Provider
		cw.setupProvider(conv.Provider)
	}
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
	msgs := buildMessages(conv, idx)
	cw.draft = ""
	_ = cw.convManager.SaveConversation(conv)
	cw.msgList.ScrollToEnd()
	cw.streaming = true
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, idx, msgs)
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
	})
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
	m.ToolCalls = append(m.ToolCalls, tc)
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
		if p, ok := cw.findProvider(name); ok {
			cw.cur.Model = p.Model
		}
		_ = cw.convManager.SaveConversation(cw.cur)
	}
	_ = config.SaveConfig(cw.cfg)
	cw.setupProvider(name)
}

// setAgentMode toggles between plain chat and the tool-enabled React agent.
func (cw *ChatWindow) setAgentMode(on bool) {
	cw.agentMode = on
	cw.cfg.UseReactAgent = on
	_ = config.SaveConfig(cw.cfg)
	cw.setupProvider(cw.provider)
}
