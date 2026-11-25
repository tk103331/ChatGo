// MCP Apps: a tool's interactive view renders inline in the message stream.
// The view is an untrusted web page served from the chatgo-mcp custom scheme,
// sandboxed in an iframe, that talks to the host over JSON-RPC. The tool call
// that produced it supplies the view with its arguments and result.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/mcp"
	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// mcpAppScheme serves MCP Apps host pages and views.
const mcpAppScheme = "chatgo-mcp"

// inlineApp is one view rendered inside a tool call's row.
type inlineApp struct {
	key    string
	convID string
	status *mcp.MCPServerStatus
	tool   mcp.MCPTool
	tc     models.ToolCall

	host *mcp.AppHost
	view *mygo.WebView
	// height is the view's content height, as the page last reported it.
	height float32
	// failure is the reason the view could not be opened, shown in its place.
	failure string
	// lastUsed orders views for eviction when many are alive.
	lastUsed time.Time
}

// mcpAppForTool finds the MCP server and tool behind a tool name that carries
// an MCP Apps view. The first initialized server wins, which matches how a
// model addresses a tool by its bare name.
func (cw *ChatWindow) mcpAppForTool(name string) (*mcp.MCPServerStatus, mcp.MCPTool, bool) {
	if name == "" || cw.mcpManager == nil {
		return nil, mcp.MCPTool{}, false
	}
	for _, status := range cw.mcpManager.manager.GetAllStatus() {
		if status == nil || status.Status != "initialized" {
			continue
		}
		for _, tool := range status.Tools {
			if tool.Name == name && tool.UIResourceURI != "" {
				return status, tool, true
			}
		}
	}
	return nil, mcp.MCPTool{}, false
}

// mcpAppKey names a tool call's view across the conversation and the call.
func (cw *ChatWindow) mcpAppKey(tc models.ToolCall) string {
	convID := ""
	if cw.cur != nil {
		convID = cw.cur.ID
	}
	return convID + "\x00" + tc.ID
}

// toggleInlineApp opens or closes the inline view under a tool call.
func (cw *ChatWindow) toggleInlineApp(key string) {
	if cw.expandedApps == nil {
		cw.expandedApps = map[string]bool{}
	}
	cw.expandedApps[key] = !cw.expandedApps[key]
	cw.invalidate()
}

// mcpAppReady reports whether a call has a finished result and a view to show.
func (cw *ChatWindow) mcpAppReady(tc models.ToolCall) bool {
	if cw.quickMode || cw.mcpApps == nil {
		return false
	}
	if tc.Result == "" && tc.Error == "" {
		return false
	}
	_, _, ok := cw.mcpAppForTool(tc.Name)
	return ok
}

// mcpAppInline draws a completed call's MCP Apps view, once its row asks for
// it. A scrolled-away view keeps its page, as a hidden tab does; only the
// oldest views are torn down once too many are alive.
func (cw *ChatWindow) mcpAppInline(c *mgo.Context, tc models.ToolCall) {
	status, tool, ok := cw.mcpAppForTool(tc.Name)
	if !ok {
		return
	}
	convID := ""
	if cw.cur != nil {
		convID = cw.cur.ID
	}
	app := cw.inlineAppFor(convID, tc, status, tool)
	if app == nil {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Gap(u).Padding(u/2, 0).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u * 2).Children(func() {
			mgo.Text(c, "交互界面").FontSize(caption).FontWeight(600).TextColor(k.TextMuted)
			if app.tool.Title != "" {
				mgo.Text(c, app.tool.Title).FontSize(caption).TextColor(k.TextMuted).
					SingleLine().Ellipsis("…")
			}
			mgo.Box(c).Grow(1)
			if mcore.IconAction(c, icons.Must("columns-2"), "在侧边栏打开").Clicked() {
				// Opening a tab changes the center area, which this frame is
				// still building; let the frame finish first.
				cw.postMain(func() { cw.openMCPAppInTab(convID, tc, status, tool) })
			}
		})
		switch {
		case app.failure != "":
			mgo.Text(c, "加载失败："+app.failure).FontSize(caption).
				TextColor(k.Danger).Selectable()
		case app.view == nil:
			mgo.Row(c).FillWidth().Padding(u*2, u*2).Gap(u * 2).AlignItems(mgo.Center).
				Radius(mtheme.CardRadius).Background(k.Surface).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 14, Label: "加载交互界面"})
				mgo.Text(c, "正在加载交互界面…").FontSize(caption).TextColor(k.TextMuted)
			})
		default:
			mgo.WebView(c, app.view).FillWidth().Height(app.height).
				Radius(mtheme.CardRadius).Border(mtheme.BorderWidth, k.Border)
		}
	})
}

