// Package ui provides the user interface for ChatGo application
// It implements a home page with centered input and a full-featured chat interface
// with streaming message support and conversation management.
package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/mcp"
	"chatgo/pkg/models"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ChatWindow represents the main chat window of the application.
// It manages two modes: home page (simple centered input) and chat interface (full conversation view).
// The chat interface supports streaming messages, multiple LLM providers, and conversation persistence.
type ChatWindow struct {
	app                 fyne.App
	window              fyne.Window
	config              *config.Config
	convManager         *models.ConversationManager
	mcpManager          *MCPManagerWrapper
	toolSelectionMgr    *ToolSelectionManager
	currentConversation *models.Conversation
	llmClient           *llm.Client
	reactClient         *llm.ReactClient

	// UI components
	convList          *widget.List
	chatArea          *container.Scroll
	messageEntry      *widget.Entry
	sendButton        *widget.Button
	providerSelect    *widget.Select
	toolSelectBtn     *widget.Button
	convListData      []models.Conversation
	messagesContainer *fyne.Container

	// Home page components
	homeContainer    *fyne.Container
	homeMessageEntry *widget.Entry
	isHomeMode       bool
}

// NewChatWindow creates a new chat window instance with the given app and configuration.
// It initializes the conversation manager, sets up the home page UI, and loads existing conversations.
// The window starts in home mode, displaying a centered input box for quick message entry.
func NewChatWindow(app fyne.App, cfg *config.Config) (*ChatWindow, error) {
	convManager, err := models.NewConversationManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation manager: %w", err)
	}

	window := app.NewWindow("ChatGo - AI Chatbot")
	window.Resize(fyne.NewSize(1000, 700))

	mcpManager := NewMCPManagerWrapper()

	cw := &ChatWindow{
		app:         app,
		window:      window,
		config:      cfg,
		convManager: convManager,
		mcpManager:  mcpManager,
		isHomeMode:  true,
	}

	// Initialize tool selection manager
	cw.toolSelectionMgr = NewToolSelectionManager(cfg, mcpManager, window)

	cw.setupHomeUI()
	cw.loadConversations()

	// Auto-initialize MCP servers
	cw.initializeMCPServers()

	return cw, nil
}

