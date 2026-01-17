// Package ui provides the user interface for ChatGo application
// It implements a home page with centered input and a full-featured chat interface
// with streaming message support and conversation management.
package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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
	currentConversation *models.Conversation
	llmClient           *llm.Client

	// UI components
	convList          *widget.List
	chatArea          *container.Scroll
	messageEntry      *widget.Entry
	sendButton        *widget.Button
	providerSelect    *widget.Select
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

	cw := &ChatWindow{
		app:         app,
		window:      window,
		config:      cfg,
		convManager: convManager,
		mcpManager:  NewMCPManagerWrapper(),
		isHomeMode:  true,
	}

	cw.setupHomeUI()
	cw.loadConversations()

	return cw, nil
}

// setupHomeUI initializes the home page with a centered input box and send button.
// This is the initial view when the application starts, allowing users to quickly begin a conversation.
// When a message is submitted, it switches to the full chat interface.
func (cw *ChatWindow) setupHomeUI() {
	// Create centered input for home page
	cw.homeMessageEntry = widget.NewMultiLineEntry()
	cw.homeMessageEntry.SetPlaceHolder("输入消息开始聊天...")
	cw.homeMessageEntry.SetMinRowsVisible(3)

	cw.homeMessageEntry.OnSubmitted = func(text string) {
		cw.handleHomeMessageSubmit()
	}

	// Create send button
	sendBtn := widget.NewButton("发送", func() {
		cw.handleHomeMessageSubmit()
	})

	// Wrap input and button in a container
	inputContainer := container.NewVBox(
		cw.homeMessageEntry,
		sendBtn,
	)

	// Create a vertically centered layout using VBox with spacers
	centerContent := container.NewVBox(
		layout.NewSpacer(),                  // Top spacer - pushes content to center
		container.NewCenter(inputContainer), // Center horizontally
		layout.NewSpacer(),                  // Bottom spacer - pushes content to center
	)

	cw.homeContainer = container.NewPadded(centerContent)
	cw.window.SetContent(cw.homeContainer)
}

// handleHomeMessageSubmit handles message submission from the home page.
// It switches to the chat UI, creates a new conversation, and sends the message.
func (cw *ChatWindow) handleHomeMessageSubmit() {
	text := cw.homeMessageEntry.Text
	if text == "" {
		return
	}

	// Switch to chat UI
	cw.switchToChatUI()

	// Create new conversation with current provider
	cw.createNewConversation()

	// Send the message
	cw.messageEntry.SetText(text)
	cw.sendMessage()
}

// switchToChatUI switches from home page mode to full chat interface mode.
// This is called when the user submits their first message from the home page.
func (cw *ChatWindow) switchToChatUI() {
	if !cw.isHomeMode {
		return
	}

	cw.isHomeMode = false
	cw.setupUI()
	cw.setupCurrentProvider()
}

