package ui

import (
	"chatgo/internal/config"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

func (cw *ChatWindow) channelTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "渠道").Font(mtheme.SerifFont).FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
			})
			mgo.Text(c, "配置 IM 渠道后，可通过即时消息触发 Agent 对话。点击卡片展开配置。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			for i := range cw.cfg.Channels {
				cw.channelCard(c, i)
			}
		})
	})
}

func (cw *ChatWindow) channelCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	ch := cw.cfg.Channels[i]
	expanded := cw.settings.channel.editing == i

	card := mgo.Column(c).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				icon := "message-square"
				mgo.Icon(c, icons.Must(icon)).Size(18, 18).TextColor(k.Text).Shrink(0)
				mgo.Text(c, ch.Name).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
				if ch.Type == "feishu" {
					state, note := cw.feishuStatus()
					st, label := feishuStatusView(state, note)
					mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
					mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				} else if ch.Type == "wechat" {
					state, note := cw.wechatStatus()
					st, label := wechatStatusView(state, note)
					mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
					mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				}
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := ch.Enabled
				if minput.Switch(c, &en, "", minput.SwitchOptions{}).Changed() {
					cw.cfg.Channels[i].Enabled = en
					cw.settings.channel.enabled = en
					_ = config.SaveConfig(cw.cfg)
					cw.restartChannels()
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearChannelForm()
					} else {
						cw.loadChannelForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.channelFormFields(c)
		}
	})
}

func feishuStatusView(state feishuConnState, note string) (mfeedback.Status, string) {
	switch state {
	case feishuConnected:
		return mfeedback.StatusSuccess, "已连接 · " + note
	case feishuConnecting:
		return mfeedback.StatusBusy, "正在连接…"
	case feishuReconnecting:
		return mfeedback.StatusBusy, "重连中…"
	case feishuError:
		return mfeedback.StatusError, "连接失败：" + note
	default:
		return mfeedback.StatusOffline, "未连接"
	}
}

func (cw *ChatWindow) channelFormFields(c *mgo.Context) {
	f := &cw.settings.channel
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "渠道名称", ReadOnly: true})
		if f.typ == "wechat" {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if minput.Button(c, "扫码登录微信", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning}).Clicked() {
					cw.startWechatLogin()
				}
				if f.scanning {
					mgo.Text(c, "等待微信扫码…").TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
			if f.appID != "" {
				minput.TextInput(c, &f.appID, minput.TextInputOptions{Label: "Bot ID", Placeholder: "iLink Bot ID（扫码后自动填入）"})
			}
			if f.appSecret != "" {
				minput.PasswordInput(c, &f.appSecret, minput.PasswordInputOptions{Label: "Bot Token", Placeholder: "iLink Bot Token（扫码后自动填入）"})
			}
			mgo.Text(c, "提示：点击「扫码登录微信」，用微信扫码授权后凭证自动填入。").
				TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		} else {
			minput.TextInput(c, &f.appID, minput.TextInputOptions{Label: "AppID", Placeholder: "飞书应用 App ID"})
			minput.PasswordInput(c, &f.appSecret, minput.PasswordInputOptions{Label: "AppSecret", Placeholder: "飞书应用 App Secret"})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if minput.Button(c, "扫码创建应用", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning}).Clicked() {
					cw.startFeishuRegister()
				}
				if f.scanning {
					mgo.Text(c, "等待飞书扫码授权…").TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
			mgo.Text(c, "提示：请在飞书开发者后台 → 事件与回调 → 订阅方式，选择「使用长连接接收事件/回调」，并添加事件 im.message.receive_v1。").
				TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
		if f.qr != nil {
			mgo.Image(c, f.qr).Size(220, 220)
		}
		if f.qrTip != "" {
			mgo.Text(c, f.qrTip).TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
		agentOpts := []minput.SelectOption[string]{{Value: "", Label: chatAgentName}}
		for _, a := range cw.cfg.Agents {
			agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
		}
		for _, a := range cw.cfg.ACPAgents {
			if a.Disabled {
				continue
			}
			agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "acp:" + a.Name, Label: "ACP · " + a.Name})
		}
		minput.Select(c, &f.agent, agentOpts, minput.SelectOptions{Label: "触发 Agent"})
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := minput.TextInput(c, &f.workspace, minput.TextInputOptions{Label: "默认工作区", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
			in.Element.Grow(1)
			if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cw.pickDirectory("选择默认工作区", func(path string) { f.workspace = path })
			}
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.saveChannel()
			}
		})
	})
}

