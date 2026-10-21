package ui

import (
	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/browserapp"
	"chatgo/internal/apps/codeeditorapp"
	"chatgo/internal/apps/logapp"
	"chatgo/internal/apps/markdownapp"
	"chatgo/internal/apps/terminalapp"
	"chatgo/internal/apps/webapp"
	"chatgo/internal/config"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// appHost adapts ChatWindow to the appkit capability set.
type appHost struct{ cw *ChatWindow }

func (h appHost) Root() string { return h.cw.fileRoot() }

func (h appHost) Workspace() string { return h.cw.workspace }

func (h appHost) Post(fn func()) { h.cw.postMain(fn) }

func (h appHost) Invalidate() { h.cw.invalidate() }

func (h appHost) Toast(msg string, sev mcore.Severity) { h.cw.toast(msg, sev) }

func (h appHost) Confirm(title, desc string, action func()) { h.cw.askConfirm(title, desc, action) }

func (h appHost) Prompt(title, label, initial string, action func(string)) {
	h.cw.askInput(title, label, initial, action)
}

func (h appHost) OpenFile(path string) { h.cw.openPreview(path) }

func (h appHost) OpenFileDialog(dir string) string {
	picks, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: h.NativeWindow(), Title: "打开文件", DefaultPath: dir})
	if err != nil || len(picks) == 0 {
		return ""
	}
	return picks[0]
}

func (h appHost) SaveFileDialog(name string) string {
	p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: h.NativeWindow(), Title: "保存文件", DefaultPath: name})
	if err != nil || p == "" {
		return ""
	}
	return p
}

func (h appHost) Language(ext string) string { return langOfExt(ext) }

func (h appHost) NewID() string { return h.cw.newID() }

func (h appHost) NativeWindow() *mygo.Window { return h.cw.nativeWindow() }

// terminalClient returns the terminal app bound to this window.
func (cw *ChatWindow) terminalClient() *terminalapp.Client {
	if cw.termApp == nil {
		cw.termApp = terminalapp.New(appHost{cw: cw})
	}
	return cw.termApp
}

// markdownClient returns the markdown editor bound to this window.
func (cw *ChatWindow) markdownClient() *markdownapp.Client {
	if cw.mdApp == nil {
		cw.mdApp = markdownapp.New(appHost{cw: cw})
		cw.mdApp.SetOpenHandler(func(path string) { cw.openAppPath("markdown", path) })
	}
	return cw.mdApp
}

// terminalApp is the Apps menu entry for the terminal.
func terminalApp() appDef {
	return appDef{
		ID:     "terminal",
		Title:  "终端",
		Desc:   "在项目目录中运行命令，命令历史与会话保留在标签页里",
		Icon:   "terminal",
		System: terminalapp.System,
		Tools: func(cw *ChatWindow) []appTool {
			return wrapTerminalTools(cw.terminalClient().Tools())
		},
		NewTab: func(cw *ChatWindow) string { return cw.terminalClient().NewTab() },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return cw.terminalClient().Title(tab)
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.terminalClient().Tabs().Active = tab
			cw.terminalClient().View(c)
		},
	}
}

// markdownApp is the Apps menu entry for the Markdown editor.
func markdownApp() appDef {
	return appDef{
		ID:     "markdown",
		Title:  "Markdown 编辑器",
		Desc:   "打开、编辑并保存项目中的 Markdown 文档，Agent 可读写文件",
		Icon:   "file-text",
		System: markdownapp.System,
		Tools: func(cw *ChatWindow) []appTool {
			return wrapMarkdownTools(cw.markdownClient().Tools())
		},
		NewTab: func(cw *ChatWindow) string { return cw.markdownClient().NewTab() },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return cw.markdownClient().Title(tab)
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.markdownClient().Tabs().Active = tab
			cw.markdownClient().View(c)
		},
	}
}