// setupHomeUI initializes the home page with a centered input box, send button, and recent conversations.
// This is the initial view when the application starts, allowing users to quickly begin a conversation.
// When a message is submitted, it switches to the full chat interface.
func (cw *ChatWindow) setupUI() {
	// Conversation list on the left
	cw.convList = widget.NewList(
		func() int { return len(cw.convListData) },
		func() fyne.CanvasObject {
			// Create a container with label and icon buttons
			label := widget.NewLabel("")
			label.TextStyle = fyne.TextStyle{Bold: false}

			// Edit icon button
			editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {})
			editBtn.Importance = widget.LowImportance

			// Delete icon button
			deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {})
			deleteBtn.Importance = widget.LowImportance

			return container.NewHBox(label, layout.NewSpacer(), editBtn, deleteBtn)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			container := obj.(*fyne.Container)
			objects := container.Objects

			label := objects[0].(*widget.Label)
			editBtn := objects[2].(*widget.Button)
			deleteBtn := objects[3].(*widget.Button)

			if id < len(cw.convListData) {
				// Format title as Chat-YYYYMMDDHHMMSS
				conv := cw.convListData[id]
				label.SetText(conv.Title)

				// Set up edit button
				editBtn.OnTapped = func() {
					cw.editConversationTitle(id)
				}

				// Set up delete button
				deleteBtn.OnTapped = func() {
					cw.deleteConversation(id)
				}
			}
		},
	)
	cw.convList.OnSelected = func(id widget.ListItemID) {
		if id < len(cw.convListData) {
			cw.loadConversation(cw.convListData[id].ID)
		}
	}

	// New conversation button
	newConvBtn := widget.NewButton("New Chat", func() {
		cw.createNewConversation()
	})

	// Settings button
	settingsBtn := widget.NewButton("Settings", func() {
		cw.showSettings()
	})

	// Conversation list with scroll
	convListScroll := container.NewScroll(cw.convList)

	// Sidebar layout: New Chat on top, Settings on bottom, list fills remaining space
	sidebar := container.NewBorder(
		newConvBtn,     // Top
		settingsBtn,    // Bottom
		nil,            // Left
		nil,            // Right
		convListScroll, // Center (fills remaining space)
	)

	// Chat area
	cw.messagesContainer = container.NewVBox()
	cw.chatArea = container.NewScroll(cw.messagesContainer)
	cw.chatArea.SetMinSize(fyne.NewSize(600, 400))
	// Disable horizontal scrolling
	cw.chatArea.Direction = container.ScrollVerticalOnly

	// Provider selector (placed above input area)
	providerNames := make([]string, len(cw.config.Providers))
	for i, p := range cw.config.Providers {
		providerNames[i] = p.Name
	}
	cw.providerSelect = widget.NewSelect(providerNames, func(selected string) {
		cw.switchProvider(selected)
	})
	cw.providerSelect.SetSelected(cw.config.CurrentProvider)

	// Initialize tool selection manager
	toolCheckGroup := cw.toolSelectionMgr.LoadToolCheckGroup()
	cw.toolSelectionMgr.SetCheckGroup(toolCheckGroup)

	// Tool selection button
	cw.toolSelectBtn = widget.NewButton("选择工具 (0)", func() {
		cw.toolSelectionMgr.ShowToolSelectionDialog()
	})
	cw.toolSelectionMgr.SetButton(cw.toolSelectBtn)

	// Message entry
	cw.messageEntry = widget.NewMultiLineEntry()
	cw.messageEntry.SetPlaceHolder("Type your message here...")
	cw.messageEntry.OnSubmitted = func(text string) {
		cw.sendMessage()
	}

	// Send button
	cw.sendButton = widget.NewButton("Send", func() {
		cw.sendMessage()
	})

	// Provider and tool bar (above input)
	providerToolBar := container.NewHBox(
		widget.NewLabel("Model:"),
		cw.providerSelect,
		widget.NewSeparator(),
		widget.NewLabel("Tools:"),
		cw.toolSelectBtn,
	)

	// Input area
	inputArea := container.NewBorder(nil, nil, nil, cw.sendButton, cw.messageEntry)
	inputAreaContainer := container.NewVBox(
		widget.NewSeparator(),
		providerToolBar,
		inputArea,
	)

	// Main layout
	mainContent := container.NewBorder(
		nil,
		inputAreaContainer,
		nil,
		nil,
		cw.chatArea,
	)

	split := container.NewHSplit(
		sidebar,
		mainContent,
	)
	split.SetOffset(0.25)

	cw.window.SetContent(split)
}

// loadConversations loads all conversations from the database and refreshes the UI list.
// Safe to call in home mode as it checks if convList is initialized.
// For home mode, only shows the 5 most recent conversations.
func (cw *ChatWindow) loadConversations() {
	conversations, err := cw.convManager.ListConversations()
	if err != nil {
		return
	}

	// Sort conversations by last message time (most recent first)
	// We need to sort based on the last message timestamp
	for i := 0; i < len(conversations); i++ {
		for j := i + 1; j < len(conversations); j++ {
			timeI := getConversationLastTime(conversations[i])
			timeJ := getConversationLastTime(conversations[j])
			if timeI.Before(timeJ) {
				conversations[i], conversations[j] = conversations[j], conversations[i]
			}
		}
	}

	cw.convListData = conversations
	// Only refresh if convList is initialized (not in home mode)
	if cw.convList != nil {
		cw.convList.Refresh()
	}
}

// loadConversation loads a specific conversation by ID and displays its messages.
func (cw *ChatWindow) loadConversation(id string) {
	conv, err := cw.convManager.LoadConversation(id)
	if err != nil {
		return
	}

	cw.currentConversation = conv
	cw.setupCurrentProvider()

	// Clear messages
	cw.messagesContainer.Objects = nil

	// Load messages
	for _, msg := range conv.Messages {
		cw.addMessageToUI(msg)
	}

	cw.chatArea.ScrollToBottom()
}

