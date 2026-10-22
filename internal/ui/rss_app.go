package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/rssapp"
)

// rssClient is the rssapp client bound to this window.
func (cw *ChatWindow) rssClient() *rssapp.Client {
	if cw.rssApp == nil {
		cw.rssApp = rssapp.New(appHost{cw: cw})
	}
	return cw.rssApp
}

// rssApp is the Apps menu entry for the RSS reader.
func rssApp() appDef {
	return appDef{
		ID:        "rss",
		Title:     "RSS 阅读器",
		Desc:      "订阅 RSS/Atom 源，导入导出 OPML，阅读文章，Agent 可代为管理",
		Icon:      "inbox",
		System:    rssapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.rssClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.rssClient().Tools()
			out := make([]appTool, 0, len(tools))
			for _, t := range tools {
				t := t
				params := make(map[string]appParam, len(t.Params))
				for name, p := range t.Params {
					params[name] = appParam{Desc: p.Desc, Required: p.Required}
				}
				out = append(out, appTool{Name: t.Name, Desc: t.Desc, Params: params, Run: func(cw *ChatWindow, args map[string]any) (string, error) {
					return t.Run(args)
				}})
			}
			return out
		},
	}
}