func (cw *ChatWindow) clearChannelForm() {
	cw.settings.channel = channelForm{editing: -1, typ: "feishu", enabled: true}
}

func (cw *ChatWindow) loadChannelForm(i int) {
	ch := cw.cfg.Channels[i]
	cw.settings.channel = channelForm{
		editing: i, name: ch.Name, typ: ch.Type, agent: ch.Agent, workspace: ch.Workspace,
		appID: ch.AppID, appSecret: ch.AppSecret, baseURL: ch.BaseURL, enabled: ch.Enabled,
	}
}

func (cw *ChatWindow) saveChannel() {
	f := &cw.settings.channel
	if strings.TrimSpace(f.name) == "" {
		cw.fail("渠道名称不能为空")
		return
	}
	typ := f.typ
	if typ == "" {
		typ = "feishu"
	}
	ch := config.Channel{Type: typ, Name: f.name, Agent: f.agent, Workspace: strings.TrimSpace(f.workspace), AppID: f.appID, AppSecret: f.appSecret, BaseURL: strings.TrimSpace(f.baseURL), Enabled: f.enabled}
	if f.editing >= 0 && f.editing < len(cw.cfg.Channels) {
		old := cw.cfg.Channels[f.editing]
		ch.ID = old.ID
		ch.ConvID = old.ConvID
		ch.ConvIDs = old.ConvIDs
		ch.ActiveConv = old.ActiveConv
		cw.cfg.Channels[f.editing] = ch
	} else {
		ch.ID = cw.newID()
		cw.cfg.Channels = append(cw.cfg.Channels, ch)
		f.editing = len(cw.cfg.Channels) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存渠道 "+ch.Name, mcore.SeveritySuccess)
	cw.restartChannels()
}

func (cw *ChatWindow) applyChannel() {
	f := &cw.settings.channel
	if f.editing < 0 || f.editing >= len(cw.cfg.Channels) {
		return
	}
	cw.cfg.Channels[f.editing].Enabled = f.enabled
	_ = config.SaveConfig(cw.cfg)
	cw.restartChannels()
}

func (cw *ChatWindow) deleteChannel() {
	f := &cw.settings.channel
	if f.editing < 0 || f.editing >= len(cw.cfg.Channels) {
		return
	}
	idx := f.editing
	name := cw.cfg.Channels[idx].Name
	cw.askConfirm("删除渠道", "确定删除「"+name+"」？", func() {
		cw.cfg.Channels = append(cw.cfg.Channels[:idx], cw.cfg.Channels[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearChannelForm()
		cw.restartChannels()
	})
}

func (cw *ChatWindow) restartChannels() {
	cw.stopFeishuChannel()
	cw.stopWechatChannel()
	cw.startFeishuChannel()
	cw.startWechatChannel()
}

// mcpStatus maps a server's state to an indicator status and a note.

func (cw *ChatWindow) acpTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "ACP").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Row(c).Gap(u).Children(func() {
					if minput.Button(c, "从目录添加", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("layout-grid")}).Clicked() {
						cw.acpCatalogOpen = true
					}
					if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")}).Clicked() {
						cw.addNewACP()
					}
				})
			})
			if len(cw.cfg.ACPAgents) == 0 {
				mgo.Text(c, "暂无连接，从目录添加或手动新增").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range cw.cfg.ACPAgents {
				cw.acpCard(c, i)
			}
		})
	})
	cw.acpCatalogDialog(c)
}
