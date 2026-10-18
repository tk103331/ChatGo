// Package mcppage renders the MCP page: a "my services" tab listing configured
// MCP servers and a marketplace tab browsing installable servers. The State
// struct (page state + editor form) is owned by the host window; data and
// side effects flow through the Options callbacks.
package mcppage

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/mcp"
	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/common"

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

// Manager is the MCP server lifecycle surface the page needs.
type Manager interface {
	GetServerStatus(name string) (*mcp.MCPServerStatus, bool)
	InitializeServer(s config.MCPServer) (*mcp.MCPServerStatus, error)
	DisconnectServer(name string) error
}

// Form is the create/edit form for one MCP server.
type Form struct {
	Editing  int
	Name     string
	Typ      string
	Enabled  bool
	Command  string
	Args     string
	Env      []devtools.KeyValue
	URL      string
	Headers  []devtools.KeyValue
	Timeout  string
	Hint     string
}

// State is the MCP page's mutable state, owned by the host window.
type State struct {
	TabSel     int
	EditorOpen bool
	MyQuery    string
	MyList     mgo.ListState
	Form       Form

	Market         []MarketServer
	MarketList     mgo.ListState
	MarketLoading  bool
	MarketLoaded   bool
	MarketErr      string
	MarketQuery    string
	MarketCategory string
	DetailOpen     bool
	Detail         MarketServer

	ToolsOpen   bool
	ToolsServer string
	Tools       []mcp.MCPTool
}

// Options supplies the page with host data and actions.
type Options struct {
	Config     func() *config.Config
	Manager    Manager
	Toast      func(msg string, sev mcore.Severity)
	Fail       func(msg string)
	Status     func() string
	SetStatus  func(s string)
	AskConfirm func(title, desc string, action func())
	MarkDirty  func()
	PostMain   func(fn func())
}

var mcpTypes = []string{"stdio", "sse", "streamable_http"}

// View renders the MCP page with its two tabs and dialogs.
func View(c *mgo.Context, state *State, opts Options) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "MCP").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		mnavigation.SegmentedControl(c, &state.TabSel, []mnavigation.Segment{
			{Label: "我的服务"}, {Label: "市场"},
		}, mnavigation.SegmentedControlOptions{Label: "MCP", Equal: true})
		if state.TabSel == 1 {
			marketTab(c, state, opts)
			return
		}
		servicesTab(c, state, opts)
	})
	editorDialog(c, state, opts)
	detailDialog(c, state, opts)
	toolsDialog(c, state)
}

func servicesTab(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u * 2).Children(func() {
			in := minput.SearchInput(c, &state.MyQuery, minput.SearchInputOptions{Placeholder: "搜索服务", Label: "搜索服务"})
			in.Element.Grow(1)
			if minput.Button(c, "新增服务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				clearForm(state)
				state.EditorOpen = true
			}
		})
		cfg := opts.Config()
		if len(cfg.MCPServers) == 0 {
			emptyState(c, state)
			return
		}
		items := filteredServers(cfg, state)
		if len(items) == 0 {
			mgo.Text(c, "没有匹配的服务").TextColor(k.TextMuted)
			return
		}
		mgo.Text(c, "共 "+strconv.Itoa(len(items))+" 个服务").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for _, i := range items {
					serverCard(c, state, opts, i)
				}
			})
		})
	})
}

func emptyState(c *mgo.Context, state *State) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("layout-grid")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有配置 MCP 服务").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "从市场安装，或手动添加一个服务器").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Row(c).Gap(u * 2).Children(func() {
			if minput.Button(c, "浏览市场", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				state.TabSel = 1
			}
			if minput.Button(c, "手动添加", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				clearForm(state)
				state.EditorOpen = true
			}
		})
	})
}

func filteredServers(cfg *config.Config, state *State) []int {
	q := strings.ToLower(strings.TrimSpace(state.MyQuery))
	if q == "" {
		out := make([]int, len(cfg.MCPServers))
		for i := range cfg.MCPServers {
			out[i] = i
		}
		return out
	}
	var out []int
	for i, s := range cfg.MCPServers {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Command), q) ||
			strings.Contains(strings.ToLower(s.URL), q) {
			out = append(out, i)
		}
	}
	return out
}

