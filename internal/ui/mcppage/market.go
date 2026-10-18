package mcppage

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/common"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/devtools"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

//go:embed mcp_servers.json
var marketData []byte

// MarketInstall is one installation method from the marketplace.
type MarketInstall struct {
	Kind    string
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// MarketExample is one usage example from the marketplace.
type MarketExample struct {
	Title       string
	Description string
	Prompt      string
}

// MarketArg describes one env or header parameter the entry needs.
type MarketArg struct {
	Description string
	Required    bool
	Example     string
}

// MarketServer is one installable server from the MCP marketplace.
type MarketServer struct {
	ID            string
	Name          string
	Description   string
	Homepage      string
	Repository    string
	Author        string
	License       string
	Categories    []string
	Tags          []string
	Examples      []MarketExample
	Installations []MarketInstall
	Arguments     map[string]MarketArg
	Command       string
	Args          []string
	Env           map[string]string
	InstallURL    string
	InstallHeader map[string]string
}

func marketTab(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if !state.MarketLoaded && !state.MarketLoading {
		loadMarket(state, opts)
	}
	mgo.Column(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			in := minput.TextInput(c, &state.MarketQuery, minput.TextInputOptions{Label: "搜索", Placeholder: "搜索 MCP 服务器"})
			in.Element.Grow(1)
			opts := []minput.SelectOption[string]{{Value: "", Label: "全部分类"}}
			for _, cat := range categories(state) {
				opts = append(opts, minput.SelectOption[string]{Value: cat, Label: cat})
			}
			minput.Select(c, &state.MarketCategory, opts, minput.SelectOptions{Label: "分类"})
		})
		switch {
		case state.MarketLoading:
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "加载中"})
				mgo.Text(c, "正在加载市场数据…").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			})
			return
		case state.MarketErr != "":
			mgo.Text(c, "加载失败："+state.MarketErr).TextColor(k.Danger).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			return
		}
		items := filteredMarket(state)
		if len(items) == 0 {
			mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("search")).Size(32, 32).TextColor(k.TextMuted)
				mgo.Text(c, "没有匹配的 MCP 服务器").TextColor(k.TextMuted)
			})
			return
		}
		mgo.Text(c, "共 "+strconv.Itoa(len(items))+" 个").TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.List(c, &state.MarketList, len(items), func(i int) {
			marketRow(c, state, opts, items[i])
		}).Grow(1).MinHeight(0)
	})
}

func marketRow(c *mgo.Context, state *State, opts Options, s MarketServer) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).Key(s.ID).FillWidth().Padding(u, u*2).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		main := mgo.ButtonBase(c).Grow(1).MinWidth(0).MinHeight(mcore.Density(c).ControlHeight()).
			Padding(0, u).Radius(mtheme.ControlRadius).Label(s.Name)
		if main.Hovered() {
			main.Background(k.SurfaceHover)
		}
		main.Children(func() {
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
							common.Tag(c, tag)
						}
					})
				}
			})
		})
		if main.Clicked() {
			state.Detail = s
			state.DetailOpen = true
		}
		if minput.Button(c, "详情", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
			state.Detail = s
			state.DetailOpen = true
		}
		if hasServer(opts.Config(), s.Name) {
			mgo.Text(c, "已添加").TextColor(k.Success).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Shrink(0)
			return
		}
		if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
			editMarketServer(state, s)
		}
	})
}

