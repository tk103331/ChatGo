package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/mcp"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/devtools"
	mfeedback "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/feedback"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// settingsState holds the settings dialog's open tab and edit forms.
type settingsState struct {
	tab         int
	prov        providerForm
	mcp         mcpForm
	builtin     builtinForm
	shortcut    shortcutForm
	channel     channelForm
	provList    mgo.ListState
	builtinList mgo.ListState
	channelList mgo.ListState
}

type channelForm struct {
	editing   int
	name      string
	typ       string
	agent     string
	workspace string
	appID     string
	appSecret string
	enabled   bool
	scanning  bool
	qr        *mgo.Bitmap
	qrTip     string
}

type shortcutForm struct {
	value string
}

type providerForm struct {
	editing  int
	name     string
	typ      string
	apiKey   string
	baseURL  string
	models   []string
	newModel string
	enabled  bool
	preset   bool
}

type mcpForm struct {
	editing int
	name    string
	typ     string
	enabled bool
	command string
	args    string
	env     []devtools.KeyValue
	url     string
	headers []devtools.KeyValue
	timeout string
	hint    string
}

type builtinForm struct {
	editing int
	enabled bool
	config  map[string]string
}

// confirmState is a reusable confirmation dialog.
type confirmState struct {
	open   bool
	active bool
	title  string
	desc   string
	action func()
}

var providerTypes = []string{"openai", "anthropic", "claude", "ollama", "custom", "qwen", "deepseek", "gemini"}
var mcpTypes = []string{"stdio", "sse", "streamable_http"}

// settingsPage shows the tabbed configuration page.
func (cw *ChatWindow) settingsPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "设置").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		if cw.status != "" {
			mgo.Text(c, cw.status).TextColor(mcore.Tokens(c).Danger).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
		}
		res := mnavigation.Tabs(c, &cw.settings.tab, []mnavigation.Tab{
			{Label: "模型", Panel: func() { cw.providersTab(c) }},
			{Label: "内置工具", Panel: func() { cw.builtinTab(c) }},
			{Label: "ACP", Panel: func() { cw.acpTab(c) }},
			{Label: "快捷键", Panel: func() { cw.shortcutTab(c) }},
			{Label: "外观", Panel: func() { cw.appearanceTab(c) }},
			{Label: "系统", Panel: func() { cw.systemTab(c) }},
			{Label: "渠道", Panel: func() { cw.channelTab(c) }},
		}, mnavigation.TabsOptions{})
		res.Element.Grow(1).MinHeight(0)
	})
}

// confirmDialog shows the pending confirmation and runs its action. It keeps
// building for one frame after closing so the dialog releases the focus.
func (cw *ChatWindow) confirmDialog(c *mgo.Context) {
	if !cw.confirm.open && !cw.confirm.active {
		return
	}
	res := moverlay.AlertDialog(c, &cw.confirm.open, moverlay.AlertDialogOptions{
		Title:        cw.confirm.title,
		Description:  cw.confirm.desc,
		ConfirmLabel: "删除",
	})
	cw.confirm.active = cw.confirm.open
	if res.Confirmed && cw.confirm.action != nil {
		cw.confirm.action()
		cw.confirm.action = nil
	}
}

func (cw *ChatWindow) askConfirm(title, desc string, action func()) {
	cw.confirm = confirmState{open: true, active: true, title: title, desc: desc, action: action}
}

// providersTab lists providers and edits the selected one.
func (cw *ChatWindow) providersTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).Fill().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
		mgo.Column(c).Width(280).Shrink(0).Gap(u).Children(func() {
			if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearProviderForm()
			}
			mgo.List(c, &cw.settings.provList, len(cw.cfg.Providers), func(i int) {
				cw.providerRow(c, i)
			}).Fill()
		})
		cw.providerFormView(c)
	})
}

