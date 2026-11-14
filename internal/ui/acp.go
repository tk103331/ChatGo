package ui

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/plugin"
	"chatgo/internal/shellenv"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// acpClientInfo names the app to the agent, the way paseo sends its own
// name and version: a CLI can show the connected client, and some adapt
// their behaviour to it.
var acpClientInfo = &acp.Implementation{Name: "ChatGo", Version: "1"}

// acpHandshakeTimeout bounds the initialize and new-session exchange.
const acpHandshakeTimeout = 60 * time.Second

type acpForm struct {
	editing    int
	name       string
	command    string
	args       string
	env        string
	cwd        string
	icon       string
	avatarSeed string
	disabled   bool
}

func (cw *ChatWindow) hasACPConn(name string) bool {
	for _, a := range cw.cfg.ACPAgents {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

// acpEntryAutoStarts reports whether a catalog entry can run the moment it
// is added: an npx or uvx command pulls its own package, so the user has
// nothing to install by hand, and a binary the shell already resolves needs
// no install either. Anything else has to be installed first, so that
// connection starts disabled with a hint instead of a warm-up that is
// bound to fail.
func acpEntryAutoStarts(e plugin.ACPPresetEntry) bool {
	base := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(e.Command, ".exe"), ".cmd"))
	switch base {
	case "npx", "uvx":
		return true
	}
	_, ok := shellenv.ResolveCommand(e.Command)
	return ok
}

func (cw *ChatWindow) addACPFromCatalog(e plugin.ACPPresetEntry) {
	if cw.hasACPConn(e.Name) {
		cw.toast("已存在同名连接", mcore.SeverityInfo)
		return
	}
	enabled := acpEntryAutoStarts(e)
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: e.Name, Command: e.Command, Args: e.Args, Env: e.Env, Disabled: !enabled,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.acpCatalogOpen = false
	if enabled {
		cw.toast("已添加并启用 "+e.Name, mcore.SeveritySuccess)
		// The warm-up starts right away: an npx connection is expected to
		// work out of the box, and its model list should greet the picker.
		cw.acpWarmConnection(e.Name)
	} else {
		cw.toast("已添加 "+e.Name+"，请先安装对应 CLI 再启用", mcore.SeveritySuccess)
	}
}

func (cw *ChatWindow) acpCatalogDialog(c *mgo.Context) {
	if !cw.acpCatalogOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	kit.Dialog(c, &cw.acpCatalogOpen, kit.DialogOptions{
		Title: "从插件添加 Agent",
		Width: 560,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.acpCatalogQuery, kit.SearchInputOptions{Placeholder: "搜索 Agent", Label: "搜索"})
			in.Element.FillWidth()
			q := strings.ToLower(strings.TrimSpace(cw.acpCatalogQuery))
			entries := cw.pluginACPCatalog()
			if q != "" {
				var kept []plugin.ACPPresetEntry
				for _, e := range entries {
					if !strings.Contains(strings.ToLower(e.Name), q) {
						continue
					}
					kept = append(kept, e)
				}
				entries = kept
			}
			if len(entries) == 0 {
				mgo.Text(c, "没有匹配的 Agent。目录由内置插件提供，可在插件设置中启用。").TextColor(k.TextMuted)
				return
			}
			groups := plugin.CatalogGroupOrder(entries, func(e plugin.ACPPresetEntry) string { return e.Group })
			mgo.Scroll(c).MaxHeight(400).Children(func() {
				mgo.Column(c).Gap(u * 2).Children(func() {
					for _, group := range groups {
						mgo.Text(c, group).FontSize(caption).FontWeight(600).TextColor(k.TextMuted)
						for _, e := range entries {
							if e.Group != group {
								continue
							}
							e := e
							mgo.Row(c).Key(e.Name).FillWidth().Gap(u*2).AlignItems(mgo.Center).
								Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
								Children(func() {
									acpIcon(c, e.Name, 24, 24)
									mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
										mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
											mgo.Text(c, e.Name).FontWeight(600).SingleLine()
											// What adding will do: an npx or uvx
											// entry runs right away; anything
											// else names an install the user
											// does first.
											if acpEntryAutoStarts(e) {
												mgo.Text(c, "自动拉取").FontSize(caption).TextColor(k.Success)
											} else {
												mgo.Text(c, "需先安装 CLI").FontSize(caption).TextColor(k.TextMuted)
											}
										})
										cmdline := e.Command + " " + strings.Join(e.Args, " ")
										mgo.Text(c, cmdline).
											Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…").
											Tooltip(cmdline)
									})
									if cw.hasACPConn(e.Name) {
										mgo.Text(c, "已添加").FontSize(caption).TextColor(k.Success).Shrink(0)
									} else if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
										cw.addACPFromCatalog(e)
									}
								})
						}
					}
				})
			})
		})
	})
}