func (cw *ChatWindow) setupCurrentProvider() {
	if cw.currentConversation == nil {
		return
	}

	// Find provider
	for _, p := range cw.config.Providers {
		if p.Name == cw.currentConversation.Provider {
			// Check if React Agent is enabled
			if cw.config.UseReactAgent {
				err := cw.setupReactAgent(p)
				if err != nil {
					fmt.Printf("Failed to setup React Agent: %v\n", err)
					// Fallback to regular client
					client, err := llm.NewClient(p)
					if err != nil {
						return
					}
					cw.llmClient = client
					cw.reactClient = nil
				}
			} else {
				// Use regular client
				client, err := llm.NewClient(p)
				if err != nil {
					return
				}
				cw.llmClient = client
				cw.reactClient = nil
			}
			break
		}
	}
}

// setupReactAgent initializes the React Agent with available tools
func (cw *ChatWindow) setupReactAgent(provider config.Provider) error {
	ctx := context.Background()

	fmt.Printf("[React Agent] ============================================\n")
	fmt.Printf("[React Agent] Setting up React Agent for provider: %s\n", provider.Name)

	// Get selected tools
	selectedTools := cw.toolSelectionMgr.GetSelectedTools()
	fmt.Printf("[React Agent] Selected tools: %d\n", len(selectedTools))
	for i, tool := range selectedTools {
		fmt.Printf("[React Agent]   [%d] %s\n", i+1, tool)
	}

	// Collect all Eino tools (both builtin and MCP)
	einoTools := make([]tool.BaseTool, 0)
	builtinCount := 0
	mcpCount := 0

	// Collect MCP tool names by server
	mcpToolsByServer := make(map[string][]string)

	for _, toolID := range selectedTools {
		if strings.HasPrefix(toolID, "builtin:") {
			// Handle builtin tools - create custom tool definitions
			toolName := strings.TrimPrefix(toolID, "builtin:")
			def, err := cw.createBuiltinToolDefinition(toolName)
			if err != nil {
				fmt.Printf("[React Agent] Warning: failed to create tool definition for %s: %v\n", toolName, err)
				continue
			}
			// Wrap as Eino tool
			wrappedTool := newBuiltinToolWrapper(def)
			einoTools = append(einoTools, wrappedTool)
			builtinCount++
			fmt.Printf("[React Agent] Added builtin tool: %s - %s\n", toolName, def.Description)

		} else if strings.HasPrefix(toolID, "mcp:") {
			// Collect MCP tool names for batch processing
			parts := strings.Split(toolID, ":")
			if len(parts) >= 3 {
				serverName := parts[1]
				toolName := parts[2]
				mcpToolsByServer[serverName] = append(mcpToolsByServer[serverName], toolName)
			}
		}
	}

	// Get MCP tools using Eino's mcp.GetTools() for each server
	for serverName, toolNames := range mcpToolsByServer {
		status, ok := cw.mcpManager.GetServerStatus(serverName)
		if !ok || status.Status != "initialized" {
			fmt.Printf("[React Agent] Warning: MCP server %s not initialized, skipping %d tools\n",
				serverName, len(toolNames))
			continue
		}

		// Use Eino's mcp.GetTools() to get properly formatted tools
		mcpTools, err := einomcp.GetTools(ctx, &einomcp.Config{
			Cli:          status.Client,
			ToolNameList: toolNames,
		})

		if err != nil {
			fmt.Printf("[React Agent] Warning: failed to get MCP tools from %s: %v\n", serverName, err)
			continue
		}

		// Add MCP tools to our collection
		for _, mcpTool := range mcpTools {
			einoTools = append(einoTools, mcpTool)
			mcpCount++
			info, _ := mcpTool.Info(ctx)
			fmt.Printf("[React Agent] Added MCP tool: %s:%s - %s\n", serverName, info.Name, info.Desc)
		}
	}

	fmt.Printf("[React Agent] Successfully loaded %d builtin tools and %d MCP tools (total: %d)\n",
		builtinCount, mcpCount, len(einoTools))

	if len(einoTools) == 0 {
		fmt.Println("[React Agent] Warning: No tools loaded. Agent will run without tools.")
	}

	// Create React Agent config
	agentConfig := &llm.ReactAgentConfig{
		MaxStep:      cw.config.ReactAgentMaxStep,
		SystemPrompt: "You are a helpful AI assistant with access to various tools. Use tools when appropriate to help answer questions. When you use a tool, carefully consider the required parameters and provide accurate values.",
	}

	// Create React Client with Eino tools directly
	reactClient, err := llm.NewReactClientWithEinoTools(provider, einoTools, agentConfig)
	if err != nil {
		return fmt.Errorf("failed to create React client: %w", err)
	}

	cw.reactClient = reactClient
	cw.llmClient = nil

	fmt.Printf("[React Agent] Successfully initialized React Agent with max_step=%d\n", cw.config.ReactAgentMaxStep)
	return nil
}