func serverStatus(opts Options, name string) (mfeedback.Status, string) {
	st, ok := opts.Manager.GetServerStatus(name)
	if !ok {
		return mfeedback.StatusOffline, "未初始化"
	}
	switch st.Status {
	case "initialized":
		return mfeedback.StatusSuccess, strconv.Itoa(len(st.Tools)) + " 个工具"
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

func serverCard(c *mgo.Context, state *State, opts Options, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cfg := opts.Config()
	s := cfg.MCPServers[i]
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
				status, note := serverStatus(opts, s.Name)
				mfeedback.StatusIndicator(c, status, mfeedback.StatusIndicatorOptions{})
				if note != "" && note != "未初始化" {
					mgo.Text(c, note).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				common.Tag(c, typeLabel(typ))
				if s.Enabled {
					common.Tag(c, "已启用")
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
				if st, ok := opts.Manager.GetServerStatus(s.Name); ok && st.Status == "initialized" && len(st.Tools) > 0 {
					if minput.Button(c, "工具 ("+strconv.Itoa(len(st.Tools))+")", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
						openTools(state, st)
					}
				}
				if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					loadForm(state, cfg, i)
					state.EditorOpen = true
				}
				if minput.Button(c, "初始化", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
					initServer(state, opts, s.Name)
				}
				if minput.Button(c, "断开", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
					_ = opts.Manager.DisconnectServer(s.Name)
					opts.Toast("已断开 "+s.Name, mcore.SeverityInfo)
				}
				if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
					deleteServer(state, opts, s.Name)
				}
			})
		})
}

func openTools(state *State, st *mcp.MCPServerStatus) {
	state.ToolsServer = st.Name
	state.Tools = append([]mcp.MCPTool(nil), st.Tools...)
	state.ToolsOpen = true
}