// pluginACPCatalog reads the ACP suggestions from the plugin manager, nil
// when no manager is up (tests without plugins).
func (cw *ChatWindow) pluginACPCatalog() []plugin.ACPPresetEntry {
	if cw.plugins == nil {
		return nil
	}
	return cw.plugins.ACPCatalog()
}

func (cw *ChatWindow) addNewACP() {
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: "New Connection", Disabled: true,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.loadACPForm(len(cw.cfg.ACPAgents) - 1)
}

func (cw *ChatWindow) acpCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	a := cw.cfg.ACPAgents[i]
	expanded := cw.acpForm.editing == i
	card := mgo.Column(c).Key(fmt.Sprintf("acp:%d", i)).FillWidth().Gap(u * 2).Padding(u * 2).Radius(mtheme.ControlRadius)
	if expanded {
		card.Background(k.Selection)
	} else {
		card.Background(k.Surface)
	}
	card.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.SpaceBetween).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				cw.acpLeading(c, a.Name, 18)
				mgo.Text(c, a.Name).FontWeight(600).Shrink(0)
				cw.acpUpdBadge(c, a.Name)
				if cw.acpModelError(a.Name) != "" {
					// A failed warm-up is visible right on the card, so a
					// CLI that cannot start never fails silently.
					mgo.Text(c, "预热失败").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
						TextColor(k.Danger).Shrink(0)
				}
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := !a.Disabled
				if mgo.Switch(c, &en).Changed() {
					cw.cfg.ACPAgents[i].Disabled = !en
					cw.acpForm.disabled = !en
					_ = config.SaveConfig(cw.cfg)
					if en {
						// Enabling warms the connection: its model list and
						// config options land in the cache for 新会话.
						cw.acpWarmConnection(a.Name)
					}
				}
				if mcore.IconAction(c, icons.Must("chevron-down"), "展开").Clicked() {
					if expanded {
						cw.clearACPForm()
					} else {
						cw.loadACPForm(i)
					}
				}
			})
		})
		if expanded {
			mgo.Box(c).FillWidth().Height(1).Background(k.Border)
			cw.acpFormFields(c)
		}
	})
}

func (cw *ChatWindow) acpFormFields(c *mgo.Context) {
	f := &cw.acpForm
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "连接名称"})
		cw.acpFormIconRow(c)
		cmd := kit.InputGroup(c, &f.command, kit.InputGroupOptions{
			Label:       "命令",
			Placeholder: "如 claude-code-acp 或 gemini",
			Buttons:     []kit.InputGroupButton{{Label: "查找", Icon: icons.Must("search")}},
		})
		if cmd.Clicked == 0 {
			cw.locateACPCommand()
		}
		kit.TextArea(c, &f.args, kit.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 60})
		kit.TextArea(c, &f.env, kit.TextAreaOptions{Label: "环境变量", Placeholder: "KEY=VALUE，每行一个", MinHeight: 60})
		kit.TextInput(c, &f.cwd, kit.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前目录"})
		// The warm-up state of the connection: its runtime answers the model
		// list and config options the 新会话 page shows, so a failure is
		// named here with a retry at hand.
		cw.acpWarmRow(c, cw.acpFormConn(f))
		// The version row describes the saved connection with the command
		// fields as currently typed, so a located command can be checked
		// before saving.
		cw.acpVersionRow(c, f)
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveACP()
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteACP()
			}
		})
	})
}

