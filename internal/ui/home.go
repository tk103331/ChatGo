package ui

import (
	"chatgo/pkg/models"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// setupHomeUI initializes the home page with a centered input box, send button, and recent conversations.
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

	// Create recent conversations section
	recentConvsLabel := widget.NewLabel("最近会话")
	recentConvsLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Create list for recent conversations (will be populated by updateRecentConversations)
	recentConvsList := widget.NewList(
		func() int {
			// Show only the 5 most recent conversations
			if len(cw.convListData) > 5 {
				return 5
			}
			return len(cw.convListData)
		},
		func() fyne.CanvasObject {
			// Title and time in the same row
			titleLabel := widget.NewLabel("")
			timeLabel := widget.NewLabel("")
			return container.NewHBox(
				titleLabel,
				layout.NewSpacer(),
				timeLabel,
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			cont := obj.(*fyne.Container)
			if id < len(cw.convListData) {
				conv := cw.convListData[id]
				titleLabel := cont.Objects[0].(*widget.Label)
				timeLabel := cont.Objects[2].(*widget.Label)

				titleLabel.SetText(conv.Title)
				titleLabel.TextStyle = fyne.TextStyle{Bold: true}

				// Format time
				if len(conv.Messages) > 0 {
					lastMsg := conv.Messages[len(conv.Messages)-1]
					timeLabel.SetText(lastMsg.Timestamp.Format("2006-01-02 15:04"))
				} else {
					timeLabel.SetText("空会话")
				}
				timeLabel.TextStyle = fyne.TextStyle{Italic: true}
			}
		},
	)

	// Make list items clickable
	recentConvsList.OnSelected = func(id widget.ListItemID) {
		if id < len(cw.convListData) {
			// Switch to chat UI and load the selected conversation
			cw.switchToChatUI()
			cw.loadConversation(cw.convListData[id].ID)
		}
	}

	// Set max height for recent conversations list (show up to 5 items)
	recentConvsScroll := container.NewScroll(recentConvsList)
	recentConvsScroll.SetMinSize(fyne.NewSize(400, 150))

	// Create recent conversations container
	recentConvsContainer := container.NewVBox(
		recentConvsLabel,
		widget.NewSeparator(),
		recentConvsScroll,
	)

	// Main home content: input section at center, recent conversations below
	homeContent := container.NewVBox(
		layout.NewSpacer(),
		inputContainer,
		widget.NewSeparator(),
		recentConvsContainer,
		layout.NewSpacer(),
	)

	cw.homeContainer = container.NewPadded(homeContent)
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

// getConversationLastTime returns the timestamp of the last message in a conversation
// If the conversation has no messages, returns a zero time
func getConversationLastTime(conv models.Conversation) time.Time {
	if len(conv.Messages) == 0 {
		return time.Time{}
	}
	return conv.Messages[len(conv.Messages)-1].Timestamp
}
