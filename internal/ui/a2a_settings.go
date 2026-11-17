package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/a2a"
	"chatgo/internal/config"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// a2aForm is the A2A settings page's working copy.
type a2aForm struct {
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

// a2aTab configures the remote agents spoken to over the Agent2Agent
// protocol, the same card layout the ACP tab uses.
func (cw *ChatWindow) a2aTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u * 2).Padding(u).MaxWidth(640).Children(func() {
			mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "A2A").FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				if kit.Button(c, "新增", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
					cw.addNewA2A()
				}
			})
			mgo.Text(c, "接入 Agent2Agent 远程 Agent。保存后可在新会话、群聊、自动任务和渠道中选择。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			if len(cw.cfg.A2AAgents) == 0 {
				mgo.Text(c, "暂无连接，点击「新增」填写 Agent Card 地址").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			for i := range cw.cfg.A2AAgents {
				cw.a2aCard(c, i)
			}
		})
	})
}

func (cw *ChatWindow) a2aCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	a := cw.cfg.A2AAgents[i]
	expanded := cw.settings.a2a.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("a2a:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				cw.a2aLeading(c, a, 18)
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
						cw.fail("请先填写 Agent Card 地址")
						return
					}
					cw.cfg.A2AAgents[i].Disabled = !en
					cw.settings.a2a.disabled = !en
					_ = config.SaveConfig(cw.cfg)
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearA2AForm()
					} else {
						cw.loadA2AForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.a2aFormFields(c)
		}
	})
}

func (cw *ChatWindow) a2aFormFields(c *mgo.Context) {
	f := &cw.settings.a2a
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "连接名称，例如 Research Agent"})
		kit.TextInput(c, &f.url, kit.TextInputOptions{
			Label: "Agent Card 地址", Placeholder: "https://agent.example.com 或完整的 agent-card.json",
		})
		kit.TextInput(c, &f.description, kit.TextInputOptions{Label: "描述", Placeholder: "可选，显示在 Agent 选择器里"})
		kit.PasswordInput(c, &f.apiKey, kit.PasswordInputOptions{
			Label: "API Key", Placeholder: "以 Authorization: Bearer <key> 发送，可留空",
		})
		kit.TextArea(c, &f.headers, kit.TextAreaOptions{
			Label: "自定义 Headers", Placeholder: "KEY=VALUE，每行一个", MinHeight: 60,
		})
		cw.a2aFormIconRow(c)
		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "测试连接", kit.ButtonOptions{Variant: kit.Secondary, Disabled: f.testing}).Clicked() {
				cw.testA2AConn()
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
				cw.saveA2A()
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteA2A()
			}
		})
	})
}

func (cw *ChatWindow) a2aFormIconRow(c *mgo.Context) {
	f := &cw.settings.a2a
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		cw.a2aFormPreviewIcon(c, f, 48)
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
						base = "A2A"
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

func (cw *ChatWindow) a2aFormPreviewIcon(c *mgo.Context, f *a2aForm, size float32) {
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

// a2aLeading draws a saved connection's mark on its card.
func (cw *ChatWindow) a2aLeading(c *mgo.Context, a config.A2AConnection, size float32) {
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

func (cw *ChatWindow) addNewA2A() {
	cw.cfg.A2AAgents = append(cw.cfg.A2AAgents, config.A2AConnection{Name: "New Agent", Disabled: true})
	_ = config.SaveConfig(cw.cfg)
	cw.loadA2AForm(len(cw.cfg.A2AAgents) - 1)
}

func (cw *ChatWindow) loadA2AForm(i int) {
	a := cw.cfg.A2AAgents[i]
	cw.settings.a2a = a2aForm{
		editing: i, name: a.Name, url: a.URL, description: a.Description,
		apiKey: a.APIKey, headers: joinKV(a.Headers), icon: a.Icon,
		avatarSeed: a.AvatarSeed, disabled: a.Disabled,
	}
}

func (cw *ChatWindow) clearA2AForm() {
	cw.settings.a2a = a2aForm{editing: -1}
}

func (cw *ChatWindow) a2aConnFromForm(f *a2aForm) config.A2AConnection {
	return config.A2AConnection{
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

func (cw *ChatWindow) saveA2A() {
	f := &cw.settings.a2a
	name, url := strings.TrimSpace(f.name), strings.TrimSpace(f.url)
	if name == "" || url == "" {
		cw.fail("名称与 Agent Card 地址不能为空")
		return
	}
	for i := range cw.cfg.A2AAgents {
		if i != f.editing && cw.cfg.A2AAgents[i].Name == name {
			cw.fail("名称已存在：" + name)
			return
		}
	}
	conn := cw.a2aConnFromForm(f)
	if f.editing >= 0 && f.editing < len(cw.cfg.A2AAgents) {
		conn.Source = cw.cfg.A2AAgents[f.editing].Source
		cw.cfg.A2AAgents[f.editing] = conn
	} else {
		cw.cfg.A2AAgents = append(cw.cfg.A2AAgents, conn)
		f.editing = len(cw.cfg.A2AAgents) - 1
	}
	f.name, f.url = conn.Name, conn.URL
	f.description, f.apiKey = conn.Description, conn.APIKey
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存 A2A 连接 "+conn.Name, mcore.SeveritySuccess)
}

func (cw *ChatWindow) deleteA2A() {
	f := &cw.settings.a2a
	if f.editing < 0 || f.editing >= len(cw.cfg.A2AAgents) {
		return
	}
	idx := f.editing
	name := cw.cfg.A2AAgents[idx].Name
	cw.askConfirm("删除 A2A 连接", "确定删除「"+name+"」？依赖它的会话、群聊和自动任务会失效。", func() {
		cw.cfg.A2AAgents = append(cw.cfg.A2AAgents[:idx], cw.cfg.A2AAgents[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearA2AForm()
		cw.toast("连接已删除", mcore.SeveritySuccess)
	})
}

// testA2AConn resolves the card and opens a transport, reporting what the
// agent advertises. It runs off the UI thread so a slow server cannot freeze
// settings.
func (cw *ChatWindow) testA2AConn() {
	f := &cw.settings.a2a
	if strings.TrimSpace(f.url) == "" {
		cw.fail("请先填写 Agent Card 地址")
		return
	}
	conn := cw.a2aConnFromForm(f)
	editing := f.editing
	f.testing = true
	f.testNote = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cli, err := a2a.New(ctx, conn)
		note := ""
		if err != nil {
			note = "连接失败：" + err.Error()
		} else {
			info := cli.Info()
			note = "已连接：" + info.Name
			if info.Description != "" {
				note += " · " + info.Description
			}
			if info.Streaming {
				note += " · 支持流式"
			}
		}
		cw.postMain(func() {
			if cw.settings.a2a.editing != editing {
				return
			}
			f.testing = false
			f.testNote = note
			cw.invalidate()
		})
	}()
}