// inlineAppFor returns the view for a tool call, starting its creation when
// this is the first frame to draw it.
func (cw *ChatWindow) inlineAppFor(convID string, tc models.ToolCall, status *mcp.MCPServerStatus, tool mcp.MCPTool) *inlineApp {
	key := convID + "\x00" + tc.ID
	cw.inlineMu.Lock()
	if cw.inlineApps == nil {
		cw.inlineApps = map[string]*inlineApp{}
	}
	if app, ok := cw.inlineApps[key]; ok {
		app.lastUsed = time.Now()
		cw.inlineMu.Unlock()
		return app
	}
	app := &inlineApp{
		key: key, convID: convID, status: status, tool: tool, tc: tc,
		height: 360, lastUsed: time.Now(),
	}
	cw.inlineApps[key] = app
	cw.inlineMu.Unlock()
	cw.pruneInlineApps(key)
	go cw.loadInlineApp(app)
	return app
}

// loadInlineApp fetches the resource and builds the web view off the main
// thread, then hands the finished view to the frame.
func (cw *ChatWindow) loadInlineApp(app *inlineApp) {
	cli, ok := cw.mcpManager.manager.GetServerClient(app.status.Name)
	if !ok {
		cw.failInlineApp(app, "MCP 服务未连接")
		return
	}
	res := cw.cachedAppResource(app.status.Name, app.tool.UIResourceURI)
	if res == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		loaded, err := mcp.ReadUIResource(ctx, cli, app.tool.UIResourceURI)
		if err != nil {
			cw.failInlineApp(app, err.Error())
			return
		}
		res = loaded
		cw.cacheAppResource(app.status.Name, app.tool.UIResourceURI, res)
	}
	cw.postMain(func() {
		if !cw.inlineAppLive(app) {
			return
		}
		cw.winMu.Lock()
		w := cw.window
		cw.winMu.Unlock()
		if w == nil {
			cw.failInlineApp(app, "窗口尚未就绪")
			return
		}
		view, err := w.NewWebView(mygo.WebViewOptions{
			// Wheel over the view scrolls the message list, as over any other
			// row, instead of scrolling the app's own page.
			ScrollToContainer: true,
			BackgroundColor:   "light-dark(#ffffff, #17191c)",
			Page: mygo.PageOptions{
				// The view comes from an MCP server, not from the app: it must
				// not reach bound Go methods or the app's own pages.
				Untrusted: true,
			},
		})
		if err != nil {
			cw.failInlineApp(app, err.Error())
			return
		}
		host, err := cw.mcpApps.Open(cli, app.status, mcp.AppOptions{
			Server:    app.status.Name,
			Tool:      app.tool,
			ToolID:    app.tc.ID,
			Arguments: app.tc.Arguments,
			Result:    app.tc.Result,
			Error:     app.tc.Error,
			Resource:  res,
			Version:   cw.appVersion(),
			Callbacks: cw.mcpAppCallbacks(app.convID),
			OnSize: func(width, height int) {
				cw.postMain(func() {
					if !cw.inlineAppLive(app) || height <= 0 {
						return
					}
					app.height = clampAppHeight(float32(height))
					cw.invalidate()
				})
			},
		})
		if err != nil {
			view.Destroy()
			cw.failInlineApp(app, err.Error())
			return
		}
		if p := view.Page(); p != nil {
			// The shell and its sandboxed view are the only pages this web
			// view may show.
			p.OnWillNavigate(func(e *mygo.NavigateEvent) {
				if strings.HasPrefix(e.URL, host.URL()) || strings.HasPrefix(e.URL, host.ViewURL()) {
					return
				}
				e.PreventDefault()
			})
			if err := p.LoadURL(host.URL()); err != nil {
				cw.mcpApps.Close(host)
				view.Destroy()
				cw.failInlineApp(app, err.Error())
				return
			}
		}
		app.host = host
		app.view = view
		cw.invalidate()
	})
}