// setupUI initializes the full chat interface with conversation list, message area, and input controls.
// This creates the main chat layout with sidebar for conversations and main area for messages.
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

	// Provider bar (above input)
	providerBar := container.NewHBox(
		widget.NewLabel("Model:"),
		cw.providerSelect,
	)

	// Input area
	inputArea := container.NewBorder(nil, nil, nil, cw.sendButton, cw.messageEntry)
	inputAreaContainer := container.NewVBox(
		widget.NewSeparator(),
		providerBar,
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
func (cw *ChatWindow) loadConversations() {
	conversations, err := cw.convManager.ListConversations()
	if err != nil {
		return
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
			client, err := llm.NewClient(p)
			if err != nil {
				return
			}
			cw.llmClient = client
			break
		}
	}
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
	if text == "" || cw.currentConversation == nil || cw.llmClient == nil {
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
		response, err := cw.llmClient.Chat(ctx, messages, func(chunk string) {
			chunkChan <- chunk
		})

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

	contentLabel := widget.NewRichTextFromMarkdown(msg.Content)
	// Enable text wrapping for RichText
	contentLabel.Wrapping = fyne.TextWrapWord

	container := container.NewVBox(
		container.NewHBox(roleLabel, widget.NewLabel(msg.Timestamp.Format("15:04"))),
		contentLabel,
		widget.NewSeparator(),
	)

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

func (cw *ChatWindow) showSettings() {
	// Create tabs for Providers and MCP Servers
	providersTab := cw.createProvidersTab(cw.window)
	mcpServersTab := cw.createMCPServersTab(cw.window)

	tabs := container.NewAppTabs(
		container.NewTabItem("Providers", providersTab),
		container.NewTabItem("MCP Servers", mcpServersTab),
	)

	// Show as dialog
	d := dialog.NewCustomConfirm("Settings", "Close", "", tabs, func(bool) {}, cw.window)
	d.Resize(fyne.NewSize(800, 500))
	d.Show()
}

func (cw *ChatWindow) createProvidersTab(parentWindow fyne.Window) fyne.CanvasObject {
	// Track selected provider
	var selectedProvider *config.Provider
	var selectedProviderIndex int = -1

	// Create form entries
	nameEntry := widget.NewEntry()
	typeEntry := widget.NewSelect([]string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}, nil)
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.Password = true
	baseURLEntry := widget.NewEntry()
	modelEntry := widget.NewEntry()

	// Provider list
	providerList := widget.NewList(
		func() int { return len(cw.config.Providers) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			container := obj.(*fyne.Container)
			label := container.Objects[1].(*widget.Label)
			if id < len(cw.config.Providers) {
				provider := cw.config.Providers[id]
				label.SetText(fmt.Sprintf("%s (%s)", provider.Name, provider.Type))
			}
		},
	)

	providerList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(cw.config.Providers) {
			selectedProvider = &cw.config.Providers[id]
			selectedProviderIndex = id

			// Populate form
			nameEntry.SetText(selectedProvider.Name)
			typeEntry.SetSelected(selectedProvider.Type)
			apiKeyEntry.SetText(selectedProvider.APIKey)
			baseURLEntry.SetText(selectedProvider.BaseURL)
			modelEntry.SetText(selectedProvider.Model)
		}
	}

	providerList.OnUnselected = func(id widget.ListItemID) {
		if selectedProviderIndex == id {
			selectedProvider = nil
			selectedProviderIndex = -1

			// Clear form
			nameEntry.SetText("")
			typeEntry.SetSelected("")
			apiKeyEntry.SetText("")
			baseURLEntry.SetText("")
			modelEntry.SetText("")
		}
	}

	// Form
	form := container.NewVBox(
		widget.NewLabel("Provider Details"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Name:"), nameEntry,
			widget.NewLabel("Type:"), typeEntry,
			widget.NewLabel("API Key:"), apiKeyEntry,
			widget.NewLabel("Base URL:"), baseURLEntry,
			widget.NewLabel("Model:"), modelEntry,
		),
	)

	// Buttons
	addBtn := widget.NewButton("Add New", func() {
		// Clear form and deselect
		selectedProvider = nil
		selectedProviderIndex = -1
		providerList.UnselectAll()
		nameEntry.SetText("")
		typeEntry.SetSelected("")
		apiKeyEntry.SetText("")
		baseURLEntry.SetText("")
		modelEntry.SetText("")
	})

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Provider name cannot be empty"), parentWindow)
			return
		}
		if typeEntry.Selected == "" {
			dialog.ShowError(fmt.Errorf("Provider type must be selected"), parentWindow)
			return
		}

		newProvider := config.Provider{
			Name:    nameEntry.Text,
			Type:    typeEntry.Selected,
			APIKey:  apiKeyEntry.Text,
			BaseURL: baseURLEntry.Text,
			Model:   modelEntry.Text,
		}

		if selectedProvider != nil {
			// Update existing provider
			*selectedProvider = newProvider
		} else {
			// Add new provider
			cw.config.Providers = append(cw.config.Providers, newProvider)
			selectedProviderIndex = len(cw.config.Providers) - 1
			selectedProvider = &cw.config.Providers[selectedProviderIndex]
		}

		config.SaveConfig(cw.config)
		providerList.Refresh()
		cw.updateProviderSelector()

		// Select the updated/new provider
		providerList.Select(selectedProviderIndex)
	})

	deleteBtn := widget.NewButton("Delete", func() {
		if selectedProvider == nil {
			dialog.ShowError(fmt.Errorf("Please select a provider to delete"), parentWindow)
			return
		}

		dialog.ShowConfirm(
			"Delete Provider",
			fmt.Sprintf("Are you sure you want to delete provider '%s'?", selectedProvider.Name),
			func(confirmed bool) {
				if confirmed {
					// Remove provider
					cw.config.Providers = append(cw.config.Providers[:selectedProviderIndex], cw.config.Providers[selectedProviderIndex+1:]...)
					config.SaveConfig(cw.config)

					// Reset selection and clear form
					selectedProvider = nil
					selectedProviderIndex = -1
					nameEntry.SetText("")
					typeEntry.SetSelected("")
					apiKeyEntry.SetText("")
					baseURLEntry.SetText("")
					modelEntry.SetText("")

					// Update UI
					providerList.Refresh()
					cw.updateProviderSelector()
				}
			},
			parentWindow,
		)
	})

	buttonContainer := container.NewHBox(addBtn, saveBtn, deleteBtn)

	// Right side container with form and buttons
	rightPanel := container.NewBorder(
		nil,
		buttonContainer,
		nil,
		nil,
		form,
	)

	// Split left and right
	split := container.NewHSplit(
		providerList,
		rightPanel,
	)
	split.SetOffset(0.4)

	return split
}

