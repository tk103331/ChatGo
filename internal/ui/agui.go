package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/agui"
	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// findAGUIConn looks up the remote service a conversation's "agui:Name"
// names.
func (cw *ChatWindow) findAGUIConn(name string) (config.AGUIConnection, bool) {
	for _, a := range cw.cfg.AGUIAgents {
		if a.Name == name {
			return a, !a.Disabled
		}
	}
	return config.AGUIConnection{}, false
}

// runAGUIChat runs one turn against a remote AG-UI service. Text streams into
// the reply placeholder and the thread id is kept for the next turn.
func (cw *ChatWindow) runAGUIChat(ctx context.Context, conv *models.Conversation, connName string, idx int, text string) {
	if conv == nil || idx < 0 || idx >= len(conv.Messages) {
		return
	}
	replyID := conv.Messages[idx].ID
	conn, ok := cw.findAGUIConn(connName)
	if !ok {
		cw.finishAGUITurn(ctx, conv, idx, replyID, nil, fmt.Errorf("未找到可用的 AG-UI 连接：%s", connName))
		return
	}
	cli, err := agui.New(conn)
	if err != nil {
		cw.finishAGUITurn(ctx, conv, idx, replyID, nil, err)
		return
	}
	res, err := cli.Chat(ctx, agui.ChatRequest{
		ThreadID: conv.AGUIThreadID,
		Text:     text,
	}, func(chunk string) {
		cw.postMain(func() {
			if idx < 0 || idx >= len(conv.Messages) {
				return
			}
			m := &conv.Messages[idx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
			cw.invalidate()
		})
	})
	cw.finishAGUITurn(ctx, conv, idx, replyID, &res, err)
}

// finishAGUITurn persists the reply and the thread the next turn continues.
func (cw *ChatWindow) finishAGUITurn(ctx context.Context, conv *models.Conversation, idx int, replyID string, res *agui.Result, err error) {
	cw.postMain(func() {
		if idx < 0 || idx >= len(conv.Messages) {
			cw.endStream(replyID)
			return
		}
		m := &conv.Messages[idx]
		if res != nil {
			if res.ThreadID != "" {
				conv.AGUIThreadID = res.ThreadID
			}
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(res.Text) != "" {
				m.Content = res.Text
				appendSegmentText(m, "text", res.Text)
			}
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