// wrapTerminalTools adapts the terminal app's tools to the menu's shape.
func wrapTerminalTools(tools []terminalapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: terminalParams(t.Params), Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// wrapMarkdownTools adapts the markdown editor's tools to the menu's shape.
func wrapMarkdownTools(tools []markdownapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: markdownParams(t.Params), Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// terminalParams converts the terminal app's parameters.
func terminalParams(params map[string]terminalapp.Param) map[string]appParam {
	out := make(map[string]appParam, len(params))
	for name, p := range params {
		out[name] = appParam{Desc: p.Desc, Required: p.Required}
	}
	return out
}

// markdownParams converts the markdown editor's parameters.
func markdownParams(params map[string]markdownapp.Param) map[string]appParam {
	out := make(map[string]appParam, len(params))
	for name, p := range params {
		out[name] = appParam{Desc: p.Desc, Required: p.Required}
	}
	return out
}

// codeClient returns the code editor bound to this window.
func (cw *ChatWindow) codeClient() *codeeditorapp.Client {
	if cw.codeApp == nil {
		cw.codeApp = codeeditorapp.New(appHost{cw: cw})
		cw.codeApp.SetOpenHandler(func(path string) { cw.openAppPath("code", path) })
	}
	return cw.codeApp
}

// browserClient returns the browser bound to this window.
func (cw *ChatWindow) browserClient() *browserapp.Client {
	if cw.browserApp == nil {
		cw.browserApp = browserapp.New(appHost{cw: cw})
	}
	return cw.browserApp
}

// logClient returns the log viewer bound to this window.
func (cw *ChatWindow) logClient() *logapp.Client {
	if cw.logApp == nil {
		cw.logApp = logapp.New(appHost{cw: cw})
		cw.logApp.SetOpenHandler(func(path string) { cw.openAppPath("logs", path) })
	}
	return cw.logApp
}

// logApp is the Apps menu entry for the log viewer.
func logApp() appDef {
	return appDef{
		ID:     "logs",
		Title:  "日志查看器",
		Desc:   "打开日志文件，支持完整加载、Tail 末尾 N 行、跟踪新增内容、按级别过滤",
		Icon:   "list",
		System: logapp.System,
		Tools: func(cw *ChatWindow) []appTool {
			return wrapLogTools(cw.logClient().Tools())
		},
		NewTab: func(cw *ChatWindow) string { return cw.logClient().NewTab() },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return cw.logClient().Title(tab)
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.logClient().Tabs().Active = tab
			cw.logClient().View(c)
		},
	}
}

// wrapLogTools adapts the log viewer's tools.
func wrapLogTools(tools []logapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		params := make(map[string]appParam, len(t.Params))
		for name, p := range t.Params {
			params[name] = appParam{Desc: p.Desc, Required: p.Required}
		}
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// codeEditorApp is the Apps menu entry for the code editor.
func codeEditorApp() appDef {
	return appDef{
		ID:     "code",
		Title:  "代码编辑器",
		Desc:   "打开、编辑并保存项目中的源文件，支持语法预览，Agent 可读写文件",
		Icon:   "file-code",
		System: codeeditorapp.System,
		Tools: func(cw *ChatWindow) []appTool {
			return wrapCodeTools(cw.codeClient().Tools())
		},
		NewTab: func(cw *ChatWindow) string { return cw.codeClient().NewTab() },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return cw.codeClient().Title(tab)
		},
		CloseTab: func(cw *ChatWindow, tab string) { cw.codeClient().Close(tab) },
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.codeClient().Tabs().Active = tab
			cw.codeClient().View(c)
		},
	}
}

// browserApp is the Apps menu entry for the browser.
func browserApp() appDef {
	return appDef{
		ID:     "browser",
		Title:  "浏览器",
		Desc:   "在多标签内嵌浏览器中打开网页，Agent 可读取页面内容",
		Icon:   "globe",
		System: browserapp.System,
		Tools: func(cw *ChatWindow) []appTool {
			return wrapBrowserTools(cw.browserClient().Tools())
		},
		NewTab: func(cw *ChatWindow) string { return cw.browserClient().NewTab("") },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return cw.browserClient().Title(tab)
		},
		CloseTab: func(cw *ChatWindow, tab string) { cw.browserClient().Close(tab) },
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.browserClient().Tabs().Active = tab
			cw.browserClient().View(c)
		},
	}
}

