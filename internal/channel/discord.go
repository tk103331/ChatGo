package channel

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/bwmarrin/discordgo"
)

// Discord connects to the Gateway over an outbound WebSocket. Reading message
// bodies needs the privileged MESSAGE_CONTENT intent, which must be enabled in
// the developer portal first; without it Discord delivers empty content and
// raises no error, so the driver says so instead of staying silent.
type Discord struct {
	state atomic.Int32
	note  atomic.Value
	sess  *discordgo.Session

	started atomic.Bool
	botID   string
}

// NewDiscord builds an unconfigured Discord driver.
func NewDiscord() *Discord { return &Discord{} }

// Type implements Driver.
func (d *Discord) Type() string { return "discord" }

// Status implements StatusReporter.
func (d *Discord) Status() (State, string) {
	note, _ := d.note.Load().(string)
	return State(d.state.Load()), note
}

func (d *Discord) setState(st State, note string) {
	d.state.Store(int32(st))
	d.note.Store(note)
}

// Start implements Driver. AppID is the bot token from the developer portal.
func (d *Discord) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	token := strings.TrimSpace(cfg.AppID)
	if token == "" {
		token = strings.TrimSpace(cfg.AppSecret)
	}
	if token == "" {
		d.setState(StateError, "凭证未配置")
		return fmt.Errorf("%w: Discord 需要 Bot Token", ErrNotConfigured)
	}
	if d.started.Swap(true) {
		return fmt.Errorf("channel: discord driver already started")
	}
	sess, err := discordgo.New("Bot " + token)
	if err != nil {
		d.setState(StateError, DetailError(err))
		return err
	}
	d.sess = sess
	// Guild messages need their own intent; direct messages do not, and both
	// are asked for so the bot works in a server and in a DM.
	sess.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	sess.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		d.botID = ready.User.ID
		d.setState(StateConnected, DetailConnected(ready.User.Username))
	})
	sess.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.Bot {
			return
		}
		text := strings.TrimSpace(m.Content)
		if text == "" {
			// Empty content on a message the bot can see is what Discord
			// sends when MESSAGE_CONTENT is not enabled for the app.
			d.setState(StateError, "收不到消息正文，请在开发者后台开启 MESSAGE_CONTENT 特权 intent")
			return
		}
		if !d.mentionsBot(m) {
			return
		}
		text = stripDiscordMention(text, d.botID)
		if text == "" {
			return
		}
		onMessage(Inbound{
			Text: text, ChatID: m.ChannelID, FromUser: m.Author.ID,
			ReplyToken: m.ID,
		})
	})
	sess.AddHandler(func(_ *discordgo.Session, _ *discordgo.Disconnect) {
		d.setState(StateReconnecting, "")
	})

	if err := sess.Open(); err != nil {
		d.setState(StateError, DetailError(err))
		return err
	}
	go func() {
		<-ctx.Done()
		_ = sess.Close()
		d.setState(StateStopped, "")
	}()
	return nil
}

// mentionsBot reports whether a guild message addresses the bot. A direct
// message always does, so it needs no mention.
func (d *Discord) mentionsBot(m *discordgo.MessageCreate) bool {
	if m.GuildID == "" {
		return true
	}
	for _, u := range m.Mentions {
		if u != nil && u.ID == d.botID {
			return true
		}
	}
	return false
}

// Send implements Driver.
func (d *Discord) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: Discord 缺少频道 ID")
	}
	if d.sess == nil {
		return fmt.Errorf("channel: Discord 尚未连接")
	}
	// Discord caps a message at 2000 characters and rejects the whole send
	// past that, so a long reply is split at paragraph boundaries.
	for _, part := range splitMessage(markdown, 1900) {
		if _, err := d.sess.ChannelMessageSend(target, part); err != nil {
			return err
		}
	}
	return nil
}

// stripDiscordMention removes the "<@123>" mention Discord puts in the text.
func stripDiscordMention(text, botID string) string {
	if botID == "" {
		return text
	}
	text = strings.ReplaceAll(text, "<@"+botID+">", "")
	return strings.ReplaceAll(text, "<@!"+botID+">", "")
}

// splitMessage breaks text into chunks no longer than limit, preferring a
// paragraph break, then a line break, then a space.
func splitMessage(text string, limit int) []string {
	var parts []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit], "\n\n")
		if cut <= 0 {
			cut = strings.LastIndex(text[:limit], "\n")
		}
		if cut <= 0 {
			cut = strings.LastIndex(text[:limit], " ")
		}
		if cut <= 0 {
			cut = limit
		}
		parts = append(parts, strings.TrimRight(text[:cut], "\n "))
		text = strings.TrimLeft(text[cut:], "\n ")
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}

var (
	_ Driver         = (*Discord)(nil)
	_ StatusReporter = (*Discord)(nil)
)

func init() { RegisterDriver("discord", func() Driver { return NewDiscord() }) }

// discordReactionEmoji names the emoji a state maps to, as the Unicode
// character Discord expects.
func discordReactionEmoji(r Reaction) string {
	switch r {
	case ReactionWorking:
		return "👀"
	case ReactionDone:
		return "✅"
	}
	return ""
}

// React implements Reactor. The reply token carries the message id, which is
// what a reaction is addressed to.
func (d *Discord) React(ctx context.Context, in Inbound, r Reaction) error {
	emoji := discordReactionEmoji(r)
	if emoji == "" || in.ChatID == "" || in.ReplyToken == "" || d.sess == nil {
		return nil
	}
	return d.sess.MessageReactionAdd(in.ChatID, in.ReplyToken, emoji)
}

var _ Reactor = (*Discord)(nil)
