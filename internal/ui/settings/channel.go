package settings

import (
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// channelTab lists the configured IM channels.
func channelTab(c *mgo.Context, state *State, opts Options) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "渠道").Font(mtheme.SerifFont).FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
			})
			mgo.Text(c, "配置 IM 渠道后，可通过即时消息触发 Agent 对话。点击卡片展开配置。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			for i := range opts.Config().Channels {
				channelCard(c, state, opts, i)
			}
		})
	})
}

// channelCard renders one channel with its expandable form.
func channelCard(c *mgo.Context, state *State, opts Options, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	ch := opts.Config().Channels[i]
	expanded := state.Channel.Editing == i

	card := mgo.Column(c).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Icon(c, icons.Must("message-square")).Size(18, 18).TextColor(k.Text).Shrink(0)
				mgo.Text(c, ch.Name).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
				if ch.Type == "feishu" {
					st, label := opts.FeishuStatusView()
					mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
					mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				} else if ch.Type == "wechat" {
					st, label := opts.WechatStatusView()
					mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
					mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				}
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := ch.Enabled
				if minput.Switch(c, &en, "", minput.SwitchOptions{}).Changed() {
					opts.Config().Channels[i].Enabled = en
					state.Channel.Enabled = en
					_ = opts.SaveConfig()
					opts.RestartChannels()
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						ClearChannelForm(state)
					} else {
						LoadChannelForm(state, opts, i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			channelFormFields(c, state, opts)
		}
	})
}

// channelFormFields renders the editable fields of the expanded channel.
func channelFormFields(c *mgo.Context, state *State, opts Options) {
	f := &state.Channel
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		minput.TextInput(c, &f.Name, minput.TextInputOptions{Label: "名称", Placeholder: "渠道名称", ReadOnly: true})
		if f.Typ == "wechat" {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if minput.Button(c, "扫码登录微信", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("globe"), Disabled: f.Scanning}).Clicked() {
					opts.StartWechatLogin()
				}
				if f.Scanning {
					mgo.Text(c, "等待微信扫码…").TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
			if f.AppID != "" {
				minput.TextInput(c, &f.AppID, minput.TextInputOptions{Label: "Bot ID", Placeholder: "iLink Bot ID（扫码后自动填入）"})
			}
			if f.AppSecret != "" {
				minput.PasswordInput(c, &f.AppSecret, minput.PasswordInputOptions{Label: "Bot Token", Placeholder: "iLink Bot Token（扫码后自动填入）"})
			}
			mgo.Text(c, "提示：点击「扫码登录微信」，用微信扫码授权后凭证自动填入。").
				TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		} else {
			minput.TextInput(c, &f.AppID, minput.TextInputOptions{Label: "AppID", Placeholder: "飞书应用 App ID"})
			minput.PasswordInput(c, &f.AppSecret, minput.PasswordInputOptions{Label: "AppSecret", Placeholder: "飞书应用 App Secret"})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if minput.Button(c, "扫码创建应用", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("globe"), Disabled: f.Scanning}).Clicked() {
					opts.StartFeishuRegister()
				}
				if f.Scanning {
					mgo.Text(c, "等待飞书扫码授权…").TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
			mgo.Text(c, "提示：请在飞书开发者后台 → 事件与回调 → 订阅方式，选择「使用长连接接收事件/回调」，并添加事件 im.message.receive_v1。").
				TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
		if f.QR != nil {
			mgo.Image(c, f.QR).Size(220, 220)
		}
		if f.QRTip != "" {
			mgo.Text(c, f.QRTip).TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
		agentOpts := []minput.SelectOption[string]{{Value: "", Label: opts.ChatAgentName}}
		for _, a := range opts.Config().Agents {
			agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
		}
		for _, a := range opts.Config().ACPAgents {
			if a.Disabled {
				continue
			}
			agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "acp:" + a.Name, Label: "ACP · " + a.Name})
		}
		minput.Select(c, &f.Agent, agentOpts, minput.SelectOptions{Label: "触发 Agent"})
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := minput.TextInput(c, &f.Workspace, minput.TextInputOptions{Label: "默认工作区", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
			in.Element.Grow(1)
			if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				opts.PickDirectory("选择默认工作区", func(path string) { f.Workspace = path })
			}
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				SaveChannel(state, opts)
			}
		})
	})
}

// ClearChannelForm closes the expanded channel form.
func ClearChannelForm(state *State) {
	state.Channel = ChannelForm{Editing: -1, Typ: "feishu", Enabled: true}
}

// LoadChannelForm fills the form from the channel at index i.
func LoadChannelForm(state *State, opts Options, i int) {
	ch := opts.Config().Channels[i]
	state.Channel = ChannelForm{
		Editing: i, Name: ch.Name, Typ: ch.Type, Agent: ch.Agent, Workspace: ch.Workspace,
		AppID: ch.AppID, AppSecret: ch.AppSecret, BaseURL: ch.BaseURL, Enabled: ch.Enabled,
	}
}

// SaveChannel validates and persists the edited channel.
func SaveChannel(state *State, opts Options) {
	f := &state.Channel
	if strings.TrimSpace(f.Name) == "" {
		opts.Fail("渠道名称不能为空")
		return
	}
	typ := f.Typ
	if typ == "" {
		typ = "feishu"
	}
	ch := config.Channel{Type: typ, Name: f.Name, Agent: f.Agent, Workspace: strings.TrimSpace(f.Workspace), AppID: f.AppID, AppSecret: f.AppSecret, BaseURL: strings.TrimSpace(f.BaseURL), Enabled: f.Enabled}
	cfg := opts.Config()
	if f.Editing >= 0 && f.Editing < len(cfg.Channels) {
		old := cfg.Channels[f.Editing]
		ch.ID = old.ID
		ch.ConvID = old.ConvID
		ch.ConvIDs = old.ConvIDs
		ch.ActiveConv = old.ActiveConv
		cfg.Channels[f.Editing] = ch
	} else {
		cfg.Channels = append(cfg.Channels, ch)
		f.Editing = len(cfg.Channels) - 1
	}
	_ = opts.SaveConfig()
	opts.Toast("已保存渠道 "+ch.Name, mcore.SeveritySuccess)
	opts.RestartChannels()
}
