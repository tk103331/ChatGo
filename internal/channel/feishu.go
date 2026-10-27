package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	"github.com/larksuite/oapi-sdk-go/v3/channel/types"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// Feishu connects over the Lark long connection, which needs no public URL.
type Feishu struct {
	state atomic.Int32
	note  atomic.Value

	api  *lark.Client
	send func(ctx context.Context, chatID, markdown string) error
	// identity is the app's bot name, learned once when the socket is ready.
	identity string

	started atomic.Bool
}

// NewFeishu builds an unconfigured Feishu driver.
func NewFeishu() *Feishu { return &Feishu{} }

// Type implements Driver.
func (f *Feishu) Type() string { return "feishu" }

// Status implements StatusReporter.
func (f *Feishu) Status() (State, string) {
	note, _ := f.note.Load().(string)
	return State(f.state.Load()), note
}

func (f *Feishu) setState(st State, note string) {
	f.state.Store(int32(st))
	f.note.Store(note)
}

// Start implements Driver. AppID and AppSecret are the Feishu app's own.
func (f *Feishu) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	appID := strings.TrimSpace(cfg.AppID)
	appSecret := strings.TrimSpace(cfg.AppSecret)
	if appID == "" || appSecret == "" {
		f.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: 飞书需要 AppID 与 AppSecret", ErrNotConfigured)
	}
	if f.started.Swap(true) {
		return fmt.Errorf("channel: feishu driver already started")
	}
	api := lark.NewClient(appID, appSecret)
	f.api = api
	f.send = func(ctx context.Context, chatID, markdown string) error {
		return feishuSend(ctx, api, chatID, markdown)
	}

	wsClient := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(dispatcher.NewEventDispatcher("", "")),
		larkws.WithAutoReconnect(true))
	ch := channel.NewChannel(api, wsClient)

	ch.OnMessage(func(_ context.Context, msg *types.NormalizedMessage) error {
		// A group message must address the bot; a direct one need not.
		if !msg.MentionedBot && msg.ChatType != "p2p" {
			return nil
		}
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			return nil
		}
		onMessage(Inbound{Text: text, ChatID: msg.ChatID, ReplyToken: msg.MessageID})
		return nil
	})

	ch.OnReady(func() {
		// The bot's own name is a fact about the app, so it is fetched once
		// and repeated on every connected state.
		if identity := ch.GetBotIdentity(ctx); identity != nil {
			f.identity = DetailConnected(identity.Name)
		}
		f.setState(StateConnected, f.identity)
	})
	ch.OnReconnecting(func() { f.setState(StateReconnecting, "") })
	ch.OnReconnected(func() { f.setState(StateConnected, f.identity) })
	ch.OnDisconnected(func() { f.setState(StateStopped, "") })
	ch.OnError(func(err error) { f.setState(StateError, DetailError(err)) })

	f.setState(StateConnecting, "")
	go func() {
		if err := ch.Start(ctx); err != nil && ctx.Err() == nil {
			f.setState(StateError, DetailError(err))
			return
		}
		f.setState(StateStopped, "")
	}()
	return nil
}

// Send implements Driver.
func (f *Feishu) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: 飞书缺少会话 ID")
	}
	if f.send == nil {
		return fmt.Errorf("channel: 飞书尚未连接")
	}
	return f.send(ctx, target, markdown)
}

// feishuSend posts a markdown message to a chat.
func feishuSend(ctx context.Context, api *lark.Client, chatID, markdown string) error {
	text, err := json.Marshal(markdown)
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`{"text":%s}`, text)
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType("text").
			Content(content).
			Build()).
		Build()
	resp, err := api.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("channel: 飞书 API 错误: %d %s", resp.Code, resp.Msg)
	}
	return nil
}

var (
	_ Driver         = (*Feishu)(nil)
	_ StatusReporter = (*Feishu)(nil)
)

func init() { RegisterDriver("feishu", func() Driver { return NewFeishu() }) }

// feishuReactionEmoji names the emoji a state maps to. Feishu's reaction API
// takes the emoji's own type name, not a Unicode character.
func feishuReactionEmoji(r Reaction) string {
	switch r {
	case ReactionWorking:
		return "Typing"
	case ReactionDone:
		return "DONE"
	}
	return ""
}

// React implements Reactor: the bot adds an emoji to the message it is
// answering, which is how a Feishu user sees the request was picked up before
// the reply lands.
func (f *Feishu) React(ctx context.Context, in Inbound, r Reaction) error {
	emoji := feishuReactionEmoji(r)
	if emoji == "" || in.ReplyToken == "" || f.api == nil {
		return nil
	}
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(in.ReplyToken).
		Body(larkim.NewCreateMessageReactionReqBodyBuilder().
			ReactionType(larkim.NewEmojiBuilder().EmojiType(emoji).Build()).
			Build()).
		Build()
	_, err := f.api.Im.V1.MessageReaction.Create(ctx, req)
	return err
}

var _ Reactor = (*Feishu)(nil)