func toolsDialog(c *mgo.Context, state *State) {
	if !state.ToolsOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	moverlay.Dialog(c, &state.ToolsOpen, moverlay.DialogOptions{
		Title: state.ToolsServer + " · 工具 (" + strconv.Itoa(len(state.Tools)) + ")",
		Width: 640,
	}, func() {
		mgo.Scroll(c).MaxHeight(420).Children(func() {
			mgo.Column(c).Gap(u * 2).Children(func() {
				if len(state.Tools) == 0 {
					mgo.Text(c, "该服务器没有可用工具").TextColor(k.TextMuted)
					return
				}
				for _, tl := range state.Tools {
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

func initServer(state *State, opts Options, name string) {
	s, ok := findServer(opts.Config(), name)
	if !ok {
		return
	}
	go func() {
		status, err := opts.Manager.InitializeServer(s)
		opts.PostMain(func() {
			switch {
			case err != nil:
				opts.Toast("初始化失败："+err.Error(), mcore.SeverityError)
			default:
				opts.Toast(name+" 初始化成功（"+strconv.Itoa(len(status.Tools))+" 个工具）", mcore.SeveritySuccess)
			}
			opts.MarkDirty()
		})
	}()
}

func findServer(cfg *config.Config, name string) (config.MCPServer, bool) {
	for _, s := range cfg.MCPServers {
		if s.Name == name {
			return s, true
		}
	}
	return config.MCPServer{}, false
}

func deleteServer(state *State, opts Options, name string) {
	opts.AskConfirm("删除 MCP 服务器", "确定删除「"+name+"」？", func() {
		_ = opts.Manager.DisconnectServer(name)
		cfg := opts.Config()
		for i, s := range cfg.MCPServers {
			if s.Name == name {
				cfg.MCPServers = append(cfg.MCPServers[:i], cfg.MCPServers[i+1:]...)
				break
			}
		}
		_ = config.SaveConfig(cfg)
		opts.MarkDirty()
		opts.Toast("已删除 "+name, mcore.SeveritySuccess)
	})
}

func editorDialog(c *mgo.Context, state *State, opts Options) {
	f := &state.Form
	moverlay.Dialog(c, &state.EditorOpen, moverlay.DialogOptions{
		Title: "MCP 服务配置",
		Width: 560,
		Actions: func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if saveForm(state, opts) {
					state.EditorOpen = false
				}
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.Editing < 0}).Clicked() {
				deleteFormServer(state, opts)
			}
		},
	}, func() { formFields(c, state, opts) })
}

func formFields(c *mgo.Context, state *State, opts Options) {
	f := &state.Form
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	cfg := opts.Config()
	mgo.Column(c).Gap(u * 2).Children(func() {
		if f.Hint != "" {
			mgo.Text(c, f.Hint).TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Font(mtheme.MonoFont)
		}
		minput.TextInput(c, &f.Name, minput.TextInputOptions{Label: "名称", Placeholder: "服务器名称"})
		typ := 0
		for i, t := range mcpTypes {
			if t == f.Typ {
				typ = i
			}
		}
		mnavigation.SegmentedControl(c, &typ, []mnavigation.Segment{
			{Label: "STDIO"}, {Label: "SSE"}, {Label: "HTTP"},
		}, mnavigation.SegmentedControlOptions{Label: "类型", Equal: true})
		f.Typ = mcpTypes[typ]
		if minput.Switch(c, &f.Enabled, "启用", minput.SwitchOptions{}).Changed() {
			saveForm(state, opts)
		}
		if f.Typ == "stdio" {
			minput.TextInput(c, &f.Command, minput.TextInputOptions{Label: "命令", Placeholder: "如 npx"})
			minput.TextArea(c, &f.Args, minput.TextAreaOptions{Label: "参数", Placeholder: "每行一个参数", MinHeight: 72})
			mgo.Text(c, "环境变量").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.Env, devtools.KeyValueInputOptions{Label: "环境变量", KeyPlaceholder: "KEY", ValuePlaceholder: "VALUE"})
		} else {
			minput.TextInput(c, &f.URL, minput.TextInputOptions{Label: "URL", Placeholder: "https://..."})
			mgo.Text(c, "请求头").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			devtools.KeyValueInput(c, &f.Headers, devtools.KeyValueInputOptions{Label: "请求头", KeyPlaceholder: "Header", ValuePlaceholder: "Value"})
			minput.TextInput(c, &f.Timeout, minput.TextInputOptions{Label: "超时（秒）", Placeholder: "30"})
		}
		if f.Editing >= 0 && f.Editing < len(cfg.MCPServers) {
			status, note := serverStatus(opts, cfg.MCPServers[f.Editing].Name)
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mfeedback.StatusIndicator(c, status, mfeedback.StatusIndicatorOptions{})
				if note != "" {
					mgo.Text(c, note).TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
			})
		}
	})
}

func loadForm(state *State, cfg *config.Config, i int) {
	s := cfg.MCPServers[i]
	typ := string(s.Type)
	if typ == "" {
		typ = "stdio"
	}
	timeout := "30"
	if s.TimeoutSeconds > 0 {
		timeout = strconv.Itoa(s.TimeoutSeconds)
	}
	state.Form = Form{
		Editing: i, Name: s.Name, Typ: typ, Enabled: s.Enabled,
		Command: s.Command, Args: strings.Join(s.Args, "\n"), Env: kvList(s.Env),
		URL: s.URL, Headers: kvList(s.Headers), Timeout: timeout,
	}
}

func clearForm(state *State) {
	state.Form = Form{Editing: -1, Typ: "stdio", Enabled: true, Timeout: "30"}
}

func saveForm(state *State, opts Options) bool {
	f := &state.Form
	cfg := opts.Config()
	if strings.TrimSpace(f.Name) == "" {
		opts.Fail("名称不能为空")
		return false
	}
	s := config.MCPServer{Name: f.Name, Type: config.MCPServerType(f.Typ), Enabled: f.Enabled}
	if f.Typ == "stdio" {
		if strings.TrimSpace(f.Command) == "" {
			opts.Fail("stdio 类型需要命令")
			return false
		}
		s.Command = f.Command
		s.Args = splitLines(f.Args)
		s.Env = kvMap(f.Env)
	} else {
		if strings.TrimSpace(f.URL) == "" {
			opts.Fail("该类型需要 URL")
			return false
		}
		s.URL = f.URL
		s.Headers = kvMap(f.Headers)
		_, _ = fmt.Sscanf(strings.TrimSpace(f.Timeout), "%d", &s.TimeoutSeconds)
	}
	if f.Editing >= 0 && f.Editing < len(cfg.MCPServers) {
		old := cfg.MCPServers[f.Editing].Name
		cfg.MCPServers[f.Editing] = s
		if old != s.Name {
			_ = opts.Manager.DisconnectServer(old)
		}
	} else {
		cfg.MCPServers = append(cfg.MCPServers, s)
		f.Editing = len(cfg.MCPServers) - 1
	}
	_ = config.SaveConfig(cfg)
	opts.SetStatus("")
	opts.Toast("已保存 MCP 服务器 "+s.Name, mcore.SeveritySuccess)
	opts.MarkDirty()
	return true
}

func deleteFormServer(state *State, opts Options) {
	f := &state.Form
	cfg := opts.Config()
	if f.Editing < 0 || f.Editing >= len(cfg.MCPServers) {
		return
	}
	idx := f.Editing
	name := cfg.MCPServers[idx].Name
	opts.AskConfirm("删除 MCP 服务器", "确定删除「"+name+"」？", func() {
		_ = opts.Manager.DisconnectServer(name)
		cfg.MCPServers = append(cfg.MCPServers[:idx], cfg.MCPServers[idx+1:]...)
		_ = config.SaveConfig(cfg)
		clearForm(state)
		state.EditorOpen = false
		opts.MarkDirty()
	})
}

func typeLabel(typ string) string {
	switch typ {
	case "stdio":
		return "STDIO"
	case "sse":
		return "SSE"
	default:
		return "HTTP"
	}
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