func (cw *ChatWindow) providerRow(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	p := cw.cfg.Providers[i]
	b := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Radius(mtheme.ControlRadius)
	if cw.settings.prov.editing == i {
		b.Background(k.Selection)
	}
	b.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.Start).Gap(u).AlignItems(mgo.Center).Children(func() {
			if svg := providerLogo(p.Type); svg != nil {
				mgo.Icon(c, svg).Size(16, 16).TextColor(k.Text).Shrink(0)
			} else {
				mgo.Icon(c, icons.Must("sliders-horizontal")).Size(16, 16).TextColor(k.TextMuted).Shrink(0)
			}
			mgo.Text(c, fmt.Sprintf("%s (%s)", p.Name, p.Type)).SingleLine().MinWidth(0).Grow(1).Ellipsis("…")
			status, col := "禁用", k.TextMuted
			if p.Enabled {
				status, col = "启用", k.Success
			}
			mgo.Text(c, status).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(col).Shrink(0)
		})
	})
	if b.Clicked() {
		cw.loadProviderForm(i)
	}
}

func (cw *ChatWindow) providerFormView(c *mgo.Context) {
	f := &cw.settings.prov
	u := mcore.Density(c).Unit()
	changed := false
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		mgo.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			mgo.Column(c).Gap(u * 2).Children(func() {
				if r := minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "Provider 名称", ReadOnly: f.preset}); r.Changed() {
					changed = true
				}
				opts := make([]minput.SelectOption[string], 0, len(providerTypes))
				for _, t := range providerTypes {
					opts = append(opts, minput.SelectOption[string]{Value: t, Label: t})
				}
				if r := minput.Select(c, &f.typ, opts, minput.SelectOptions{Placeholder: "类型", Label: "类型", Disabled: f.preset}); r.Changed() {
					changed = true
				}
				if r := minput.PasswordInput(c, &f.apiKey, minput.PasswordInputOptions{Label: "API Key", Placeholder: "API Key"}); r.Changed() {
					changed = true
				}
				if r := minput.TextInput(c, &f.baseURL, minput.TextInputOptions{Label: "Base URL", Placeholder: "Base URL"}); r.Changed() {
					changed = true
				}
				mgo.Text(c, "模型").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
				for _, m := range f.models {
					m := m
					mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
						mgo.Text(c, m).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						if mcore.IconAction(c, icons.Must("x"), "移除模型").Clicked() {
							f.models = removeStr(f.models, m)
							changed = true
						}
					})
				}
				mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
					in := minput.TextInput(c, &f.newModel, minput.TextInputOptions{Placeholder: "添加模型名"})
					in.Element.Grow(1)
					if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
						if name := strings.TrimSpace(f.newModel); name != "" {
							if !containsStr(f.models, name) {
								f.models = append(f.models, name)
							}
							f.newModel = ""
							changed = true
						}
					}
				})
			})
		})
		mgo.Row(c).Grow(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.saveProvider()
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteProvider()
			}
			mgo.Box(c).Grow(1)
			if minput.Switch(c, &f.enabled, "启用", minput.SwitchOptions{}).Changed() {
				changed = true
			}
		})
	})
	if changed && f.editing >= 0 {
		cw.persistProvider()
	}
}

func (cw *ChatWindow) loadProviderForm(i int) {
	p := cw.cfg.Providers[i]
	cw.settings.prov = providerForm{
		editing: i, name: p.Name, typ: p.Type, apiKey: p.APIKey,
		baseURL: p.BaseURL, models: append([]string(nil), p.ModelsList()...), enabled: p.Enabled,
		preset: p.Preset || providerIsPreset(p),
	}
}

// providerIsPreset reports whether a provider matches a shipped example.
func providerIsPreset(p config.Provider) bool {
	for _, pr := range config.ProviderPresets() {
		if pr.Name == p.Name && pr.Type == p.Type {
			return true
		}
	}
	return false
}

func (cw *ChatWindow) clearProviderForm() {
	cw.settings.prov = providerForm{editing: -1, typ: "openai", enabled: false}
}

func (cw *ChatWindow) saveProvider() {
	if cw.persistProvider() {
		cw.status = ""
		cw.toast("已保存模型 "+cw.settings.prov.name, mcore.SeveritySuccess)
	}
}

// persistProvider writes the edited provider into cfg and saves it.
func (cw *ChatWindow) persistProvider() bool {
	f := &cw.settings.prov
	if strings.TrimSpace(f.name) == "" {
		return false
	}
	model := ""
	if len(f.models) > 0 {
		model = f.models[0]
	}
	p := config.Provider{
		Name: f.name, Type: f.typ, APIKey: f.apiKey,
		BaseURL: f.baseURL, Model: model, Models: append([]string(nil), f.models...), Enabled: f.enabled, Preset: f.preset,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.Providers) {
		cw.cfg.Providers[f.editing] = p
	} else {
		cw.cfg.Providers = append(cw.cfg.Providers, p)
		f.editing = len(cw.cfg.Providers) - 1
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.fail("保存失败: " + err.Error())
		return false
	}
	if cw.provider == p.Name {
		cw.clientDirty.Store(true)
	}
	return true
}

