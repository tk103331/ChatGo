package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/agui"
	"chatgo/internal/config"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// aguiForm is the AG-UI settings page's working copy.
type aguiForm struct {
	editing     int
	name        string
	url         string
	description string
	apiKey      string
	headers     string
	icon        string
	avatarSeed  string
	disabled    bool
	testing     bool
	testNote    string
}

// aguiTab configures the remote agents spoken to over the Agent2Agent
// protocol, the same card layout the ACP tab uses.
func (cw *ChatWindow) aguiTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "AG-UI").FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				if kit.Button(c, "新增", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
					cw.addNewAGUI()
				}
			})
			mgo.Text(c, "接入 AG-UI 远程服务（Agent 运行端点）。保存后可在新会话和对话中选择。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			if len(cw.cfg.AGUIAgents) == 0 {
				mgo.Text(c, "暂无连接，点击「新增」填写运行端点 URL").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range cw.cfg.AGUIAgents {
				cw.aguiCard(c, i)
			}
		})
	})
}

func (cw *ChatWindow) aguiCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	a := cw.cfg.AGUIAgents[i]
	expanded := cw.settings.agui.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("agui:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				cw.aguiLeading(c, a, 18)
				mgo.Column(c).Gap(2).Children(func() {
					mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Children(func() {
						mgo.Text(c, a.Name).FontWeight(600).Shrink(0)
						if a.Disabled {
							mgo.Text(c, "已禁用").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
								TextColor(k.TextMuted).Shrink(0)
						}
					})
					mgo.Text(c, a.URL).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
						TextColor(k.TextMuted).SingleLine().Ellipsis("…")
				})
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := !a.Disabled
				if mgo.Switch(c, &en).Changed() {
					if en && strings.TrimSpace(a.URL) == "" {
						cw.fail("请先填写运行端点 URL")
						return
					}
					cw.cfg.AGUIAgents[i].Disabled = !en
					cw.settings.agui.disabled = !en
					_ = config.SaveConfig(cw.cfg)
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearAGUIForm()
					} else {
						cw.loadAGUIForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.aguiFormFields(c)
		}
	})
}

func (cw *ChatWindow) aguiFormFields(c *mgo.Context) {
	f := &cw.settings.agui
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "连接名称，例如 Research Agent"})
		kit.TextInput(c, &f.url, kit.TextInputOptions{
			Label: "运行端点 URL", Placeholder: "https://agent.example.com/agui",
		})
		kit.TextInput(c, &f.description, kit.TextInputOptions{Label: "描述", Placeholder: "可选，显示在 Agent 选择器里"})
		kit.PasswordInput(c, &f.apiKey, kit.PasswordInputOptions{
			Label: "API Key", Placeholder: "以 Authorization: Bearer <key> 发送，可留空",
		})
		kit.TextArea(c, &f.headers, kit.TextAreaOptions{
			Label: "自定义 Headers", Placeholder: "KEY=VALUE，每行一个", MinHeight: 60,
		})
		cw.aguiFormIconRow(c)
		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "测试连接", kit.ButtonOptions{Variant: kit.Secondary, Disabled: f.testing}).Clicked() {
				cw.testAGUIConn()
			}
			if f.testing {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
			}
			if f.testNote != "" {
				col := k.TextMuted
				if strings.HasPrefix(f.testNote, "连接失败") {
					col = k.Danger
				} else if strings.HasPrefix(f.testNote, "已连接") {
					col = k.Success
				}
				mgo.Text(c, f.testNote).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(col).SingleLine().Ellipsis("…").Tooltip(f.testNote)
			}
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveAGUI()
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteAGUI()
			}
		})
	})
}

func (cw *ChatWindow) aguiFormIconRow(c *mgo.Context) {
	f := &cw.settings.agui
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		cw.aguiFormPreviewIcon(c, f, 48)
		mgo.Column(c).Grow(1).MinWidth(0).Gap(u).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if mcore.IconAction(c, icons.Must("image"), "选择图标").Tooltip("选择图标").Clicked() {
					cw.pickIconFile(func(path string) {
						f.icon = path
						f.avatarSeed = ""
					})
				}
				if mcore.IconAction(c, icons.Must("refresh-cw"), "生成图标").Tooltip("生成图标").Clicked() {
					base := strings.TrimSpace(f.name)
					if base == "" {
						base = "AG-UI"
					}
					f.avatarSeed = fmt.Sprintf("%s#%d", base, time.Now().UnixNano())
					f.icon = ""
					cw.invalidate()
				}
				if f.icon != "" || f.avatarSeed != "" {
					if mcore.IconAction(c, icons.Must("x"), "清除图标").Tooltip("清除图标").Clicked() {
						f.icon = ""
						f.avatarSeed = ""
					}
				}
				if f.icon != "" {
					mgo.Text(c, filepath.Base(f.icon)).FontSize(caption).TextColor(k.TextMuted).
						SingleLine().Ellipsis("…").Tooltip(f.icon)
				}
			})
		})
	})
}

