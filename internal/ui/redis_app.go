package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/redisapp"
)

// redisClient is the redisapp client bound to this window.
func (cw *ChatWindow) redisClient() *redisapp.Client {
	if cw.redisApp == nil {
		cw.redisApp = redisapp.New(appHost{cw: cw})
	}
	return cw.redisApp
}

// redisApp is the Apps menu entry for the Redis client.
func redisApp() appDef {
	return appDef{
		ID:        "redis",
		Title:     "Redis 客户端",
		Desc:      "浏览键、查看各类型值、执行 Redis 命令、查看服务器信息，Agent 可代为操作",
		Icon:      "key-round",
		System:    redisapp.System,
		Static:    true,
		ChatW:     400,
		NewTab:    func(cw *ChatWindow) string { return "" },
		Workspace: func(cw *ChatWindow, c *mgo.Context) { cw.redisClient().View(c) },
		Tools: func(cw *ChatWindow) []appTool {
			tools := cw.redisClient().Tools()
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
