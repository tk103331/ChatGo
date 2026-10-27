package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/channel"
	"chatgo/internal/config"
)

// Every platform the app ships has a driver, so the settings card never offers
// a channel type that cannot connect.
func TestEveryDefaultChannelHasDriver(t *testing.T) {
	for _, ch := range config.DefaultChannelsForTest() {
		if channel.NewDriver(ch.Type) == nil {
			t.Errorf("no driver for the shipped channel %q (%s)", ch.Name, ch.Type)
		}
	}
}

// Two channels of one platform run side by side: each gets its own driver
// instance instead of sharing one set of fields.
func TestChannelsRunIndependently(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	a := config.Channel{ID: "a", Type: "telegram", Name: "A", Enabled: true}
	b := config.Channel{ID: "b", Type: "telegram", Name: "B", Enabled: true}
	cw.cfg.Channels = []config.Channel{a, b}

	// Credentials are empty, so each start fails on its own and records its
	// own error; neither replaces the other's runtime.
	cw.startChannel(a)
	cw.startChannel(b)

	cw.channels.mu.Lock()
	n := len(cw.channels.runtimes)
	_, hasA := cw.channels.runtimes["a"]
	_, hasB := cw.channels.runtimes["b"]
	cw.channels.mu.Unlock()
	if n != 2 || !hasA || !hasB {
		t.Fatalf("runtimes = %d (a=%v b=%v), want both channels running", n, hasA, hasB)
	}
}

