package channel

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// Every platform describes the same condition with the same words, so a user
// reading two channel cards sees one vocabulary rather than seven.
func TestSummarizeUsesOneVocabulary(t *testing.T) {
	cases := []struct {
		st     State
		detail string
		want   string
	}{
		{StateConnected, "chatgo_bot", "已连接 · chatgo_bot"},
		{StateConnected, "", "已连接"},
		{StateConnecting, "ignored", "连接中"},
		{StateReconnecting, "", "重连中"},
		{StateError, "凭证无效", "连接失败 · 凭证无效"},
		{StateStopped, "ignored", "未连接"},
	}
	for _, c := range cases {
		if got := Summarize(c.st, c.detail).String(); got != c.want {
			t.Errorf("Summarize(%v, %q) = %q, want %q", c.st, c.detail, got, c.want)
		}
	}
}

// A driver reports the fact only; the wording belongs to Summarize, so a
// driver cannot invent its own phrasing.
func TestSummaryForDriver(t *testing.T) {
	d := NewTelegram()
	if got := SummaryFor(d).String(); got != "未连接" {
		t.Fatalf("a fresh driver is %q, want 未连接", got)
	}
}

// A bot name is shown with the @ mark once, whichever way it arrives.
func TestDetailConnected(t *testing.T) {
	if got := DetailConnected("bot"); got != "@bot" {
		t.Errorf("DetailConnected(bot) = %q", got)
	}
	if got := DetailConnected("@bot"); got != "@bot" {
		t.Errorf("DetailConnected(@bot) = %q", got)
	}
	if got := DetailConnected("  "); got != "" {
		t.Errorf("an empty name must stay empty, got %q", got)
	}
}

// A platform error is cut to one line, because a multi-line server response
// would break the card's layout.
func TestDetailErrorKeepsOneLine(t *testing.T) {
	err := errors.New("first line\nsecond line\nthird")
	if got := DetailError(err); got != "first line" {
		t.Fatalf("DetailError = %q", got)
	}
	if got := DetailError(nil); got != "" {
		t.Fatalf("a nil error must be empty, got %q", got)
	}
	long := strings.Repeat("x", 400)
	got := DetailError(errors.New(long))
	if utf8.RuneCountInString(got) > 161 {
		t.Fatalf("a long error was not shortened: %d characters", utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("a shortened error should end with an ellipsis: %q", got)
	}
}

// The summary of a driver that has just failed names the platform's reason and
// nothing else.
func TestSummaryOfFailedDriver(t *testing.T) {
	d := NewSlack()
	if err := d.Start(t.Context(), Config{Type: "slack"}, func(Inbound) {}); err == nil {
		t.Fatal("starting without credentials must fail")
	}
	summary := SummaryFor(d)
	if summary.State != StateError {
		t.Fatalf("state is %v, want error", summary.State)
	}
	// The detail names what is missing rather than a generic failure, so the
	// card tells the user what to fill in.
	if summary.Detail == "" {
		t.Fatalf("summary is %q, want the platform's reason", summary.String())
	}
	if !strings.HasPrefix(summary.String(), "连接失败 · ") {
		t.Fatalf("summary is %q, want the canonical failure wording", summary.String())
	}
}

// Each platform names the identity it confirmed, and the name survives the
// repeated connected states a polling driver reports on every round.
func TestConnectedIdentityPerPlatform(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		// Telegram keeps the @name getMe returned instead of blanking it on
		// the next poll.
		{"telegram", Summarize(StateConnected, DetailConnected("chatgo_bot")).String(), "已连接 · @chatgo_bot"},
		// Slack names the bot (User), not the workspace (Team).
		{"slack", Summarize(StateConnected, DetailConnected("chatgo")).String(), "已连接 · @chatgo"},
		// Discord's name carries no @ mark.
		{"discord", Summarize(StateConnected, DetailConnected("ChatGo")).String(), "已连接 · @ChatGo"},
		// WeChat has no identity endpoint, so it reports the product name.
		{"wechat", Summarize(StateConnected, wechatIdentity).String(), "已连接 · 微信ClawBot"},
		// A platform with nothing to report says only that it is connected.
		{"dingtalk", Summarize(StateConnected, "").String(), "已连接"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
}

// A poll round must repeat the identity learned at startup, never blank it.
func TestTelegramKeepsIdentityAcrossPolls(t *testing.T) {
	d := NewTelegram()
	d.identity = DetailConnected("chatgo_bot")
	d.setState(StateConnected, d.identity)
	if summary := SummaryFor(d); summary.Detail != "@chatgo_bot" {
		t.Fatalf("the first round reports %q", summary.Detail)
	}
	// The poll loop sets the same value on every successful round.
	d.setState(StateConnected, d.identity)
	if summary := SummaryFor(d); summary.Detail != "@chatgo_bot" {
		t.Fatalf("a later round blanked the identity: %q", summary.Detail)
	}
}
