package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/llm"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// appDef describes one Agent-driven application in the Apps menu: a set of
// operations exposed to the Agent as tools, a left multi-tab work area and the
// shared chat pane on the right.
type appDef struct {
	ID       string
	Title    string
	Desc     string
	Icon     string
	System   string
	Tools    func(cw *ChatWindow) []appTool
	NewTab   func(cw *ChatWindow) string
	TabLabel func(cw *ChatWindow, tab string) string
	Body     func(cw *ChatWindow, c *mgo.Context, tab string)
}

// appTool is one operation an app offers the Agent.
type appTool struct {
	Name   string
	Desc   string
	Params map[string]appParam
	Run    func(cw *ChatWindow, args map[string]any) (string, error)
}

// appParam is one parameter of an app tool.
type appParam struct {
	Desc     string
	Required bool
}

// apps returns the registered applications.
func apps() []appDef {
	return []appDef{
		terminalApp(),
		markdownApp(),
		gitApp(),
	}
}

// appByID returns the app with the given id.
func appByID(id string) (appDef, bool) {
	for _, a := range apps() {
		if a.ID == id {
			return a, true
		}
	}
	return appDef{}, false
}

// ensureAppState prepares the per-app runtime state.
func (cw *ChatWindow) ensureAppState() {
	if cw.appsLoaded {
		return
	}
	cw.appsLoaded = true
	cw.appTabs = map[string][]string{}
	cw.appTab = map[string]string{}
	cw.appConv = map[string]string{}
}

// appsPage is the Apps menu: a grid of application cards.
func (cw *ChatWindow) appsPage(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Text(c, "Apps").
			FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Shrink(0)
		mgo.Text(c, "基于 Agent 的应用：左侧操作、右侧对话，操作能力向 Agent 开放").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).Shrink(0)
		list := apps()
		cardRows(c, 3, len(list), func(i int) { cw.appCard(c, list[i]) })
	})
}

// appCard is one application tile.
func (cw *ChatWindow) appCard(c *mgo.Context, a appDef) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	b := mgo.ButtonBase(c).Key("app:"+a.ID).FillWidth().Padding(u*3).Gap(u*2).
		Radius(mtheme.CardRadius).AlignItems(mgo.Start).Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must(a.Icon)).Size(24, 24).TextColor(k.Accent)
		mgo.Text(c, a.Title).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600)
		if a.Desc != "" {
			mgo.Text(c, a.Desc).FontSize(caption).TextColor(k.TextMuted).MaxLines(2)
		}
	})
	if b.Clicked() {
		cw.openApp(a.ID)
	}
}

// openApp enters an app, starting its chat session on first use.
func (cw *ChatWindow) openApp(id string) {
	cw.ensureAppState()
	if cw.cur != nil && !strings.HasPrefix(cw.cur.Agent, "app:") {
		cw.preAppConvID = cw.cur.ID
	}
	cw.appActive = id
	cw.appTabList(id)
	cw.startAppSession(id)
	cw.invalidate()
}

// appWorkspace lays out the active app: the left work area and the chat pane.
func (cw *ChatWindow) appWorkspace(c *mgo.Context) {
	a, ok := appByID(cw.appActive)
	if !ok {
		cw.appActive = ""
		cw.appsPage(c)
		return
	}
	if cw.appLeftW == 0 {
		cw.appLeftW = 460
	}
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Children(func() {
		cw.appHeader(c, a)
		mgo.Row(c).Grow(1).MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
			mgo.Box(c).Width(cw.appLeftW).Shrink(0).MinWidth(0).MinHeight(0).Children(func() {
				cw.appLeft(c, a)
			})
			mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				cw.appChat(c)
			})
		})
	})
}

// appHeader shows the app title and a way back to the card grid.
func (cw *ChatWindow) appHeader(c *mgo.Context, a appDef) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Height(navItemSize).Shrink(0).Padding(0, u*3).
		Gap(u).AlignItems(mgo.Center).BorderWidth(0, 0, mtheme.BorderWidth, 0).
		BorderColor(k.Border).Children(func() {
		if mcore.IconAction(c, icons.Must("chevron-left"), "返回应用列表").Clicked() {
			cw.appActive = ""
		}
		mgo.Icon(c, icons.Must(a.Icon)).Size(18, 18).TextColor(k.Accent)
		mgo.Text(c, a.Title).
			FontSize(mcore.FontSize(c, mtheme.BodySize)).FontWeight(600)
	})
}

// appToolsFor returns the tools the given app exposes to the Agent.
func (cw *ChatWindow) appToolsFor(id string) []appTool {
	a, ok := appByID(id)
	if !ok || a.Tools == nil {
		return nil
	}
	return a.Tools(cw)
}

// appSystemPrompt is the system prompt for an app's chat session.
func (cw *ChatWindow) appSystemPrompt(id string) string {
	a, ok := appByID(id)
	if !ok {
		return ""
	}
	if a.System != "" {
		return a.System
	}
	return "你是 " + a.Title + " 应用助手，使用该应用提供的工具完成用户的任务。"
}

