package channel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// WeComBaseURL is the intelligent robot's long-connection endpoint.
const WeComBaseURL = "wss://openws.work.weixin.qq.com"

// The robot protocol's own commands. It is not a standard event stream: the
// client authenticates with a subscribe frame, keeps the socket alive with a
// ping, receives messages as callbacks and answers with respond frames.
const (
	wecomCmdSubscribe = "aibot_subscribe"
	wecomCmdCallback  = "aibot_msg_callback"
	wecomCmdEvent     = "aibot_event_callback"
	wecomCmdRespond   = "aibot_respond_msg"
	wecomCmdPing      = "ping"
)

// wecomHeartbeat is how often the socket is pinged; the platform expects one
// about every 30 seconds and drops a silent connection.
const wecomHeartbeat = 30 * time.Second

// WeCom connects through the intelligent robot's long-connection protocol. The
// platform publishes only Node.js and Python SDKs, so the frames are spoken
// directly here. A robot holds one connection at a time: a new subscribe
// displaces the old socket.
type WeCom struct {
	baseURL string
	state   atomic.Int32
	note    atomic.Value

	mu      sync.Mutex
	conn    *websocket.Conn
	writeMu sync.Mutex
	seq     atomic.Uint64

	started atomic.Bool
}

// NewWeCom builds an unconfigured WeCom driver.
func NewWeCom() *WeCom { return &WeCom{} }

// Type implements Driver.
func (w *WeCom) Type() string { return "wecom" }

// Status implements StatusReporter.
func (w *WeCom) Status() (State, string) {
	note, _ := w.note.Load().(string)
	return State(w.state.Load()), note
}

func (w *WeCom) setState(st State, note string) {
	w.state.Store(int32(st))
	w.note.Store(note)
}