// createBuiltinToolDefinition creates a tool definition for a builtin tool
func (cw *ChatWindow) createBuiltinToolDefinition(toolName string) (llm.ToolDefinition, error) {
	// Find the tool in config
	var builtinTool *config.BuiltinTool
	for i, t := range cw.config.BuiltinTools {
		if t.Name == toolName && t.Enabled {
			builtinTool = &cw.config.BuiltinTools[i]
			break
		}
	}

	if builtinTool == nil {
		return llm.ToolDefinition{}, fmt.Errorf("builtin tool %s not found or not enabled", toolName)
	}

	def := llm.ToolDefinition{
		Name:        builtinTool.Name,
		Description: config.GetBuiltinToolDescription(builtinTool.Type),
		Parameters:  make(map[string]*schema.ParameterInfo),
	}

	// Build parameter info based on tool type
	requiredFields := config.GetRequiredConfigFields(builtinTool.Type)
	configFields := config.GetBuiltinToolConfigFields(builtinTool.Type)

	for _, field := range configFields {
		required := false
		for _, reqField := range requiredFields {
			if field == reqField {
				required = true
				break
			}
		}

		def.Parameters[field] = &schema.ParameterInfo{
			Type:     schema.String,
			Desc:     fmt.Sprintf("%s parameter for %s tool", field, builtinTool.Name),
			Required: required,
		}
	}

	// Implement actual tool handler for builtin tools
	// For now, return a placeholder handler
	def.Handler = func(ctx context.Context, arguments string) (string, error) {
		fmt.Printf("[Tool Execution] Executing builtin tool: %s with args: %s\n", toolName, arguments)

		// TODO: Implement actual tool execution logic
		// For now, return a simulated response
		return fmt.Sprintf("Tool %s executed successfully with args: %s\n\n(Note: Actual tool execution not yet implemented)", toolName, arguments), nil
	}

	return def, nil
}

// newBuiltinToolWrapper creates an Eino tool wrapper for builtin tools
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

// builtinToolWrapper wraps a builtin tool as an Eino InvokableTool
type builtinToolWrapper struct {
	info    *schema.ToolInfo
	handler func(ctx context.Context, arguments string) (string, error)
}

// Info returns the tool information
func (w *builtinToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.info, nil
}

// InvokableRun executes the tool (required by InvokableTool interface)
func (w *builtinToolWrapper) InvokableRun(ctx context.Context, arguments string) (string, error) {
	return w.handler(ctx, arguments)
}

// StreamableRun executes the tool with streaming (optional, returns not supported)
func (w *builtinToolWrapper) StreamableRun(ctx context.Context, arguments string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported for this tool")
}

func (cw *ChatWindow) switchProvider(providerName string) {
	cw.config.CurrentProvider = providerName

	// Update current conversation provider if exists
	if cw.currentConversation != nil {
		cw.currentConversation.Provider = providerName

		for _, p := range cw.config.Providers {
			if p.Name == providerName {
				cw.currentConversation.Model = p.Model
				client, err := llm.NewClient(p)
				if err == nil {
					cw.llmClient = client
				}
				break
			}
		}

		cw.convManager.SaveConversation(cw.currentConversation)
	}

	config.SaveConfig(cw.config)
}

func (cw *ChatWindow) createNewConversation() {
	providerName := cw.providerSelect.Selected
	model := ""

	for _, p := range cw.config.Providers {
		if p.Name == providerName {
			model = p.Model
			break
		}
	}

	// Format: Chat-YYYYMMDDHHMMSS
	title := fmt.Sprintf("Chat-%s", time.Now().Format("20060102150405"))

	conv, err := cw.convManager.CreateConversation(
		title,
		providerName,
		model,
	)
	if err != nil {
		return
	}

	cw.currentConversation = conv
	cw.setupCurrentProvider()
	cw.loadConversations()

	// Clear messages
	cw.messagesContainer.Objects = nil
	cw.messagesContainer.Refresh()
}

