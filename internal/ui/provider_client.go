package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/mcp"
	"chatgo/internal/tools/eino"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

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
	if g, ok := cw.cfg.FindProviderGroup(name); ok {
		cw.setupGroup(g)
		return
	}
	cw.groupClient, cw.groupReact = nil, nil
	p, ok := cw.findProvider(name)
	if !ok {
		p, ok = cw.firstEnabled()
	}
	if !ok {
		if g, ok := cw.firstGroup(); ok {
			cw.setupGroup(g)
			return
		}
		cw.llmClient, cw.reactClient = nil, nil
		cw.setSetupError("尚未配置可用模型，请在设置 → 模型中添加并启用一个模型")
		return
	}
	cw.provider = p.Name
	cw.cfg.CurrentProvider = p.Name
	// The selected model must belong to this provider: switching providers
	// keeps cw.model, and sending another provider's model name upstream is
	// rejected as an unknown model.
	if ms := p.ModelsList(); len(ms) > 0 && !containsStr(ms, cw.model) {
		cw.model = ms[0]
	}
	p.Model = cw.model
	if err := p.Validate(); err != nil {
		cw.llmClient, cw.reactClient = nil, nil
		cw.setSetupError(err.Error())
		return
	}
	if cw.agentMode || len(cw.selectedToolIDs()) > 0 {
		if err := cw.setupReactAgent(p); err == nil {
			cw.clearSetupError()
			return
		}
	}
	client, err := llm.NewClient(p)
	if err != nil {
		cw.llmClient, cw.reactClient = nil, nil
		cw.setSetupError(err.Error())
		return
	}
	cw.llmClient, cw.reactClient = client, nil
	cw.clearSetupError()
}

// setupGroup builds the routing client for a provider group.
func (cw *ChatWindow) setupGroup(g config.ProviderGroup) {
	cw.provider = g.Name
	cw.cfg.CurrentProvider = g.Name
	// A group picks the model per request through its routing strategy, so the
	// active model is cleared: holding one here would pin every request to it.
	cw.model = ""
	if len(g.ModelsList()) == 0 {
		cw.llmClient, cw.reactClient, cw.groupClient, cw.groupReact = nil, nil, nil, nil
		cw.setSetupError("模型组「" + g.Name + "」尚未勾选任何模型，请在设置 → 模型中配置")
		return
	}
	if cw.agentMode || len(cw.selectedToolIDs()) > 0 {
		einoTools, systemPrompt := cw.agentToolsAndPrompt()
		rc, err := llm.NewGroupReactClient(g, cw.cfg.Providers, einoTools, &llm.ReactAgentConfig{
			MaxStep:      cw.cfg.ReactAgentMaxStep,
			SystemPrompt: systemPrompt,
		})
		if err == nil {
			cw.llmClient, cw.reactClient, cw.groupClient = nil, nil, nil
			cw.groupReact = rc
			cw.clearSetupError()
			return
		}
	}
	c, err := llm.NewGroupClient(g, cw.cfg.Providers)
	if err != nil {
		cw.llmClient, cw.reactClient, cw.groupClient, cw.groupReact = nil, nil, nil, nil
		cw.setSetupError("模型组「" + g.Name + "」初始化失败：" + err.Error())
		return
	}
	cw.llmClient, cw.reactClient, cw.groupReact = nil, nil, nil
	cw.groupClient = c
	cw.clearSetupError()
}

// firstGroup returns the first enabled provider group, used when no provider
// is enabled but a group is.
func (cw *ChatWindow) firstGroup() (config.ProviderGroup, bool) {
	if groups := cw.cfg.EnabledGroups(); len(groups) > 0 {
		return groups[0], true
	}
	return config.ProviderGroup{}, false
}

// setupReactAgent builds the tool-enabled React agent from cfg and tool state.
// An App conversation carries only that App's tools; every other conversation
// carries the selected builtin and MCP tools.

func (cw *ChatWindow) setupReactAgent(provider config.Provider) error {
	einoTools, systemPrompt := cw.agentToolsAndPrompt()
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
			if bt, ok := cw.findBuiltinTool(name); ok && eino.IsBuiltinTool(bt.Type) {
				if t, delegated, err := eino.BuiltinTool(bt.Type, bt.Config); delegated {
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

// agentToolsAndPrompt returns the tools and system prompt of the current
// agent, app or built-in chat, used by both client builders.
func (cw *ChatWindow) agentToolsAndPrompt() ([]tool.BaseTool, string) {
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
	return einoTools, systemPrompt
}

// setSetupError records why no model client could be built. It is kept apart
// from cw.status so a model problem is shown next to the composer and the model
// settings, instead of on whatever settings tab happens to be open.
func (cw *ChatWindow) setSetupError(msg string) {
	cw.setupErr = msg
}

func (cw *ChatWindow) clearSetupError() {
	cw.setupErr = ""
}

// modelSetupError returns the pending model configuration problem, if any.
func (cw *ChatWindow) modelSetupError() string {
	return cw.setupErr
}
