package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
)

// DingTalk receives bot messages over Stream Mode: the SDK opens an outbound
// WebSocket, so no public URL is needed. Replies go to the session webhook the
// callback carries, which stays valid for about an hour and a half.
type DingTalk struct {
	state atomic.Int32
	note  atomic.Value
	http  *http.Client

	started atomic.Bool
	// stream is kept so a stop can close the socket the SDK holds open.
	stream *client.StreamClient
}

// NewDingTalk builds an unconfigured DingTalk driver.
func NewDingTalk() *DingTalk { return &DingTalk{} }

// Type implements Driver.
func (d *DingTalk) Type() string { return "dingtalk" }

// Status implements StatusReporter.
func (d *DingTalk) Status() (State, string) {
	note, _ := d.note.Load().(string)
	return State(d.state.Load()), note
}

func (d *DingTalk) setState(st State, note string) {
	d.state.Store(int32(st))
	d.note.Store(note)
}

// Start implements Driver. AppID is the app's Client ID (formerly AppKey) and
// AppSecret its Client Secret (formerly AppSecret).
func (d *DingTalk) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	clientID := strings.TrimSpace(cfg.AppID)
	clientSecret := strings.TrimSpace(cfg.AppSecret)
	if clientID == "" || clientSecret == "" {
		d.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: 钉钉需要 Client ID 与 Client Secret", ErrNotConfigured)
	}
	if d.started.Swap(true) {
		return fmt.Errorf("channel: dingtalk driver already started")
	}
	if d.http == nil {
		d.http = &http.Client{Timeout: 20 * time.Second}
	}
	d.setState(StateConnecting, "")

	stream := client.NewStreamClient(
		client.WithAppCredential(client.NewAppCredentialConfig(clientID, clientSecret)),
		// The SDK reconnects on its own after a drop; without this the channel
		// would go silent for good the first time the network blinks.
		client.WithAutoReconnect(true),
	)
	d.stream = stream
	stream.RegisterChatBotCallbackRouter(func(_ context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
		text := strings.TrimSpace(data.Text.Content)
		if text == "" {
			return []byte(""), nil
		}
		// A group message only reaches the bot when it is @-mentioned, but
		// the mention itself stays in the text; dropping it keeps the prompt
		// clean.
		text = stripDingTalkMention(text)
		if text == "" {
			return []byte(""), nil
		}
		onMessage(Inbound{
			Text:       text,
			ChatID:     data.ConversationId,
			FromUser:   data.SenderStaffId,
			ReplyToken: data.SessionWebhook,
		})
		return []byte(""), nil
	})

	// Start dials the socket and returns as soon as the connection is up; the
	// SDK then keeps reading on its own goroutine and reconnects after a drop.
	// A nil return is therefore success, not the end of the connection, which
	// is why the state is only reported from here.
	if err := stream.Start(ctx); err != nil {
		d.setState(StateError, DetailError(err))
		return err
	}
	d.setState(StateConnected, "")

	// The SDK's reader outlives Start and has no callback for a drop it cannot
	// recover from, so the socket is closed when the channel is stopped and
	// the state falls back to stopped then.
	go func() {
		<-ctx.Done()
		stream.Close()
		d.setState(StateStopped, "")
	}()
	return nil
}

// Send implements Driver. The target is the conversation id, but DingTalk only
// accepts a reply through the session webhook, so the webhook is passed as the
// reply token by the caller.
func (d *DingTalk) Send(ctx context.Context, target, markdown string) error {
	// Without the per-message webhook there is no way to address a DingTalk
	// conversation, so a reply outside a received message is refused rather
	// than silently dropped.
	return fmt.Errorf("channel: 钉钉只能在收到消息后回复（缺少 sessionWebhook）")
}

// SendToWebhook posts a markdown reply to a session webhook captured from an
// inbound message. It is what the channel's reply callback calls.
func (d *DingTalk) SendToWebhook(ctx context.Context, webhook, markdown string) error {
	if strings.TrimSpace(webhook) == "" {
		return fmt.Errorf("channel: 钉钉缺少 sessionWebhook")
	}
	// A driver used only for a reply has never started, so its client is nil;
	// the webhook needs no credentials and can be posted to directly.
	if d.http == nil {
		d.http = &http.Client{Timeout: 20 * time.Second}
	}
	body, err := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "ChatGo",
			"text":  markdown,
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("channel: 钉钉响应无法解析: %w", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("channel: 钉钉发送失败: %d %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// stripDingTalkMention removes the "@机器人" prefix DingTalk leaves in the
// text of a group message, keeping the rest intact.
func stripDingTalkMention(text string) string {
	t := strings.TrimSpace(text)
	for strings.HasPrefix(t, "@") {
		// The mention runs to the first space; a mention without a space is
		// the whole message and leaves nothing to answer.
		sp := strings.IndexAny(t, " \u3000")
		if sp < 0 {
			return ""
		}
		t = strings.TrimSpace(t[sp:])
	}
	return t
}

var (
	_ Driver         = (*DingTalk)(nil)
	_ StatusReporter = (*DingTalk)(nil)
)

func init() { RegisterDriver("dingtalk", func() Driver { return NewDingTalk() }) }