// failInlineApp records why a view could not be shown; the row draws the
// reason in place of the page.
func (cw *ChatWindow) failInlineApp(app *inlineApp, reason string) {
	cw.postMain(func() {
		if !cw.inlineAppLive(app) {
			return
		}
		app.failure = reason
		cw.invalidate()
	})
}

// inlineAppLive reports whether the frame still wants this view. A view the
// user scrolled away from, or whose conversation closed, is dropped.
func (cw *ChatWindow) inlineAppLive(app *inlineApp) bool {
	cw.inlineMu.Lock()
	defer cw.inlineMu.Unlock()
	return cw.inlineApps[app.key] == app
}

// dropInlineApp tears down one inline view, keeping its entry out of the
// next frame.
func (cw *ChatWindow) dropInlineApp(key string) {
	cw.inlineMu.Lock()
	app := cw.inlineApps[key]
	delete(cw.inlineApps, key)
	cw.inlineMu.Unlock()
	if app == nil {
		return
	}
	if app.view != nil {
		app.view.Destroy()
	}
	if app.host != nil && cw.mcpApps != nil {
		cw.mcpApps.Close(app.host)
	}
}

// maxInlineApps is how many views stay alive at once. Scrolled-away views
// keep their pages, as hidden tabs do, until older ones are evicted.
const maxInlineApps = 8

// pruneInlineApps tears down the least recently drawn view when too many are
// alive, keeping the one just requested.
func (cw *ChatWindow) pruneInlineApps(keep string) {
	cw.inlineMu.Lock()
	var oldest *inlineApp
	if len(cw.inlineApps) > maxInlineApps {
		for key, app := range cw.inlineApps {
			if key == keep {
				continue
			}
			if oldest == nil || app.lastUsed.Before(oldest.lastUsed) {
				oldest = app
			}
		}
		// Do not evict a view drawn moments ago: another row in this same
		// frame may still be placing it.
		if oldest != nil && time.Since(oldest.lastUsed) < 2*time.Second {
			oldest = nil
		}
		if oldest != nil {
			delete(cw.inlineApps, oldest.key)
		}
	}
	cw.inlineMu.Unlock()
	if oldest == nil {
		return
	}
	if oldest.view != nil {
		oldest.view.Destroy()
	}
	if oldest.host != nil && cw.mcpApps != nil {
		cw.mcpApps.Close(oldest.host)
	}
}

// cachedAppResource returns a view's HTML if it was read before.
func (cw *ChatWindow) cachedAppResource(server, uri string) *mcp.AppResource {
	cw.appResMu.Lock()
	defer cw.appResMu.Unlock()
	if cw.appResources == nil {
		return nil
	}
	return cw.appResources[server+"\x00"+uri]
}

// cacheAppResource keeps a fetched view for the next time a row is drawn.
func (cw *ChatWindow) cacheAppResource(server, uri string, res *mcp.AppResource) {
	cw.appResMu.Lock()
	defer cw.appResMu.Unlock()
	if cw.appResources == nil {
		cw.appResources = map[string]*mcp.AppResource{}
	}
	cw.appResources[server+"\x00"+uri] = res
}

