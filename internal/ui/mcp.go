package ui

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/devtools"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

//go:embed mcp_servers.json
var mcpMarketData []byte

// mcpMarketInstall is one installation method from the marketplace.
type mcpMarketInstall struct {
	Kind    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// mcpMarketExample is one usage example from the marketplace.
type mcpMarketExample struct {
	Title       string
	Description string
	Prompt      string
}

// mcpMarketArg describes one env or header parameter the entry needs.
type mcpMarketArg struct {
	Description string
	Required    bool
	Example     string
}

// mcpMarketServer is one installable server from the MCP marketplace.
type mcpMarketServer struct {
	ID            string
	Name          string
	Description   string
	Homepage      string
	Repository    string
	Author        string
	License       string
	Categories    []string
	Tags          []string
	Examples      []mcpMarketExample
	Installations []mcpMarketInstall
	Arguments     map[string]mcpMarketArg
	Command       string
	Args          []string
	Env           map[string]string
	InstallURL    string
	InstallHeader map[string]string
}

// mcpPage manages MCP servers and browses the marketplace.
func (cw *ChatWindow) mcpPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "MCP").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		mnavigation.SegmentedControl(c, &cw.mcp.tabSel, []mnavigation.Segment{
			{Label: "我的服务"}, {Label: "市场"},
		}, mnavigation.SegmentedControlOptions{Label: "MCP", Equal: true})
		if cw.mcp.tabSel == 1 {
			cw.mcpMarketTab(c)
			return
		}
		cw.mcpTab(c)
	})
	cw.mcpEditor(c)
	cw.mcpDetailDialog(c)
	cw.mcpToolsDialog(c)
}

// mcpDetailDialog shows one marketplace server's full metadata.
func (cw *ChatWindow) mcpDetailDialog(c *mgo.Context) {
	if !cw.mcp.detailOpen {
		return
	}
	d := cw.mcp.detail
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	meta := func(c *mgo.Context, label, val string) {
		if val != "" {
			mgo.Text(c, label+val).FontSize(caption).TextColor(k.TextMuted)
		}
	}
	moverlay.Dialog(c, &cw.mcp.detailOpen, moverlay.DialogOptions{
		Title: d.Name,
		Width: 640,
		Actions: func() {
			if !cw.hasMCPServer(d.Name) {
				if minput.Button(c, "添加到我的服务", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
					cw.editMarketServer(d)
				}
			}
			link := d.Homepage
			if link == "" {
				link = d.Repository
			}
			if link != "" && minput.Button(c, "打开主页", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				c.OpenURL(link)
			}
		},
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if d.Description != "" {
				mgo.Text(c, d.Description).TextColor(k.TextMuted)
			}
			meta(c, "作者：", d.Author)
			meta(c, "许可：", d.License)
			meta(c, "主页：", d.Homepage)
			meta(c, "仓库：", d.Repository)
			if len(d.Categories) > 0 {
				meta(c, "分类：", strings.Join(d.Categories, " / "))
			}
			if len(d.Tags) > 0 {
				meta(c, "标签：", strings.Join(d.Tags, ", "))
			}
			if len(d.Installations) > 0 {
				mgo.Text(c, "安装方式").FontWeight(600)
				for _, in := range d.Installations {
					line := in.Command
					if len(in.Args) > 0 {
						line += " " + strings.Join(in.Args, " ")
					}
					mgo.Column(c).Gap(u).Padding(u, u*2).Radius(mtheme.CardRadius).
						Border(mtheme.BorderWidth, k.Border).Children(func() {
						mgo.Text(c, in.Kind).FontSize(caption).TextColor(k.TextMuted)
						mgo.Text(c, line).Font(mtheme.MonoFont).FontSize(caption)
					})
				}
			}
			if len(d.Examples) > 0 {
				mgo.Text(c, "示例").FontWeight(600)
				for _, ex := range d.Examples {
					mgo.Text(c, "• "+ex.Prompt).FontSize(caption).TextColor(k.TextMuted)
				}
			}
		})
	})
}