func (cw *ChatWindow) editConversationTitle(id widget.ListItemID) {
	if id < 0 || id >= len(cw.convListData) {
		return
	}

	conv := &cw.convListData[id]

	// Create entry for editing title
	entry := widget.NewEntry()
	entry.SetText(conv.Title)
	entry.SetPlaceHolder("Enter new title")

	// Create form
	form := container.NewVBox(
		widget.NewLabel("Edit Conversation Title"),
		widget.NewSeparator(),
		entry,
	)

	// Show dialog
	d := dialog.NewCustomConfirm("Edit Title", "Save", "Cancel", form, func(save bool) {
		if save && entry.Text != "" {
			// Update title
			conv.Title = entry.Text

			// Save to database
			err := cw.convManager.SaveConversation(conv)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to save title: %w", err), cw.window)
				return
			}

			// Refresh list
			cw.convList.Refresh()

			// If this is the current conversation, update window title
			if cw.currentConversation != nil && cw.currentConversation.ID == conv.ID {
				cw.window.SetTitle(fmt.Sprintf("ChatGo - %s", conv.Title))
			}
		}
	}, cw.window)

	d.Resize(fyne.NewSize(400, 200))
	d.Show()
}

func (cw *ChatWindow) deleteConversation(id widget.ListItemID) {
	if id < 0 || id >= len(cw.convListData) {
		return
	}

	conv := cw.convListData[id]

	// Show confirmation dialog
	dialog.ShowConfirm(
		"Delete Conversation",
		fmt.Sprintf("Are you sure you want to delete '%s'?", conv.Title),
		func(confirmed bool) {
			if confirmed {
				// Delete from database
				err := cw.convManager.DeleteConversation(conv.ID)
				if err != nil {
					dialog.ShowError(fmt.Errorf("failed to delete conversation: %w", err), cw.window)
					return
				}

				// If this is the current conversation, clear it
				if cw.currentConversation != nil && cw.currentConversation.ID == conv.ID {
					cw.currentConversation = nil
					cw.messagesContainer.Objects = nil
					cw.messagesContainer.Refresh()
				}

				// Reload list
				cw.loadConversations()
			}
		},
		cw.window,
	)
}

// sendMessage sends a user message to the LLM and displays the response with streaming.
// The request is performed asynchronously using goroutines to avoid blocking the UI.
// Streaming updates are sent through a channel to update the UI in real-time.
func (cw *ChatWindow) sendMessage() {
	text := cw.messageEntry.Text
	if text == "" || cw.currentConversation == nil {
		return
	}

	// Debug: Log which client is being used
	if cw.reactClient != nil {
		fmt.Printf("[DEBUG] Using React Client (Agent mode)\n")
	} else if cw.llmClient != nil {
		fmt.Printf("[DEBUG] Using Regular LLM Client\n")
	} else {
		fmt.Printf("[DEBUG] ERROR: No valid client available!\n")
		return
	}

	// Clear input
	cw.messageEntry.SetText("")

	// Create user message
	userMsg := models.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	}

	cw.currentConversation.Messages = append(cw.currentConversation.Messages, userMsg)
	cw.addMessageToUI(userMsg)
	cw.convManager.SaveConversation(cw.currentConversation)

	// Create assistant message placeholder
	assistantMsg := models.Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()+1),
		Role:      "assistant",
		Content:   "",
		Timestamp: time.Now(),
	}

	// Add placeholder for streaming
	msgLabel := cw.addStreamingMessageToUI(assistantMsg)

	// Prepare messages
	messages := make([]llm.ChatMessage, len(cw.currentConversation.Messages))
	for i, msg := range cw.currentConversation.Messages {
		messages[i] = llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Channel for streaming updates
	chunkChan := make(chan string)
	doneChan := make(chan struct{})

	// Goroutine to handle streaming updates
	go func() {
		for {
			select {
			case chunk := <-chunkChan:
				assistantMsg.Content += chunk
				// Update UI using goroutine-safe method
				cw.messageEntry.Refresh() // Force refresh to trigger UI update
				msgLabel.ParseMarkdown(assistantMsg.Content)
				cw.messagesContainer.Refresh()
				cw.chatArea.ScrollToBottom()
			case <-doneChan:
				return
			}
		}
	}()

	// Send to LLM asynchronously in goroutine
	go func() {
		defer close(doneChan)

		ctx := context.Background()
		var response *llm.ChatResponse
		var err error

		// Use React Client if available, otherwise use regular client
		if cw.reactClient != nil {
			response, err = cw.reactClient.Chat(ctx, messages, func(chunk string) {
				chunkChan <- chunk
			})
		} else if cw.llmClient != nil {
			response, err = cw.llmClient.Chat(ctx, messages, func(chunk string) {
				chunkChan <- chunk
			})
		} else {
			err = fmt.Errorf("no valid client available")
		}

		if err != nil {
			assistantMsg.Content = fmt.Sprintf("Error: %v", err)
		} else {
			assistantMsg.Content = response.Content
		}

		// Final update with complete content
		msgLabel.ParseMarkdown(assistantMsg.Content)
		cw.currentConversation.Messages = append(cw.currentConversation.Messages, assistantMsg)
		cw.convManager.SaveConversation(cw.currentConversation)
		cw.chatArea.ScrollToBottom()
	}()
}