// wrapCodeTools adapts the code editor's tools.
func wrapCodeTools(tools []codeeditorapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		params := make(map[string]appParam, len(t.Params))
		for name, p := range t.Params {
			params[name] = appParam{Desc: p.Desc, Required: p.Required}
		}
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// wrapBrowserTools adapts the browser's tools.
func wrapBrowserTools(tools []browserapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		params := make(map[string]appParam, len(t.Params))
		for name, p := range t.Params {
			params[name] = appParam{Desc: p.Desc, Required: p.Required}
		}
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// webAppClient returns the client for a web app, creating it on first use.
func (cw *ChatWindow) webAppClient(id string) *webapp.Client {
	if cl, ok := cw.webApps[id]; ok {
		return cl
	}
	a, ok := webAppByID(id)
	if !ok {
		return nil
	}
	cl := webapp.New(appHost{cw: cw}, a)
	if cw.webApps == nil {
		cw.webApps = map[string]*webapp.Client{}
	}
	cw.webApps[id] = cl
	return cl
}

// webApps turns the discovered web apps into menu entries.
func webApps() []appDef {
	var out []appDef
	for _, a := range webapp.Discover() {
		a := a
		out = append(out, webAppDef(a))
	}
	return out
}

// webAppByID returns the manifest of a discovered web app.
func webAppByID(id string) (webapp.App, bool) {
	for _, a := range webapp.Discover() {
		if a.ID == id {
			return a, true
		}
	}
	return webapp.App{}, false
}

// loadWebApps discovers web apps and registers their protocols. Custom
// schemes must be registered before the first window is created, because a
// window only routes the schemes known at creation time.
func (cw *ChatWindow) loadWebApps() {
	cw.webAppList = webapp.Discover()
	for _, a := range cw.webAppList {
		cl := cw.webAppClient(a.ID)
		if cl == nil {
			continue
		}
		if err := cl.Register(); err != nil {
			delete(cw.webApps, a.ID)
		}
	}
}

// webAppDef adapts one web app manifest to the Apps menu.
func webAppDef(a webapp.App) appDef {
	id := "webapp:" + a.ID
	return appDef{
		ID:     id,
		Title:  a.Name,
		Desc:   a.Description,
		Icon:   a.Icon,
		System: a.SystemPrompt(),
		Static: true,
		NewTab: func(cw *ChatWindow) string { return "" },
		TabLabel: func(cw *ChatWindow, tab string) string {
			return a.Name
		},
		Workspace: func(cw *ChatWindow, c *mgo.Context) {
			cl := cw.webAppClient(a.ID)
			if cl == nil {
				return
			}
			cl.View(c)
		},
		Tools: func(cw *ChatWindow) []appTool {
			cl := cw.webAppClient(a.ID)
			if cl == nil {
				return nil
			}
			return wrapWebTools(cl.Tools())
		},
		CloseTab: func(cw *ChatWindow, tab string) {
			if cl := cw.webAppClient(a.ID); cl != nil {
				cl.Close()
			}
		},
	}
}

// wrapWebTools adapts a web app's tools to the menu's shape.
func wrapWebTools(tools []webapp.Tool) []appTool {
	out := make([]appTool, 0, len(tools))
	for _, t := range tools {
		t := t
		params := make(map[string]appParam, len(t.Params))
		for _, spec := range t.Params {
			params[spec.Name] = appParam{Desc: spec.Description, Required: spec.Required}
		}
		out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(_ *ChatWindow, args map[string]any) (string, error) {
			return t.Run(args)
		}})
	}
	return out
}

// webAppMCPServers returns the MCP servers every loaded web app declares, so
// their tools join the same registry as configured MCP servers.
func (cw *ChatWindow) webAppMCPServers() []config.MCPServer {
	var out []config.MCPServer
	for _, a := range cw.webAppList {
		cl := cw.webAppClient(a.ID)
		if cl == nil {
			continue
		}
		out = append(out, cl.MCPServers()...)
	}
	return out
}

// mountWebAppMCP initializes web app MCP endpoints in the background once.
func (cw *ChatWindow) mountWebAppMCP() {
	servers := cw.webAppMCPServers()
	if len(servers) == 0 {
		return
	}
	go func() {
		for _, srv := range servers {
			if _, ok := cw.mcpManager.GetServerStatus(srv.Name); ok {
				continue
			}
			_, _ = cw.mcpManager.manager.InitializeServer(srv)
		}
	}()
}

// appSkillPrompt returns extra skill text for a web app's session.
func (cw *ChatWindow) webAppSkillPrompt(appID string) string {
	cl := cw.webAppClient(appID)
	if cl == nil {
		return ""
	}
	return cl.SkillPrompt()
}