// removeStr returns s without v.
func removeStr(s []string, v string) []string {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func (cw *ChatWindow) deleteProvider() {
	f := &cw.settings.prov
	if f.editing < 0 || f.editing >= len(cw.cfg.Providers) {
		return
	}
	idx := f.editing
	name := cw.cfg.Providers[idx].Name
	cw.askConfirm("删除模型", "确定删除「"+name+"」？", func() {
		cw.cfg.Providers = append(cw.cfg.Providers[:idx], cw.cfg.Providers[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearProviderForm()
		if cw.provider == name {
			cw.clientDirty.Store(true)
		}
	})
}

// mcpTab shows the configured MCP servers as a card grid.
func (cw *ChatWindow) mcpTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, fmt.Sprintf("已配置 %d 个服务", len(cw.cfg.MCPServers))).
				TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Grow(1)
			if minput.Button(c, "新增服务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearMCPForm()
				cw.mcpEditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for i := range cw.cfg.MCPServers {
					cw.mcpServerCard(c, i)
				}
			})
		})
	})
}

func (cw *ChatWindow) mcpServerCard(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	s := cw.cfg.MCPServers[i]
	typ := string(s.Type)
	if typ == "" {
		typ = "stdio"
	}
	mgo.Column(c).Key("mcp:"+s.Name).Gap(u*2).Padding(u*3).FillWidth().
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
		Children(func() {
			mgo.Row(c).AlignItems(mgo.Center).Gap(u).Children(func() {
				if svg := mcpLogo(); svg != nil {
					mgo.Icon(c, svg).Size(18, 18).TextColor(k.Text)
				}
				mgo.Text(c, s.Name).Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
				status, _ := mcpStatus(cw, s.Name)
				mfeedback.StatusIndicator(c, status, mfeedback.StatusIndicatorOptions{})
			})
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				agentTag(c, mcpTypeLabel(typ))
				agentTag(c, map[bool]string{true: "已启用", false: "已禁用"}[s.Enabled])
			})
			if s.Command != "" {
				mgo.Text(c, s.Command+" "+strings.Join(s.Args, " ")).TextColor(k.TextMuted).
					FontSize(caption).SingleLine().Ellipsis("…").MaxLines(1)
			} else if s.URL != "" {
				mgo.Text(c, s.URL).TextColor(k.TextMuted).FontSize(caption).SingleLine().Ellipsis("…").MaxLines(1)
			}
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				if st, ok := cw.mcpManager.GetServerStatus(s.Name); ok && st.Status == "initialized" && len(st.Tools) > 0 {
					if minput.Button(c, "工具 ("+strconv.Itoa(len(st.Tools))+")", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
						cw.openMCPTools(s.Name)
					}
				}
				if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					cw.loadMCPForm(i)
					cw.mcpEditorOpen = true
				}
				if minput.Button(c, "初始化", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
					cw.initMCPServer(s.Name)
				}
				if minput.Button(c, "断开", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
					_ = cw.mcpManager.manager.DisconnectServer(s.Name)
					cw.toast("已断开 "+s.Name, mcore.SeverityInfo)
				}
				if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
					cw.deleteMCPServer(s.Name)
				}
			})
		})
}

// openMCPTools snapshots an initialized server's tools for the dialog.
func (cw *ChatWindow) openMCPTools(name string) {
	st, ok := cw.mcpManager.GetServerStatus(name)
	if !ok {
		return
	}
	cw.mcpToolsServer = name
	cw.mcpTools = append([]mcp.MCPTool(nil), st.Tools...)
	cw.mcpToolsOpen = true
}