func (cw *ChatWindow) addMessageToUI(msg models.Message) {
	roleLabel := widget.NewLabel(msg.Role)
	roleLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Build message container parts
	parts := []fyne.CanvasObject{
		container.NewHBox(roleLabel, widget.NewLabel(msg.Timestamp.Format("15:04"))),
	}

	// Add tool call information if present
	if len(msg.ToolCalls) > 0 {
		for i, toolCall := range msg.ToolCalls {
			toolIcon := "🔧"
			statusIcon := "✅"
			if toolCall.Error != "" {
				statusIcon = "❌"
			}

			toolLabel := widget.NewLabel(fmt.Sprintf("%s 工具调用 #%d: %s", toolIcon, i+1, toolCall.Name))
			toolLabel.TextStyle = fyne.TextStyle{Bold: true}

			// Create tool call details container
			toolDetails := container.NewVBox()

			// Add arguments if present
			if toolCall.Arguments != "" {
				argsLabel := widget.NewLabel(fmt.Sprintf("参数: %s", toolCall.Arguments))
				argsLabel.Wrapping = fyne.TextWrapWord
				argsLabel.TextStyle = fyne.TextStyle{Italic: true}
				toolDetails.Add(argsLabel)
			}

			// Add result if present
			if toolCall.Result != "" {
				resultLabel := widget.NewLabel(fmt.Sprintf("结果: %s", toolCall.Result))
				resultLabel.Wrapping = fyne.TextWrapWord
				toolDetails.Add(resultLabel)
			}

			// Add error if present
			if toolCall.Error != "" {
				errorLabel := widget.NewLabel(fmt.Sprintf("错误: %s", toolCall.Error))
				errorLabel.Wrapping = fyne.TextWrapWord
				errorLabel.Importance = widget.DangerImportance
				toolDetails.Add(errorLabel)
			}

			// Create expandable tool call container
			toolContainer := container.NewVBox(
				container.NewHBox(toolLabel, widget.NewLabel(statusIcon)),
				container.NewPadded(toolDetails),
				widget.NewSeparator(),
			)

			// Add a card-like border for tool calls
			toolCard := container.NewBorder(
				widget.NewSeparator(),
				nil,
				nil,
				nil,
				toolContainer,
			)

			parts = append(parts, toolCard)
		}
	}

	// Add message content
	contentLabel := widget.NewRichTextFromMarkdown(msg.Content)
	// Enable text wrapping for RichText
	contentLabel.Wrapping = fyne.TextWrapWord

	parts = append(parts, contentLabel, widget.NewSeparator())

	container := container.NewVBox(parts...)

	cw.messagesContainer.Add(container)
	cw.messagesContainer.Refresh()
}

