package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/dockerapp"
)

// dockerClient is the dockerapp client bound to this window.
func (cw *ChatWindow) dockerClient() *dockerapp.Client {
	if cw.dockerApp == nil {
		cw.dockerApp = dockerapp.New(appHost{cw: cw})
	}
	return cw.dockerApp
}

// dockerApp is the Apps menu entry for the Docker manager.
func dockerApp() appDef {
	return appDef{
		ID:        "docker",
		Title:     "Docker 管理",
		Desc:      "列出并管理容器、镜像、卷与网络，查看日志，Agent 可代为执行 Docker 操作",
		Icon:      "archive",
		System:    dockerapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.dockerClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.dockerClient().Tools()
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