// mcpToolsDialog lists the tools an initialized server exposes.
func (cw *ChatWindow) mcpToolsDialog(c *mgo.Context) {
	if !cw.mcpToolsOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	moverlay.Dialog(c, &cw.mcpToolsOpen, moverlay.DialogOptions{
		Title: cw.mcpToolsServer + " · 工具 (" + strconv.Itoa(len(cw.mcpTools)) + ")",
		Width: 640,
	}, func() {
		mgo.Scroll(c).MaxHeight(420).Children(func() {
			mgo.Column(c).Gap(u * 2).Children(func() {
				if len(cw.mcpTools) == 0 {
					mgo.Text(c, "该服务器没有可用工具").TextColor(k.TextMuted)
					return
				}
				for _, tl := range cw.mcpTools {
					tl := tl
					mgo.Column(c).Gap(u).Padding(u*2).FillWidth().
						Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
						Children(func() {
							mgo.Text(c, tl.Name).FontWeight(600).Font(mtheme.MonoFont)
							if tl.Description != "" {
								mgo.Text(c, tl.Description).TextColor(k.TextMuted).FontSize(caption)
							}
						})
				}
			})
		})
	})
}

func (cw *ChatWindow) initMCPServer(name string) {
	s, ok := cw.findMCPServer(name)
	if !ok {
		return
	}
	go func() {
		status, err := cw.mcpManager.manager.InitializeServer(s)
		cw.postMain(func() {
			switch {
			case err != nil:
				cw.toast("初始化失败："+err.Error(), mcore.SeverityError)
			default:
				cw.toast(fmt.Sprintf("%s 初始化成功（%d 个工具）", name, len(status.Tools)), mcore.SeveritySuccess)
			}
			cw.clientDirty.Store(true)
		})
	}()
}

func (cw *ChatWindow) findMCPServer(name string) (config.MCPServer, bool) {
	for _, s := range cw.cfg.MCPServers {
		if s.Name == name {
			return s, true
		}
	}
	return config.MCPServer{}, false
}

func (cw *ChatWindow) deleteMCPServer(name string) {
	cw.askConfirm("删除 MCP 服务器", "确定删除「"+name+"」？", func() {
		_ = cw.mcpManager.manager.DisconnectServer(name)
		for i, s := range cw.cfg.MCPServers {
			if s.Name == name {
				cw.cfg.MCPServers = append(cw.cfg.MCPServers[:i], cw.cfg.MCPServers[i+1:]...)
				break
			}
		}
		_ = config.SaveConfig(cw.cfg)
		cw.clientDirty.Store(true)
		cw.toast("已删除 "+name, mcore.SeveritySuccess)
	})
}

// mcpEditor is the create/edit dialog for a server.
func (cw *ChatWindow) mcpEditor(c *mgo.Context) {
	f := &cw.settings.mcp
	moverlay.Dialog(c, &cw.mcpEditorOpen, moverlay.DialogOptions{
		Title: "MCP 服务配置",
		Width: 560,
		Actions: func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if cw.saveMCP() {
					cw.mcpEditorOpen = false
				}
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteMCP()
			}
		},
	}, func() { cw.mcpFormFields(c) })
}

func (cw *ChatWindow) mcpFormFields(c *mgo.Context) {
	f := &cw.settings.mcp
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Gap(u * 2).Children(func() {
		if f.hint != "" {
			mgo.Text(c, f.hint).TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Font(mtheme.MonoFont)
		}
		minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "服务器名称"})
		typ := 0
		for i, t := range mcpTypes {
			if t == f.typ {
				typ = i
			}
		}
		mnavigation.SegmentedControl(c, &typ, []mnavigation.Segment{
			{Label: "STDIO"}, {Label: "SSE"}, {Label: "HTTP"},
		}, mnavigation.SegmentedControlOptions{Label: "类型", Equal: true})
		f.typ = mcpTypes[typ]
		if minput.Switch(c, &f.enabled, "启用", minput.SwitchOptions{}).Changed() {
			cw.saveMCP()
		}
		if f.typ == "stdio" {
			minput.TextInput(c, &f.command, minput.TextInputOptions{Label: "命令", Placeholder: "如 npx"})
			minput.TextArea(c, &f.args, minput.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 72})
			mgo.Text(c, "环境变量").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.env, devtools.KeyValueInputOptions{Label: "环境变量", KeyPlaceholder: "KEY", ValuePlaceholder: "VALUE"})
		} else {
			minput.TextInput(c, &f.url, minput.TextInputOptions{Label: "URL", Placeholder: "https://..."})
			mgo.Text(c, "请求头").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.headers, devtools.KeyValueInputOptions{Label: "请求头", KeyPlaceholder: "Header", ValuePlaceholder: "Value"})
			minput.TextInput(c, &f.timeout, minput.TextInputOptions{Label: "超时（秒）", Placeholder: "30"})
		}
		if f.editing >= 0 && f.editing < len(cw.cfg.MCPServers) {
			status, note := mcpStatus(cw, cw.cfg.MCPServers[f.editing].Name)
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mfeedback.StatusIndicator(c, status, mfeedback.StatusIndicatorOptions{})
				if note != "" {
					mgo.Text(c, note).TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
		}
	})
}