func (cw *ChatWindow) showProviderDialog(settingsWin fyne.Window, provider *config.Provider, providerList *widget.List) {
	title := "Add Provider"
	if provider != nil {
		title = "Edit Provider"
	}

	nameEntry := widget.NewEntry()
	typeEntry := widget.NewSelect([]string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}, nil)
	apiKeyEntry := widget.NewEntry()
	apiKeyEntry.Password = true
	baseURLEntry := widget.NewEntry()
	modelEntry := widget.NewEntry()

	if provider != nil {
		nameEntry.SetText(provider.Name)
		typeEntry.SetSelected(provider.Type)
		apiKeyEntry.SetText(provider.APIKey)
		baseURLEntry.SetText(provider.BaseURL)
		modelEntry.SetText(provider.Model)
	}

	form := container.NewGridWithColumns(2,
		widget.NewLabel("Name:"), nameEntry,
		widget.NewLabel("Type:"), typeEntry,
		widget.NewLabel("API Key:"), apiKeyEntry,
		widget.NewLabel("Base URL:"), baseURLEntry,
		widget.NewLabel("Model:"), modelEntry,
	)

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Provider name cannot be empty"), settingsWin)
			return
		}
		if typeEntry.Selected == "" {
			dialog.ShowError(fmt.Errorf("Provider type must be selected"), settingsWin)
			return
		}

		newProvider := config.Provider{
			Name:    nameEntry.Text,
			Type:    typeEntry.Selected,
			APIKey:  apiKeyEntry.Text,
			BaseURL: baseURLEntry.Text,
			Model:   modelEntry.Text,
		}

		if provider != nil {
			// Update existing provider
			*provider = newProvider
		} else {
			// Add new provider
			cw.config.Providers = append(cw.config.Providers, newProvider)
		}

		config.SaveConfig(cw.config)
		providerList.Refresh()
		cw.updateProviderSelector()
	})

	content := container.NewVBox(
		form,
		container.NewHBox(layout.NewSpacer(), saveBtn),
	)

	d := dialog.NewCustomConfirm(title, "Save", "Cancel", content, func(response bool) {
		if response {
			// Save is handled in saveBtn
		}
	}, settingsWin)

	// Hook up save button to close dialog
	saveBtn.OnTapped = func() {
		if nameEntry.Text != "" && typeEntry.Selected != "" {
			newProvider := config.Provider{
				Name:    nameEntry.Text,
				Type:    typeEntry.Selected,
				APIKey:  apiKeyEntry.Text,
				BaseURL: baseURLEntry.Text,
				Model:   modelEntry.Text,
			}

			if provider != nil {
				*provider = newProvider
			} else {
				cw.config.Providers = append(cw.config.Providers, newProvider)
			}

			config.SaveConfig(cw.config)
			providerList.Refresh()
			cw.updateProviderSelector()
			d.Hide()
		}
	}

	d.Show()
}

func (cw *ChatWindow) updateProviderSelector() {
	providerNames := make([]string, len(cw.config.Providers))
	for i, p := range cw.config.Providers {
		providerNames[i] = p.Name
	}
	cw.providerSelect.Options = providerNames
	cw.providerSelect.Refresh()
}

