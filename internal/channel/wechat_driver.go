package channel

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// wechatIdentity is the fixed name the WeChat channel reports. The iLink bot
// API exposes no identity of its own — the login hands back a bot id and token
// and nothing else — so the card names the product the user is talking to
// rather than leaving the state bare.
const wechatIdentity = "微信ClawBot"

// WeChatDriver connects to the WeChat iLink bot API, which delivers messages
// by long polling.
type WeChatDriver struct {
	state atomic.Int32
	note  atomic.Value

	client  *WeChatClient
	replyTo atomic.Value // last inbound, for context-token replies

	started atomic.Bool
}

// NewWeChatDriver builds an unconfigured WeChat driver.
func NewWeChatDriver() *WeChatDriver { return &WeChatDriver{} }

// Type implements Driver.
func (w *WeChatDriver) Type() string { return "wechat" }

// Status implements StatusReporter.
func (w *WeChatDriver) Status() (State, string) {
	note, _ := w.note.Load().(string)
	return State(w.state.Load()), note
}

func (w *WeChatDriver) setState(st State, note string) {
	w.state.Store(int32(st))
	w.note.Store(note)
}

// Start implements Driver. AppID is the iLink bot id and AppSecret its token,
// both filled in by the QR login.
func (w *WeChatDriver) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	botID := strings.TrimSpace(cfg.AppID)
	token := strings.TrimSpace(cfg.AppSecret)
	if botID == "" || token == "" {
		w.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: 微信需要 Bot ID 与 Bot Token（扫码登录后自动填入）", ErrNotConfigured)
	}
	if w.started.Swap(true) {
		return fmt.Errorf("channel: wechat driver already started")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = BaseURL
	}
	w.client = NewWeChatClient(baseURL, token, botID)
	w.setState(StateConnecting, "")
	go w.poll(ctx, onMessage)
	return nil
}

// poll is the iLink long-poll loop.
func (w *WeChatDriver) poll(ctx context.Context, onMessage func(Inbound)) {
	buf := ""
	for {
		if ctx.Err() != nil {
			w.setState(StateStopped, "")
			return
		}
		resp, err := w.client.GetUpdates(ctx, buf)
		if err != nil {
			if ctx.Err() != nil {
				w.setState(StateStopped, "")
				return
			}
			w.setState(StateReconnecting, "")
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		w.setState(StateConnected, wechatIdentity)
		if resp.GetUpdatesBuf != "" {
			buf = resp.GetUpdatesBuf
		}
		for _, msg := range resp.Msgs {
			// Only a finished text message is answered; the API reports the
			// rest of a message's lifecycle on the same stream.
			if msg.MessageType != 1 || msg.MessageState != 2 {
				continue
			}
			text := ExtractText(msg)
			if text == "" {
				continue
			}
			w.replyTo.Store(msg.ContextToken)
			onMessage(Inbound{
				Text: text, ChatID: msg.FromUserID, FromUser: msg.FromUserID,
				ReplyToken: msg.ContextToken,
			})
		}
	}
}

// Send implements Driver. iLink wants the context token the inbound message
// carried, which is remembered for the last conversation.
func (w *WeChatDriver) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: 微信缺少用户 ID")
	}
	token, _ := w.replyTo.Load().(string)
	return w.client.SendText(ctx, target, markdown, token)
}

var (
	_ Driver         = (*WeChatDriver)(nil)
	_ StatusReporter = (*WeChatDriver)(nil)
)

func init() { RegisterDriver("wechat", func() Driver { return NewWeChatDriver() }) }