func (cw *ChatWindow) loadMCPForm(i int) {
	s := cw.cfg.MCPServers[i]
	typ := string(s.Type)
	if typ == "" {
		typ = "stdio"
	}
	timeout := "30"
	if s.TimeoutSeconds > 0 {
		timeout = fmt.Sprintf("%d", s.TimeoutSeconds)
	}
	cw.settings.mcp = mcpForm{
		editing: i, name: s.Name, typ: typ, enabled: s.Enabled,
		command: s.Command, args: strings.Join(s.Args, "\n"), env: kvList(s.Env),
		url: s.URL, headers: kvList(s.Headers), timeout: timeout,
	}
}

func (cw *ChatWindow) clearMCPForm() {
	cw.settings.mcp = mcpForm{editing: -1, typ: "stdio", enabled: true, timeout: "30"}
}

func (cw *ChatWindow) saveMCP() bool {
	f := &cw.settings.mcp
	if strings.TrimSpace(f.name) == "" {
		cw.fail("名称不能为空")
		return false
	}
	s := config.MCPServer{Name: f.name, Type: config.MCPServerType(f.typ), Enabled: f.enabled}
	if f.typ == "stdio" {
		if strings.TrimSpace(f.command) == "" {
			cw.fail("stdio 类型需要命令")
			return false
		}
		s.Command = f.command
		s.Args = splitLines(f.args)
		s.Env = kvMap(f.env)
	} else {
		if strings.TrimSpace(f.url) == "" {
			cw.fail("该类型需要 URL")
			return false
		}
		s.URL = f.url
		s.Headers = kvMap(f.headers)
		_, _ = fmt.Sscanf(strings.TrimSpace(f.timeout), "%d", &s.TimeoutSeconds)
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.MCPServers) {
		old := cw.cfg.MCPServers[f.editing].Name
		cw.cfg.MCPServers[f.editing] = s
		if old != s.Name {
			_ = cw.mcpManager.manager.DisconnectServer(old)
		}
	} else {
		cw.cfg.MCPServers = append(cw.cfg.MCPServers, s)
		f.editing = len(cw.cfg.MCPServers) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.status = ""
	cw.toast("已保存 MCP 服务器 "+s.Name, mcore.SeveritySuccess)
	cw.clientDirty.Store(true)
	return true
}
func (cw *ChatWindow) deleteMCP() {
	f := &cw.settings.mcp
	if f.editing < 0 || f.editing >= len(cw.cfg.MCPServers) {
		return
	}
	idx := f.editing
	name := cw.cfg.MCPServers[idx].Name
	cw.askConfirm("删除 MCP 服务器", "确定删除「"+name+"」？", func() {
		_ = cw.mcpManager.manager.DisconnectServer(name)
		cw.cfg.MCPServers = append(cw.cfg.MCPServers[:idx], cw.cfg.MCPServers[idx+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearMCPForm()
		cw.mcpEditorOpen = false
		cw.clientDirty.Store(true)
	})
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func mcpTypeLabel(typ string) string {
	switch typ {
	case "stdio":
		return "STDIO"
	case "sse":
		return "SSE"
	default:
		return "HTTP"
	}
}

func kvList(m map[string]string) []devtools.KeyValue {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]devtools.KeyValue, 0, len(keys))
	for _, k := range keys {
		out = append(out, devtools.KeyValue{Key: k, Value: m[k], Enabled: true})
	}
	return out
}

func kvMap(rows []devtools.KeyValue) map[string]string {
	m := map[string]string{}
	for _, r := range rows {
		if r.Enabled && strings.TrimSpace(r.Key) != "" {
			m[strings.TrimSpace(r.Key)] = r.Value
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func parseKV(s string) map[string]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

func joinKV(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	lines := make([]string, 0, len(m))
	for k, v := range m {
		lines = append(lines, k+"="+v)
	}
	return strings.Join(lines, "\n")
}

// builtinTab lists the built-in tools and edits the selected one.
func (cw *ChatWindow) builtinTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).Fill().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
		mgo.Column(c).Width(280).Shrink(0).Children(func() {
			mgo.List(c, &cw.settings.builtinList, len(cw.cfg.BuiltinTools), func(i int) {
				cw.builtinRow(c, i)
			}).Fill()
		})
		cw.builtinFormView(c)
	})
}

func (cw *ChatWindow) builtinRow(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	t := cw.cfg.BuiltinTools[i]
	b := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Radius(mtheme.ControlRadius)
	if cw.settings.builtin.editing == i {
		b.Background(k.Selection)
	}
	b.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.Start).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, fmt.Sprintf("%s (%s)", t.Name, t.Type)).SingleLine().MinWidth(0).Grow(1).Ellipsis("…")
			status, col := "禁用", k.TextMuted
			if t.Enabled {
				status, col = "启用", k.Success
			}
			mgo.Text(c, status).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(col).Shrink(0)
		})
	})
	if b.Clicked() {
		cw.loadBuiltinForm(i)
	}
}