func (cw *ChatWindow) aguiFormPreviewIcon(c *mgo.Context, f *aguiForm, size float32) {
	if f.icon != "" && drawIconImage(c, cw.loadIconImage(f.icon), size) {
		return
	}
	if f.avatarSeed != "" {
		if bmp := cw.agentBitmap(f.avatarSeed); bmp != nil {
			mgo.Image(c, bmp).Size(size, size).Shrink(0)
		}
		return
	}
	mgo.Icon(c, icons.Must("globe")).Size(size, size).Shrink(0).TextColor(mcore.Tokens(c).Accent)
}

// aguiLeading draws a saved connection's mark on its card.
func (cw *ChatWindow) aguiLeading(c *mgo.Context, a config.AGUIConnection, size float32) {
	if a.Icon != "" && drawIconImage(c, cw.loadIconImage(a.Icon), size) {
		return
	}
	if a.AvatarSeed != "" {
		if bmp := cw.agentBitmap(a.AvatarSeed); bmp != nil {
			mgo.Image(c, bmp).Size(size, size).Shrink(0)
			return
		}
	}
	mgo.Icon(c, icons.Must("globe")).Size(size, size).Shrink(0).TextColor(mcore.Tokens(c).Accent)
}

func (cw *ChatWindow) addNewAGUI() {
	cw.cfg.AGUIAgents = append(cw.cfg.AGUIAgents, config.AGUIConnection{Name: "New Agent", Disabled: true})
	_ = config.SaveConfig(cw.cfg)
	cw.loadAGUIForm(len(cw.cfg.AGUIAgents) - 1)
}

func (cw *ChatWindow) loadAGUIForm(i int) {
	a := cw.cfg.AGUIAgents[i]
	cw.settings.agui = aguiForm{
		editing: i, name: a.Name, url: a.URL, description: a.Description,
		apiKey: a.APIKey, headers: joinKV(a.Headers), icon: a.Icon,
		avatarSeed: a.AvatarSeed, disabled: a.Disabled,
	}
}

func (cw *ChatWindow) clearAGUIForm() {
	cw.settings.agui = aguiForm{editing: -1}
}

func (cw *ChatWindow) aguiConnFromForm(f *aguiForm) config.AGUIConnection {
	return config.AGUIConnection{
		Name:        strings.TrimSpace(f.name),
		URL:         strings.TrimSpace(f.url),
		Description: strings.TrimSpace(f.description),
		APIKey:      strings.TrimSpace(f.apiKey),
		Headers:     parseKV(f.headers),
		Icon:        f.icon,
		AvatarSeed:  f.avatarSeed,
		Disabled:    f.disabled,
	}
}

func (cw *ChatWindow) saveAGUI() {
	f := &cw.settings.agui
	name, url := strings.TrimSpace(f.name), strings.TrimSpace(f.url)
	if name == "" || url == "" {
		cw.fail("名称与运行端点 URL 不能为空")
		return
	}
	for i := range cw.cfg.AGUIAgents {
		if i != f.editing && cw.cfg.AGUIAgents[i].Name == name {
			cw.fail("名称已存在：" + name)
			return
		}
	}
	conn := cw.aguiConnFromForm(f)
	if f.editing >= 0 && f.editing < len(cw.cfg.AGUIAgents) {
		conn.Source = cw.cfg.AGUIAgents[f.editing].Source
		cw.cfg.AGUIAgents[f.editing] = conn
	} else {
		cw.cfg.AGUIAgents = append(cw.cfg.AGUIAgents, conn)
		f.editing = len(cw.cfg.AGUIAgents) - 1
	}
	f.name, f.url = conn.Name, conn.URL
	f.description, f.apiKey = conn.Description, conn.APIKey
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存 AG-UI 连接 "+conn.Name, mcore.SeveritySuccess)
}

func (cw *ChatWindow) deleteAGUI() {
	f := &cw.settings.agui
	if f.editing < 0 || f.editing >= len(cw.cfg.AGUIAgents) {
		return
	}
	idx := f.editing
	name := cw.cfg.AGUIAgents[idx].Name
	cw.askConfirm("删除 AG-UI 连接", "确定删除「"+name+"」？依赖它的会话、群聊和自动任务会失效。", func() {
		cw.cfg.AGUIAgents = append(cw.cfg.AGUIAgents[:idx], cw.cfg.AGUIAgents[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearAGUIForm()
		cw.toast("连接已删除", mcore.SeveritySuccess)
	})
}

// testAGUIConn resolves the card and opens a transport, reporting what the
// agent advertises. It runs off the UI thread so a slow server cannot freeze
// settings.
func (cw *ChatWindow) testAGUIConn() {
	f := &cw.settings.agui
	if strings.TrimSpace(f.url) == "" {
		cw.fail("请先填写运行端点 URL")
		return
	}
	conn := cw.aguiConnFromForm(f)
	editing := f.editing
	f.testing = true
	f.testNote = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		note := ""
		if cli, err := agui.New(conn); err != nil {
			note = "连接失败：" + err.Error()
		} else if status, err := cli.Ping(ctx); err != nil {
			note = "连接失败：" + err.Error()
		} else {
			note = "服务可达（" + status + "），发送消息即可验证"
		}
		cw.postMain(func() {
			if cw.settings.agui.editing != editing {
				return
			}
			f.testing = false
			f.testNote = note
			cw.invalidate()
		})
	}()
}
