package webapp

import (
	"chatgo/internal/apps/appkit"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// Standalone opens one web app in its own window.
func Standalone(a App) {
	rt := &appkit.Standalone{Dir: a.Dir, Lang: appkit.DefaultLanguage}
	cl := New(rt, a)
	if err := cl.Register(); err != nil {
		rt.Toast("注册应用协议失败: "+err.Error(), mcore.SeverityError)
	}
	appkit.Run(appkit.Window{
		Title:  a.Name,
		Width:  1280,
		Height: 860,
		Body: func(c *mgo.Context, _ *appkit.Standalone) {
			cl.View(c)
		},
		Tools:  appkitTools(cl.Tools()),
		System: a.SystemPrompt() + cl.SkillPrompt(),
	}, rt)
}

func appkitTools(tools []Tool) []appkit.Tool {
	out := make([]appkit.Tool, len(tools))
	for i, t := range tools {
		params := make(map[string]appkit.Param, len(t.Params))
		for _, p := range t.Params {
			params[p.Name] = appkit.Param{Desc: p.Description, Required: p.Required}
		}
		out[i] = appkit.Tool{Name: t.Name, Desc: t.Desc, Params: params, Run: t.Run}
	}
	return out
}
