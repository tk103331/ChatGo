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

type acpCatalogEntry struct {
	Name        string
	Command     string
	Args        []string
	Description string
	Homepage    string
	// Env carries the extra environment an adapter needs — paseo's catalog
	// entries set things like disabling a CLI's own auto-update, which
	// otherwise races the app's own upgrade button.
	Env map[string]string
}

// acpCatalog lists the ACP agents the app knows how to start, with the
// command line each one's own documentation says speaks ACP — checked
// against paseo's catalog (packages/app/src/data/acp-provider-catalog.ts):
// a CLI started without its acp subcommand would open an interactive
// session instead of the protocol.
func acpCatalog() []acpCatalogEntry {
	return []acpCatalogEntry{
		{Name: "ClaudeCode", Command: "npx", Args: []string{"-y", "@zed-industries/claude-code-acp"}, Description: "Anthropic Claude Code", Homepage: "https://claude.com"},
		{Name: "ClaudeAgentSDK", Command: "npx", Args: []string{"-y", "@zed-industries/claude-agent-acp"}, Description: "Claude Agent SDK via Zed 适配器", Homepage: "https://github.com/zed-industries/claude-agent-acp"},
		{Name: "OpenCode", Command: "opencode", Args: []string{"acp"}, Description: "OpenCode AI 编程助手", Homepage: "https://github.com/sst/opencode"},
		{Name: "CodexCLI", Command: "npx", Args: []string{"-y", "@agentclientprotocol/codex-acp"}, Description: "OpenAI Codex CLI", Homepage: "https://github.com/openai/codex"},
		{Name: "Cursor", Command: "cursor-agent", Args: []string{"acp"}, Description: "Cursor CLI Agent", Homepage: "https://cursor.com"},
		{Name: "GeminiCLI", Command: "gemini", Args: []string{"--acp"}, Description: "Google Gemini CLI", Homepage: "https://github.com/google-gemini/gemini-cli"},
		{Name: "KiroCLI", Command: "kiro-cli", Args: []string{"acp"}, Description: "AWS Kiro CLI", Homepage: "https://kiro.dev"},
		{Name: "GitHubCopilot", Command: "copilot", Description: "GitHub Copilot CLI", Homepage: "https://github.com/features/copilot"},
		{Name: "Goose", Command: "goose", Args: []string{"acp"}, Description: "Block Goose AI Agent", Homepage: "https://block.github.io/goose/"},
		{Name: "QwenCode", Command: "qwen", Args: []string{"--acp"}, Description: "阿里通义灵码 Qwen Code", Homepage: "https://github.com/QwenLM/qwen-code"},
		{Name: "KimiCLI", Command: "kimi", Args: []string{"acp"}, Description: "月之暗面 Kimi Code CLI", Homepage: "https://github.com/MoonshotAI/kimi-cli"},
		{Name: "GLMAgent", Command: "npx", Args: []string{"-y", "glm-acp-agent"}, Description: "智谱 GLM Coding Plan ACP Agent（glm-5.3 系列）", Homepage: "https://github.com/stefandevo/glm-acp-agent"},
		{Name: "Grok", Command: "grok", Args: []string{"agent", "stdio"}, Description: "xAI Grok Build（需 SuperGrok 或 X Premium+ 订阅）", Homepage: "https://docs.x.ai/build/overview"},
		{Name: "Junie", Command: "junie", Args: []string{"--acp", "true"}, Description: "JetBrains Junie CLI", Homepage: "https://junie.jetbrains.com/docs/junie-cli-acp.html"},
		{Name: "Kilo", Command: "kilo", Args: []string{"acp"}, Description: "Kilo 开源编程 Agent", Homepage: "https://kilo.ai/docs/code-with-ai/platforms/cli"},
		{Name: "MiniMaxCode", Command: "npx", Args: []string{"-y", "-p", "@minimax-ai/code", "mcode", "acp"}, Description: "MiniMax Code 终端编程 Agent", Homepage: "https://agent.minimax.io"},
		{Name: "DevinCLI", Command: "devin", Args: []string{"acp"}, Description: "Cognition Devin 终端版", Homepage: "https://cli.devin.ai"},
		{Name: "Amp", Command: "amp-acp", Description: "Amp 前沿编程 Agent 的 ACP 包装", Homepage: "https://github.com/tao12345666333/amp-acp"},
		{Name: "AugmentCode", Command: "npx", Args: []string{"-y", "@augmentcode/auggie", "--acp"}, Env: map[string]string{"AUGMENT_DISABLE_AUTO_UPDATE": "1"}, Description: "Augment Code Auggie CLI", Homepage: "https://www.augmentcode.com/"},
		{Name: "TRAECLI", Command: "traecli", Args: []string{"acp", "serve"}, Description: "字节跳动 TRAE CLI", Homepage: "https://docs.trae.cn/cli_get-started-with-trae-cli"},
		{Name: "CodebuddyCode", Command: "codebuddy", Args: []string{"--acp"}, Description: "腾讯云 CodeBuddy CLI", Homepage: "https://www.codebuddy.cn/cli/"},
		{Name: "CortexCode", Command: "cortex", Args: []string{"acp", "serve"}, Description: "Snowflake Cortex Code", Homepage: "https://docs.snowflake.com/en/user-guide/cortex-code/cortex-code-cli"},
		{Name: "CodeWhale", Command: "codewhale", Args: []string{"serve", "--acp"}, Description: "CodeWhale 终端编程 Agent", Homepage: "https://codewhale.net/"},
		{Name: "CrowCLI", Command: "crow-cli", Args: []string{"acp"}, Description: "Crow 极简 ACP 编程 Agent", Homepage: "https://crow-ai.dev/"},
		{Name: "DeepAgents", Command: "npx", Args: []string{"-y", "deepagents-acp"}, Description: "LangChain DeepAgents", Homepage: "https://docs.langchain.com/oss/javascript/deepagents/overview"},
		{Name: "DimCode", Command: "npx", Args: []string{"-y", "dimcode", "acp"}, Description: "DimCode 编程 Agent", Homepage: "https://dimcode.dev"},
		{Name: "Dirac", Command: "npx", Args: []string{"-y", "dirac-cli", "--acp"}, Description: "Dirac 开源编程 Agent", Homepage: "https://dirac.run"},
		{Name: "FastAgent", Command: "uvx", Args: []string{"--from", "fast-agent-acp", "fast-agent-acp", "-x"}, Description: "fast-agent 多后端编程 Agent", Homepage: "https://fast-agent.ai/acp/"},
		{Name: "Hermes", Command: "hermes", Args: []string{"acp"}, Description: "Nous Research Hermes 自进化 Agent", Homepage: "https://hermes-agent.nousresearch.com"},
		{Name: "MinionCode", Command: "uvx", Args: []string{"--from", "minion-code", "minion-code", "acp"}, Description: "Minion Code 编程助手", Homepage: "https://github.com/femto/minion-code"},
		{Name: "MistralVibe", Command: "vibe-acp", Description: "Mistral Vibe 开源编程助手", Homepage: "https://github.com/mistralai/mistral-vibe"},
		{Name: "Nova", Command: "npx", Args: []string{"-y", "@compass-ai/nova", "acp"}, Description: "Compass AI Nova", Homepage: "https://www.compassap.ai/portfolio/nova.html"},
		{Name: "Poolside", Command: "pool", Args: []string{"acp"}, Description: "Poolside 编程 Agent", Homepage: "https://docs.poolside.ai/cli/pool"},
		{Name: "Stakpak", Command: "stakpak", Args: []string{"acp"}, Description: "Stakpak 开源 Rust DevOps Agent", Homepage: "https://stakpak.dev/"},
		{Name: "VTCode", Command: "vtcode", Args: []string{"acp"}, Env: map[string]string{"VT_ACP_ENABLED": "1"}, Description: "VT Code 开源编程 Agent", Homepage: "https://github.com/vinhnx/VTCode"},
		{Name: "Autohand", Command: "npx", Args: []string{"-y", "@autohandai/autohand-acp"}, Description: "Autohand Code", Homepage: "https://www.autohand.ai/cli/"},
		{Name: "PiG", Command: "pig-acp", Description: "PiG — Pi 的 Go 实现（需先 pig install npm:@pi-in-go/pigpen-acp 并 pig acp install 构建 pig-acp）", Homepage: "https://pi-in-go.dev"},
		{Name: "Cline", Command: "npx", Args: []string{"-y", "cline", "--acp"}, Description: "Cline AI 助手", Homepage: "https://cline.bot"},
		{Name: "Dockercagent", Command: "cagent", Description: "Docker cagent", Homepage: "https://github.com/docker/cagent"},
		{Name: "FactoryDroid", Command: "droid", Args: []string{"exec", "--output-format", "acp-daemon"}, Env: map[string]string{"DROID_DISABLE_AUTO_UPDATE": "true", "FACTORY_DROID_AUTO_UPDATE_ENABLED": "false"}, Description: "Factory Droid", Homepage: "https://factory.ai"},
		{Name: "OpenHands", Command: "openhands", Args: []string{"acp"}, Description: "OpenHands AI Agent", Homepage: "https://docs.openhands.dev"},
		{Name: "QoderCLI", Command: "npx", Args: []string{"-y", "@qoder-ai/qodercli", "--acp"}, Description: "Qoder CLI", Homepage: "https://docs.qoder.com/cli/acp"},
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

// acpEntryAutoStarts reports whether a catalog entry can run the moment it
// is added: an npx or uvx command pulls its own package, so the user has
// nothing to install by hand, and a binary the shell already resolves needs
// no install either. Anything else has to be installed first, so that
// connection starts disabled with a hint instead of a warm-up that is
// bound to fail.
func acpEntryAutoStarts(e acpCatalogEntry) bool {
	base := strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(e.Command, ".exe"), ".cmd"))
	switch base {
	case "npx", "uvx":
		return true
	}
	_, ok := shellenv.ResolveCommand(e.Command)
	return ok
}

func (cw *ChatWindow) addACPFromCatalog(e acpCatalogEntry) {
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
