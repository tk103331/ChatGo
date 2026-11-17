package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"

	"chatgo/internal/a2a"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// webRunA2A runs one browser turn against a remote A2A agent. The server
// finalizes the reply and records errors; this method only streams text and
// keeps the task identity on the conversation.
func (cw *ChatWindow) webRunA2A(ctx context.Context, conv *models.Conversation, replyIdx int, req webui.RunRequest, live webui.Live) error {
	name := strings.TrimPrefix(req.Agent, "a2a:")
	conn, ok := cw.findA2AConn(name)
	if !ok {
		return fmt.Errorf("未找到可用的 A2A 连接：%s", name)
	}
	live.Update(func(c *models.Conversation) {
		setA2AConversationAgent(c, req.Agent)
	})

	text := ""
	for i := replyIdx - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			text = conv.Messages[i].Content
			break
		}
	}

	cli, err := a2a.New(ctx, conn)
	if err != nil {
		return err
	}
	taskID := sdk.TaskID(conv.A2ATaskID)
	res, err := cli.Chat(ctx, a2a.ChatRequest{
		Text:      text,
		TaskID:    taskID,
		ContextID: conv.A2AContextID,
	}, func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
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
		m := &c.Messages[replyIdx]
		applyA2AReply(m, &res)
		applyA2AResult(c, &res)
	})
	return err
}
