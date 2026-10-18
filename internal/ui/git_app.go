package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/gitapp"
)

// gitClient is the gitapp client bound to this window.
func (cw *ChatWindow) gitClient() *gitapp.Client {
	if cw.gitApp == nil {
		cw.gitApp = gitapp.New(appHost{cw: cw})
	}
	return cw.gitApp
}

// gitApp is the Apps menu entry for the Git client.
func gitApp() appDef {
	return appDef{
		ID:        "git",
		Title:     "Git 客户端",
		Desc:      "查看变更、对比 diff、暂存与提交，Agent 可代为执行 Git 操作",
		Icon:      "git-branch",
		System:    gitapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.gitClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.gitClient().Tools()
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