func (cw *ChatWindow) builtinFormView(c *mgo.Context) {
	f := &cw.settings.builtin
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		if f.editing < 0 || f.editing >= len(cw.cfg.BuiltinTools) {
			mgo.Text(c, "从左侧选择一个内置工具进行配置").TextColor(k.TextMuted)
			return
		}
		tool := cw.cfg.BuiltinTools[f.editing]
		mgo.Text(c, tool.Type).FontWeight(600)
		mgo.Text(c, config.GetBuiltinToolDescription(tool.Type)).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Switch(c, &f.enabled, "启用", minput.SwitchOptions{}).Changed() {
			cw.saveBuiltin()
		}
		required := config.GetRequiredConfigFields(tool.Type)
		for _, field := range config.GetBuiltinToolConfigFields(tool.Type) {
			label := field
			for _, r := range required {
				if r == field {
					label = field + " *"
				}
			}
			v := f.config[field]
			minput.TextInput(c, &v, minput.TextInputOptions{Label: label, Placeholder: field})
			f.config[field] = v
		}
		if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
			cw.saveBuiltin()
		}
	})
}

func (cw *ChatWindow) loadBuiltinForm(i int) {
	t := cw.cfg.BuiltinTools[i]
	cfgMap := map[string]string{}
	for k, v := range t.Config {
		cfgMap[k] = v
	}
	cw.settings.builtin = builtinForm{editing: i, enabled: t.Enabled, config: cfgMap}
}

func (cw *ChatWindow) saveBuiltin() {
	f := &cw.settings.builtin
	if f.editing < 0 || f.editing >= len(cw.cfg.BuiltinTools) {
		return
	}
	t := cw.cfg.BuiltinTools[f.editing]
	t.Enabled = f.enabled
	t.Config = f.config
	if t.Enabled {
		if err := config.ValidateBuiltinToolConfig(t); err != nil {
			cw.fail(err.Error())
			return
		}
	}
	cw.cfg.BuiltinTools[f.editing] = t
	_ = config.SaveConfig(cw.cfg)
	cw.status = ""
	cw.toast("已保存内置工具配置", mcore.SeveritySuccess)
	cw.clientDirty.Store(true)
}

// shortcutTab edits the global shortcut that opens the focused interface.
func (cw *ChatWindow) shortcutTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "快捷键").Font(mtheme.SerifFont).FontWeight(600)
		if cw.status != "" {
			mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(small).
				Font(mtheme.MonoFont)
		}
		mgo.Text(c, "快捷对话").FontSize(small).TextColor(k.TextMuted)
		f := &cw.settings.shortcut
		minput.TextInput(c, &f.value, minput.TextInputOptions{
			Label: "快捷对话快捷键", Placeholder: defaultQuickShortcut,
		})
		mgo.Text(c, "示例：CmdOrCtrl+Shift+Space、Alt+Space、CmdOrCtrl+Shift+K").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.cfg.QuickChatShortcut = strings.TrimSpace(f.value)
				cw.saveQuickShortcut()
				cw.toast("快捷键已更新", mcore.SeveritySuccess)
			}
			if minput.Button(c, "恢复默认", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				f.value = defaultQuickShortcut
				cw.cfg.QuickChatShortcut = defaultQuickShortcut
				cw.saveQuickShortcut()
				cw.toast("已恢复默认快捷键", mcore.SeveritySuccess)
			}
		})
	})
}