// appTabList returns the app's open tabs, creating the first when empty.
func (cw *ChatWindow) appTabList(id string) []string {
	cw.ensureAppState()
	if len(cw.appTabs[id]) == 0 {
		if a, ok := appByID(id); ok && a.NewTab != nil {
			cw.appTabs[id] = []string{a.NewTab(cw)}
			cw.appTab[id] = cw.appTabs[id][0]
		}
	}
	return cw.appTabs[id]
}

// appTabLabel labels a tab for the tab bar.
func (cw *ChatWindow) appTabLabel(id, tab string) string {
	if a, ok := appByID(id); ok && a.TabLabel != nil {
		return a.TabLabel(cw, tab)
	}
	if tab == "" {
		return "未命名"
	}
	return tab
}

// addAppTab opens a new work object in the app and selects it.
func (cw *ChatWindow) addAppTab(id string) {
	a, ok := appByID(id)
	if !ok || a.NewTab == nil {
		return
	}
	cw.ensureAppState()
	tab := a.NewTab(cw)
	cw.appTabs[id] = append(cw.appTabs[id], tab)
	cw.appTab[id] = tab
}

// closeAppTab closes a tab and selects a neighbour.
func (cw *ChatWindow) closeAppTab(id string, i int) {
	tabs := cw.appTabs[id]
	if i < 0 || i >= len(tabs) {
		return
	}
	closed := tabs[i]
	cw.appTabs[id] = append(tabs[:i], tabs[i+1:]...)
	if cw.appTab[id] == closed {
		switch {
		case len(cw.appTabs[id]) == 0:
			cw.appTab[id] = ""
		case i >= len(cw.appTabs[id]):
			cw.appTab[id] = cw.appTabs[id][len(cw.appTabs[id])-1]
		default:
			cw.appTab[id] = cw.appTabs[id][i]
		}
	}
}

// appLeft is the left work area: the tab bar and the active tab's body.
func (cw *ChatWindow) appLeft(c *mgo.Context, a appDef) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	tabs := cw.appTabList(a.ID)
	active := cw.appTab[a.ID]
	mgo.Column(c).Fill().Background(k.Background).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			closing := -1
			for i, tab := range tabs {
				i, tab := i, tab
				variant := kit.Ghost
				if tab == active {
					variant = kit.Secondary
				}
				label := cw.appTabLabel(a.ID, tab)
				if kit.Button(c, label, kit.ButtonOptions{Variant: variant, Label: label}).Clicked() {
					cw.appTab[a.ID] = tab
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭标签").Clicked() {
					closing = i
				}
			}
			mgo.Box(c).Grow(1)
			if kit.Button(c, "新建", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("plus")}).Clicked() {
				cw.addAppTab(a.ID)
			}
			if closing >= 0 {
				cw.closeAppTab(a.ID, closing)
			}
		})
		if a.Body != nil {
			mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				a.Body(cw, c, cw.appTab[a.ID])
			})
		}
	})
}

// appChat is the right chat pane of the app, reusing the shared composer and
// message list against the app's own session.
func (cw *ChatWindow) appChat(c *mgo.Context) {
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(mcore.Tokens(c).Background).
		Children(func() { cw.messageArea(c) })
}

// buildAppTools adapts the app's operations to Eino tools for the React agent.
func (cw *ChatWindow) buildAppTools(id string) []tool.BaseTool {
	specs := cw.appToolsFor(id)
	out := make([]tool.BaseTool, 0, len(specs))
	for _, spec := range specs {
		out = append(out, llm.ObserveTool(newAppTool(cw, spec)))
	}
	return out
}

// appToolWrapper adapts an appTool to Eino's tool interface.
type appToolWrapper struct {
	info *schema.ToolInfo
	run  func(cw *ChatWindow, args map[string]any) (string, error)
	cw   *ChatWindow
}

func newAppTool(cw *ChatWindow, spec appTool) tool.BaseTool {
	params := make(map[string]*schema.ParameterInfo, len(spec.Params))
	for name, p := range spec.Params {
		params[name] = &schema.ParameterInfo{
			Type:     schema.String,
			Desc:     p.Desc,
			Required: p.Required,
		}
	}
	return &appToolWrapper{
		cw: cw,
		info: &schema.ToolInfo{
			Name:        spec.Name,
			Desc:        spec.Desc,
			ParamsOneOf: schema.NewParamsOneOfByParams(params),
		},
		run: spec.Run,
	}
}

func (w *appToolWrapper) Info(context.Context) (*schema.ToolInfo, error) { return w.info, nil }

func (w *appToolWrapper) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	args := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
	}
	return w.run(w.cw, args)
}

func (w *appToolWrapper) StreamableRun(context.Context, string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported")
}

// argString reads a string argument.
func argString(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
