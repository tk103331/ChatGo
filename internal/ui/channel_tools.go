package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"chatgo/internal/channel"
	"chatgo/internal/config"
	"chatgo/internal/llm"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func channelToolParams(toolType string) map[string]*schema.ParameterInfo {
	switch toolType {
	case "feishu_send_message":
		return map[string]*schema.ParameterInfo{
			"text": {Type: schema.String, Desc: "消息文本", Required: true},
		}
	case "feishu_send_card":
		return map[string]*schema.ParameterInfo{
			"card": {Type: schema.String, Desc: "飞书交互卡片 JSON", Required: true},
		}
	case "wechat_send_message", "telegram_send_message", "dingtalk_send_message",
		"slack_send_message", "discord_send_message", "wecom_send_message":
		return map[string]*schema.ParameterInfo{
			"text": {Type: schema.String, Desc: "消息文本", Required: true},
		}
	}
	return map[string]*schema.ParameterInfo{}
}

type channelMsgContext struct {
	ChatID       string
	FromUser     string
	ContextToken string
}

func (cw *ChatWindow) channelToolHandler(ch *config.Channel, t *config.ChannelTool, msgCtx *channelMsgContext) func(context.Context, string) (string, error) {
	switch t.Type {
	case "feishu_send_message":
		return func(_ context.Context, args string) (string, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			chatID := strings.TrimSpace(t.Config["default_chat_id"])
			if msgCtx != nil {
				chatID = strings.TrimSpace(msgCtx.ChatID)
			}
			if chatID == "" {
				return "", fmt.Errorf("缺少会话 ID（当前消息未携带且未配置 default_chat_id）")
			}
			if strings.TrimSpace(in.Text) == "" {
				return "", fmt.Errorf("缺少必填参数 text")
			}
			if err := feishuSendMsg(ch.AppID, ch.AppSecret, chatID, "text", fmt.Sprintf(`{"text":%s}`, mustJSON(in.Text))); err != nil {
				return "", err
			}
			return "已发送", nil
		}
	case "feishu_send_card":
		return func(_ context.Context, args string) (string, error) {
			var in struct {
				Card string `json:"card"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			chatID := strings.TrimSpace(t.Config["default_chat_id"])
			if msgCtx != nil {
				chatID = strings.TrimSpace(msgCtx.ChatID)
			}
			if chatID == "" {
				return "", fmt.Errorf("缺少会话 ID（当前消息未携带且未配置 default_chat_id）")
			}
			if strings.TrimSpace(in.Card) == "" {
				return "", fmt.Errorf("缺少必填参数 card")
			}
			if err := feishuSendMsg(ch.AppID, ch.AppSecret, chatID, "interactive", in.Card); err != nil {
				return "", err
			}
			return "已发送卡片", nil
		}
	case "wechat_send_message":
		return func(_ context.Context, args string) (string, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			toUser := strings.TrimSpace(t.Config["default_to_user"])
			contextToken := ""
			if msgCtx != nil {
				toUser = strings.TrimSpace(msgCtx.FromUser)
				contextToken = msgCtx.ContextToken
			}
			if toUser == "" {
				return "", fmt.Errorf("缺少目标用户（当前消息未携带且未配置 default_to_user）")
			}
			if strings.TrimSpace(in.Text) == "" {
				return "", fmt.Errorf("缺少必填参数 text")
			}
			baseURL := ch.BaseURL
			if baseURL == "" {
				baseURL = channel.BaseURL
			}
			client := channel.NewWeChatClient(baseURL, ch.AppSecret, ch.AppID)
			if err := client.SendText(context.Background(), toUser, in.Text, contextToken); err != nil {
				return "", err
			}
			return "已发送", nil
		}
	case "telegram_send_message", "dingtalk_send_message", "slack_send_message",
		"discord_send_message", "wecom_send_message":
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if strings.TrimSpace(in.Text) == "" {
				return "", fmt.Errorf("缺少必填参数 text")
			}
			target := strings.TrimSpace(t.Config["default_chat_id"])
			if msgCtx != nil && msgCtx.ChatID != "" {
				target = strings.TrimSpace(msgCtx.ChatID)
			}
			if target == "" {
				return "", fmt.Errorf("缺少会话 ID（当前消息未携带且未配置 default_chat_id）")
			}
			return cw.sendViaChannel(ctx, ch, t.Type, target, msgCtx, in.Text)
		}
	}
	return func(context.Context, string) (string, error) {
		return "渠道工具 " + t.Type + " 暂未实现", nil
	}
}

// sendViaChannel sends text through the running driver of the channel the
// message came from, so a tool never needs the platform's credentials twice.
func (cw *ChatWindow) sendViaChannel(ctx context.Context, ch *config.Channel, toolType, target string, msgCtx *channelMsgContext, text string) (string, error) {
	driver := cw.channelDriver(ch.ID)
	if driver == nil {
		return "", fmt.Errorf("渠道 %s 未连接，无法发送", ch.Name)
	}
	// A platform that only accepts a reply quoting the inbound frame is
	// handled here, the same way the reply path does.
	if replyToken := replyTokenOf(msgCtx); replyToken != "" {
		if d, ok := driver.(interface {
			SendToWebhook(context.Context, string, string) error
		}); ok {
			if err := d.SendToWebhook(ctx, replyToken, text); err != nil {
				return "", err
			}
			return "已发送", nil
		}
		if d, ok := driver.(interface {
			SendReply(context.Context, string, string, string) error
		}); ok {
			if err := d.SendReply(ctx, target, replyToken, text); err != nil {
				return "", err
			}
			return "已发送", nil
		}
	}
	if err := driver.Send(ctx, target, text); err != nil {
		return "", err
	}
	return "已发送", nil
}

// replyTokenOf is the inbound frame token a reply must quote, when the driver
// recorded one.
func replyTokenOf(msgCtx *channelMsgContext) string {
	if msgCtx == nil {
		return ""
	}
	return strings.TrimSpace(msgCtx.ContextToken)
}

func feishuSendMsg(appID, appSecret, chatID, msgType, content string) error {
	client := lark.NewClient(appID, appSecret)
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build()
	resp, err := client.Im.V1.Message.Create(context.Background(), req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("飞书 API 错误: %d %s", resp.Code, resp.Msg)
	}
	return nil
}

func mustJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (cw *ChatWindow) buildChannelTools(ch *config.Channel, msgCtx *channelMsgContext) []tool.BaseTool {
	var tools []tool.BaseTool
	for i := range ch.Tools {
		t := &ch.Tools[i]
		if !t.Enable {
			continue
		}
		def := llm.ToolDefinition{
			Name:        t.Type,
			Description: config.GetChannelToolDescription(t.Type),
			Parameters:  channelToolParams(t.Type),
			Handler:     cw.channelToolHandler(ch, t, msgCtx),
		}
		tools = append(tools, llm.ObserveTool(newBuiltinToolWrapper(def)))
	}
	log.Printf("[channel-tools] channel=%s type=%s enabled=%d total=%d", ch.Name, ch.Type, len(tools), len(ch.Tools))
	return tools
}

func combineTools(base, extra []tool.BaseTool) []tool.BaseTool {
	if len(extra) == 0 {
		return base
	}
	combined := make([]tool.BaseTool, 0, len(base)+len(extra))
	combined = append(combined, base...)
	combined = append(combined, extra...)
	return combined
}
