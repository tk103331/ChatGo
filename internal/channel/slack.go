package channel

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Slack connects through Socket Mode, an outbound WebSocket that needs no
// request URL. It requires two tokens: a bot token (xoxb-) for Web API calls
// and an app-level token (xapp-) carrying connections:write to open the socket.
type Slack struct {
	state atomic.Int32
	note  atomic.Value
	api   *slack.Client

	started atomic.Bool
	botID   string
}

// NewSlack builds an unconfigured Slack driver.
func NewSlack() *Slack { return &Slack{} }

// Type implements Driver.
func (s *Slack) Type() string { return "slack" }

// Status implements StatusReporter.
func (s *Slack) Status() (State, string) {
	note, _ := s.note.Load().(string)
	return State(s.state.Load()), note
}

func (s *Slack) setState(st State, note string) {
	s.state.Store(int32(st))
	s.note.Store(note)
}

// Start implements Driver. AppID is the bot token, AppSecret the app-level
// token; the optional "app_token" option is accepted as an alias so a user who
// pastes them in either order still connects.
func (s *Slack) Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error {
	botToken := strings.TrimSpace(cfg.AppID)
	appToken := strings.TrimSpace(cfg.AppSecret)
	if alias := cfg.Option("app_token"); alias != "" {
		appToken = alias
	}
	if strings.HasPrefix(botToken, "xapp-") && strings.HasPrefix(appToken, "xoxb-") {
		botToken, appToken = appToken, botToken
	}
	if botToken == "" || appToken == "" {
		s.setState(StateError, "需要 Bot Token 与 App Token")
		return fmt.Errorf("%w: Slack 需要 Bot Token (xoxb-) 与 App Token (xapp-)", ErrNotConfigured)
	}
	if !strings.HasPrefix(botToken, "xoxb-") {
		s.setState(StateError, "Bot Token 应以 xoxb- 开头")
		return fmt.Errorf("%w: Bot Token 应以 xoxb- 开头", ErrNotConfigured)
	}
	if !strings.HasPrefix(appToken, "xapp-") {
		s.setState(StateError, "App Token 应以 xapp- 开头")
		return fmt.Errorf("%w: App Token 应以 xapp- 开头", ErrNotConfigured)
	}
	if s.started.Swap(true) {
		return fmt.Errorf("channel: slack driver already started")
	}

	s.api = slack.New(botToken, slack.OptionAppLevelToken(appToken))
	auth, err := s.api.AuthTestContext(ctx)
	if err != nil {
		s.setState(StateError, DetailError(err))
		return err
	}
	s.botID = auth.UserID
	// The bot's own name is User, not Team: Team is the workspace, and showing
	// it as the bot would label the connection with the wrong identity.
	s.setState(StateConnected, DetailConnected(auth.User))

	sock := socketmode.New(s.api)
	handler := socketmode.NewSocketmodeHandler(sock)
	handler.Handle(socketmode.EventTypeConnecting, func(*socketmode.Event, *socketmode.Client) {
		s.setState(StateConnecting, "")
	})
	handler.Handle(socketmode.EventTypeConnected, func(*socketmode.Event, *socketmode.Client) {
		s.setState(StateConnected, DetailConnected(auth.User))
	})
	handler.Handle(socketmode.EventTypeConnectionError, func(*socketmode.Event, *socketmode.Client) {
		s.setState(StateReconnecting, "")
	})
	handler.HandleEvents(slackevents.AppMention, func(evt *socketmode.Event, _ *socketmode.Client) {
		raw, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		msg, ok := raw.InnerEvent.Data.(*slackevents.AppMentionEvent)
		if !ok {
			return
		}
		text := strings.TrimSpace(stripSlackMention(msg.Text, s.botID))
		if text == "" {
			return
		}
		onMessage(Inbound{Text: text, ChatID: msg.Channel, FromUser: msg.User, ReplyToken: msg.TimeStamp})
	})
	handler.HandleEvents(slackevents.Message, func(evt *socketmode.Event, _ *socketmode.Client) {
		raw, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		msg, ok := raw.InnerEvent.Data.(*slackevents.MessageEvent)
		if !ok {
			return
		}
		// A direct message arrives as a message event; channel traffic is
		// handled by the app_mention route so the bot does not answer every
		// line of a conversation it can see.
		if msg.ChannelType != "im" {
			return
		}
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			return
		}
		onMessage(Inbound{
			Text: text, ChatID: msg.Channel, FromUser: msg.User,
			ReplyToken: msg.TimeStamp, FromBot: msg.BotID != "",
		})
	})

	go func() {
		if err := handler.RunEventLoopContext(ctx); err != nil && ctx.Err() == nil {
			s.setState(StateError, DetailError(err))
			return
		}
		s.setState(StateStopped, "")
	}()
	return nil
}

// Send implements Driver.
func (s *Slack) Send(ctx context.Context, target, markdown string) error {
	if target == "" {
		return fmt.Errorf("channel: Slack 缺少会话 ID")
	}
	_, _, err := s.api.PostMessageContext(ctx, target, slack.MsgOptionText(markdown, false))
	return err
}

// stripSlackMention removes the "<@U123>" prefix Slack puts in the text of a
// mention, leaving the prompt.
func stripSlackMention(text, botID string) string {
	if botID == "" {
		return text
	}
	return strings.ReplaceAll(text, "<@"+botID+">", "")
}

var (
	_ Driver         = (*Slack)(nil)
	_ StatusReporter = (*Slack)(nil)
)

func init() { RegisterDriver("slack", func() Driver { return NewSlack() }) }

// slackReactionEmoji names the emoji a state maps to, using Slack's own
// reaction names.
func slackReactionEmoji(r Reaction) string {
	switch r {
	case ReactionWorking:
		return "eyes"
	case ReactionDone:
		return "white_check_mark"
	}
	return ""
}

// React implements Reactor. Slack reports a reaction the bot already added as
// an error, which is ignored: the point is that the message carries the mark,
// not who added it last.
func (s *Slack) React(ctx context.Context, in Inbound, r Reaction) error {
	emoji := slackReactionEmoji(r)
	if emoji == "" || in.ChatID == "" || in.ReplyToken == "" || s.api == nil {
		return nil
	}
	err := s.api.AddReactionContext(ctx, emoji, slack.ItemRef{
		Channel: in.ChatID, Timestamp: in.ReplyToken,
	})
	if err != nil && strings.Contains(err.Error(), "already_reacted") {
		return nil
	}
	return err
}

var _ Reactor = (*Slack)(nil)