// acpFormIconRow shows the connection icon preview with pick/generate actions.
func (cw *ChatWindow) acpFormIconRow(c *mgo.Context) {
	f := &cw.acpForm
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		cw.acpFormPreviewIcon(c, f, 48)
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
						base = "ACP"
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

// acpFormPreviewIcon draws the icon a form currently describes, before saving.
func (cw *ChatWindow) acpFormPreviewIcon(c *mgo.Context, f *acpForm, size float32) {
	if f.icon != "" && drawIconImage(c, cw.loadIconImage(f.icon), size) {
		return
	}
	if f.avatarSeed != "" {
		if bmp := cw.agentBitmap(f.avatarSeed); bmp != nil {
			mgo.Image(c, bmp).Size(size, size).Shrink(0)
		}
		return
	}
	if acpIcon(c, f.name, size, size) {
		return
	}
	mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).Shrink(0).TextColor(mcore.Tokens(c).Accent)
}

// pickIconFile opens a native image chooser and hands the path to cb.
func (cw *ChatWindow) pickIconFile(cb func(string)) {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: w, Title: "选择图标",
			Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		cw.postMain(func() {
			cb(paths[0])
			cw.invalidate()
		})
	}()
}

// locateACPCommand rewrites the form command to the absolute path of the
// executable found on PATH or in common install directories.
func (cw *ChatWindow) locateACPCommand() {
	f := &cw.acpForm
	name := strings.TrimSpace(f.command)
	if name == "" {
		cw.toast("请先填写命令", mcore.SeverityInfo)
		return
	}
	path, ok := shellenv.ResolveCommand(name)
	if !ok {
		cw.toast("未找到命令 "+name, mcore.SeverityError)
		return
	}
	f.command = path
	cw.invalidate()
	cw.toast("已定位到 "+path, mcore.SeveritySuccess)
}

func (cw *ChatWindow) loadACPForm(i int) {
	a := cw.cfg.ACPAgents[i]
	cw.acpForm = acpForm{
		editing: i, name: a.Name, command: a.Command,
		args: strings.Join(a.Args, "\n"), env: joinKV(a.Env), cwd: a.Cwd,
		icon: a.Icon, avatarSeed: a.AvatarSeed, disabled: a.Disabled,
	}
}

func (cw *ChatWindow) clearACPForm() {
	cw.acpForm = acpForm{editing: -1}
}

func (cw *ChatWindow) saveACP() {
	f := &cw.acpForm
	if strings.TrimSpace(f.name) == "" || strings.TrimSpace(f.command) == "" {
		cw.toast("名称与命令不能为空", mcore.SeverityError)
		return
	}
	conn := config.ACPConnection{
		Name: f.name, Command: f.command, Args: splitLines(f.args),
		Env: parseKV(f.env), Cwd: f.cwd, Icon: f.icon, AvatarSeed: f.avatarSeed, Disabled: f.disabled,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.ACPAgents) {
		cw.cfg.ACPAgents[f.editing] = conn
	} else {
		cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, conn)
		f.editing = len(cw.cfg.ACPAgents) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存连接 "+conn.Name, mcore.SeveritySuccess)
}

func (cw *ChatWindow) deleteACP() {
	f := &cw.acpForm
	if f.editing < 0 || f.editing >= len(cw.cfg.ACPAgents) {
		return
	}
	idx := f.editing
	name := cw.cfg.ACPAgents[idx].Name
	cw.askConfirm("删除连接", "确定删除「"+name+"」？", func() {
		cw.acpCloseConn(name)
		cw.cfg.ACPAgents = append(cw.cfg.ACPAgents[:idx], cw.cfg.ACPAgents[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearACPForm()
		cw.toast("连接已删除", mcore.SeveritySuccess)
	})
}
