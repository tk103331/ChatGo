package ui

import (
	"strings"

	"chatgo/internal/channel"
	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

func (cw *ChatWindow) channelTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "渠道").FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
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
				mgo.Text(c, ch.Name).FontWeight(600).Shrink(0)
				// The platform is named on the card so two channels of one
				// kind stay tellable apart.
				mgo.Text(c, channelTypeName(ch.Type)).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(k.TextMuted).Shrink(0)
				state, note := cw.channelStatus(ch.ID)
				st, label := channelStatusView(state, note)
				kit.StatusIndicator(c, st, kit.StatusIndicatorOptions{})
				mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := ch.Enabled
				if mgo.Switch(c, &en).Changed() {
					if en {
						// Turning a channel on without its credentials would
						// only produce a connection error on every frame.
						if msg := channelConfigError(cw.cfg.Channels[i]); msg != "" {
							cw.fail(msg)
							return
						}
					}
					cw.cfg.Channels[i].Enabled = en
					cw.settings.channel.enabled = en
					_ = config.SaveConfig(cw.cfg)
					if en {
						cw.startChannel(cw.cfg.Channels[i])
					} else {
						cw.stopChannel(cw.cfg.Channels[i].ID)
					}
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

// channelStatusView maps a driver state to the card's indicator and label.
// The words come from channel.Summarize, so every platform describes the same
// condition identically.
func channelStatusView(state channel.State, detail string) (kit.Status, string) {
	summary := channel.Summarize(state, detail)
	switch summary.State {
	case channel.StateConnected:
		return kit.StatusSuccess, summary.String()
	case channel.StateConnecting, channel.StateReconnecting:
		return kit.StatusBusy, summary.String()
	case channel.StateError:
		return kit.StatusError, summary.String()
	default:
		return kit.StatusOffline, summary.String()
	}
}

func (cw *ChatWindow) channelFormFields(c *mgo.Context) {
	f := &cw.settings.channel
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "渠道名称", ReadOnly: true})
		cw.channelCredentialFields(c)
		agentOpts := []kit.SelectOption[string]{{Value: "", Label: chatAgentName}}
		for _, a := range cw.cfg.Agents {
			agentOpts = append(agentOpts, kit.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
		}
		for _, a := range cw.cfg.ACPAgents {
			if a.Disabled {
				continue
			}
			agentOpts = append(agentOpts, kit.SelectOption[string]{Value: "acp:" + a.Name, Label: "ACP · " + a.Name})
		}
		kit.Select(c, &f.agent, agentOpts, kit.SelectOptions{Label: "触发 Agent"})
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := kit.TextInput(c, &f.workspace, kit.TextInputOptions{Label: "默认工作区", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
			in.Element.Grow(1)
			if kit.Button(c, "选择", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.pickDirectory("选择默认工作区", func(path string) { f.workspace = path })
			}
		})
		if len(f.tools) > 0 {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			mgo.Text(c, "渠道工具").FontWeight(600)
			mgo.Text(c, "以下工具仅在来源于本渠道的消息中加载，使用本渠道的凭证执行。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			for ti := range f.tools {
				ct := &f.tools[ti]
				mgo.Column(c).FillWidth().Gap(u).Padding(u * 2).Background(k.Surface).
					Radius(mtheme.ControlRadius).Children(func() {
					mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
						mgo.Column(c).Gap(2).Grow(1).MinWidth(0).Children(func() {
							mgo.Text(c, config.GetChannelToolDisplayName(ct.Type)).FontWeight(600)
							mgo.Text(c, config.GetChannelToolDescription(ct.Type)).
								FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
						})
						mgo.Switch(c, &ct.Enable)
					})
					for _, field := range config.GetChannelToolConfigFields(ct.Type) {
						v := ct.Config[field]
						kit.TextInput(c, &v, kit.TextInputOptions{Label: field, Placeholder: field})
						if ct.Config == nil {
							ct.Config = map[string]string{}
						}
						ct.Config[field] = v
					}
				})
			}
		}
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
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
	tools := make([]config.ChannelTool, len(ch.Tools))
	for j, t := range ch.Tools {
		tools[j] = config.ChannelTool{Type: t.Type, Enable: t.Enable, Config: map[string]string{}}
		for k, v := range t.Config {
			tools[j].Config[k] = v
		}
	}
	opts := map[string]string{}
	for k, v := range ch.Options {
		opts[k] = v
	}
	cw.settings.channel = channelForm{
		editing: i, name: ch.Name, typ: ch.Type, agent: ch.Agent, workspace: ch.Workspace,
		appID: ch.AppID, appSecret: ch.AppSecret, baseURL: ch.BaseURL, enabled: ch.Enabled,
		tools: tools, options: opts,
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
	ch := config.Channel{Type: typ, Name: f.name, Agent: f.agent, Workspace: strings.TrimSpace(f.workspace), AppID: f.appID, AppSecret: f.appSecret, BaseURL: strings.TrimSpace(f.baseURL), Enabled: f.enabled, Options: f.options}
	if f.editing >= 0 && f.editing < len(cw.cfg.Channels) {
		old := cw.cfg.Channels[f.editing]
		ch.ID = old.ID
		ch.ConvID = old.ConvID
		ch.ConvIDs = old.ConvIDs
		ch.ActiveConv = old.ActiveConv
		ch.Tools = f.tools
		cw.cfg.Channels[f.editing] = ch
	} else {
		ch.ID = cw.newID()
		config.EnsureChannelTools(&ch)
		cw.cfg.Channels = append(cw.cfg.Channels, ch)
		f.editing = len(cw.cfg.Channels) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存渠道 "+ch.Name, mcore.SeveritySuccess)
	// Only this channel reconnects; the others keep their connections.
	cw.reloadChannel(ch.ID)
}

func (cw *ChatWindow) deleteChannel() {
	f := &cw.settings.channel
	if f.editing < 0 || f.editing >= len(cw.cfg.Channels) {
		return
	}
	idx := f.editing
	id := cw.cfg.Channels[idx].ID
	name := cw.cfg.Channels[idx].Name
	cw.askConfirm("删除渠道", "确定删除「"+name+"」？", func() {
		cw.cfg.Channels = append(cw.cfg.Channels[:idx], cw.cfg.Channels[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearChannelForm()
		// Only the deleted channel goes down; the rest stay connected.
		cw.stopChannel(id)
	})
}

// mcpStatus maps a server's state to an indicator status and a note.

func (cw *ChatWindow) acpTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "ACP").
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Row(c).Gap(u).Children(func() {
					if kit.Button(c, "从目录添加", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("layout-grid")}).Clicked() {
						cw.acpCatalogOpen = true
					}
					if kit.Button(c, "新增", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
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

// channelCredentialFields draws the credential inputs a platform needs. The
// first two fields map to AppID and AppSecret; any further ones live in the
// channel's Options map, so a platform with more than a token pair needs no
// new form code.
func (cw *ChatWindow) channelCredentialFields(c *mgo.Context) {
	f := &cw.settings.channel
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	fields := channelFields(f.typ)
	for i, field := range fields {
		var val *string
		switch i {
		case 0:
			val = &f.appID
		case 1:
			val = &f.appSecret
		default:
			if f.options == nil {
				f.options = map[string]string{}
			}
			v := f.options[field.Key]
			val = &v
			defer func(key, v string) {
				if f.options == nil {
					f.options = map[string]string{}
				}
				f.options[key] = v
			}(field.Key, v)
		}
		if field.Secret {
			kit.PasswordInput(c, val, kit.PasswordInputOptions{Label: field.Label, Placeholder: field.Placeholder})
			continue
		}
		kit.TextInput(c, val, kit.TextInputOptions{Label: field.Label, Placeholder: field.Placeholder})
	}
	// The two platforms with a sign-in flow keep their buttons; every other
	// platform just takes the pasted credentials.
	switch f.typ {
	case "wechat":
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "扫码登录微信", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning}).Clicked() {
				cw.startWechatLogin()
			}
			if f.scanning {
				mgo.Text(c, "等待微信扫码…").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
		})
	case "feishu":
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "扫码创建应用", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning}).Clicked() {
				cw.startFeishuRegister()
			}
			if f.scanning {
				mgo.Text(c, "等待飞书扫码授权…").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
		})
	}
	cw.channelQuickButtons(c, f.typ)
	cw.channelQuickCode(c)
	cw.channelQuickNotice(c)
	for _, field := range fields {
		if field.Hint != "" {
			mgo.Text(c, field.Hint).TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		}
	}
	cw.channelSetupGuide(c, f.typ)
}

