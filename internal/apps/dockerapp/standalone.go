package dockerapp

import (
	"chatgo/internal/apps/appkit"

	mgo "github.com/egoist/mygo/ui"
)

// Standalone opens the Docker manager in its own window.
func Standalone(root string) {
	rt := &appkit.Standalone{Dir: root, Lang: appkit.DefaultLanguage}
	cl := New(rt)
	appkit.Run(appkit.Window{
		Title:  "Docker 管理",
		Width:  1100,
		Height: 720,
		Body: func(c *mgo.Context, _ *appkit.Standalone) {
			if cl.snapshot().lastLoad.IsZero() {
				cl.loadPage(pageContainers)
			}
			cl.View(c)
		},
		Tools:  appkitTools(cl.Tools()),
		System: System,
	}, rt)
}

func appkitTools(tools []Tool) []appkit.Tool {
	out := make([]appkit.Tool, len(tools))
	for i, t := range tools {
		params := make(map[string]appkit.Param, len(t.Params))
		for name, p := range t.Params {
			params[name] = appkit.Param{Desc: p.Desc, Required: p.Required}
		}
		out[i] = appkit.Tool{Name: t.Name, Desc: t.Desc, Params: params, Run: t.Run}
	}
	return out
}