func (cw *ChatWindow) mcpMarketTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if !cw.mcp.marketLoaded && !cw.mcp.marketLoading {
		cw.loadMCPMarket()
	}
	mgo.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			in := minput.TextInput(c, &cw.mcp.marketQuery, minput.TextInputOptions{Label: "搜索", Placeholder: "搜索 MCP 服务器"})
			in.Element.Grow(1)
			opts := []minput.SelectOption[string]{{Value: "", Label: "全部分类"}}
			for _, cat := range cw.mcpCategories() {
				opts = append(opts, minput.SelectOption[string]{Value: cat, Label: cat})
			}
			minput.Select(c, &cw.mcp.marketCategory, opts, minput.SelectOptions{Label: "分类"})
		})
		switch {
		case cw.mcp.marketLoading:
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "加载中"})
				mgo.Text(c, "正在加载市场数据…").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			})
			return
		case cw.mcp.marketErr != "":
			mgo.Text(c, "加载失败："+cw.mcp.marketErr).TextColor(k.Danger).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			return
		}
		items := cw.filteredMarket()
		if len(items) == 0 {
			mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("search")).Size(32, 32).TextColor(k.TextMuted)
				mgo.Text(c, "没有匹配的 MCP 服务器").TextColor(k.TextMuted)
			})
			return
		}
		mgo.Text(c, "共 "+strconv.Itoa(len(items))+" 个").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.List(c, &cw.mcp.marketList, len(items), func(i int) {
			cw.mcpMarketRow(c, items[i])
		}).Grow(1).MinHeight(0)
	})
}

func (cw *ChatWindow) mcpMarketRow(c *mgo.Context, s mcpMarketServer) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).Key(s.ID).FillWidth().Padding(u, u*2).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		mgo.Column(c).Grow(1).MinWidth(0).Gap(u).Children(func() {
			mgo.Text(c, s.Name).FontWeight(600).SingleLine()
			if s.Description != "" {
				mgo.Text(c, s.Description).TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MaxLines(2)
			}
			tags := append(append([]string{}, s.Categories...), s.Tags...)
			if len(tags) > 0 {
				mgo.Row(c).Gap(u).Wrap().Children(func() {
					for i, tag := range tags {
						if i >= 8 {
							break
						}
						agentTag(c, tag)
					}
				})
			}
		})
		if minput.Button(c, "详情", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
			cw.mcp.detail = s
			cw.mcp.detailOpen = true
		}
		if cw.hasMCPServer(s.Name) {
			mgo.Text(c, "已添加").TextColor(k.Success).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
			return
		}
		if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
			cw.editMarketServer(s)
		}
	})
}

