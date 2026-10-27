package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"chatgo/internal/channel"
	"chatgo/internal/config"
)

// channelRuntime is one configured channel's live connection: the driver, the
// context that keeps it alive, and the status it last reported.
type channelRuntime struct {
	id     string
	typ    string
	driver channel.Driver
	cancel context.CancelFunc
	// state and note are refreshed from the driver on every frame the
	// settings card is drawn, so a reconnect shows up without a callback.
	mu    sync.Mutex
	state channel.State
	note  string
}

// channelManager owns every channel connection. Each configured channel gets
// its own driver instance, so several channels of one platform — two Telegram
// bots, say — run side by side instead of fighting over one set of fields.
type channelManager struct {
	mu       sync.Mutex
	runtimes map[string]*channelRuntime
}

func newChannelManager() *channelManager {
	return &channelManager{runtimes: map[string]*channelRuntime{}}
}

// start brings up every enabled channel, replacing whatever was running.
func (cw *ChatWindow) startChannels() {
	cw.stopChannels()
	for i := range cw.cfg.Channels {
		ch := cw.cfg.Channels[i]
		if !ch.Enabled {
			continue
		}
		cw.startChannel(ch)
	}
}

// reloadChannel brings one channel's connection in line with its saved
// configuration: it reconnects when the channel is enabled, and disconnects
// when it is not. Every other channel is left running untouched, so saving one
// form never drops a working connection elsewhere.
func (cw *ChatWindow) reloadChannel(id string) {
	ch := cw.channelByID(id)
	if ch == nil || !ch.Enabled {
		cw.stopChannel(id)
		return
	}
	cw.startChannel(*ch)
}

// startChannel connects one channel, replacing a previous connection of the
// same id.
func (cw *ChatWindow) startChannel(ch config.Channel) {
	driver := channel.NewDriver(ch.Type)
	if driver == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	rt := &channelRuntime{id: ch.ID, typ: ch.Type, driver: driver, cancel: cancel}
	cw.channels.mu.Lock()
	if old := cw.channels.runtimes[ch.ID]; old != nil && old.cancel != nil {
		old.cancel()
	}
	cw.channels.runtimes[ch.ID] = rt
	cw.channels.mu.Unlock()

	cfg := channel.Config{
		Type: ch.Type, Name: ch.Name,
		AppID: ch.AppID, AppSecret: ch.AppSecret, BaseURL: ch.BaseURL,
		Options: ch.Options,
	}
	channelID := ch.ID
	if err := driver.Start(ctx, cfg, func(in channel.Inbound) {
		cw.onChannelInbound(channelID, in)
	}); err != nil {
		cw.setChannelNote(channelID, channel.StateError, channel.DetailError(err))
	}
}

// stopChannels cancels every running connection.
func (cw *ChatWindow) stopChannels() {
	cw.channels.mu.Lock()
	runtimes := cw.channels.runtimes
	cw.channels.runtimes = map[string]*channelRuntime{}
	cw.channels.mu.Unlock()
	for _, rt := range runtimes {
		if rt.cancel != nil {
			rt.cancel()
		}
	}
}

// stopChannel cancels one connection, if it is running.
func (cw *ChatWindow) stopChannel(id string) {
	cw.channels.mu.Lock()
	rt := cw.channels.runtimes[id]
	delete(cw.channels.runtimes, id)
	cw.channels.mu.Unlock()
	if rt != nil && rt.cancel != nil {
		rt.cancel()
	}
}

