package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// TelegramBaseURL is the Bot API host; tests point a driver elsewhere.
const TelegramBaseURL = "https://api.telegram.org"

// telegramPollWait is how long getUpdates holds the connection open. Telegram
// caps it at 50 seconds; staying under keeps the request from being cut by
// intermediaries.
const telegramPollWait = 45

// Telegram receives messages by long-polling getUpdates, which needs no public
// URL and no inbound connection, so a desktop app can host it as-is.
type Telegram struct {
	baseURL string
	token   string
	http    *http.Client

	// offset is the update_id to ask for next: Telegram repeats every update
	// at or past it, so advancing it is what acknowledges them.
	offset  atomic.Int64
	state   atomic.Int32
	note    atomic.Value
	started atomic.Bool
	// identity is the bot's @name, learned once at startup and repeated on
	// every connected state so the card keeps showing who answered.
	identity string
}

// NewTelegram builds an unconfigured Telegram driver.
func NewTelegram() *Telegram { return &Telegram{} }

// Type implements Driver.
func (t *Telegram) Type() string { return "telegram" }

// Status implements StatusReporter.
func (t *Telegram) Status() (State, string) {
	st := State(t.state.Load())
	note, _ := t.note.Load().(string)
	return st, note
}

func (t *Telegram) setState(st State, note string) {
	t.state.Store(int32(st))
	t.note.Store(note)
}

// Start implements Driver. The token is the bot's own, from @BotFather.
func (t *Telegram) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	token := strings.TrimSpace(cfg.AppID)
	if token == "" {
		token = strings.TrimSpace(cfg.AppSecret)
	}
	if token == "" {
		t.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: Telegram 需要 Bot Token", ErrNotConfigured)
	}
	t.token = token
	t.baseURL = strings.TrimSpace(cfg.BaseURL)
	if t.baseURL == "" {
		t.baseURL = TelegramBaseURL
	}
	if t.http == nil {
		t.http = &http.Client{Timeout: telegramPollWait*time.Second + 15*time.Second}
	}
	if t.started.Swap(true) {
		return fmt.Errorf("channel: telegram driver already started")
	}

	me, err := t.call(ctx, "getMe", nil)
	if err != nil {
		t.setState(StateError, DetailError(err))
		return err
	}
	var bot struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	}
	_ = json.Unmarshal(me, &bot)
	// The bot's name is a fact about the account, so it is kept on the driver:
	// the poll loop reports the connected state on every successful round and
	// would otherwise wipe the name off the card a few seconds after it
	// appeared.
	t.identity = DetailConnected(bot.Username)
	t.setState(StateConnected, t.identity)

	go t.poll(ctx, bot.ID, onMessage)
	return nil
}

