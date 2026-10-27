package channel

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// State is a channel connection's lifecycle state, shared by every driver so
// the settings card can draw one indicator whatever the platform.
type State int

const (
	StateStopped State = iota
	StateConnecting
	StateConnected
	StateReconnecting
	StateError
)

// Inbound is one message a driver received, already flattened to what the
// agent needs: the text, who sent it, and whatever the driver must echo back
// to reply in the same conversation.
type Inbound struct {
	// Text is the message body, with any @-mention of the bot removed.
	Text string
	// ChatID names the conversation to reply in. A driver sets it to the
	// platform's own identifier; the UI never interprets it.
	ChatID string
	// FromUser is the sender, used when a channel tool must address the user
	// rather than the conversation.
	FromUser string
	// ReplyToken carries driver-private state a reply needs — a Feishu
	// context token, a DingTalk session webhook, a WeCom req_id.
	ReplyToken string
	// FromBot reports a message the bot itself sent, which drivers that
	// receive their own messages must drop.
	FromBot bool
}

// Config is the credential set a driver starts with. It is a copy of the
// channel's saved configuration, so a running driver never reads the mutable
// config while the user edits it.
type Config struct {
	// Type is the driver's own kind, e.g. "telegram".
	Type string
	// Name labels the channel in notifications and errors.
	Name string
	// AppID and AppSecret are the two primary credentials. What they mean is
	// the driver's business: a bot token, an app key, a client id.
	AppID     string
	AppSecret string
	// BaseURL overrides the driver's default endpoint.
	BaseURL string
	// Options holds platform-specific fields that do not fit the pair above,
	// such as Slack's app-level token or Discord's guild allow-list.
	Options map[string]string
}

// Option reads a platform-specific field, trimmed.
func (c Config) Option(key string) string {
	if c.Options == nil {
		return ""
	}
	return strings.TrimSpace(c.Options[key])
}

// Driver is one platform's connection. A driver owns its transport, receives
// messages on its own goroutines and reports them through the handler it was
// started with; the UI never speaks a platform protocol directly.
type Driver interface {
	// Type is the identifier the configuration stores, matching the registry.
	Type() string
	// Start connects and begins delivering messages to onMessage. It returns
	// as soon as the connection is set up; the connection then lives until
	// ctx is cancelled.
	Start(ctx context.Context, cfg Config, onMessage func(Inbound)) error
	// Send delivers markdown text to a conversation previously seen in an
	// Inbound. An empty target means the driver's configured default.
	Send(ctx context.Context, target, markdown string) error
}

// StatusReporter is implemented by drivers that can say whether they are
// connected, so the settings card shows a live indicator.
type StatusReporter interface {
	Status() (State, string)
}

// Reaction is one emoji a driver can attach to a message.
type Reaction int

const (
	// ReactionWorking marks a message the agent is still answering.
	ReactionWorking Reaction = iota
	// ReactionDone marks a message that has been answered.
	ReactionDone
)

// Reactor is implemented by drivers whose platform can acknowledge a message
// with an emoji. A driver that does not implement it simply gets no reaction,
// which is why the interface is separate from Driver: Telegram, for instance,
// has no way to react to a message on the bot's behalf without a paid account.
type Reactor interface {
	// React attaches the reaction to the message identified by in's reply
	// token. Removing a reaction the bot did not add is a no-op, so calling
	// it twice is safe.
	React(ctx context.Context, in Inbound, r Reaction) error
}

// DriverFactory builds a driver for one channel type.
type DriverFactory func() Driver

var (
	driversMu sync.RWMutex
	drivers   = map[string]DriverFactory{}
)

// RegisterDriver adds a platform to the registry. It panics on a duplicate,
// which is a programming error, not a runtime condition.
func RegisterDriver(typ string, factory DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if _, dup := drivers[typ]; dup {
		panic("channel: driver registered twice: " + typ)
	}
	drivers[typ] = factory
}

// NewDriver builds the driver for typ, or nil when no platform is registered
// under that name.
func NewDriver(typ string) Driver {
	driversMu.RLock()
	f := drivers[typ]
	driversMu.RUnlock()
	if f == nil {
		return nil
	}
	return f()
}

// DriverTypes lists the registered platform types in sorted order.
func DriverTypes() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	out := make([]string, 0, len(drivers))
	for t := range drivers {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// DriverType is implemented by drivers to answer for the registry key.
// It exists so a driver can be registered from its own package without the
// registry needing to know its name.
func DriverType(d Driver) string { return d.Type() }

// ErrNotConfigured reports a driver started without the credentials it needs.
// The message is written for the settings card, which shows it verbatim, so it
// stays short and in the interface's own language.
var ErrNotConfigured = fmt.Errorf("凭证未配置")
