package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	mchat "github.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/data"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"

	"github.com/ZacharyZhang-NY/MujicaUI/agent"

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
	status := mchat.MessageNoStatus
	if streaming {
		status = mchat.MessageStreaming
	}
	mchat.MessageBubble(c, role, mchat.MessageBubbleOptions{}, func() {
		model := ""
		if msg.Role == "assistant" && cw.cur != nil {
			model = cw.cur.Model
		}
		mchat.MessageHeader(c, mchat.MessageHeaderOptions{Model: model, Time: msg.Timestamp, Status: status})
		if len(msg.ToolCalls) > 0 {
			cw.toolCalls(c, msg)
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
		cw.messageActions(c, i, msg, streaming)
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

// messageRowACP renders an ACP conversation turn with agent cards.
func (cw *ChatWindow) messageRowACP(c *mgo.Context, i int, msg models.Message) {
	u := mcore.Density(c).Unit()
	streaming := cw.streaming && cw.cur != nil && i == len(cw.cur.Messages)-1 && msg.Role == "assistant"
	switch msg.Role {
	case "user":
		mchat.MessageBubble(c, mchat.MessageUser, mchat.MessageBubbleOptions{}, func() {
			r := mchat.MarkdownView(c, msg.Content)
			if url, ok := r.Link(); ok {
				c.OpenURL(url)
			}
		})
	case "system":
		mchat.MessageBubble(c, mchat.MessageSystem, mchat.MessageBubbleOptions{}, func() {
			mchat.MarkdownView(c, msg.Content)
		})
	default:
		card := agent.AgentCard(c)
		card.FillWidth()
		card.Children(func() {
			mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				state, label := agent.AgentDone, "回答完成"
				if streaming {
					state, label = agent.AgentRunning, "运行中"
				}
				agent.AgentStatus(c, state, agent.AgentStatusOptions{Label: label})
				if cw.cur != nil {
					agent.AgentCaption(c, strings.TrimPrefix(cw.cur.Agent, "acp:"))
				}
			})
			for _, tc := range msg.ToolCalls {
				if tc.Name == "" {
					continue
				}
				state := agent.AgentDone
				switch {
				case tc.Error != "":
					state = agent.AgentFailed
				case tc.Result == "":
					state = agent.AgentRunning
				}
				agent.ToolCallCard(c, agent.ToolCall{
					ID: tc.ID, Name: tc.Name, Args: tc.Arguments,
					Result: tc.Result, Error: tc.Error, State: state,
				}, agent.ToolCallCardOptions{})
			}
			switch {
			case strings.TrimSpace(msg.Content) != "":
				r := mchat.MarkdownView(c, msg.Content)
				if url, ok := r.Link(); ok {
					c.OpenURL(url)
				}
			case streaming:
				mchat.TypingIndicator(c, "Agent")
			}
		})
	}
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

// toolCalls folds a message's tool calls under a collapsible header.
func (cw *ChatWindow) toolCalls(c *mgo.Context, msg models.Message) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	title := fmt.Sprintf("工具调用 (%d)", len(msg.ToolCalls))
	open := mgo.Local(c.Root(), "toolopen:"+msg.ID, func() bool { return false })
	data.Collapsible(c, open, data.CollapsibleOptions{Title: title, Trigger: func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).TextColor(k.TextMuted).Children(func() {
			mgo.Icon(c, icons.Must("terminal")).Size(14, 14)
			mgo.Text(c, title).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).SingleLine()
		})
	}}, func() {
		for _, tc := range msg.ToolCalls {
			toolCallView(c, tc)
		}
	})
}

// toolCallView shows a tool call's name, arguments and result.
func toolCallView(c *mgo.Context, tc models.ToolCall) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u).Padding(u, u*2).Radius(mtheme.CardRadius).
		Border(mtheme.BorderWidth, k.Border).FillWidth().Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, tc.Name).FontWeight(600).SingleLine()
			switch {
			case tc.Error != "":
				mgo.Text(c, "失败").FontSize(small).TextColor(k.Danger)
			case tc.Result != "":
				mgo.Text(c, "完成").FontSize(small).TextColor(k.Success)
			default:
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12, Label: "执行中"})
			}
		})
		if tc.Arguments != "" {
			mgo.Text(c, tc.Arguments).FontSize(small).TextColor(k.TextMuted)
		}
		if tc.Result != "" {
			mgo.Text(c, truncate(tc.Result, 800)).FontSize(small).TextColor(k.TextMuted)
		}
		if tc.Error != "" {
			mgo.Text(c, tc.Error).FontSize(small).TextColor(k.Danger)
		}
	})
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
	msgs := cw.chatMessages(conv, i)
	cw.streaming = true
	cw.msgList.ScrollToEnd()
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, i, msgs)
}