// wecomFrame is the envelope every frame shares.
type wecomFrame struct {
	Cmd     string            `json:"cmd,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	ErrCode *int              `json:"errcode,omitempty"`
	ErrMsg  string            `json:"errmsg,omitempty"`
}

// wecomInbound is the body of a message callback.
type wecomInbound struct {
	MsgID    string `json:"msgid"`
	BotID    string `json:"aibotid"`
	ChatID   string `json:"chatid"`
	ChatType string `json:"chattype"`
	MsgType  string `json:"msgtype"`
	From     struct {
		UserID string `json:"userid"`
	} `json:"from"`
	Text struct {
		Content string `json:"content"`
	} `json:"text"`
	Mixed json.RawMessage `json:"mixed"`
	Voice struct {
		Content string `json:"content"`
	} `json:"voice"`
}

// Start implements Driver. AppID is the robot's BotID and AppSecret its
// long-connection secret.
func (w *WeCom) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	botID := strings.TrimSpace(cfg.AppID)
	secret := strings.TrimSpace(cfg.AppSecret)
	if botID == "" || secret == "" {
		w.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: 企业微信需要 BotID 与 Secret（在智能机器人配置页开启长连接后获取）", ErrNotConfigured)
	}
	if w.started.Swap(true) {
		return fmt.Errorf("channel: wecom driver already started")
	}
	w.baseURL = strings.TrimSpace(cfg.BaseURL)
	if w.baseURL == "" {
		w.baseURL = WeComBaseURL
	}
	w.setState(StateConnecting, "")

	go w.run(ctx, botID, secret, onMessage)
	return nil
}

// run keeps a connection up until ctx is cancelled, reconnecting with
// exponential backoff.
func (w *WeCom) run(ctx context.Context, botID, secret string, onMessage func(Inbound)) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			w.setState(StateStopped, "")
			return
		}
		startedAt := time.Now()
		_ = w.connect(ctx, botID, secret, onMessage)
		if ctx.Err() != nil {
			w.setState(StateStopped, "")
			return
		}
		// A connection that lived long enough was healthy; start the backoff
		// over rather than inheriting an old, long delay.
		if time.Since(startedAt) > 2*wecomHeartbeat {
			backoff = time.Second
		}
		w.setState(StateReconnecting, "")
		select {
		case <-ctx.Done():
			w.setState(StateStopped, "")
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// connect dials, subscribes and reads frames until the socket fails.
func (w *WeCom) connect(ctx context.Context, botID, secret string, onMessage func(Inbound)) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, w.baseURL, nil)
	if err != nil {
		w.setState(StateError, DetailError(err))
		return err
	}
	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.conn = nil
		w.mu.Unlock()
		conn.Close()
	}()

	if err := w.subscribe(ctx, botID, secret); err != nil {
		w.setState(StateError, DetailError(err))
		return err
	}
	w.setState(StateConnected, "")

	heartCtx, heartCancel := context.WithCancel(ctx)
	defer heartCancel()
	go w.heartbeat(heartCtx)

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var frame wecomFrame
		if err := json.Unmarshal(raw, &frame); err != nil {
			continue
		}
		switch frame.Cmd {
		case wecomCmdCallback:
			w.handleMessage(frame, onMessage)
		case wecomCmdEvent:
			// Enter-chat and card events need no action for a plain bot; the
			// frame is acknowledged by simply being read.
		}
	}
}

// subscribe authenticates the socket, which the platform requires before it
// delivers anything.
func (w *WeCom) subscribe(ctx context.Context, botID, secret string) error {
	reqID := w.nextReqID("subscribe")
	frame := map[string]any{
		"cmd":     wecomCmdSubscribe,
		"headers": map[string]string{"req_id": reqID},
		"body":    map[string]string{"bot_id": botID, "secret": secret},
	}
	if err := w.writeJSON(frame); err != nil {
		return fmt.Errorf("channel: 企业微信订阅失败: %w", err)
	}
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("channel: 企业微信连接已关闭")
	}
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var resp wecomFrame
	if err := conn.ReadJSON(&resp); err != nil {
		return fmt.Errorf("channel: 企业微信订阅无响应: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if resp.ErrCode != nil && *resp.ErrCode != 0 {
		return fmt.Errorf("channel: 企业微信订阅被拒绝: %d %s", *resp.ErrCode, resp.ErrMsg)
	}
	return nil
}

// heartbeat pings the platform so it keeps the socket open.
func (w *WeCom) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(wecomHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = w.writeJSON(map[string]any{
				"cmd":     wecomCmdPing,
				"headers": map[string]string{"req_id": w.nextReqID("ping")},
			})
		}
	}
}

// handleMessage turns a callback frame into an Inbound and hands it over.
func (w *WeCom) handleMessage(frame wecomFrame, onMessage func(Inbound)) {
	var body wecomInbound
	if err := json.Unmarshal(frame.Body, &body); err != nil {
		return
	}
	text := strings.TrimSpace(body.Text.Content)
	if text == "" {
		// A voice message carries its transcription instead of text.
		text = strings.TrimSpace(body.Voice.Content)
	}
	if text == "" {
		return
	}
	text = strings.TrimSpace(stripDingTalkMention(text))
	if text == "" {
		return
	}
	chatID := body.ChatID
	if chatID == "" {
		chatID = body.From.UserID
	}
	onMessage(Inbound{
		Text: text, ChatID: chatID, FromUser: body.From.UserID,
		ReplyToken: frame.Headers["req_id"],
	})
}

// Send implements Driver, replying in the conversation the message came from.
func (w *WeCom) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: 企业微信缺少会话 ID")
	}
	return w.sendMarkdown(target, markdown)
}

// SendReply answers the callback whose req_id is given. The platform requires
// a reply to quote the request it belongs to.
func (w *WeCom) SendReply(ctx context.Context, chatID, reqID, markdown string) error {
	return w.sendMarkdownWithReq(chatID, reqID, markdown)
}

func (w *WeCom) sendMarkdown(chatID, markdown string) error {
	return w.sendMarkdownWithReq(chatID, "", markdown)
}

func (w *WeCom) sendMarkdownWithReq(chatID, reqID, markdown string) error {
	if reqID == "" {
		reqID = w.nextReqID("send")
	}
	frame := map[string]any{
		"cmd":     wecomCmdRespond,
		"headers": map[string]string{"req_id": reqID},
		"body": map[string]any{
			"msgtype":  "markdown",
			"markdown": map[string]string{"content": markdown},
		},
	}
	if err := w.writeJSON(frame); err != nil {
		return err
	}
	return nil
}

// writeJSON serializes frame writes: gorilla allows one concurrent writer.
func (w *WeCom) writeJSON(v any) error {
	w.mu.Lock()
	conn := w.conn
	w.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("channel: 企业微信尚未连接")
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return conn.WriteJSON(v)
}

// nextReqID makes a request id unique within the connection.
func (w *WeCom) nextReqID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), w.seq.Add(1))
}

var (
	_ Driver         = (*WeCom)(nil)
	_ StatusReporter = (*WeCom)(nil)
)

func init() { RegisterDriver("wecom", func() Driver { return NewWeCom() }) }
