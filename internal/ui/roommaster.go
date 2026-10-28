package ui

import (
	"context"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
)

// masterNoneToken is the moderator's reply when nobody should speak.
const masterNoneToken = "NONE"

// masterSelfPrefix marks a reply the moderator gives directly, in place of
// waking any member.
const masterSelfPrefix = "SELF:"

// masterSystemPrompt instructs the moderator. It asks for names only, in
// speaking order, because free-form prose cannot be parsed back into a
// roster reliably.
const masterSystemPrompt = `你是群聊主持人。根据用户的最新消息和对话上下文，从成员名单中挑出本轮应当发言的成员。

规则：
1. 只输出成员名，用逗号分隔，按发言顺序排列，不要输出任何其他内容。
2. 消息点名某个成员时，只选他。
3. 开放性问题可以选多个成员，把最合适的放在最前面。
4. 没有成员需要发言时，只输出 ` + masterNoneToken + `。
5. 问题简单、你自己就能答好时，不选成员，输出 ` + masterSelfPrefix + ` 加你的回答。
6. 「群聊历史归档」是检索到的更早讨论，带说话人和时间。越新的消息越可信；归档已经足以回答时，用它直接 ` + masterSelfPrefix + ` 作答并注明出处，不再唤醒成员。`

// masterChatFunc is the moderator call, resolved on the main thread so the
// turn goroutine never reads the configuration.
type masterChatFunc func(context.Context, []llm.ChatMessage) (*llm.ChatResponse, error)

// masterClient picks the moderator model: the room's designated model when
// set, otherwise the window's current provider or group - CrewAI's
// manager_llm. The moderator is a bare model with its own prompt, never a
// member: it outputs a roster, and a persona would only distort it.
func (cw *ChatWindow) masterClient(r config.Room) masterChatFunc {
	provName, model := cw.provider, ""
	if master := strings.TrimSpace(r.Master); master != "" {
		if p, m := splitModelRef(master); p != "" {
			provName, model = p, m
		}
	}
	if g, ok := cw.cfg.FindProviderGroup(provName); ok {
		if gc, err := llm.NewGroupClient(g, cw.cfg.Providers); err == nil {
			return gc.ChatNonBlocking
		}
	}
	provider, ok := cw.findProvider(provName)
	if !ok {
		provider, ok = cw.firstEnabled()
	}
	if !ok {
		return nil
	}
	if model != "" {
		provider.Model = model
	}
	client, err := llm.NewClient(provider)
	if err != nil {
		return nil
	}
	return client.ChatNonBlocking
}

// masterNarrow runs the moderator at the head of a turn: it narrows the
// candidates to the picked members, syncs the roster queue, and records the
// decision as a system message so the routing is readable in the log. A
// moderator that answers by itself ends the turn with its reply as a message
// of its own.
// masterNarrow runs the moderator at the head of a turn, as a speaking
// member: a placeholder message streams while it decides, then carries its
// assignment or its own answer. It narrows the candidates to the picked
// members and syncs the roster queue; a pick of everyone withdraws the
// placeholder, because confirming the default is not worth a message.
func (cw *ChatWindow) masterNarrow(ctx context.Context, kbID string, conv *models.Conversation, members []config.RoomMember) []config.RoomMember {
	ready := make(chan string, 1)
	cw.postMain(func() {
		reply := conv.NewAgentMessage(cw.newID(), masterAuthorID, "主持人", time.Now())
		conv.Messages = append(conv.Messages, reply)
		cw.beginStream(conv.ID, reply.ID, nil)
		cw.room.list.ScrollToEnd()
		cw.invalidate()
		ready <- reply.ID
	})
	replyID := <-ready

	history := ""
	if q := roomLastHumanText(conv); q != "" {
		history = cw.roomHistorySearch(kbID, q, 6)
	}
	picked, selfReply := masterPick(ctx, cw.room.masterChat, conv, members, history)

	done := make(chan struct{})
	cw.postMain(func() {
		defer close(done)
		cw.endStream(replyID)
		i := roomIndexOf(conv, replyID)
		if i < 0 {
			return
		}
		msg := &conv.Messages[i]
		switch {
		case selfReply != "":
			msg.Content = selfReply
			cw.room.queue = nil
			cw.room.turnOrder = nil
		case len(picked) != len(members):
			queue := make([]string, 0, len(picked))
			order := make([]string, 0, len(picked))
			for _, m := range picked {
				queue = append(queue, m.Agent)
				order = append(order, m.Agent)
			}
			cw.room.queue = queue
			cw.room.turnOrder = order
			msg.Content = masterAssignText(picked)
		default:
			// Everyone speaks anyway: withdraw the placeholder.
			conv.Messages = append(conv.Messages[:i], conv.Messages[i+1:]...)
		}
		_ = cw.convManager.SaveConversation(conv)
		cw.room.list.ScrollToEnd()
		cw.invalidate()
	})
	<-done
	if selfReply != "" {
		return nil
	}
	return picked
}