// A channel missing its credentials says which field is missing instead of
// starting and failing on every frame.
func TestChannelConfigErrorNamesMissingField(t *testing.T) {
	cases := []struct {
		ch   config.Channel
		want string
	}{
		{config.Channel{Type: "telegram"}, "缺少 Bot Token"},
		{config.Channel{Type: "slack", AppID: "xoxb-1"}, "缺少 App Token"},
		{config.Channel{Type: "discord", AppID: "tok"}, ""},
		{config.Channel{Type: "nope"}, "暂不支持"},
	}
	for _, c := range cases {
		got := channelConfigError(c.ch)
		if c.want == "" {
			if got != "" {
				t.Errorf("%s: unexpected error %q", c.ch.Type, got)
			}
			continue
		}
		if got == "" {
			t.Errorf("%s: no error, want one mentioning %q", c.ch.Type, c.want)
			continue
		}
		if !contains(got, c.want) {
			t.Errorf("%s: error %q does not mention %q", c.ch.Type, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// The platform field descriptions cover every shipped type, so the form never
// falls back to a generic pair for a platform that needs something else.
func TestChannelFieldsCoverPlatforms(t *testing.T) {
	for _, typ := range []string{"feishu", "wechat", "telegram", "dingtalk", "slack", "discord", "wecom"} {
		fields := channelFields(typ)
		if len(fields) < 1 {
			t.Errorf("%s has no credential field", typ)
		}
		if fields[0].Key != "app_id" {
			t.Errorf("%s's first field is %q, want app_id", typ, fields[0].Key)
		}
	}
}

// Every platform explains where its console is and what must be enabled there,
// so the settings card never leaves the user without a next step.
func TestChannelSetupGuidesCoverPlatforms(t *testing.T) {
	for _, typ := range []string{"feishu", "wechat", "telegram", "dingtalk", "slack", "discord", "wecom"} {
		setup := channelSetupFor(typ)
		if len(setup.Console) == 0 {
			t.Errorf("%s: no console page", typ)
		}
		if len(setup.Permissions) == 0 {
			t.Errorf("%s: no permission guidance", typ)
		}
		for _, entry := range setup.Console {
			if entry.Label == "" || entry.Detail == "" {
				t.Errorf("%s: console entry %+v is incomplete", typ, entry)
			}
			if !strings.HasPrefix(entry.URL, "https://") {
				t.Errorf("%s: console URL %q is not https", typ, entry.URL)
			}
		}
		for _, entry := range setup.Permissions {
			if entry.Label == "" || entry.Detail == "" {
				t.Errorf("%s: permission entry %+v is incomplete", typ, entry)
			}
			if entry.URL != "" && !strings.HasPrefix(entry.URL, "https://") {
				t.Errorf("%s: permission URL %q is not https", typ, entry.URL)
			}
		}
	}
}

// Saving one channel's form reconnects that channel alone: the others keep the
// very same runtime, so a working connection elsewhere is never dropped.
func TestSavingOneChannelLeavesOthersRunning(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	other := config.Channel{ID: "other", Type: "telegram", Name: "Other", Enabled: true}
	target := config.Channel{ID: "target", Type: "discord", Name: "Target", Enabled: true}
	cw.cfg.Channels = []config.Channel{other, target}
	cw.startChannel(other)
	cw.startChannel(target)

	cw.channels.mu.Lock()
	otherRT := cw.channels.runtimes["other"]
	targetRT := cw.channels.runtimes["target"]
	cw.channels.mu.Unlock()
	if otherRT == nil || targetRT == nil {
		t.Fatal("both channels should be running before the save")
	}

	// Editing and saving the target channel is what saveChannel does.
	cw.settings.channel = channelForm{
		editing: 1, name: "Target", typ: "discord", appID: "tok", enabled: true,
	}
	cw.saveChannel()

	cw.channels.mu.Lock()
	afterOther := cw.channels.runtimes["other"]
	afterTarget := cw.channels.runtimes["target"]
	cw.channels.mu.Unlock()
	if afterOther != otherRT {
		t.Error("saving another channel replaced the untouched channel's runtime")
	}
	if afterTarget == nil || afterTarget == targetRT {
		t.Error("the saved channel was not reconnected")
	}
}

// Deleting a channel disconnects only that one.
func TestDeletingChannelStopsOnlyThatChannel(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	a := config.Channel{ID: "a", Type: "telegram", Name: "A", Enabled: true}
	b := config.Channel{ID: "b", Type: "discord", Name: "B", Enabled: true}
	cw.cfg.Channels = []config.Channel{a, b}
	cw.startChannel(a)
	cw.startChannel(b)

	cw.channels.mu.Lock()
	aRT := cw.channels.runtimes["a"]
	cw.channels.mu.Unlock()

	cw.settings.channel = channelForm{editing: 1}
	cw.deleteChannel()
	// The delete goes through the confirm dialog, so run its action.
	if !cw.confirm.open {
		t.Fatal("deleting a channel must ask for confirmation")
	}
	cw.confirm.action()

	cw.channels.mu.Lock()
	afterA := cw.channels.runtimes["a"]
	_, hasB := cw.channels.runtimes["b"]
	cw.channels.mu.Unlock()
	if afterA != aRT {
		t.Error("deleting a channel disturbed another channel's runtime")
	}
	if hasB {
		t.Error("the deleted channel is still running")
	}
}

// Turning a channel off disconnects it and nothing else.
func TestDisablingChannelStopsOnlyThatChannel(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	a := config.Channel{ID: "a", Type: "telegram", Name: "A", Enabled: true}
	b := config.Channel{ID: "b", Type: "discord", Name: "B", Enabled: true}
	cw.cfg.Channels = []config.Channel{a, b}
	cw.startChannel(a)
	cw.startChannel(b)

	cw.cfg.Channels[0].Enabled = false
	cw.reloadChannel("a")

	cw.channels.mu.Lock()
	_, hasA := cw.channels.runtimes["a"]
	_, hasB := cw.channels.runtimes["b"]
	cw.channels.mu.Unlock()
	if hasA {
		t.Error("the disabled channel is still running")
	}
	if !hasB {
		t.Error("disabling one channel stopped another")
	}
}

// A platform with a reaction API acknowledges the message as soon as it
// arrives, and marks it done once the reply is sent.
func TestChannelReactionFiresOnInbound(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	fake := &recordingDriver{}
	cw.channels.runtimes["c1"] = &channelRuntime{id: "c1", typ: "fake", driver: fake}

	in := channel.Inbound{Text: "hi", ChatID: "chat", ReplyToken: "msg-1"}
	cw.reactToChannelMessage("c1", in, channel.ReactionWorking)
	if got := fake.wait(t); got != channel.ReactionWorking {
		t.Fatalf("reaction is %v, want the working mark", got)
	}
	cw.reactToChannelMessage("c1", in, channel.ReactionDone)
	if got := fake.wait(t); got != channel.ReactionDone {
		t.Fatalf("reaction is %v, want the done mark", got)
	}
}

// A driver without a reaction API is skipped, and nothing panics.
func TestChannelReactionSkippedWhenUnsupported(t *testing.T) {
	cw := testWindow()
	cw.channels = newChannelManager()
	cw.channels.runtimes["c1"] = &channelRuntime{id: "c1", typ: "plain", driver: &plainDriver{}}
	cw.reactToChannelMessage("c1", channel.Inbound{Text: "hi"}, channel.ReactionWorking)
	// Reaching here without a panic is the assertion; the reply path must not
	// depend on the reaction succeeding.
}

type recordingDriver struct {
	mu       sync.Mutex
	reaction channel.Reaction
	got      bool
}

func (d *recordingDriver) Type() string { return "fake" }
func (d *recordingDriver) Start(context.Context, channel.Config, func(channel.Inbound)) error {
	return nil
}
func (d *recordingDriver) Send(context.Context, string, string) error { return nil }
func (d *recordingDriver) React(_ context.Context, _ channel.Inbound, r channel.Reaction) error {
	d.mu.Lock()
	d.reaction, d.got = r, true
	d.mu.Unlock()
	return nil
}

func (d *recordingDriver) wait(t *testing.T) channel.Reaction {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		if d.got {
			r := d.reaction
			d.got = false
			d.mu.Unlock()
			return r
		}
		d.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no reaction recorded")
	return channel.ReactionWorking
}

type plainDriver struct{}

func (d *plainDriver) Type() string { return "plain" }
func (d *plainDriver) Start(context.Context, channel.Config, func(channel.Inbound)) error {
	return nil
}
func (d *plainDriver) Send(context.Context, string, string) error { return nil }

// A platform without a code flow reports that instead of failing obscurely,
// so the form can tell the user to type the credentials.
func TestQuickLoginRejectsUnsupportedPlatform(t *testing.T) {
	cw := testWindow()
	cw.settings.channel = channelForm{typ: "telegram"}
	_, err := cw.channelQuickBegin("telegram")
	if err == nil {
		t.Fatal("Telegram has no quick setup and must say so")
	}
	if !strings.Contains(err.Error(), "不支持扫码接入") {
		t.Fatalf("error is %q, want an explanation", err)
	}
}

// The platforms with a code flow are the ones the form offers a scan button
// for, so the UI and the drivers cannot drift apart.
func TestQuickLoginPlatformsMatchDrivers(t *testing.T) {
	for _, typ := range []string{"wecom", "dingtalk"} {
		if _, ok := channel.NewDriver(typ).(channel.QRLoginer); !ok {
			t.Errorf("%s is offered a scan button but has no code flow", typ)
		}
	}
	for _, typ := range []string{"telegram", "discord", "feishu", "wechat", "slack"} {
		if _, ok := channel.NewDriver(typ).(channel.QRLoginer); ok {
			t.Errorf("%s has a code flow but is not offered one", typ)
		}
	}
}

// The code a scan flow returns is drawn by the quick-setup block, under the
// button that asked for it, so the user's eye follows button then code.
func TestQuickCodeRendersUnderTheButton(t *testing.T) {
	cw := testWindow()
	// A 1x1 PNG stands in for a rendered code.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	bmp, err := mgo.DecodeBitmap(png)
	if err != nil {
		t.Fatalf("fixture PNG is invalid: %v", err)
	}
	cw.settings.channel = channelForm{
		typ: "wecom", name: "企微", qr: bmp, qrTip: "请用企业微信扫码",
		qrNotice: "这是提示",
	}
	// The block draws the code, its tip and the notice; rendering without a
	// panic is what the layout is for, and the tip must reach the frame.
	view := func(c *mgo.Context) {
		mgo.Column(c).FillWidth().Children(func() {
			cw.channelQuickCode(c)
			cw.channelQuickNotice(c)
		})
	}
	tr := mgo.NewTester(view, 900, 700)
	for range 2 {
		tr.Frame()
	}
	if !tr.HasText("请用企业微信扫码") {
		t.Fatal("the code's tip is not drawn")
	}
	if !tr.HasText("这是提示") {
		t.Fatal("the notice is not drawn")
	}
}