func detailDialog(c *mgo.Context, state *State, opts Options) {
	if !state.DetailOpen {
		return
	}
	d := state.Detail
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	meta := func(c *mgo.Context, label, val string) {
		if val != "" {
			mgo.Text(c, label+val).FontSize(caption).TextColor(k.TextMuted)
		}
	}
	moverlay.Dialog(c, &state.DetailOpen, moverlay.DialogOptions{
		Title: d.Name,
		Width: 640,
		Actions: func() {
			if !hasServer(opts.Config(), d.Name) {
				if minput.Button(c, "添加到我的服务", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
					editMarketServer(state, d)
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

func categories(state *State) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range state.Market {
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

func filteredMarket(state *State) []MarketServer {
	q := strings.ToLower(strings.TrimSpace(state.MarketQuery))
	cat := state.MarketCategory
	if q == "" && cat == "" {
		return state.Market
	}
	var out []MarketServer
	for _, s := range state.Market {
		if cat != "" && !common.ContainsStr(s.Categories, cat) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(s.Name), q) && !strings.Contains(strings.ToLower(s.Description), q) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func hasServer(cfg *config.Config, name string) bool {
	for _, s := range cfg.MCPServers {
		if strings.EqualFold(s.Name, name) {
			return true
		}
	}
	return false
}

func editMarketServer(state *State, s MarketServer) {
	clearForm(state)
	f := &state.Form
	f.Name = s.Name
	f.Enabled = true
	f.Hint = marketHint(s)
	if s.InstallURL != "" {
		f.Typ = "streamable_http"
		f.URL = s.InstallURL
		f.Headers = marketKVRows(s.InstallHeader, s.Arguments)
	} else {
		f.Typ = "stdio"
		f.Command = s.Command
		f.Args = strings.Join(s.Args, "\n")
		f.Env = marketKVRows(s.Env, s.Arguments)
	}
	state.DetailOpen = false
	state.EditorOpen = true
}

func marketKVRows(values map[string]string, args map[string]MarketArg) []devtools.KeyValue {
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

func marketHint(s MarketServer) string {
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

func loadMarket(state *State, opts Options) {
	state.MarketLoading = true
	state.MarketErr = ""
	go func() {
		var raw map[string]marketRaw
		if err := json.Unmarshal(marketData, &raw); err != nil {
			opts.PostMain(func() { state.MarketLoading = false; state.MarketErr = err.Error() })
			return
		}
		list := make([]MarketServer, 0, len(raw))
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
			s := MarketServer{
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
		opts.PostMain(func() {
			state.Market = list
			state.MarketLoaded = true
			state.MarketLoading = false
		})
	}()
}

type marketRawInstall struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type marketRawExample struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

type marketRawArg struct {
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Example     string `json:"example"`
}

type marketRaw struct {
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
	Examples      []marketRawExample          `json:"examples"`
	Arguments     map[string]marketRawArg     `json:"arguments"`
	Installations map[string]marketRawInstall `json:"installations"`
}

func marketInstalls(m map[string]marketRawInstall) []MarketInstall {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]MarketInstall, 0, len(keys))
	for _, k := range keys {
		in := m[k]
		out = append(out, MarketInstall{
			Kind: k, Command: in.Command, Args: in.Args,
			Env: in.Env, URL: in.URL, Headers: in.Headers,
		})
	}
	return out
}

func marketArgs(in map[string]marketRawArg) map[string]MarketArg {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]MarketArg, len(in))
	for k, a := range in {
		out[k] = MarketArg{Description: a.Description, Required: a.Required, Example: a.Example}
	}
	return out
}

func marketExamples(in []marketRawExample) []MarketExample {
	out := make([]MarketExample, 0, len(in))
	for _, e := range in {
		if e.Prompt == "" {
			continue
		}
		out = append(out, MarketExample{Title: e.Title, Description: e.Description, Prompt: e.Prompt})
	}
	return out
}

func pickInstallation(installations map[string]marketRawInstall) (MarketInstall, bool) {
	for _, pref := range []string{"npm", "uvx", "python", "cli", "docker"} {
		if in, ok := installations[pref]; ok && in.Command != "" {
			return MarketInstall{Kind: pref, Command: in.Command, Args: in.Args, Env: in.Env}, true
		}
	}
	for _, in := range installations {
		if in.URL != "" {
			return MarketInstall{Kind: "http", URL: in.URL, Headers: in.Headers}, true
		}
	}
	for _, in := range installations {
		if in.Command != "" && in.Type != "custom" {
			return MarketInstall{Kind: in.Type, Command: in.Command, Args: in.Args, Env: in.Env}, true
		}
	}
	return MarketInstall{}, false
}
