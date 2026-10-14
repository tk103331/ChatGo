package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdata "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"

	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/agent"

	"chatgo/pkg/models"
)

// messageRow renders one message: header, tool calls, content and actions.
func (cw *ChatWindow) messageRow(c *mgo.Context, i int, msg models.Message) {
	if cw.editingIdx == i {
		cw.editMessage(c, i, msg)
		return
	}
	streaming := cw.streaming && cw.cur != nil && i == len(cw.cur.Messages)-1 && msg.Role == "assistant"
	role := roleOf(msg)
	opts := mchat.MessageBubbleOptions{MaxWidth: 900}
	if role == mchat.MessageAssistant {
		opts.Avatar = cw.agentAvatar(c)
	}
	mchat.MessageBubble(c, role, opts, func() {
		model := ""
		if msg.Role == "assistant" && cw.cur != nil {
			model = cw.headerModel()
		}
		mchat.MessageHeader(c, mchat.MessageHeaderOptions{Model: model, Time: msg.Timestamp})
		if len(msg.Segments) > 0 {
			cw.messageSegments(c, msg, streaming)
		} else {
			cw.messageLegacy(c, msg, streaming)
		}
		cw.messageActions(c, i, msg, streaming)
	})
}

// messageSegments renders an assistant reply's ordered blocks: a thinking
// block, text, or a run of tool calls, each in the order the model produced
// them, so a multi-step ReAct turn stays thinking, tool, text, thinking, tool.
func (cw *ChatWindow) messageSegments(c *mgo.Context, msg models.Message, streaming bool) {
	toolIdx := 0
	for i := 0; i < len(msg.Segments); {
		seg := msg.Segments[i]
		switch seg.Kind {
		case "reasoning":
			open := mgo.Local(c.Root(), fmt.Sprintf("reason:%s:%d", msg.ID, i), func() bool { return false })
			cw.reasoningBlock(c, open, seg.Text)
			i++
		case "tool":
			var calls []models.ToolCall
			for i < len(msg.Segments) && msg.Segments[i].Kind == "tool" {
				if tc := msg.Segments[i].ToolCall; tc != nil {
					calls = append(calls, *tc)
				}
				i++
			}
			cw.toolCallsGroup(c, msg, calls, toolIdx)
			toolIdx++
		case "text":
			if strings.TrimSpace(seg.Text) != "" {
				r := mchat.MarkdownView(c, seg.Text)
				if url, ok := r.Link(); ok {
					c.OpenURL(url)
				}
			}
			i++
		default:
			i++
		}
	}
	if cw.emptySegments(msg) && streaming {
		mchat.TypingIndicator(c, "Assistant")
	}
}

// messageLegacy renders a message that predates segments.
func (cw *ChatWindow) messageLegacy(c *mgo.Context, msg models.Message, streaming bool) {
	if msg.Role == "assistant" && strings.TrimSpace(msg.Reasoning) != "" {
		open := mgo.Local(c.Root(), "reason:"+msg.ID, func() bool { return false })
		cw.reasoningBlock(c, open, msg.Reasoning)
	}
	if len(msg.ToolCalls) > 0 {
		cw.toolCallsGroup(c, msg, msg.ToolCalls, 0)
	}
	switch {
	case strings.TrimSpace(msg.Content) != "":
		r := mchat.MarkdownView(c, msg.Content)
		if url, ok := r.Link(); ok {
			c.OpenURL(url)
		}
	case streaming:
		mchat.TypingIndicator(c, "Assistant")
	}
}

// emptySegments reports whether a message has no visible text, thinking or tool
// content yet.
func (cw *ChatWindow) emptySegments(msg models.Message) bool {
	for _, s := range msg.Segments {
		if s.Kind == "tool" && s.ToolCall != nil {
			return false
		}
		if strings.TrimSpace(s.Text) != "" {
			return false
		}
	}
	return true
}