// channelSetupGuide draws where a platform's console lives and which
// permissions the connection needs, so the form is usable without leaving the
// app to read the platform's documentation.
func (cw *ChatWindow) channelSetupGuide(c *mgo.Context, typ string) {
	setup := channelSetupFor(typ)
	if len(setup.Console) == 0 && len(setup.Permissions) == 0 {
		return
	}
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Gap(u).Padding(u * 2).Background(k.Surface).
		Radius(mtheme.ControlRadius).Children(func() {
		if len(setup.Console) > 0 {
			mgo.Text(c, "后台管理页面").FontWeight(600).FontSize(caption)
			for _, entry := range setup.Console {
				cw.channelConsoleRow(c, entry)
			}
		}
		if len(setup.Permissions) > 0 {
			if len(setup.Console) > 0 {
				mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			}
			mgo.Text(c, "需要开通的权限与配置").FontWeight(600).FontSize(caption)
			for _, entry := range setup.Permissions {
				cw.channelConsoleRow(c, entry)
			}
		}
	})
}

// channelConsoleRow is one guide entry: a label, an optional button that opens
// the console page, and the detail under it.
func (cw *ChatWindow) channelConsoleRow(c *mgo.Context, entry channelConsole) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Gap(2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, icons.Must("circle-check")).Size(12, 12).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, entry.Label).FontSize(caption).FontWeight(500).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if entry.URL != "" {
				if kit.Button(c, "打开", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
					c.OpenURL(entry.URL)
				}
			}
		})
		if entry.Detail != "" {
			mgo.Text(c, entry.Detail).FontSize(caption).TextColor(k.TextMuted)
		}
	})
}
