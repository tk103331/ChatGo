package ui

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/shellenv"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

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

type acpCatalogEntry struct {
	Name        string
	Command     string
	Args        []string
	Description string
	Homepage    string
}

func acpCatalog() []acpCatalogEntry {
	return []acpCatalogEntry{
		{Name: "ClaudeCode", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}, Description: "Anthropic Claude Code", Homepage: "https://claude.com"},
		{Name: "ClaudeAgentSDK", Command: "npx", Args: []string{"-y", "@zed-industries/claude-agent-acp"}, Description: "Claude Agent SDK via Zed 适配器", Homepage: "https://github.com/zed-industries/claude-agent-acp"},
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}, Description: "OpenCode AI 编程助手", Homepage: "https://github.com/sst/opencode"},
		{Name: "CodexCLI", Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp"}, Description: "OpenAI Codex CLI", Homepage: "https://github.com/openai/codex"},
		{Name: "Cursor", Command: "agent", Args: []string{"acp"}, Description: "Cursor CLI Agent", Homepage: "https://cursor.com"},
		{Name: "GeminiCLI", Command: "gemini", Description: "Google Gemini CLI", Homepage: "https://github.com/google-gemini/gemini-cli"},
		{Name: "KiroCLI", Command: "kiro-cli", Args: []string{"acp"}, Description: "AWS Kiro CLI", Homepage: "https://kiro.dev"},
		{Name: "GitHubCopilot", Command: "copilot", Description: "GitHub Copilot CLI", Homepage: "https://github.com/features/copilot"},
		{Name: "Goose", Command: "goose", Description: "Block Goose AI Agent", Homepage: "https://block.github.io/goose/"},
		{Name: "QwenCode", Command: "qwen", Description: "阿里通义灵码 Qwen Code", Homepage: "https://github.com/QwenLM/qwen-code"},
		{Name: "KimiCLI", Command: "kimi", Description: "月之暗面 Kimi CLI", Homepage: "https://github.com/MoonshotAI/kimi-cli"},
		{Name: "Cline", Command: "npx", Args: []string{"-y", "@cline/cline-acp-adapter"}, Description: "Cline AI 助手", Homepage: "https://cline.bot"},
		{Name: "Dockercagent", Command: "cagent", Description: "Docker cagent", Homepage: "https://github.com/docker/cagent"},
		{Name: "FactoryDroid", Command: "droid", Description: "Factory Droid", Homepage: "https://factory.ai"},
		{Name: "AugmentCode", Command: "augment", Args: []string{"acp"}, Description: "Augment Code CLI", Homepage: "https://docs.augmentcode.com/cli/acp"},
		{Name: "OpenHands", Command: "openhands", Args: []string{"acp"}, Description: "OpenHands AI Agent", Homepage: "https://docs.openhands.dev"},
		{Name: "QoderCLI", Command: "qoder", Args: []string{"acp"}, Description: "Qoder CLI", Homepage: "https://docs.qoder.com/cli/acp"},
		{Name: "PiAgent", Command: "npx", Args: []string{"-y", "pi-acp"}, Description: "Pi coding agent ACP 适配器", Homepage: "https://github.com/victor-software-house/pi-acp"},
		{Name: "DeepSeekHarness", Command: "npx", Args: []string{"-y", "deepseek-acp"}, Description: "DeepSeek Harness ACP 适配器", Homepage: "https://github.com/xintaofei/deepseek-acp"},
		{Name: "OhMyPi", Command: "omp", Args: []string{"acp"}, Description: "Oh My Pi coding agent", Homepage: "https://omp.sh"},
	}
}

func (cw *ChatWindow) hasACPConn(name string) bool {
	for _, a := range cw.cfg.ACPAgents {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

func (cw *ChatWindow) addACPFromCatalog(e acpCatalogEntry) {
	if cw.hasACPConn(e.Name) {
		cw.toast("已存在同名连接", mcore.SeverityInfo)
		return
	}
	disabled := true
	switch e.Name {
	case "PiAgent", "CodexCLI":
		disabled = false
	}
	cw.cfg.ACPAgents = append(cw.cfg.ACPAgents, config.ACPConnection{
		Name: e.Name, Command: e.Command, Args: e.Args, Disabled: disabled,
	})
	_ = config.SaveConfig(cw.cfg)
	cw.acpCatalogOpen = false
	if disabled {
		cw.toast("已添加 "+e.Name+"，请先安装对应 CLI 再启用", mcore.SeveritySuccess)
	} else {
		cw.toast("已添加并启用 "+e.Name, mcore.SeveritySuccess)
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
		Title: "从目录添加 Agent",
		Width: 560,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.acpCatalogQuery, kit.SearchInputOptions{Placeholder: "搜索 Agent", Label: "搜索"})
			in.Element.FillWidth()
			q := strings.ToLower(strings.TrimSpace(cw.acpCatalogQuery))
			var items []acpCatalogEntry
			for _, e := range acpCatalog() {
				if q != "" && !strings.Contains(strings.ToLower(e.Name), q) && !strings.Contains(strings.ToLower(e.Description), q) {
					continue
				}
				items = append(items, e)
			}
			if len(items) == 0 {
				mgo.Text(c, "没有匹配的 Agent").TextColor(k.TextMuted)
				return
			}
			mgo.Scroll(c).MaxHeight(400).Children(func() {
				mgo.Column(c).Gap(u).Children(func() {
					for _, e := range items {
						e := e
						mgo.Row(c).Key(e.Name).FillWidth().Gap(u*2).AlignItems(mgo.Center).
							Padding(u, u*2).Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border).
							Children(func() {
								acpIcon(c, e.Name, 24, 24)
								mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
									mgo.Text(c, e.Name).FontWeight(600).SingleLine()
									mgo.Text(c, e.Command+" "+strings.Join(e.Args, " ")).
										Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
									if e.Description != "" {
										mgo.Text(c, e.Description).FontSize(caption).TextColor(k.TextMuted).MaxLines(1)
									}
								})
								if cw.hasACPConn(e.Name) {
									mgo.Text(c, "已添加").FontSize(caption).TextColor(k.Success).Shrink(0)
								} else if kit.Button(c, "添加", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
									cw.addACPFromCatalog(e)
								}
							})
					}
				})
			})
		})
	})
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
				mgo.Text(c, a.Name).Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
			})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				en := !a.Disabled
				if kit.Switch(c, &en, "", kit.SwitchOptions{}).Changed() {
					cw.cfg.ACPAgents[i].Disabled = !en
					cw.acpForm.disabled = !en
					_ = config.SaveConfig(cw.cfg)
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
	mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).Shrink(0).TextColor(mcore.Tokens(c).AccentText)
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