// mcpAppCallbacks connects a view to the window's chat, links and theme.
func (cw *ChatWindow) mcpAppCallbacks(convID string) mcp.AppCallbacks {
	// Read the theme and locale once, on the main thread, rather than from
	// the bridge's goroutines while the window may be repainting.
	theme, locale := cw.mcpAppTheme(), string(cw.locale)
	return mcp.AppCallbacks{
		OpenLink: func(raw string) error {
			return mygo.Shell.OpenExternal(raw)
		},
		PostMessage: func(text string) error {
			cw.postMain(func() {
				if cw.cur != nil && cw.cur.ID == convID {
					if strings.TrimSpace(cw.draft) != "" {
						cw.draft = strings.TrimRight(cw.draft, "\n") + "\n\n" + text
					} else {
						cw.draft = text
					}
					cw.invalidate()
					cw.toast("应用已把消息填入输入框，确认后发送", mcore.SeverityInfo)
					return
				}
				if cw.drafts == nil {
					cw.drafts = map[string]string{}
				}
				cw.drafts[convID] = text
				cw.toast("应用已把消息备好，切回该会话即可发送", mcore.SeverityInfo)
			})
			return nil
		},
		UpdateModelContext: func(content, structured json.RawMessage) {
			text := appContextText(content, structured)
			if text == "" {
				return
			}
			cw.postMain(func() {
				if cw.appModelContext == nil {
					cw.appModelContext = map[string]string{}
				}
				cw.appModelContext[convID] = "以下是应用界面提供的上下文，供本轮回答参考：\n" + text
			})
		},
		Log: func(level, message string) {
			if message == "" {
				return
			}
			fmt.Printf("[MCP Apps %s] %s\n", level, message)
		},
		Theme:  func() string { return theme },
		Locale: func() string { return locale },
	}
}

// mcpAppTheme reports the window's appearance to a view, falling back to the
// system's when the window follows it.
func (cw *ChatWindow) mcpAppTheme() (name string) {
	switch cw.themeMode {
	case mcore.Dark:
		return "dark"
	case mcore.Light:
		return "light"
	}
	defer func() {
		if recover() != nil {
			name = ""
		}
	}()
	if mygo.Theme.IsDark() {
		return "dark"
	}
	return "light"
}

// appContextText flattens the content a view sent with
// ui/update-model-context: text blocks first, then structured JSON.
func appContextText(content, structured json.RawMessage) string {
	var parts []string
	var one struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &one); err == nil && one.Text != "" {
		parts = append(parts, one.Text)
	} else {
		var many []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(content, &many); err == nil {
			for _, c := range many {
				if c.Text != "" {
					parts = append(parts, c.Text)
				}
			}
		}
	}
	if s := strings.TrimSpace(string(structured)); s != "" && s != "null" {
		parts = append(parts, "结构化数据：\n"+s)
	}
	return strings.Join(parts, "\n\n")
}

// clampAppHeight keeps a view tall enough to be usable and short enough to
// leave the conversation readable.
func clampAppHeight(h float32) float32 {
	if h < 140 {
		return 140
	}
	if h > 720 {
		return 720
	}
	return h
}