// masterAssignText is the moderator's assignment, spoken as a reply that @s
// the picked members, the way a human moderator would hand a question over.
func masterAssignText(picked []config.RoomMember) string {
	if len(picked) == 0 {
		return "本轮无需成员发言。"
	}
	names := make([]string, 0, len(picked))
	for _, m := range picked {
		names = append(names, "@"+roomMemberName(m))
	}
	return strings.Join(names, " ") + " 请处理这个问题。"
}

// masterAuthorID marks the moderator's notices in a conversation.
const masterAuthorID = "master"

// masterPick asks the moderator who speaks this turn. It returns the picked
// members, or the moderator's own reply when it chose to answer itself. Any
// failure - no client, an error, an unparseable reply - falls back to every
// candidate, because a broken moderator must degrade to the default, not to
// silence.
func masterPick(ctx context.Context, chat masterChatFunc, conv *models.Conversation, members []config.RoomMember, history string) ([]config.RoomMember, string) {
	if chat == nil || len(members) <= 1 {
		return members, ""
	}
	resp, err := chat(ctx, []llm.ChatMessage{
		{Role: "system", Content: masterSystemPrompt},
		{Role: "user", Content: masterTranscript(conv, members, history)},
	})
	if err != nil || resp == nil {
		return members, ""
	}
	if self, ok := masterSelfReply(resp.Content); ok {
		return nil, self
	}
	picked, decided := parseMasterPick(resp.Content, members)
	if !decided {
		return members, ""
	}
	return picked, ""
}

// masterSelfReply extracts the moderator's own answer from a SELF: reply.
// A bare SELF: with nothing behind it is not an answer and falls through to
// the roster parser.
func masterSelfReply(reply string) (string, bool) {
	t := strings.TrimSpace(reply)
	if len(t) < len(masterSelfPrefix) || !strings.EqualFold(t[:len(masterSelfPrefix)], masterSelfPrefix) {
		return "", false
	}
	body := strings.TrimSpace(t[len(masterSelfPrefix):])
	if body == "" {
		return "", false
	}
	return body, true
}

// masterTranscript is the moderator's reading: the roster and the tail of
// the conversation, newest last.
func masterTranscript(conv *models.Conversation, members []config.RoomMember, history string) string {
	var b strings.Builder
	b.WriteString("成员名单：")
	for i, m := range members {
		if i > 0 {
			b.WriteString("、")
		}
		b.WriteString(roomMemberName(m))
	}
	if history != "" {
		b.WriteString("\n\n群聊历史归档（检索自知识库，越新越可信）：\n")
		b.WriteString(history)
	}
	b.WriteString("\n\n最近的对话：\n")
	msgs := conv.Messages
	start := len(msgs) - 8
	if start < 0 {
		start = 0
	}
	for _, msg := range msgs[start:] {
		body := strings.TrimSpace(msg.Content)
		if body == "" {
			continue
		}
		b.WriteString("[")
		b.WriteString(roomSpeakerName(msg))
		b.WriteString("]: ")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// parseMasterPick reads the moderator's name list back into members, in the
// moderator's order, dropping duplicates and unknown names. The second
// result reports whether the reply was a decision at all: a reply naming
// nobody that is not NONE is noise, and the caller falls back.
func parseMasterPick(reply string, members []config.RoomMember) ([]config.RoomMember, bool) {
	t := strings.TrimSpace(reply)
	if t == "" {
		return nil, false
	}
	if strings.EqualFold(strings.Trim(t, ".。!！"), masterNoneToken) {
		return nil, true
	}
	fields := strings.FieldsFunc(t, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '\n' || r == ';' || r == '；'
	})
	seen := map[string]bool{}
	out := make([]config.RoomMember, 0, len(members))
	for _, f := range fields {
		name := strings.TrimSpace(f)
		if name == "" {
			continue
		}
		for _, m := range members {
			if strings.EqualFold(roomMemberName(m), name) && !seen[m.Agent] {
				seen[m.Agent] = true
				out = append(out, m)
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
