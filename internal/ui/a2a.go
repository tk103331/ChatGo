package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"

	"chatgo/internal/a2a"
	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// findA2AConn looks up the remote agent a conversation's "a2a:Name" names.
func (cw *ChatWindow) findA2AConn(name string) (config.A2AConnection, bool) {
	for _, a := range cw.cfg.A2AAgents {
		if a.Name == name {
			return a, !a.Disabled
		}
	}
	return config.A2AConnection{}, false
}

// applyA2AResult stores the continuation a turn returned. A terminal task is
// cleared; its context stays so a new task still belongs to the same agent
// conversation.
func applyA2AResult(conv *models.Conversation, res *a2a.Result) {
	if conv == nil || res == nil {
		return
	}
	if res.TaskID != "" {
		conv.A2ATaskID = string(res.TaskID)
	}
	if res.ContextID != "" {
		conv.A2AContextID = res.ContextID
	}
	if res.State.Terminal() {
		conv.A2ATaskID = ""
	}
}

// setA2AConversationAgent points a conversation at one remote agent, dropping
// task state that belonged to a different one. Context and task ids are not
// portable across agents, so reusing them after a switch would address the
// wrong remote task.
func setA2AConversationAgent(conv *models.Conversation, ref string) {
	if conv == nil || conv.Agent == ref {
		return
	}
	conv.Agent = ref
	conv.A2ATaskID = ""
	conv.A2AContextID = ""
	conv.A2ASessions = nil
}

// applyA2AReply fills a reply placeholder with text the streaming path did
// not already write.
func applyA2AReply(m *models.Message, res *a2a.Result) {
	if m == nil || res == nil || strings.TrimSpace(m.Content) != "" {
		return
	}
	if text := strings.TrimSpace(res.Text); text != "" {
		m.Content = res.Text
		appendSegmentText(m, "text", res.Text)
		return
	}
	if res.InputRequired {
		m.Content = "（远程 Agent 需要更多输入）"
		appendSegmentText(m, "text", m.Content)
	}
}

// runA2AChat runs one turn against a remote Agent2Agent agent. It follows the
// ACP path: text streams into the reply placeholder, and the task/context the
// agent returns is kept on the conversation for the next turn.
func (cw *ChatWindow) runA2AChat(ctx context.Context, conv *models.Conversation, connName string, idx int, text string) {
	if conv == nil || idx < 0 || idx >= len(conv.Messages) {
		return
	}
	replyID := conv.Messages[idx].ID

	conn, ok := cw.findA2AConn(connName)
	if !ok {
		cw.finishA2ATurn(ctx, conv, idx, replyID, nil, fmt.Errorf("未找到可用的 A2A 连接：%s", connName))
		return
	}
	cli, err := a2a.New(ctx, conn)
	if err != nil {
		cw.finishA2ATurn(ctx, conv, idx, replyID, nil, err)
		return
	}

	taskID := sdk.TaskID(conv.A2ATaskID)
	res, err := cli.Chat(ctx, a2a.ChatRequest{
		Text:      text,
		TaskID:    taskID,
		ContextID: conv.A2AContextID,
	}, func(chunk string) {
		cw.postMain(func() {
			m := &conv.Messages[idx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
			cw.invalidate()
		})
	})
	// A stopped turn should stop the remote task too, not only the local
	// stream. Cancellation is best effort: the task may already be terminal.
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
	cw.finishA2ATurn(ctx, conv, idx, replyID, &res, err)
}

// finishA2ATurn persists the reply and the task identity the next turn
// continues. It runs on the UI goroutine and tolerates res being nil when the
// turn failed before reaching the agent.
func (cw *ChatWindow) finishA2ATurn(ctx context.Context, conv *models.Conversation, idx int, replyID string, res *a2a.Result, err error) {
	cw.postMain(func() {
		if idx < 0 || idx >= len(conv.Messages) {
			cw.endStream(replyID)
			return
		}
		m := &conv.Messages[idx]
		if res != nil {
			applyA2AReply(m, res)
			applyA2AResult(conv, res)
		}
		if err != nil && ctx.Err() == nil {
			if strings.TrimSpace(m.Content) == "" {
				m.Content = "⚠️ " + err.Error()
			} else {
				m.Content += "\n\n⚠️ " + err.Error()
			}
		}
		if t := turnOf(conv, replyID); t != nil {
			t.Assistant = m.Content
			t.EndedAt = time.Now()
		}
		cw.endStream(replyID)
		conv.UpdatedAt = time.Now()
		_ = cw.convManager.SaveConversation(conv)
		cw.reloadConvs()
		cw.invalidate()
		cw.notifySessionComplete(conv, m.Content)
	})
}