// openMCPAppInTab shows a call's view in a center tab beside the
// conversation, focusing the tab when it is already open.
func (cw *ChatWindow) openMCPAppInTab(convID string, tc models.ToolCall, status *mcp.MCPServerStatus, tool mcp.MCPTool) {
	id := "mcpapp:" + convID + ":" + tc.ID
	if cw.focusCenterTab(id) != nil {
		return
	}
	// The tab shows the same view; fold and release the inline one so the
	// call does not keep two live copies alive.
	key := cw.mcpAppKey(tc)
	if cw.expandedApps[key] {
		cw.expandedApps[key] = false
		cw.dropInlineApp(key)
		cw.invalidate()
	}
	go func() {
		res := cw.cachedAppResource(status.Name, tool.UIResourceURI)
		if res == nil {
			cli, ok := cw.mcpManager.manager.GetServerClient(status.Name)
			if !ok {
				cw.postMain(func() { cw.toast("MCP 服务未连接："+status.Name, mcore.SeverityError) })
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			loaded, err := mcp.ReadUIResource(ctx, cli, tool.UIResourceURI)
			if err != nil {
				cw.postMain(func() { cw.toastErrorDetail("无法打开界面", err.Error()) })
				return
			}
			res = loaded
			cw.cacheAppResource(status.Name, tool.UIResourceURI, res)
		}
		cw.postMain(func() { cw.createMCPAppTab(convID, tc, status, tool, res) })
	}()
}

// createMCPAppTab builds the web view and tab for one view.
func (cw *ChatWindow) createMCPAppTab(convID string, tc models.ToolCall, status *mcp.MCPServerStatus, tool mcp.MCPTool, res *mcp.AppResource) {
	id := "mcpapp:" + convID + ":" + tc.ID
	if cw.focusCenterTab(id) != nil {
		return
	}
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	if w == nil {
		cw.toast("窗口尚未就绪", mcore.SeverityError)
		return
	}
	cli, ok := cw.mcpManager.manager.GetServerClient(status.Name)
	if !ok {
		cw.toast("MCP 服务未连接："+status.Name, mcore.SeverityError)
		return
	}
	view, err := w.NewWebView(mygo.WebViewOptions{
		BackgroundColor: "light-dark(#ffffff, #17191c)",
		Page: mygo.PageOptions{
			// The view comes from an MCP server, not from the app.
			Untrusted: true,
		},
	})
	if err != nil {
		cw.toastErrorDetail("WebView 不可用", err.Error())
		return
	}
	host, err := cw.mcpApps.Open(cli, status, mcp.AppOptions{
		Server:    status.Name,
		Tool:      tool,
		ToolID:    tc.ID,
		Arguments: tc.Arguments,
		Result:    tc.Result,
		Error:     tc.Error,
		Resource:  res,
		Version:   cw.appVersion(),
		Callbacks: cw.mcpAppCallbacks(convID),
	})
	if err != nil {
		view.Destroy()
		cw.toastErrorDetail("无法打开界面", err.Error())
		return
	}
	if p := view.Page(); p != nil {
		p.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if strings.HasPrefix(e.URL, host.URL()) || strings.HasPrefix(e.URL, host.ViewURL()) {
				return
			}
			e.PreventDefault()
		})
		if err := p.LoadURL(host.URL()); err != nil {
			cw.mcpApps.Close(host)
			view.Destroy()
			cw.toastErrorDetail("无法加载界面", err.Error())
			return
		}
	}
	title := tool.Title
	if title == "" {
		title = tool.Name
	}
	cw.openCenterTab(cw.center.focus, &centerTab{
		ID: id, Path: host.URL(), Title: "应用 · " + title, Kind: centerWeb,
		Web: &webTab{
			ID: "app-" + host.Token(), Title: "应用 · " + title, URL: host.URL(),
			view: view, App: host, ConvID: convID,
		},
	})
}

// mcpAppBody draws a tabbed view: the page fills the tab, without an address
// bar.
func (cw *ChatWindow) mcpAppBody(c *mgo.Context, tab *centerTab) {
	if tab.Web == nil || tab.Web.view == nil {
		cw.centerLoading(c)
		return
	}
	mgo.WebView(c, tab.Web.view).Fill()
}

// mcpAppClosed tears down a tab's view when its tab closes.
func (cw *ChatWindow) mcpAppClosed(tab *centerTab) {
	if tab == nil || tab.Web == nil || tab.Web.App == nil {
		return
	}
	if cw.mcpApps != nil {
		cw.mcpApps.Close(tab.Web.App)
	}
}