func (cw *ChatWindow) addStreamingMessageToUI(msg models.Message) *widget.RichText {
	roleLabel := widget.NewLabel(msg.Role)
	roleLabel.TextStyle = fyne.TextStyle{Bold: true}

	contentLabel := widget.NewRichTextFromMarkdown("")
	// Enable text wrapping for RichText
	contentLabel.Wrapping = fyne.TextWrapWord

	container := container.NewVBox(
		container.NewHBox(roleLabel, widget.NewLabel(msg.Timestamp.Format("15:04"))),
		contentLabel,
		widget.NewSeparator(),
	)

	cw.messagesContainer.Add(container)
	cw.messagesContainer.Refresh()
	cw.chatArea.ScrollToBottom()

	return contentLabel
}

// Show displays the chat window
func (cw *ChatWindow) Show() {
	cw.window.ShowAndRun()
}

// MCPManagerWrapper wraps the MCP manager for UI use
type MCPManagerWrapper struct {
	manager *mcp.Manager
}

func NewMCPManagerWrapper() *MCPManagerWrapper {
	return &MCPManagerWrapper{
		manager: mcp.NewManager(),
	}
}

// InitializeAllServers initializes all configured MCP servers
func (m *MCPManagerWrapper) InitializeAllServers(servers []config.MCPServer) map[string]*mcp.MCPServerStatus {
	return m.manager.InitializeAll(servers)
}

// GetServerStatus returns the status of a specific server
func (m *MCPManagerWrapper) GetServerStatus(name string) (*mcp.MCPServerStatus, bool) {
	return m.manager.GetServerStatus(name)
}

// GetAllStatus returns all server statuses
func (m *MCPManagerWrapper) GetAllStatus() map[string]*mcp.MCPServerStatus {
	return m.manager.GetAllStatus()
}

// GetServerTools returns the tools for a specific server
func (m *MCPManagerWrapper) GetServerTools(name string) ([]mcp.MCPTool, bool) {
	return m.manager.GetServerTools(name)
}

// GetAllTools returns all tools from all initialized servers
func (m *MCPManagerWrapper) GetAllTools() map[string][]mcp.MCPTool {
	return m.manager.GetAllTools()
}

// ReinitializeServer reinitializes a server
func (m *MCPManagerWrapper) ReinitializeServer(cfg config.MCPServer) (*mcp.MCPServerStatus, error) {
	return m.manager.ReinitializeServer(cfg)
}

// DisconnectServer disconnects a specific server
func (m *MCPManagerWrapper) DisconnectServer(name string) error {
	return m.manager.DisconnectServer(name)
}

// initializeMCPServers initializes all configured MCP servers on startup
// This runs asynchronously to avoid blocking the UI
func (cw *ChatWindow) initializeMCPServers() {
	if len(cw.config.MCPServers) == 0 {
		fmt.Println("No MCP servers configured")
		return
	}

	fmt.Printf("Initializing %d MCP server(s)...\n", len(cw.config.MCPServers))

	// Use a WaitGroup to track when all servers have been initialized
	var wg sync.WaitGroup
	successCount := int64(0)

	// Initialize each server in its own goroutine for parallel execution
	for _, server := range cw.config.MCPServers {
		// Skip disabled servers
		if !server.Enabled {
			fmt.Printf("  ⊘ Skipping disabled MCP server '%s'\n", server.Name)
			continue
		}

		wg.Add(1)
		go func(srv config.MCPServer) {
			defer wg.Done()
			fmt.Printf("  Initializing MCP server '%s' (%s)...\n", srv.Name, srv.Type)
			status, err := cw.mcpManager.manager.InitializeServer(srv)
			if err != nil {
				fmt.Printf("  ✗ Failed to initialize '%s': %v\n", srv.Name, err)
			} else {
				toolCount := len(status.Tools)
				fmt.Printf("  ✓ Successfully initialized '%s' (%d tool%s)\n",
					srv.Name, toolCount, map[bool]string{true: "s", false: ""}[toolCount != 1])
				atomic.AddInt64(&successCount, 1)
			}
		}(server)
	}

	// Count enabled servers for final message
	enabledCount := 0
	for _, server := range cw.config.MCPServers {
		if server.Enabled {
			enabledCount++
		}
	}

	// Wait for all servers to finish initialization in a separate goroutine
	go func() {
		wg.Wait()
		fmt.Printf("MCP server initialization complete: %d/%d successful\n",
			atomic.LoadInt64(&successCount), enabledCount)
	}()
}
