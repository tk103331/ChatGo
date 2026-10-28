package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"chatgo/internal/config"
	"chatgo/internal/kb"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
)

// ensureRoomKBID returns the room's knowledge-base id, creating the base on
// first use and recording it in the configuration. It reads and writes the
// configuration, so it must run on the main thread; the id it returns is
// what the turn goroutines carry.
func (cw *ChatWindow) ensureRoomKBID(roomID string) string {
	r, ok := cw.cfg.FindRoom(roomID)
	if !ok {
		return ""
	}
	if r.KBID != "" {
		return r.KBID
	}
	db, err := kb.OpenDefault()
	if err != nil {
		return ""
	}
	defer db.Close()
	id, err := db.CreateKnowledgeBase("群聊·"+r.Name, "群聊消息自动归档")
	if err != nil {
		return ""
	}
	for i := range cw.cfg.Rooms {
		if cw.cfg.Rooms[i].ID == roomID {
			cw.cfg.Rooms[i].KBID = id
			break
		}
	}
	_ = config.SaveConfig(cw.cfg)
	return id
}

// roomArchiveMessage stores one spoken message in the room's knowledge base,
// so history outlives the context window. Each message is one document whose
// title carries the speaker and the time, which is what retrieval shows. It
// touches only the knowledge-base file, so any goroutine may call it with a
// kbID resolved on the main thread.
func (cw *ChatWindow) roomArchiveMessage(kbID string, msg models.Message) {
	body := strings.TrimSpace(roomFinalTextOf(msg))
	if kbID == "" || body == "" {
		return
	}
	db, err := kb.OpenDefault()
	if err != nil {
		return
	}
	defer db.Close()
	title := fmt.Sprintf("[%s] %s #%d", msg.Timestamp.Format("2006-01-02 15:04"), roomSpeakerName(msg), msg.Seq)
	_, _ = db.ImportText(kbID, title, body)
}

// roomHistorySearch retrieves archived room messages for a query, formatted
// for a prompt. Results carry their title, so the reader sees who said it
// and when - the recency a moderator weighs.
func (cw *ChatWindow) roomHistorySearch(kbID, query string, topK int) string {
	if kbID == "" {
		return ""
	}
	db, err := kb.OpenDefault()
	if err != nil {
		return ""
	}
	defer db.Close()
	if topK < 1 || topK > 20 {
		topK = 6
	}
	results, err := db.Search(kbID, query, topK)
	if err != nil || len(results) == 0 {
		return ""
	}
	var b strings.Builder
	for i, res := range results {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(res.Doc)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(res.Text))
		b.WriteString("\n")
	}
	return b.String()
}

// roomHistoryToolID is the room-history tool's name on a member's tool list.
const roomHistoryToolID = "room_history"

// roomHistoryToolInfo describes the tool to the model.
const roomHistoryToolDesc = "检索本群聊的历史消息归档。输入关键词，返回相关的历史发言（带说话人与时间）。当前上下文里看不到的更早讨论可以用它找回。"

// roomHistoryHandler executes a member's room_history call.
func (cw *ChatWindow) roomHistoryHandler(kbID string) func(context.Context, string) (string, error) {
	return func(_ context.Context, args string) (string, error) {
		var in struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
		if strings.TrimSpace(in.Query) == "" {
			return "", fmt.Errorf("缺少必填参数 query")
		}
		out := cw.roomHistorySearch(kbID, in.Query, in.TopK)
		if out == "" {
			return "没有找到相关的历史消息", nil
		}
		return out, nil
	}
}

// roomHistoryTool wraps the handler as a tool a member's ReAct client runs.
func (cw *ChatWindow) roomHistoryTool(kbID string) tool.BaseTool {
	return llm.ObserveTool(newBuiltinToolWrapper(llm.ToolDefinition{
		Name:        roomHistoryToolID,
		Description: roomHistoryToolDesc,
		Parameters: map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "检索关键词或问题", Required: true},
			"top_k": {Type: schema.Integer, Desc: "返回条数，默认 6，最多 20"},
		},
		Handler: cw.roomHistoryHandler(kbID),
	}))
}

// roomLastHumanText is the user's newest message, the query the moderator's
// history retrieval runs on.
func roomLastHumanText(conv *models.Conversation) string {
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			return strings.TrimSpace(conv.Messages[i].Content)
		}
	}
	return ""
}

// roomSendToolID is the in-room messaging tool's name.
const roomSendToolID = "room_send"

// roomSendToolDesc describes the tool to the model.
const roomSendToolDesc = "在群里发送一条消息。可以在内容里 @成员名 把任务交接给他；本轮你发言结束后，被 @ 的成员会接着发言。适合发布阶段性结论、附上文档（docs）内容、或把后续工作移交给更合适的成员。"

// roomSendHandler posts a member's message into the conversation and queues
// the members it @-mentions for the running turn. who is the sending member.
func (cw *ChatWindow) roomSendHandler(r config.Room, who config.RoomMember) func(context.Context, string) (string, error) {
	return func(_ context.Context, args string) (string, error) {
		var in struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
		body := strings.TrimSpace(in.Content)
		if body == "" {
			return "", fmt.Errorf("缺少必填参数 content")
		}
		mentioned := roomAtMentioned(r.ActiveMembers(), body)
		done := make(chan struct{})
		cw.postMain(func() {
			defer close(done)
			conv := cw.room.turnConv
			if conv == nil {
				return
			}
			msg := conv.NewAgentMessage(cw.newID(), who.Agent, roomMemberName(who), time.Now())
			msg.Content = body
			conv.Messages = append(conv.Messages, msg)
			for _, m := range mentioned {
				cw.room.handoff = append(cw.room.handoff, m.Agent)
			}
			go cw.roomArchiveMessage(r.KBID, msg)
			cw.room.list.ScrollToEnd()
			cw.invalidate()
		})
		<-done
		if len(mentioned) > 0 {
			names := make([]string, 0, len(mentioned))
			for _, m := range mentioned {
				names = append(names, roomMemberName(m))
			}
			return "消息已发送，已交接给：" + strings.Join(names, "、"), nil
		}
		return "消息已发送", nil
	}
}

// roomSendTool wraps the handler for a member's ReAct client.
func (cw *ChatWindow) roomSendTool(r config.Room, who config.RoomMember) tool.BaseTool {
	return llm.ObserveTool(newBuiltinToolWrapper(llm.ToolDefinition{
		Name:        roomSendToolID,
		Description: roomSendToolDesc,
		Parameters: map[string]*schema.ParameterInfo{
			"content": {Type: schema.String, Desc: "消息内容，可包含 @成员名 交接任务，可附文档内容", Required: true},
		},
		Handler: cw.roomSendHandler(r, who),
	}))
}