// appearanceTab changes the theme mode and language.
func (cw *ChatWindow) appearanceTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "外观").Font(mtheme.SerifFont).FontWeight(600)
		mgo.Text(c, "主题模式").FontSize(small).TextColor(k.TextMuted)
		mode := int(cw.themeMode)
		mnavigation.SegmentedControl(c, &mode, []mnavigation.Segment{
			{Label: "跟随系统"}, {Label: "浅色"}, {Label: "深色"},
		}, mnavigation.SegmentedControlOptions{Label: "主题模式", Equal: true})
		if mcore.Mode(mode) != cw.themeMode {
			cw.themeMode = mcore.Mode(mode)
			cw.saveAppearance()
		}
		mgo.Text(c, "语言").FontSize(small).TextColor(k.TextMuted)
		loc := 0
		if cw.locale == mcore.EnUS {
			loc = 1
		}
		mnavigation.SegmentedControl(c, &loc, []mnavigation.Segment{
			{Label: "中文"}, {Label: "English"},
		}, mnavigation.SegmentedControlOptions{Label: "语言", Equal: true})
		want := mcore.ZhCN
		if loc == 1 {
			want = mcore.EnUS
		}
		if want != cw.locale {
			cw.locale = want
			cw.saveAppearance()
		}
	})
}

func (cw *ChatWindow) systemTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "系统").Font(mtheme.SerifFont).FontWeight(600)
		if minput.Switch(c, &cw.cfg.ShowTrayIcon, "显示托盘图标", minput.SwitchOptions{}).Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "在系统托盘 / 菜单栏显示 ChatGo 图标。点击图标打开程序，右键菜单可快速新建会话并进入最近会话。").
			FontSize(small).TextColor(k.TextMuted)
		if minput.Switch(c, &cw.cfg.RunInBackground, "后台运行（关闭窗口时最小化到托盘）", minput.SwitchOptions{}).Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "关闭窗口时不退出程序，而是隐藏到托盘继续后台运行，并隐藏任务栏 / Dock 图标（自动启用托盘图标）。").
			FontSize(small).TextColor(k.TextMuted)
	})
}

func (cw *ChatWindow) saveSystemSettings() {
	if cw.cfg.RunInBackground {
		cw.cfg.ShowTrayIcon = true
	}
	_ = config.SaveConfig(cw.cfg)
	cw.syncTray()
	cw.toast("系统设置已保存", mcore.SeveritySuccess)
}

func (cw *ChatWindow) channelTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).Fill().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
		mgo.Column(c).Width(280).Shrink(0).Gap(u).Children(func() {
			cw.feishuStatusLine(c)
			cw.wechatStatusLine(c)
			if minput.Button(c, "新增", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearChannelForm()
			}
			mgo.List(c, &cw.settings.channelList, len(cw.cfg.Channels), func(i int) {
				cw.channelRow(c, i)
			}).Fill()
		})
		cw.channelFormView(c)
	})
}

func (cw *ChatWindow) feishuStatusLine(c *mgo.Context) {
	state, note := cw.feishuStatus()
	st, label := feishuStatusView(state, note)
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "飞书连接").Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
		mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
		mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
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

func (cw *ChatWindow) wechatStatusLine(c *mgo.Context) {
	state, note := cw.wechatStatus()
	st, label := wechatStatusView(state, note)
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "微信 ClawBot").Font(mtheme.SerifFont).FontWeight(600).Shrink(0)
		mfeedback.StatusIndicator(c, st, mfeedback.StatusIndicatorOptions{})
		mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
	})
}

