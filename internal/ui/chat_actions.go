package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/pkg/models"
)

// turnFor returns the turn whose reply is msg.
func (cw *ChatWindow) turnFor(msg models.Message) (models.Turn, bool) {
	if cw.cur == nil {
		return models.Turn{}, false
	}
	if t := turnOf(cw.cur, msg.ID); t != nil {
		return *t, true
	}
	return models.Turn{}, false
}

// turnModelLabel names the model a turn was generated with.
func (cw *ChatWindow) turnModelLabel(msg models.Message) string {
	t, ok := cw.turnFor(msg)
	if !ok || (t.Model == "" && t.Provider == "") {
		return ""
	}
	return modelLabel(modelRef(t.Provider, t.Model))
}

// tokenLabelFor formats a message's token usage for the panel.
func (cw *ChatWindow) tokenLabelFor(msg models.Message) string {
	t, ok := cw.turnFor(msg)
	if !ok || t.Total <= 0 {
		return ""
	}
	return fmt.Sprintf("↑%s ↓%s = %s tokens", formatCount(t.Input), formatCount(t.Output), formatCount(t.Total))
}

// applyEdit replaces an edited message, drops what followed and re-answers.
func (cw *ChatWindow) applyEdit(i int, text string) {
	if cw.cur == nil {
		return
	}
	conv := cw.cur
	conv.Messages[i].Content = text
	conv.Messages = conv.Messages[:i+1]
	_ = cw.convManager.SaveConversation(conv)
	cw.resendFrom(i + 1)
}

// regenerate drops an assistant reply and asks for a new one.
func (cw *ChatWindow) regenerate(i int) {
	if cw.cur == nil || i <= 0 {
		return
	}
	conv := cw.cur
	prev := conv.Messages[i]
	t := turnOf(conv, prev.ID)
	conv.Messages = conv.Messages[:i]
	reply := models.Message{
		ID: cw.newID(), Role: "assistant", Timestamp: time.Now(),
	}
	conv.Messages = append(conv.Messages, reply)
	if t != nil {
		t.ReplyMsgID = reply.ID
		t.Assistant = ""
		t.Input, t.Output, t.Total = 0, 0, 0
		t.EndedAt = time.Time{}
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.streamFrom(conv, i)
}

// resendFrom streams a new assistant reply appended at index i.
func (cw *ChatWindow) resendFrom(i int) {
	if cw.cur == nil || i == 0 {
		return
	}
	conv := cw.cur
	prev := conv.Messages[i-1]
	reply := models.Message{ID: cw.newID(), Role: "assistant", Timestamp: time.Now()}
	conv.Messages = append(conv.Messages, reply)
	cw.appendTurn(conv, prev, reply)
	cw.streamFrom(conv, i)
}

// appendTurn records a turn for an edited message's new reply.
func (cw *ChatWindow) appendTurn(conv *models.Conversation, userMsg, reply models.Message) {
	conv.Turns = append(conv.Turns, models.Turn{
		ID:         cw.newID(),
		UserMsgID:  userMsg.ID,
		ReplyMsgID: reply.ID,
		User:       userMsg.Content,
		Provider:   cw.provider,
		Model:      cw.model,
		StartedAt:  time.Now(),
	})
}

func (cw *ChatWindow) streamFrom(conv *models.Conversation, i int) {
	cw.streaming = true
	cw.panel.List.ScrollToEnd()
	if strings.HasPrefix(conv.Agent, "acp:") {
		_ = cw.convManager.SaveConversation(conv)
		cw.acpChatStart(conv, strings.TrimPrefix(conv.Agent, "acp:"), i, lastUserText(conv, i))
		return
	}
	cw.applyTurnModelFor(conv, conv.Messages[i].ID)
	msgs := cw.chatMessages(conv, i)
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, i, msgs)
}

// applyTurnModelFor rebuilds the client for the model a turn was recorded with.
func (cw *ChatWindow) applyTurnModelFor(conv *models.Conversation, replyID string) {
	t := turnOf(conv, replyID)
	if t == nil {
		return
	}
	cw.applyModel(t.Provider, t.Model)
}

// applyModel switches the active provider and model, rebuilding the client.
func (cw *ChatWindow) applyModel(provider, model string) {
	if provider == "" && model == "" {
		return
	}
	if provider == cw.provider && model == cw.model {
		return
	}
	if provider != "" {
		cw.provider = provider
	}
	if model != "" {
		cw.model = model
	}
	cw.setupProvider(cw.provider)
}

func lastUserText(conv *models.Conversation, before int) string {
	for j := before - 1; j >= 0; j-- {
		if conv.Messages[j].Role == "user" {
			return conv.Messages[j].Content
		}
	}
	return ""
}

func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