// poll is the long-poll loop: ask for updates, hand each text message to the
// handler, then advance the offset past everything received.
func (t *Telegram) poll(ctx context.Context, botID int64, onMessage func(Inbound)) {
	for {
		if ctx.Err() != nil {
			t.setState(StateStopped, "")
			return
		}
		raw, err := t.call(ctx, "getUpdates", map[string]any{
			"offset":          t.offset.Load(),
			"timeout":         telegramPollWait,
			"allowed_updates": []string{"message"},
		})
		if err != nil {
			if ctx.Err() != nil {
				t.setState(StateStopped, "")
				return
			}
			t.setState(StateReconnecting, "")
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		t.setState(StateConnected, t.identity)

		var updates []struct {
			UpdateID int64 `json:"update_id"`
			Message  *struct {
				MessageID int64 `json:"message_id"`
				From      *struct {
					ID       int64  `json:"id"`
					Username string `json:"username"`
				} `json:"from"`
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				Text string `json:"text"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &updates); err != nil {
			t.setState(StateError, Detailf("无法解析更新: %v", err))
			continue
		}
		for _, u := range updates {
			t.offset.Store(u.UpdateID + 1)
			m := u.Message
			if m == nil {
				continue
			}
			text := strings.TrimSpace(m.Text)
			if text == "" {
				continue
			}
			in := Inbound{
				Text:       text,
				ChatID:     strconv.FormatInt(m.Chat.ID, 10),
				ReplyToken: strconv.FormatInt(m.MessageID, 10),
			}
			if m.From != nil {
				in.FromUser = strconv.FormatInt(m.From.ID, 10)
				// Telegram never delivers a bot's own messages to itself
				// through getUpdates, but the check keeps a forwarded copy
				// from looping back.
				in.FromBot = m.From.ID == botID
			}
			onMessage(in)
		}
	}
}

// Send implements Driver, replying in the chat the message came from.
func (t *Telegram) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: telegram 缺少会话 ID")
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return fmt.Errorf("channel: telegram 会话 ID 无效: %w", err)
	}
	// Markdown is converted to Telegram's own subset: it rejects the whole
	// message on a parse error, so the safest reply is plain text with the
	// emphasis markers Telegram understands.
	_, err = t.call(ctx, "sendMessage", map[string]any{
		"chat_id":    chatID,
		"text":       telegramMarkdown(markdown),
		"parse_mode": "MarkdownV2",
	})
	return err
}

// call performs one Bot API request and returns its result.
func (t *Telegram) call(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	// A driver used before Start — or a reaction sent from a fresh instance —
	// has no client yet; building one here keeps every call self-sufficient.
	if t.http == nil {
		t.http = &http.Client{Timeout: telegramPollWait*time.Second + 15*time.Second}
	}
	var body io.Reader
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	url := t.baseURL + "/bot" + t.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("channel: telegram 响应无法解析: %w", err)
	}
	if !envelope.OK {
		desc := envelope.Description
		if desc == "" {
			desc = "请求失败"
		}
		return nil, fmt.Errorf("channel: telegram %s: %s", method, desc)
	}
	return envelope.Result, nil
}

// telegramMarkdown converts the assistant's Markdown to Telegram's MarkdownV2.
// Telegram rejects an entire message on one malformed entity, so the
// conversion is deterministic: known constructs become entities and every
// other reserved character is escaped.
func telegramMarkdown(text string) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			// A fence is emitted verbatim; only backslash and backtick are
			// special inside one.
			b.WriteString("```")
			if inFence {
				inFence = false
			} else {
				inFence = true
				// The language tag follows the opening fence.
				b.WriteString(escapeV2Code(strings.TrimPrefix(trimmed, "```")))
			}
			if i < len(lines)-1 {
				b.WriteString("\n")
			}
			continue
		}
		if inFence {
			b.WriteString(escapeV2Code(line))
		} else {
			b.WriteString(telegramInline(line))
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// telegramInline converts one line outside a fence: headings lose their mark,
// then emphasis, inline code and links become MarkdownV2 entities while the
// rest is escaped.
func telegramInline(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	indent := line[:len(line)-len(trimmed)]
	if strings.HasPrefix(trimmed, "#") {
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		indent = ""
	}
	var b strings.Builder
	b.WriteString(indent)
	for i := 0; i < len(trimmed); {
		switch {
		case strings.HasPrefix(trimmed[i:], "```"):
			b.WriteString("```")
			i += 3
		case trimmed[i] == '`':
			end := strings.IndexByte(trimmed[i+1:], '`')
			if end < 0 {
				b.WriteString("\\`")
				i++
				continue
			}
			b.WriteString("`")
			b.WriteString(escapeV2Code(trimmed[i+1 : i+1+end]))
			b.WriteString("`")
			i += end + 2
		case strings.HasPrefix(trimmed[i:], "**") || strings.HasPrefix(trimmed[i:], "__"):
			marker := trimmed[i : i+2]
			end := strings.Index(trimmed[i+2:], marker)
			if end < 0 {
				b.WriteString(escapeV2(trimmed[i : i+1]))
				i++
				continue
			}
			b.WriteString("*")
			b.WriteString(escapeV2(trimmed[i+2 : i+2+end]))
			b.WriteString("*")
			i += end + 4
		case trimmed[i] == '*' || trimmed[i] == '_':
			marker := trimmed[i]
			end := standaloneMarker(trimmed[i+1:], marker)
			if end < 0 {
				b.WriteString(escapeV2(trimmed[i : i+1]))
				i++
				continue
			}
			b.WriteString("_")
			b.WriteString(escapeV2(trimmed[i+1 : i+1+end]))
			b.WriteString("_")
			i += end + 2
		case trimmed[i] == '[':
			close := strings.IndexByte(trimmed[i:], ']')
			if close < 0 || i+close+1 >= len(trimmed) || trimmed[i+close+1] != '(' {
				b.WriteString(escapeV2("["))
				i++
				continue
			}
			urlEnd := strings.IndexByte(trimmed[i+close+1:], ')')
			if urlEnd < 0 {
				b.WriteString(escapeV2("["))
				i++
				continue
			}
			label := trimmed[i+1 : i+close]
			url := trimmed[i+close+2 : i+close+1+urlEnd]
			b.WriteString("[")
			b.WriteString(escapeV2(label))
			b.WriteString("](")
			b.WriteString(escapeV2URL(url))
			b.WriteString(")")
			i += close + 1 + urlEnd + 1
		default:
			// A rune is copied whole: slicing one byte would cut a multi-byte
			// character in half and produce mojibake for any non-ASCII text.
			r, size := utf8.DecodeRuneInString(trimmed[i:])
			b.WriteString(escapeV2(string(r)))
			i += size
		}
	}
	return b.String()
}

// standaloneMarker finds the next marker that is not part of a doubled pair,
// so a lone asterisk in prose cannot pair with the first half of **bold**.
func standaloneMarker(s string, marker byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] != marker {
			continue
		}
		if marker == '*' || marker == '_' {
			if i+1 < len(s) && s[i+1] == marker {
				i++
				continue
			}
			if i > 0 && s[i-1] == marker {
				continue
			}
		}
		return i
	}
	return -1
}

// escapeV2 escapes the characters MarkdownV2 reserves in plain text.
func escapeV2(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("_*[]()~`>#+-=|{}.!\\", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeV2Code escapes only what is special inside a code span.
func escapeV2Code(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '`' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// escapeV2URL escapes the closing parenthesis inside a link target.
func escapeV2URL(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), ")", "\\)")
}

// Compile-time checks.
var (
	_ Driver         = (*Telegram)(nil)
	_ StatusReporter = (*Telegram)(nil)
)

func init() { RegisterDriver("telegram", func() Driver { return NewTelegram() }) }

// telegramReactionEmoji names the emoji a state maps to.
func telegramReactionEmoji(r Reaction) string {
	switch r {
	case ReactionWorking:
		return "👀"
	case ReactionDone:
		return "👍"
	}
	return ""
}

// React implements Reactor through setMessageReaction, which Bot API 7.0
// added. It is only available to bots on a paid Telegram Business account, so
// a failure is expected on a free bot and is left to the caller to ignore.
func (t *Telegram) React(ctx context.Context, in Inbound, r Reaction) error {
	emoji := telegramReactionEmoji(r)
	if emoji == "" || in.ChatID == "" || in.ReplyToken == "" || t.token == "" {
		return nil
	}
	chatID, err := strconv.ParseInt(in.ChatID, 10, 64)
	if err != nil {
		return err
	}
	messageID, err := strconv.ParseInt(in.ReplyToken, 10, 64)
	if err != nil {
		return err
	}
	_, err = t.call(ctx, "setMessageReaction", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	})
	return err
}

var _ Reactor = (*Telegram)(nil)
