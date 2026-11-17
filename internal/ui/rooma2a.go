package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"

	"chatgo/internal/a2a"
	"chatgo/internal/config"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// roomA2AText is the prompt a remote member receives: the same room framing
// ACP gets, because a remote agent has no transcript of its own here and the
// room history has to travel with every turn.
func (cw *ChatWindow) roomA2AText(conv *models.Conversation, m config.RoomMember) string {
	return cw.roomACPText(conv, m)
}

// roomA2ASession applies a turn's continuation to one member's slot.
func roomA2ASession(conv *models.Conversation, agent string, res *a2a.Result) {
	s := conv.A2ASessionFor(agent)
	if res.TaskID != "" {
		s.TaskID = string(res.TaskID)
	}
	if res.ContextID != "" {
		s.ContextID = res.ContextID
	}
	if res.State.Terminal() {
		s.TaskID = ""
	}
	conv.SetA2ASession(agent, s)
}

// runRoomA2AMember generates one remote member's reply and blocks until it is
// done, so the serial room turn keeps its ordering.
func (cw *ChatWindow) runRoomA2AMember(ctx context.Context, r config.Room, conv *models.Conversation, m config.RoomMember) {
	name := roomMemberName(m)
	conn, ok := cw.findA2AConn(strings.TrimPrefix(m.Agent, "a2a:"))
	if !ok {
		cw.postMain(func() {
			cw.roomAppendError(conv, m, name, fmt.Errorf("未找到可用的 A2A 连接：%s", strings.TrimPrefix(m.Agent, "a2a:")))
		})
		return
	}
	prompt := cw.roomA2AText(conv, m)

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

	cli, err := a2a.New(ctx, conn)
	if err != nil {
		cw.finishRoomA2AMember(conv, m, replyID, nil, err, r)
		return
	}
	sess := conv.A2ASessionFor(m.Agent)
	taskID := sdk.TaskID(sess.TaskID)
	res, err := cli.Chat(ctx, a2a.ChatRequest{
		Text:      prompt,
		TaskID:    taskID,
		ContextID: sess.ContextID,
	}, func(chunk string) {
		cw.postMain(func() {
			if i := roomIndexOf(conv, replyID); i >= 0 {
				conv.Messages[i].Content += chunk
				appendSegmentText(&conv.Messages[i], "text", chunk)
			}
		})
	})
	if ctx.Err() != nil {
		cancelID := res.TaskID
		if cancelID == "" {
			cancelID = taskID
		}
		if cancelID != "" {
			go func(id sdk.TaskID) {
				cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = cli.CancelTask(cctx, id)
			}(cancelID)
		}
	}
	cw.finishRoomA2AMember(conv, m, replyID, &res, err, r)
}

// finishRoomA2AMember writes a member's result into the room conversation,
// records its continuation, and handles PASS and handoffs.
func (cw *ChatWindow) finishRoomA2AMember(conv *models.Conversation, m config.RoomMember, replyID string, res *a2a.Result, err error, r config.Room) {
	done := make(chan struct{})
	cw.postMain(func() {
		defer close(done)
		cw.endStream(replyID)
		cw.room.speaking = ""
		i := roomIndexOf(conv, replyID)
		if i < 0 {
			_ = cw.convManager.SaveConversation(conv)
			cw.invalidate()
			return
		}
		msg := &conv.Messages[i]
		if res != nil {
			applyA2AReply(msg, res)
			roomA2ASession(conv, m.Agent, res)
		}
		if err != nil && !strings.HasPrefix(msg.Content, "⚠️") {
			if strings.TrimSpace(msg.Content) == "" {
				msg.Content = "⚠️ " + err.Error()
			} else {
				msg.Content += "\n\n⚠️ " + err.Error()
			}
		}
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

// webRunRoomA2AMember is the browser turn's remote member.
func (cw *ChatWindow) webRunRoomA2AMember(ctx context.Context, room config.Room, conv *models.Conversation, m config.RoomMember, live webui.Live) (handoff []string, pass bool) {
	name := roomMemberName(m)
	conn, ok := cw.findA2AConn(strings.TrimPrefix(m.Agent, "a2a:"))
	if !ok {
		cw.webRoomAppendError(conv, m, name, fmt.Errorf("未找到可用的 A2A 连接：%s", strings.TrimPrefix(m.Agent, "a2a:")), live)
		return nil, false
	}
	prompt := cw.roomA2AText(conv, m)
	replyID := cw.newID()
	live.Update(func(c *models.Conversation) {
		reply := c.NewAgentMessage(replyID, m.Agent, name, time.Now())
		reply.QuoteID = roomTurnPromptID(c)
		c.Messages = append(c.Messages, reply)
	})
	cli, err := a2a.New(ctx, conn)
	if err != nil {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				c.Messages[i].Content = "⚠️ " + err.Error()
			}
		})
		return nil, false
	}
	sess := conv.A2ASessionFor(m.Agent)
	taskID := sdk.TaskID(sess.TaskID)
	res, err := cli.Chat(ctx, a2a.ChatRequest{
		Text:      prompt,
		TaskID:    taskID,
		ContextID: sess.ContextID,
	}, func(chunk string) {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				c.Messages[i].Content += chunk
				appendSegmentText(&c.Messages[i], "text", chunk)
			}
		})
	})
	if ctx.Err() != nil {
		cancelID := res.TaskID
		if cancelID == "" {
			cancelID = taskID
		}
		if cancelID != "" {
			go func(id sdk.TaskID) {
				cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = cli.CancelTask(cctx, id)
			}(cancelID)
		}
	}
	live.Update(func(c *models.Conversation) {
		i := roomIndexOf(c, replyID)
		if i < 0 {
			return
		}
		msg := &c.Messages[i]
		applyA2AReply(msg, &res)
		roomA2ASession(c, m.Agent, &res)
		if err != nil && !strings.HasPrefix(msg.Content, "⚠️") {
			if strings.TrimSpace(msg.Content) == "" {
				msg.Content = "⚠️ " + err.Error()
			} else {
				msg.Content += "\n\n⚠️ " + err.Error()
			}
		}
		if roomIsPass(msg.Content) {
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			pass = true
			return
		}
		if !strings.HasPrefix(msg.Content, "⚠️") {
			for _, h := range roomAtMentioned(room.ActiveMembers(), msg.Content) {
				if h.Agent != m.Agent {
					handoff = append(handoff, h.Agent)
				}
			}
			archived := *msg
			go cw.roomArchiveMessage(room.KBID, archived)
		}
	})
	return handoff, pass
}
