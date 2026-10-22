package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/mcp"
	"chatgo/internal/ui/assets"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/devtools"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
)

func (cw *ChatWindow) mcpTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u * 2).Children(func() {
			in := kit.SearchInput(c, &cw.mcp.myQuery, kit.SearchInputOptions{Placeholder: "搜索服务", Label: "搜索服务"})
			in.Element.Grow(1)
			if kit.Button(c, "新增服务", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearMCPForm()
				cw.mcp.editorOpen = true
			}
		})
		if len(cw.cfg.MCPServers) == 0 {
			cw.mcpEmptyState(c)
			return
		}
		items := cw.filteredMCPServers()
		if len(items) == 0 {
			mgo.Text(c, "没有匹配的服务").TextColor(k.TextMuted)
			return
		}
		mgo.Text(c, fmt.Sprintf("共 %d 个服务", len(items))).TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for _, i := range items {
					cw.mcpServerCard(c, i)
				}
			})
		})
	})
}

// mcpEmptyState shows a centered call-to-action when no servers are configured.

func (cw *ChatWindow) mcpEmptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("layout-grid")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有配置 MCP 服务").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "从市场安装，或手动添加一个服务器").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Row(c).Gap(u * 2).Children(func() {
			if kit.Button(c, "浏览市场", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.mcp.tabSel = 1
			}
			if kit.Button(c, "手动添加", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
				cw.clearMCPForm()
				cw.mcp.editorOpen = true
			}
		})
	})
}

// filteredMCPServers returns indices of configured servers matching the search query.

func (cw *ChatWindow) filteredMCPServers() []int {
	q := strings.ToLower(strings.TrimSpace(cw.mcp.myQuery))
	if q == "" {
		out := make([]int, len(cw.cfg.MCPServers))
		for i := range cw.cfg.MCPServers {
			out[i] = i
		}
		return out
	}
	var out []int
	for i, s := range cw.cfg.MCPServers {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Command), q) ||
			strings.Contains(strings.ToLower(s.URL), q) {
			out = append(out, i)
		}
	}
	return out
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
				if svg := assets.MCPLogo(); svg != nil {
					mgo.Icon(c, svg).Size(18, 18).TextColor(k.Text)
				}
				mgo.Text(c, s.Name).Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
				status, note := mcpStatus(cw, s.Name)
				kit.StatusIndicator(c, status, kit.StatusIndicatorOptions{})
				if note != "" && note != "未初始化" {
					mgo.Text(c, note).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				agentTag(c, mcpTypeLabel(typ))
				if s.Enabled {
					agentTag(c, "已启用")
				} else {
					mgo.Text(c, "已禁用").FontSize(caption).TextColor(k.TextMuted).
						Padding(0, u*1.5).Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, k.Border).SingleLine()
				}
			})
			if s.Command != "" {
				mgo.Text(c, s.Command+" "+strings.Join(s.Args, " ")).TextColor(k.TextMuted).
					FontSize(caption).SingleLine().Ellipsis("…").MaxLines(1)
			} else if s.URL != "" {
				mgo.Text(c, s.URL).TextColor(k.TextMuted).FontSize(caption).SingleLine().Ellipsis("…").MaxLines(1)
			}
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				if st, ok := cw.mcpManager.GetServerStatus(s.Name); ok && st.Status == "initialized" && len(st.Tools) > 0 {
					if kit.Button(c, "工具 ("+strconv.Itoa(len(st.Tools))+")", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
						cw.openMCPTools(s.Name)
					}
				}
				if kit.Button(c, "编辑", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
					cw.loadMCPForm(i)
					cw.mcp.editorOpen = true
				}
				if kit.Button(c, "初始化", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
					cw.initMCPServer(s.Name)
				}
				if kit.Button(c, "断开", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
					_ = cw.mcpManager.manager.DisconnectServer(s.Name)
					cw.toast("已断开 "+s.Name, mcore.SeverityInfo)
				}
				if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
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
	cw.mcp.toolsServer = name
	cw.mcp.tools = append([]mcp.MCPTool(nil), st.Tools...)
	cw.mcp.toolsOpen = true
}

// mcpToolsDialog lists the tools an initialized server exposes.

func (cw *ChatWindow) mcpToolsDialog(c *mgo.Context) {
	if !cw.mcp.toolsOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	moverlay.Dialog(c, &cw.mcp.toolsOpen, moverlay.DialogOptions{
		Title: cw.mcp.toolsServer + " · 工具 (" + strconv.Itoa(len(cw.mcp.tools)) + ")",
		Width: 640,
	}, func() {
		mgo.Scroll(c).MaxHeight(420).Children(func() {
			mgo.Column(c).Gap(u * 2).Children(func() {
				if len(cw.mcp.tools) == 0 {
					mgo.Text(c, "该服务器没有可用工具").TextColor(k.TextMuted)
					return
				}
				for _, tl := range cw.mcp.tools {
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
	moverlay.Dialog(c, &cw.mcp.editorOpen, moverlay.DialogOptions{
		Title: "MCP 服务配置",
		Width: 560,
		Actions: func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				if cw.saveMCP() {
					cw.mcp.editorOpen = false
				}
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger, Disabled: f.editing < 0}).Clicked() {
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
		kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "服务器名称"})
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
		if kit.Switch(c, &f.enabled, "启用", kit.SwitchOptions{}).Changed() {
			cw.saveMCP()
		}
		if f.typ == "stdio" {
			kit.TextInput(c, &f.command, kit.TextInputOptions{Label: "命令", Placeholder: "如 npx"})
			kit.TextArea(c, &f.args, kit.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 72})
			mgo.Text(c, "环境变量").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.env, devtools.KeyValueInputOptions{Label: "环境变量", KeyPlaceholder: "KEY", ValuePlaceholder: "VALUE"})
		} else {
			kit.TextInput(c, &f.url, kit.TextInputOptions{Label: "URL", Placeholder: "https://..."})
			mgo.Text(c, "请求头").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.headers, devtools.KeyValueInputOptions{Label: "请求头", KeyPlaceholder: "Header", ValuePlaceholder: "Value"})
			kit.TextInput(c, &f.timeout, kit.TextInputOptions{Label: "超时（秒）", Placeholder: "30"})
		}
		if f.editing >= 0 && f.editing < len(cw.cfg.MCPServers) {
			status, note := mcpStatus(cw, cw.cfg.MCPServers[f.editing].Name)
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				kit.StatusIndicator(c, status, kit.StatusIndicatorOptions{})
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
		cw.mcp.editorOpen = false
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

func mcpStatus(cw *ChatWindow, name string) (kit.Status, string) {
	st, ok := cw.mcpManager.GetServerStatus(name)
	if !ok {
		return kit.StatusOffline, "未初始化"
	}
	switch st.Status {
	case "initialized":
		return kit.StatusSuccess, fmt.Sprintf("%d 个工具", len(st.Tools))
	case "error":
		note := "初始化失败"
		if st.Error != nil {
			note = st.Error.Error()
		}
		return kit.StatusError, note
	default:
		return kit.StatusBusy, st.Status
	}
}

// askDeleteConversation opens the delete confirmation for a conversation.

type mcpState struct {
	tabSel         int
	editorOpen     bool
	myQuery        string
	myList         mgo.ListState
	market         []mcpMarketServer
	marketList     mgo.ListState
	marketLoading  bool
	marketLoaded   bool
	marketErr      string
	marketQuery    string
	marketCategory string
	detailOpen     bool
	detail         mcpMarketServer
	toolsOpen      bool
	toolsServer    string
	tools          []mcp.MCPTool
}
