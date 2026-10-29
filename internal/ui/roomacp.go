package ui

import (
	"context"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// roomACPSessionKey is the session slot of one ACP member in a room: the
// conversation hosts several coding CLIs, so each needs its own slot.
func roomACPSessionKey(convID, agent string) string {
	return convID + "\x00" + agent
}

// runRoomACPMember generates one ACP member's reply and blocks until it is
// done, so the serial turn keeps its ordering. The CLI's events land on the
// conversation's last assistant message, which the placeholder below holds
// for exactly this turn.
func (cw *ChatWindow) runRoomACPMember(ctx context.Context, r config.Room, conv *models.Conversation, m config.RoomMember) {
	name := roomMemberName(m)
	connName := strings.TrimPrefix(m.Agent, "acp:")
	conn, ok := cw.findACPConn(connName)
	if !ok {
		cw.postMain(func() { cw.roomAppendError(conv, m, name, errRoomMissing) })
		return
	}
	prompt := cw.roomACPText(conv, m)

	ready := make(chan string, 1)
	cw.postMain(func() {
		reply := conv.NewAgentMessage(cw.newID(), m.Agent, name, time.Now())
		reply.QuoteID = roomTurnPromptID(conv)
		conv.Messages = append(conv.Messages, reply)
		cw.room.speaking = m.Agent
		cw.room.queue = roomDrop(cw.room.queue, m.Agent)
		cw.beginStream(conv.ID, reply.ID, nil)
		cw.room.list.ScrollToEnd()
		cw.invalidate()
		ready <- reply.ID
	})
	replyID := <-ready

	key := roomACPSessionKey(conv.ID, m.Agent)
	rt, err := cw.acpEnsureSessionAt(conv, conn, key)
	if err != nil {
		cw.postMain(func() {
			cw.endStream(replyID)
			if i := roomIndexOf(conv, replyID); i >= 0 {
				conv.Messages[i].Content = "⚠️ " + err.Error()
			}
			_ = cw.convManager.SaveConversation(conv)
			cw.invalidate()
		})
		return
	}
	// The room's own cwd wins, so every member works in one project.
	_, perr := rt.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: rt.sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	cw.acp.mu.Lock()
	delete(cw.acp.greeting, rt.sessionID)
	cw.acp.mu.Unlock()

	done := make(chan struct{})
	cw.postMain(func() {
		defer close(done)
		i := roomIndexOf(conv, replyID)
		if i < 0 {
			cw.endStream(replyID)
			return
		}
		msg := &conv.Messages[i]
		if perr != nil && msg.Content == "" {
			msg.Content = "⚠️ " + perr.Error()
		}
		cw.endStream(replyID)
		if roomIsPass(msg.Content) {
			conv.Messages = append(conv.Messages[:i], conv.Messages[i+1:]...)
			cw.room.passed[m.Agent] = true
		} else if !strings.HasPrefix(msg.Content, "⚠️") {
			for _, h := range roomAtMentioned(r.ActiveMembers(), msg.Content) {
				if h.Agent != m.Agent {
					cw.room.handoff = append(cw.room.handoff, h.Agent)
				}
			}
			archived := *msg
			go cw.roomArchiveMessage(r.KBID, archived)
		}
		_ = cw.convManager.SaveConversation(conv)
		cw.invalidate()
	})
	<-done
}

// roomACPText is the prompt an ACP member receives: its identity, the
// recent conversation, and the ask. A CLI keeps its own transcript, but the
// other members' words only reach it through this text.
func (cw *ChatWindow) roomACPText(conv *models.Conversation, m config.RoomMember) string {
	name := roomMemberName(m)
	var b strings.Builder
	b.WriteString("你在群聊里，你的身份是「")
	b.WriteString(name)
	b.WriteString("」。群里有用户和其他 AI 助手。\n\n最近的对话：\n")
	msgs := conv.Messages
	start := len(msgs) - 12
	if start < 0 {
		start = 0
	}
	for _, msg := range msgs[start:] {
		if msg.ID == "" {
			continue
		}
		body := strings.TrimSpace(msg.Content)
		if body == "" || msg.AuthorKind == models.AuthorKindSystem {
			continue
		}
		if msg.AuthorID == m.Agent && msg.AuthorKind == models.AuthorKindAgent {
			// The member's own last reply is the turn being asked for.
			continue
		}
		b.WriteString("[")
		b.WriteString(roomSpeakerName(msg))
		b.WriteString("]: ")
		b.WriteString(body)
		b.WriteString("\n")
	}
	b.WriteString("\n请以「")
	b.WriteString(name)
	b.WriteString("」的身份回复最新消息。不需要你发言时，只输出 ")
	b.WriteString(roomPassToken)
	b.WriteString("。")
	return b.String()
}