func (cw *ChatWindow) mcpCategories() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range cw.mcp.market {
		for _, cat := range s.Categories {
			if cat != "" && !seen[cat] {
				seen[cat] = true
				out = append(out, cat)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (cw *ChatWindow) filteredMarket() []mcpMarketServer {
	q := strings.ToLower(strings.TrimSpace(cw.mcp.marketQuery))
	cat := cw.mcp.marketCategory
	if q == "" && cat == "" {
		return cw.mcp.market
	}
	var out []mcpMarketServer
	for _, s := range cw.mcp.market {
		if cat != "" && !containsStr(s.Categories, cat) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(s.Name), q) && !strings.Contains(strings.ToLower(s.Description), q) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (cw *ChatWindow) hasMCPServer(name string) bool {
	for _, s := range cw.cfg.MCPServers {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

// editMarketServer prefills the editor from a marketplace entry so the user
// can complete its arguments before adding.
func (cw *ChatWindow) editMarketServer(s mcpMarketServer) {
	cw.clearMCPForm()
	f := &cw.settings.mcp
	f.name = s.Name
	f.enabled = true
	f.hint = marketHint(s)
	if s.InstallURL != "" {
		f.typ = "streamable_http"
		f.url = s.InstallURL
		f.headers = marketKVRows(s.InstallHeader, s.Arguments)
	} else {
		f.typ = "stdio"
		f.command = s.Command
		f.args = strings.Join(s.Args, "\n")
		f.env = marketKVRows(s.Env, s.Arguments)
	}
	cw.mcp.detailOpen = false
	cw.mcp.editorOpen = true
}

// marketKVRows merges install values with the arguments' examples, keyed and sorted.
func marketKVRows(values map[string]string, args map[string]mcpMarketArg) []devtools.KeyValue {
	keys := map[string]bool{}
	for k := range values {
		keys[k] = true
	}
	for k := range args {
		keys[k] = true
	}
	if len(keys) == 0 {
		return nil
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]devtools.KeyValue, 0, len(names))
	for _, k := range names {
		v := values[k]
		if v == "" {
			v = args[k].Example
		}
		out = append(out, devtools.KeyValue{Key: k, Value: v, Enabled: true})
	}
	return out
}

// marketHint describes the installation and each parameter the entry needs.
func marketHint(s mcpMarketServer) string {
	var b strings.Builder
	if s.InstallURL != "" {
		b.WriteString("来自市场 · HTTP：" + s.InstallURL)
	} else {
		b.WriteString("来自市场 · " + s.Command)
		if len(s.Args) > 0 {
			b.WriteString(" " + strings.Join(s.Args, " "))
		}
	}
	if len(s.Arguments) > 0 {
		names := make([]string, 0, len(s.Arguments))
		for k := range s.Arguments {
			names = append(names, k)
		}
		sort.Strings(names)
		b.WriteString("\n参数说明：")
		for _, k := range names {
			a := s.Arguments[k]
			b.WriteString("\n· " + k)
			if a.Required {
				b.WriteString("（必填）")
			}
			if a.Description != "" {
				b.WriteString(" - " + a.Description)
			}
			if a.Example != "" {
				b.WriteString("  例：" + a.Example)
			}
		}
	}
	return b.String()
}

// loadMCPMarket parses the embedded marketplace data in the background.
func (cw *ChatWindow) loadMCPMarket() {
	cw.mcp.marketLoading = true
	cw.mcp.marketErr = ""
	go func() {
		var raw map[string]mcpMarketRaw
		if err := json.Unmarshal(mcpMarketData, &raw); err != nil {
			cw.postMain(func() { cw.mcp.marketLoading = false; cw.mcp.marketErr = err.Error() })
			return
		}
		list := make([]mcpMarketServer, 0, len(raw))
		for id, e := range raw {
			name := e.DisplayName
			if name == "" {
				name = e.Name
			}
			if name == "" {
				name = id
			}
			chosen, ok := pickInstallation(e.Installations)
			if !ok {
				continue
			}
			s := mcpMarketServer{
				ID: id, Name: name, Description: e.Description, Homepage: e.Homepage,
				Repository: e.Repository.URL, Author: e.Author.Name, License: e.License,
				Categories: e.Categories, Tags: e.Tags,
				Examples:      marketExamples(e.Examples),
				Arguments:     marketArgs(e.Arguments),
				Installations: marketInstalls(e.Installations),
			}
			if chosen.URL != "" {
				s.InstallURL = chosen.URL
				s.InstallHeader = chosen.Headers
			} else {
				s.Command, s.Args, s.Env = chosen.Command, chosen.Args, chosen.Env
			}
			list = append(list, s)
		}
		sort.Slice(list, func(i, j int) bool {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
		cw.postMain(func() {
			cw.mcp.market = list
			cw.mcp.marketLoaded = true
			cw.mcp.marketLoading = false
		})
	}()
}

type mcpMarketRawInstall struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type mcpMarketRawExample struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

type mcpMarketRawArg struct {
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Example     string `json:"example"`
}

type mcpMarketRaw struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Homepage    string   `json:"homepage"`
	License     string   `json:"license"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	Repository  struct {
		URL string `json:"url"`
	} `json:"repository"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
	Examples      []mcpMarketRawExample          `json:"examples"`
	Arguments     map[string]mcpMarketRawArg     `json:"arguments"`
	Installations map[string]mcpMarketRawInstall `json:"installations"`
}

func marketInstalls(m map[string]mcpMarketRawInstall) []mcpMarketInstall {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]mcpMarketInstall, 0, len(keys))
	for _, k := range keys {
		in := m[k]
		out = append(out, mcpMarketInstall{
			Kind: k, Command: in.Command, Args: in.Args,
			Env: in.Env, URL: in.URL, Headers: in.Headers,
		})
	}
	return out
}

func marketArgs(in map[string]mcpMarketRawArg) map[string]mcpMarketArg {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]mcpMarketArg, len(in))
	for k, a := range in {
		out[k] = mcpMarketArg{Description: a.Description, Required: a.Required, Example: a.Example}
	}
	return out
}

func marketExamples(in []mcpMarketRawExample) []mcpMarketExample {
	out := make([]mcpMarketExample, 0, len(in))
	for _, e := range in {
		if e.Prompt == "" {
			continue
		}
		out = append(out, mcpMarketExample{Title: e.Title, Description: e.Description, Prompt: e.Prompt})
	}
	return out
}

func pickInstallation(installations map[string]mcpMarketRawInstall) (mcpMarketInstall, bool) {
	for _, pref := range []string{"npm", "uvx", "python", "cli", "docker"} {
		if in, ok := installations[pref]; ok && in.Command != "" {
			return mcpMarketInstall{Kind: pref, Command: in.Command, Args: in.Args, Env: in.Env}, true
		}
	}
	for _, in := range installations {
		if in.URL != "" {
			return mcpMarketInstall{Kind: "http", URL: in.URL, Headers: in.Headers}, true
		}
	}
	for _, in := range installations {
		if in.Command != "" && in.Type != "custom" {
			return mcpMarketInstall{Kind: in.Type, Command: in.Command, Args: in.Args, Env: in.Env}, true
		}
	}
	return mcpMarketInstall{}, false
}