// reasoningBlock shows the model's thinking process as a collapsed block.
func (cw *ChatWindow) reasoningBlock(c *mgo.Context, open *bool, reasoning string) {
	mdata.Collapsible(c, open, mdata.CollapsibleOptions{Title: "思考过程"}, func() {
		mgo.Text(c, reasoning).TextColor(mcore.Tokens(c).TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}

func (cw *ChatWindow) messageActions(c *mgo.Context, i int, msg models.Message, streaming bool) {
	if streaming {
		return
	}
	u := mcore.Density(c).Unit()
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		if mcore.IconAction(c, icons.Must("copy"), "复制").Tooltip("复制").Clicked() {
			c.WriteClipboard(msg.Content)
			cw.toast("已复制到剪贴板", mcore.SeveritySuccess)
		}
		if msg.Role == "user" {
			if mcore.IconAction(c, icons.Must("pencil"), "编辑").Tooltip("编辑").Clicked() {
				cw.editingIdx = i
				cw.editDraft = msg.Content
			}
			return
		}
		if mcore.IconAction(c, icons.Must("refresh-cw"), "重新生成").Tooltip("重新生成").Clicked() {
			cw.regenerate(i)
		}
	})
}

func (cw *ChatWindow) headerModel() string {
	if cw.cur == nil {
		return ""
	}
	if strings.HasPrefix(cw.cur.Agent, "acp:") {
		return ""
	}
	return cw.cur.Model
}

func roleOf(msg models.Message) mchat.MessageRole {
	switch msg.Role {
	case "user":
		return mchat.MessageUser
	case "system":
		return mchat.MessageSystem
	default:
		return mchat.MessageAssistant
	}
}

func (cw *ChatWindow) toolCallsGroup(c *mgo.Context, msg models.Message, calls []models.ToolCall, groupIdx int) {
	items := toolCallsForGroup(msg, calls, groupIdx)
	if len(items) == 0 {
		return
	}
	open := mgo.Local(c.Root(), fmt.Sprintf("toolgroup:%s:%d", msg.ID, groupIdx), func() bool { return false })
	agent.ToolCallGroup(c, open, items, agent.ToolCallGroupOptions{})
}

func toolCallsForGroup(msg models.Message, calls []models.ToolCall, groupIdx int) []agent.ToolCall {
	used := make(map[string]bool, len(calls))
	out := make([]agent.ToolCall, 0, len(calls))
	for i, tc := range calls {
		if tc.Name == "" {
			continue
		}
		id := tc.ID
		if id == "" || used[id] {
			id = fmt.Sprintf("%s-%d-%d", msg.ID, groupIdx, i)
		}
		used[id] = true
		state := agent.AgentRunning
		switch {
		case tc.Error != "" || tc.Status == "failed":
			state = agent.AgentFailed
		case tc.Status == "completed" || tc.Status == "" && tc.Result != "":
			state = agent.AgentDone
		}
		out = append(out, agent.ToolCall{
			ID: id, Name: tc.Name, Args: tc.Arguments, Result: tc.Result, Error: tc.Error,
			State: state, Duration: time.Duration(tc.DurationMs) * time.Millisecond,
		})
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// editMessage edits a sent user message in place.
func (cw *ChatWindow) editMessage(c *mgo.Context, i int, msg models.Message) {
	r := mchat.MessageEditor(c, &cw.editDraft, mchat.MessageEditorOptions{Original: msg.Content})
	switch {
	case r.Saved():
		cw.applyEdit(i)
	case r.Canceled():
		cw.editingIdx = -1
	}
}

// applyEdit replaces an edited message, drops what followed and re-answers.
func (cw *ChatWindow) applyEdit(i int) {
	if cw.cur == nil {
		cw.editingIdx = -1
		return
	}
	conv := cw.cur
	conv.Messages[i].Content = cw.editDraft
	conv.Messages = conv.Messages[:i+1]
	cw.editingIdx = -1
	_ = cw.convManager.SaveConversation(conv)
	cw.resendFrom(i + 1)
}

// regenerate drops an assistant reply and asks for a new one.
func (cw *ChatWindow) regenerate(i int) {
	if cw.cur == nil || i <= 0 {
		return
	}
	conv := cw.cur
	conv.Messages = conv.Messages[:i]
	_ = cw.convManager.SaveConversation(conv)
	cw.resendFrom(i)
}

// resendFrom streams a new assistant reply appended at index i.
func (cw *ChatWindow) resendFrom(i int) {
	if cw.cur == nil || i == 0 {
		return
	}
	conv := cw.cur
	conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Timestamp: time.Now()})
	cw.streaming = true
	cw.msgList.ScrollToEnd()
	if strings.HasPrefix(conv.Agent, "acp:") {
		_ = cw.convManager.SaveConversation(conv)
		cw.acpChatStart(conv, strings.TrimPrefix(conv.Agent, "acp:"), i, lastUserText(conv, i))
		return
	}
	msgs := cw.chatMessages(conv, i)
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, i, msgs)
}

func lastUserText(conv *models.Conversation, before int) string {
	for j := before - 1; j >= 0; j-- {
		if conv.Messages[j].Role == "user" {
			return conv.Messages[j].Content
		}
	}
	return ""
}