func (cw *ChatWindow) createMCPServersTab(parentWindow fyne.Window) fyne.CanvasObject {
	// Track selected MCP server
	var selectedServer *config.MCPServer
	var selectedServerIndex int = -1

	// Create form entries
	nameEntry := widget.NewEntry()
	commandEntry := widget.NewEntry()
	argsEntry := widget.NewMultiLineEntry()
	argsEntry.SetPlaceHolder("Enter arguments separated by new lines\ne.g.:\n-y\n@modelcontextprotocol/server-filesystem\n/path/to/files")
	envEntry := widget.NewMultiLineEntry()
	envEntry.SetPlaceHolder("Enter environment variables as KEY=VALUE, one per line\ne.g.:\nPATH=/usr/local/bin\nNODE_ENV=production")

	// MCP Server list
	mcpList := widget.NewList(
		func() int { return len(cw.config.MCPServers) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ComputerIcon()),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			container := obj.(*fyne.Container)
			label := container.Objects[1].(*widget.Label)
			if id < len(cw.config.MCPServers) {
				server := cw.config.MCPServers[id]
				label.SetText(fmt.Sprintf("%s (%s)", server.Name, server.Command))
			}
		},
	)

	mcpList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(cw.config.MCPServers) {
			selectedServer = &cw.config.MCPServers[id]
			selectedServerIndex = id

			// Populate form
			nameEntry.SetText(selectedServer.Name)
			commandEntry.SetText(selectedServer.Command)
			if len(selectedServer.Args) > 0 {
				argsEntry.SetText(strings.Join(selectedServer.Args, "\n"))
			} else {
				argsEntry.SetText("")
			}
			if len(selectedServer.Env) > 0 {
				envLines := make([]string, 0, len(selectedServer.Env))
				for k, v := range selectedServer.Env {
					envLines = append(envLines, fmt.Sprintf("%s=%s", k, v))
				}
				envEntry.SetText(strings.Join(envLines, "\n"))
			} else {
				envEntry.SetText("")
			}
		}
	}

	mcpList.OnUnselected = func(id widget.ListItemID) {
		if selectedServerIndex == id {
			selectedServer = nil
			selectedServerIndex = -1

			// Clear form
			nameEntry.SetText("")
			commandEntry.SetText("")
			argsEntry.SetText("")
			envEntry.SetText("")
		}
	}

	// Form
	form := container.NewVBox(
		widget.NewLabel("MCP Server Details"),
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			widget.NewLabel("Name:"), nameEntry,
			widget.NewLabel("Command:"), commandEntry,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Args:"),
			container.NewScroll(argsEntry),
		),
		container.NewGridWithColumns(2,
			widget.NewLabel("Env:"),
			container.NewScroll(envEntry),
		),
	)

	// Set minimum sizes for multi-line entries
	argsEntry.SetMinRowsVisible(3)
	envEntry.SetMinRowsVisible(3)

	// Buttons
	addBtn := widget.NewButton("Add New", func() {
		// Clear form and deselect
		selectedServer = nil
		selectedServerIndex = -1
		mcpList.UnselectAll()
		nameEntry.SetText("")
		commandEntry.SetText("")
		argsEntry.SetText("")
		envEntry.SetText("")
	})

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Server name cannot be empty"), parentWindow)
			return
		}
		if commandEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Command cannot be empty"), parentWindow)
			return
		}

		// Parse args
		args := []string{}
		if strings.TrimSpace(argsEntry.Text) != "" {
			args = strings.Split(strings.TrimSpace(argsEntry.Text), "\n")
		}

		// Parse env
		env := make(map[string]string)
		if strings.TrimSpace(envEntry.Text) != "" {
			envLines := strings.Split(strings.TrimSpace(envEntry.Text), "\n")
			for _, line := range envLines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}

		newServer := config.MCPServer{
			Name:    nameEntry.Text,
			Command: commandEntry.Text,
			Args:    args,
			Env:     env,
		}

		if selectedServer != nil {
			// Update existing server
			*selectedServer = newServer
		} else {
			// Add new server
			cw.config.MCPServers = append(cw.config.MCPServers, newServer)
			selectedServerIndex = len(cw.config.MCPServers) - 1
			selectedServer = &cw.config.MCPServers[selectedServerIndex]
		}

		config.SaveConfig(cw.config)
		mcpList.Refresh()

		// Select the updated/new server
		mcpList.Select(selectedServerIndex)
	})

	deleteBtn := widget.NewButton("Delete", func() {
		if selectedServer == nil {
			dialog.ShowError(fmt.Errorf("Please select a server to delete"), parentWindow)
			return
		}

		dialog.ShowConfirm(
			"Delete MCP Server",
			fmt.Sprintf("Are you sure you want to delete MCP server '%s'?", selectedServer.Name),
			func(confirmed bool) {
				if confirmed {
					// Remove MCP server
					cw.config.MCPServers = append(cw.config.MCPServers[:selectedServerIndex], cw.config.MCPServers[selectedServerIndex+1:]...)
					config.SaveConfig(cw.config)

					// Reset selection and clear form
					selectedServer = nil
					selectedServerIndex = -1
					nameEntry.SetText("")
					commandEntry.SetText("")
					argsEntry.SetText("")
					envEntry.SetText("")

					mcpList.Refresh()
				}
			},
			parentWindow,
		)
	})

	buttonContainer := container.NewHBox(addBtn, saveBtn, deleteBtn)

	// Right side container with form and buttons
	rightPanel := container.NewBorder(
		nil,
		buttonContainer,
		nil,
		nil,
		form,
	)

	// Split left and right
	split := container.NewHSplit(
		mcpList,
		rightPanel,
	)
	split.SetOffset(0.4)

	return split
}