func wechatStatusView(state feishuConnState, note string) (mfeedback.Status, string) {
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

func (cw *ChatWindow) channelRow(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	ch := cw.cfg.Channels[i]
	b := mgo.ButtonBase(c).FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Radius(mtheme.ControlRadius)
	if cw.settings.channel.editing == i {
		b.Background(k.Selection)
	}
	b.Children(func() {
		mgo.Row(c).FillWidth().Justify(mgo.Start).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, fmt.Sprintf("%s (%s)", ch.Name, ch.Type)).SingleLine().MinWidth(0).Grow(1).Ellipsis("…")
			status, col := "禁用", k.TextMuted
			if ch.Enabled {
				status, col = "启用", k.Success
			}
			mgo.Text(c, status).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(col).Shrink(0)
		})
	})
	if b.Clicked() {
		cw.loadChannelForm(i)
	}
}

func (cw *ChatWindow) channelFormView(c *mgo.Context) {
	f := &cw.settings.channel
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "渠道名称"})
		minput.Select(c, &f.typ, []minput.SelectOption[string]{
			{Value: "feishu", Label: "飞书"},
			{Value: "wechat", Label: "微信 ClawBot"},
		}, minput.SelectOptions{Label: "类型"})
		if f.typ == "wechat" {
			minput.TextInput(c, &f.appID, minput.TextInputOptions{Label: "OpenClaw 地址", Placeholder: "http://localhost:3000"})
			minput.PasswordInput(c, &f.appSecret, minput.PasswordInputOptions{Label: "令牌", Placeholder: "OpenClaw 鉴权令牌（可选）"})
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				if minput.Button(c, "扫码绑定微信", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("globe"), Disabled: f.scanning}).Clicked() {
					cw.startWechatBind()
				}
				if f.scanning {
					mgo.Text(c, "等待微信扫码授权…").TextColor(mcore.Tokens(c).TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
			mgo.Text(c, "提示：需先安装本地 OpenClaw 实例并执行 npx -y @tencent-weixin/openclaw-weixin-cli@latest install 安装微信通道插件。").
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
			mgo.Text(c, "提示：请在飞书开发者后台 → 事件与回调 → 订阅方式，选择「使用长连接接收事件/回调」，并添加事件 im.message.receive_v1（保存时需本地长连接在线）。").
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
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteChannel()
			}
			mgo.Box(c).Grow(1)
			if minput.Switch(c, &f.enabled, "启用", minput.SwitchOptions{}).Changed() {
				cw.applyChannel()
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
		appID: ch.AppID, appSecret: ch.AppSecret, enabled: ch.Enabled,
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
	ch := config.Channel{Type: typ, Name: f.name, Agent: f.agent, Workspace: strings.TrimSpace(f.workspace), AppID: f.appID, AppSecret: f.appSecret, Enabled: f.enabled}
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
func mcpStatus(cw *ChatWindow, name string) (mfeedback.Status, string) {
	st, ok := cw.mcpManager.GetServerStatus(name)
	if !ok {
		return mfeedback.StatusOffline, "未初始化"
	}
	switch st.Status {
	case "initialized":
		return mfeedback.StatusSuccess, fmt.Sprintf("%d 个工具", len(st.Tools))
	case "error":
		note := "初始化失败"
		if st.Error != nil {
			note = st.Error.Error()
		}
		return mfeedback.StatusError, note
	default:
		return mfeedback.StatusBusy, st.Status
	}
}

// askDeleteConversation opens the delete confirmation for a conversation.
func (cw *ChatWindow) askDeleteConversation(id, title string) {
	cw.askConfirm("删除会话", "确定删除「"+title+"」？此操作不可撤销。", func() {
		cw.acpConvClose(id)
		_ = cw.convManager.DeleteConversation(id)
		if cw.cur != nil && cw.cur.ID == id {
			cw.cur = nil
			cw.mode = modeHome
		}
		cw.reloadConvs()
		cw.toast("会话已删除", mcore.SeveritySuccess)
	})
}

// acpTab configures the ACP connections used by ACP agents.
func (cw *ChatWindow) acpTab(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).Fill().Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
		mgo.Column(c).Width(300).Shrink(0).Children(func() {
			mgo.List(c, &cw.acpList, len(cw.cfg.ACPAgents), func(i int) {
				cw.acpRow(c, i)
			}).Fill()
		})
		mgo.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			cw.acpFormView(c)
		})
	})
}

func (cw *ChatWindow) fail(msg string) {
	cw.status = msg
	cw.toast(msg, mcore.SeverityError)
}