// channelStatus reports a channel's connection state for the settings card.
func (cw *ChatWindow) channelStatus(id string) (channel.State, string) {
	cw.channels.mu.Lock()
	rt := cw.channels.runtimes[id]
	cw.channels.mu.Unlock()
	if rt == nil {
		return channel.StateStopped, "未连接"
	}
	if reporter, ok := rt.driver.(channel.StatusReporter); ok {
		return reporter.Status()
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.state, rt.note
}

// setChannelNote records a state the manager itself observed, such as a
// driver that refused to start.
func (cw *ChatWindow) setChannelNote(id string, st channel.State, note string) {
	cw.channels.mu.Lock()
	rt := cw.channels.runtimes[id]
	cw.channels.mu.Unlock()
	if rt == nil {
		return
	}
	rt.mu.Lock()
	rt.state, rt.note = st, note
	rt.mu.Unlock()
	cw.postMain(func() { cw.invalidate() })
}

// onChannelInbound runs the agent for a message a driver received and sends
// the reply back through the same driver.
func (cw *ChatWindow) onChannelInbound(channelID string, in channel.Inbound) {
	if in.FromBot {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return
	}
	cw.notifyChannelMessage(channelID, text)
	// Acknowledge the message with an emoji the moment it arrives, the way the
	// Feishu channel always has: the user sees the bot picked it up before the
	// answer is ready. Platforms without a reaction API are skipped silently.
	cw.reactToChannelMessage(channelID, in, channel.ReactionWorking)
	cw.runChannelAgent(channelID, text, &channelMsgContext{
		ChatID: in.ChatID, FromUser: in.FromUser, ContextToken: in.ReplyToken,
	}, func(reply string) {
		cw.sendChannelReply(channelID, in, reply)
		cw.reactToChannelMessage(channelID, in, channel.ReactionDone)
	})
}

// reactToChannelMessage attaches a state emoji to a received message through
// the running driver, when the platform supports reactions.
func (cw *ChatWindow) reactToChannelMessage(channelID string, in channel.Inbound, r channel.Reaction) {
	driver := cw.channelDriver(channelID)
	if driver == nil {
		return
	}
	reactor, ok := driver.(channel.Reactor)
	if !ok {
		return
	}
	go func() {
		_ = reactor.React(context.Background(), in, r)
	}()
}

// sendChannelReply delivers a reply through the driver that received the
// message, falling back to a driver-specific reply path when the platform
// needs the inbound frame's own token.
func (cw *ChatWindow) sendChannelReply(channelID string, in channel.Inbound, reply string) {
	cw.channels.mu.Lock()
	rt := cw.channels.runtimes[channelID]
	cw.channels.mu.Unlock()
	if rt == nil {
		return
	}
	ctx := context.Background()
	// DingTalk and WeCom only accept a reply that quotes the inbound frame,
	// which the driver exposes as a separate method.
	switch d := rt.driver.(type) {
	case interface {
		SendToWebhook(context.Context, string, string) error
	}:
		if err := d.SendToWebhook(ctx, in.ReplyToken, reply); err != nil {
			cw.setChannelNote(channelID, channel.StateError, channel.DetailError(err))
		}
		return
	case interface {
		SendReply(context.Context, string, string, string) error
	}:
		if err := d.SendReply(ctx, in.ChatID, in.ReplyToken, reply); err != nil {
			cw.setChannelNote(channelID, channel.StateError, channel.DetailError(err))
		}
		return
	}
	if err := rt.driver.Send(ctx, in.ChatID, reply); err != nil {
		cw.setChannelNote(channelID, channel.StateError, channel.DetailError(err))
	}
}

// channelDriver returns the running driver for a channel, for a tool that must
// use the channel's own connection.
func (cw *ChatWindow) channelDriver(id string) channel.Driver {
	cw.channels.mu.Lock()
	defer cw.channels.mu.Unlock()
	if rt := cw.channels.runtimes[id]; rt != nil {
		return rt.driver
	}
	return nil
}

// channelField describes one credential input a platform needs, so the
// settings card can draw the right form without a switch per platform.
type channelField struct {
	Key         string
	Label       string
	Placeholder string
	Secret      bool
	Hint        string
}

// channelFields lists the credential fields a platform needs. The first two
// map to AppID and AppSecret; any further ones live in Options.
func channelFields(typ string) []channelField {
	switch typ {
	case "feishu":
		return []channelField{
			{Key: "app_id", Label: "AppID", Placeholder: "飞书应用 App ID"},
			{Key: "app_secret", Label: "AppSecret", Placeholder: "飞书应用 App Secret", Secret: true},
		}
	case "wechat":
		return []channelField{
			{Key: "app_id", Label: "Bot ID", Placeholder: "iLink Bot ID（扫码后自动填入）"},
			{Key: "app_secret", Label: "Bot Token", Placeholder: "iLink Bot Token（扫码后自动填入）", Secret: true},
		}
	case "telegram":
		return []channelField{
			{Key: "app_id", Label: "Bot Token", Placeholder: "从 @BotFather 获取，如 123456:ABC-DEF…", Secret: true,
				Hint: "在 Telegram 里找 @BotFather 发送 /newbot，按提示创建后把得到的 Token 填在这里。"},
		}
	case "dingtalk":
		return []channelField{
			{Key: "app_id", Label: "Client ID", Placeholder: "钉钉应用 Client ID（旧称 AppKey）"},
			{Key: "app_secret", Label: "Client Secret", Placeholder: "钉钉应用 Client Secret（旧称 AppSecret）", Secret: true,
				Hint: "在钉钉开发者后台创建企业内部应用，开启机器人能力并选择 Stream 模式；事件订阅无需公网地址。"},
		}
	case "slack":
		return []channelField{
			{Key: "app_id", Label: "Bot Token", Placeholder: "xoxb- 开头，安装到工作区后获得", Secret: true},
			{Key: "app_secret", Label: "App Token", Placeholder: "xapp- 开头，需 connections:write 权限", Secret: true,
				Hint: "在 Slack App 设置里开启 Socket Mode，生成 App-Level Token（connections:write）；事件订阅 app_mention 与 message.im。"},
		}
	case "discord":
		return []channelField{
			{Key: "app_id", Label: "Bot Token", Placeholder: "Discord 开发者后台 Bot 页的 Token", Secret: true,
				Hint: "必须在 Bot 页面开启 MESSAGE_CONTENT 特权 intent，否则收不到消息正文。"},
		}
	case "wecom":
		return []channelField{
			{Key: "app_id", Label: "BotID", Placeholder: "智能机器人 BotID"},
			{Key: "app_secret", Label: "Secret", Placeholder: "长连接专用 Secret", Secret: true,
				Hint: "在企业微信管理后台的智能机器人配置页开启「API 模式 → 长连接」后获取 BotID 与 Secret。同一机器人同时只能保持一条长连接。"},
		}
	}
	return []channelField{
		{Key: "app_id", Label: "AppID", Placeholder: "应用 ID"},
		{Key: "app_secret", Label: "AppSecret", Placeholder: "应用密钥", Secret: true},
	}
}

// channelTypeName is the human name of a platform type.
func channelTypeName(typ string) string {
	switch typ {
	case "feishu":
		return "飞书"
	case "wechat":
		return "微信"
	case "telegram":
		return "Telegram"
	case "dingtalk":
		return "钉钉"
	case "slack":
		return "Slack"
	case "discord":
		return "Discord"
	case "wecom":
		return "企业微信"
	}
	return typ
}

// channelTypeSupports reports whether a platform has a registered driver, so
// the settings card can refuse to enable one that cannot connect.
func channelTypeSupports(typ string) bool {
	return channel.NewDriver(typ) != nil
}

// channelConfigError explains why a channel cannot start, or "" when it can.
func channelConfigError(ch config.Channel) string {
	if !channelTypeSupports(ch.Type) {
		return "暂不支持该渠道类型：" + ch.Type
	}
	fields := channelFields(ch.Type)
	if len(fields) == 0 {
		return ""
	}
	if strings.TrimSpace(ch.AppID) == "" {
		return "缺少 " + fields[0].Label
	}
	if len(fields) > 1 && strings.TrimSpace(ch.AppSecret) == "" {
		return "缺少 " + fields[1].Label
	}
	return ""
}

// describeChannel is a one-line summary for a log or a toast.
func describeChannel(ch config.Channel) string {
	name := strings.TrimSpace(ch.Name)
	if name == "" {
		name = channelTypeName(ch.Type)
	}
	return fmt.Sprintf("%s (%s)", name, channelTypeName(ch.Type))
}

// channelConsole is one entry of a platform's setup guide: a named console
// page, or a permission that must be granted there.
type channelConsole struct {
	// Label names the page or permission.
	Label string
	// URL is the console page, empty for a permission with no single page.
	URL string
	// Detail explains what to enable, in the user's terms.
	Detail string
}

// channelSetup is a platform's setup guide: where its console lives and which
// permissions the connection needs. The settings card draws it above the
// credential inputs, so a user who has never created a bot knows where to go
// and what to switch on.
type channelSetup struct {
	// Console is the platform's bot or app management page.
	Console []channelConsole
	// Permissions are the scopes and switches the driver depends on.
	Permissions []channelConsole
}

// channelSetupFor returns a platform's setup guide.
func channelSetupFor(typ string) channelSetup {
	switch typ {
	case "feishu":
		return channelSetup{
			Console: []channelConsole{
				{Label: "飞书开放平台 · 开发者后台", URL: "https://open.feishu.cn/app",
					Detail: "创建或选择一个企业自建应用，从「凭证与基础信息」取 App ID 与 App Secret。"},
			},
			Permissions: []channelConsole{
				{Label: "事件订阅方式：长连接", Detail: "事件与回调 → 订阅方式 → 选「使用长连接接收事件/回调」。"},
				{Label: "事件：im.message.receive_v1", Detail: "接收消息事件，必须添加，否则收不到任何消息。"},
				{Label: "权限：im:message", Detail: "读取用户发给机器人的单聊消息。"},
				{Label: "权限：im:message:send_as_bot", Detail: "以机器人身份发送消息；渠道工具发消息也依赖它。"},
			},
		}
	case "wechat":
		return channelSetup{
			Console: []channelConsole{
				{Label: "微信 iLink 机器人", URL: "https://ilinkai.weixin.qq.com",
					Detail: "无需手动注册：点下方「扫码登录微信」，授权后 Bot ID 与 Token 自动填入。"},
			},
			Permissions: []channelConsole{
				{Label: "扫码授权", Detail: "用微信扫码并在手机上确认，凭证即绑定到当前渠道。"},
			},
		}
	case "telegram":
		return channelSetup{
			Console: []channelConsole{
				{Label: "@BotFather", URL: "https://t.me/BotFather",
					Detail: "发送 /newbot，按提示取名字与用户名，BotFather 会返回一串 Bot Token。"},
			},
			Permissions: []channelConsole{
				{Label: "无需额外权限", Detail: "Bot Token 自带收发消息能力；长轮询不需要公网地址。"},
				{Label: "隐私模式（可选）", Detail: "若希望机器人在群里看到所有消息，向 @BotFather 发送 /setprivacy 并选择 Disable。"},
			},
		}
	case "dingtalk":
		return channelSetup{
			Console: []channelConsole{
				{Label: "钉钉开放平台 · 应用开发", URL: "https://open-dev.dingtalk.com/fe/app",
					Detail: "创建企业内部应用，在「凭证与基础信息」取 Client ID 与 Client Secret。"},
			},
			Permissions: []channelConsole{
				{Label: "机器人能力", Detail: "应用能力 → 机器人 → 开启，消息接收模式选「Stream 模式」。"},
				{Label: "权限：企业内机器人发送消息", Detail: "用于回复消息，未开通时回调能收到但回复会失败。"},
				{Label: "发布应用", Detail: "应用需发布到本企业，机器人才能被成员使用。"},
			},
		}
	case "slack":
		return channelSetup{
			Console: []channelConsole{
				{Label: "Slack API · Your Apps", URL: "https://api.slack.com/apps",
					Detail: "Create New App → From scratch，装到工作区后从 Install App 页取 Bot Token（xoxb-）。"},
			},
			Permissions: []channelConsole{
				{Label: "Socket Mode", Detail: "Settings → Socket Mode → 开启，并生成 App-Level Token（xapp-），scope 选 connections:write。"},
				{Label: "权限：chat:write", Detail: "发送消息，回复与渠道工具都依赖它。"},
				{Label: "权限：app_mentions:read", Detail: "接收频道里 @机器人 的事件。"},
				{Label: "权限：im:history", Detail: "接收私聊消息。"},
				{Label: "事件订阅", Detail: "Event Subscriptions → 添加 app_mention 与 message.im。"},
			},
		}
	case "discord":
		return channelSetup{
			Console: []channelConsole{
				{Label: "Discord Developer Portal", URL: "https://discord.com/developers/applications",
					Detail: "New Application → Bot，Reset Token 后复制 Bot Token。"},
			},
			Permissions: []channelConsole{
				{Label: "特权 intent：MESSAGE_CONTENT", Detail: "Bot 页 → Privileged Gateway Intents → 开启 MESSAGE_CONTENT，否则收不到消息正文。"},
				{Label: "邀请机器人", Detail: "OAuth2 → URL Generator → scope 选 bot，权限至少 Send Messages 与 Read Message History。"},
			},
		}
	case "wecom":
		return channelSetup{
			Console: []channelConsole{
				{Label: "企业微信管理后台 · 智能机器人", URL: "https://work.weixin.qq.com/wework_admin/frame#/aiBot",
					Detail: "创建智能机器人，在配置页开启「API 模式」并选择「长连接」。"},
			},
			Permissions: []channelConsole{
				{Label: "API 模式：长连接", Detail: "开启后可取 BotID 与长连接专用 Secret；与「设置接收消息回调地址」互斥。"},
				{Label: "可见范围", Detail: "在机器人配置里设置可使用成员或部门，否则成员看不到机器人。"},
				{Label: "单连接限制", Detail: "同一机器人同时只能保持一条长连接，新连接会踢掉旧的。"},
			},
		}
	}
	return channelSetup{}
}
