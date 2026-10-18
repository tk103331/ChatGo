package chatpanel

import (
	"fmt"
	"strings"
	"time"

	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/agent"
	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdata "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// messageRow renders one message: header, tool calls, content and actions.
func (p *Panel) messageRow(c *mgo.Context, i int, msg models.Message, total int, opts Options) {
	if p.Editing == i {
		p.editMessage(c, i, msg, opts)
		return
	}
	streaming := opts.Streaming != nil && opts.Streaming() && i == total-1 && msg.Role == "assistant"
	role := roleOf(msg)
	bubbleOpts := mchat.MessageBubbleOptions{MaxWidth: 900}
	if role == mchat.MessageAssistant && opts.Avatar != nil {
		bubbleOpts.Avatar = opts.Avatar(c)
	}
	mchat.MessageBubble(c, role, bubbleOpts, func() {
		model := ""
		if msg.Role == "assistant" && opts.ModelLabel != nil {
			model = opts.ModelLabel(msg)
		}
		mchat.MessageHeader(c, mchat.MessageHeaderOptions{Model: model, Time: msg.Timestamp})
		if len(msg.Segments) > 0 {
			p.messageSegments(c, msg, streaming)
		} else {
			p.messageLegacy(c, msg, streaming)
		}
		p.messageActions(c, i, msg, streaming, opts)
	})
}

// messageSegments renders an assistant reply's ordered blocks: a thinking
// block, text, or a run of tool calls, each in the order the model produced
// them, so a multi-step ReAct turn stays thinking, tool, text, thinking, tool.
func (p *Panel) messageSegments(c *mgo.Context, msg models.Message, streaming bool) {
	toolIdx := 0
	for i := 0; i < len(msg.Segments); {
		seg := msg.Segments[i]
		switch seg.Kind {
		case "reasoning":
			open := mgo.Local(c.Root(), fmt.Sprintf("reason:%s:%d", msg.ID, i), func() bool { return false })
			reasoningBlock(c, open, seg.Text)
			i++
		case "tool":
			var calls []models.ToolCall
			for i < len(msg.Segments) && msg.Segments[i].Kind == "tool" {
				if tc := msg.Segments[i].ToolCall; tc != nil {
					calls = append(calls, *tc)
				}
				i++
			}
			toolCallsGroup(c, msg, calls, toolIdx)
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
	if emptySegments(msg) && streaming {
		mchat.TypingIndicator(c, "Assistant")
	}
}

// messageLegacy renders a message that predates segments.
func (p *Panel) messageLegacy(c *mgo.Context, msg models.Message, streaming bool) {
	if msg.Role == "assistant" && strings.TrimSpace(msg.Reasoning) != "" {
		open := mgo.Local(c.Root(), "reason:"+msg.ID, func() bool { return false })
		reasoningBlock(c, open, msg.Reasoning)
	}
	if len(msg.ToolCalls) > 0 {
		toolCallsGroup(c, msg, msg.ToolCalls, 0)
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

// emptySegments reports whether a message has no visible text, thinking or
// tool content yet.
func emptySegments(msg models.Message) bool {
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
func reasoningBlock(c *mgo.Context, open *bool, reasoning string) {
	mdata.Collapsible(c, open, mdata.CollapsibleOptions{Title: "思考过程"}, func() {
		mgo.Text(c, reasoning).TextColor(mcore.Tokens(c).TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}

// messageActions renders the copy, edit and regenerate actions.
func (p *Panel) messageActions(c *mgo.Context, i int, msg models.Message, streaming bool, opts Options) {
	if streaming {
		return
	}
	u := mcore.Density(c).Unit()
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		if mcore.IconAction(c, icons.Must("copy"), "复制").Tooltip("复制").Clicked() {
			c.WriteClipboard(msg.Content)
			if opts.OnCopy != nil {
				opts.OnCopy(msg.Content)
			}
		}
		if msg.Role == "user" {
			if opts.EditSaved != nil {
				if mcore.IconAction(c, icons.Must("pencil"), "编辑").Tooltip("编辑").Clicked() {
					p.Editing = i
					p.EditDraft = msg.Content
				}
			}
			return
		}
		if mcore.IconAction(c, icons.Must("refresh-cw"), "重新生成").Tooltip("重新生成").Clicked() {
			if opts.Regenerate != nil {
				opts.Regenerate(i)
			}
		}
		if opts.TokenLabel != nil {
			if label := opts.TokenLabel(msg); label != "" {
				mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(mcore.Tokens(c).TextMuted).SingleLine()
			}
		}
	})
}

// editMessage edits a sent user message in place.
func (p *Panel) editMessage(c *mgo.Context, i int, msg models.Message, opts Options) {
	r := mchat.MessageEditor(c, &p.EditDraft, mchat.MessageEditorOptions{Original: msg.Content})
	switch {
	case r.Saved():
		if opts.EditSaved != nil {
			opts.EditSaved(i, p.EditDraft)
		}
		p.Editing = -1
	case r.Canceled():
		p.Editing = -1
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

func toolCallsGroup(c *mgo.Context, msg models.Message, calls []models.ToolCall, groupIdx int) {
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
