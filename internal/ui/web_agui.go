package ui

import (
	"context"
	"fmt"
	"strings"

	"chatgo/internal/agui"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// webRunAGUI runs one turn through a remote AG-UI service for the web and
// phone APIs.
func (cw *ChatWindow) webRunAGUI(ctx context.Context, conv *models.Conversation, replyIdx int, req webui.RunRequest, live webui.Live) error {
	name := strings.TrimPrefix(req.Agent, "agui:")
	conn, ok := cw.findAGUIConn(name)
	if !ok {
		return fmt.Errorf("未找到可用的 AG-UI 连接：%s", name)
	}
	live.Update(func(c *models.Conversation) {
		if c.Agent != req.Agent {
			c.Agent = req.Agent
			c.AGUIThreadID = ""
		}
	})

	text := ""
	for i := replyIdx - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			text = conv.Messages[i].Content
			break
		}
	}
	cli, err := agui.New(conn)
	if err != nil {
		return err
	}
	res, err := cli.Chat(ctx, agui.ChatRequest{ThreadID: conv.AGUIThreadID, Text: text}, func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
		})
	})
	live.Update(func(c *models.Conversation) {
		if res.ThreadID != "" {
			c.AGUIThreadID = res.ThreadID
		}
		m := &c.Messages[replyIdx]
		if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(res.Text) != "" {
			m.Content = res.Text
			appendSegmentText(m, "text", res.Text)
		}
	})
	return err
}