func (cw *ChatWindow) showMCPServerDialog(settingsWin fyne.Window, server *config.MCPServer, mcpList *widget.List) {
	title := "Add MCP Server"
	if server != nil {
		title = "Edit MCP Server"
	}

	nameEntry := widget.NewEntry()
	commandEntry := widget.NewEntry()
	argsEntry := widget.NewMultiLineEntry()
	argsEntry.SetPlaceHolder("Enter arguments separated by new lines\ne.g.:\n-y\n@modelcontextprotocol/server-filesystem\n/path/to/files")
	envEntry := widget.NewMultiLineEntry()
	envEntry.SetPlaceHolder("Enter environment variables as KEY=VALUE, one per line\ne.g.:\nPATH=/usr/local/bin\nNODE_ENV=production")

	if server != nil {
		nameEntry.SetText(server.Name)
		commandEntry.SetText(server.Command)
		if len(server.Args) > 0 {
			argsEntry.SetText(strings.Join(server.Args, "\n"))
		}
		if len(server.Env) > 0 {
			envLines := make([]string, 0, len(server.Env))
			for k, v := range server.Env {
				envLines = append(envLines, fmt.Sprintf("%s=%s", k, v))
			}
			envEntry.SetText(strings.Join(envLines, "\n"))
		}
	}

	form := container.NewGridWithColumns(2,
		widget.NewLabel("Name:"), nameEntry,
		widget.NewLabel("Command:"), commandEntry,
		widget.NewLabel("Args:"), container.NewGridWithColumns(1, argsEntry),
		widget.NewLabel("Env:"), container.NewGridWithColumns(1, envEntry),
	)

	content := container.NewVBox(
		form,
	)

	var d dialog.Dialog

	saveBtn := widget.NewButton("Save", func() {
		if nameEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Server name cannot be empty"), settingsWin)
			return
		}
		if commandEntry.Text == "" {
			dialog.ShowError(fmt.Errorf("Command cannot be empty"), settingsWin)
			return
		}

		// Parse args
		args := []string{}
		if argsEntry.Text != "" {
			args = strings.Split(strings.TrimSpace(argsEntry.Text), "\n")
		}

		// Parse env
		env := make(map[string]string)
		if envEntry.Text != "" {
			envLines := strings.Split(strings.TrimSpace(envEntry.Text), "\n")
			for _, line := range envLines {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
				}
			}
		}

		newServer := config.MCPServer{
			Name:    nameEntry.Text,
			Command: commandEntry.Text,
			Args:    args,
			Env:     env,
		}

		if server != nil {
			*server = newServer
		} else {
			cw.config.MCPServers = append(cw.config.MCPServers, newServer)
		}

		config.SaveConfig(cw.config)
		mcpList.Refresh()
		d.Hide()
	})

	d = dialog.NewCustomConfirm(title, "Save", "Cancel", content, func(response bool) {
		if response {
			saveBtn.OnTapped()
		}
	}, settingsWin)

	d.Show()
}

// Show displays the chat window
func (cw *ChatWindow) Show() {
	cw.window.ShowAndRun()
}

// MCPManagerWrapper wraps the MCP manager for UI use
type MCPManagerWrapper struct {
	// Add MCP manager instance when needed
}

func NewMCPManagerWrapper() *MCPManagerWrapper {
	return &MCPManagerWrapper{}
}
