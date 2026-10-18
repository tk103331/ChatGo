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
	}, rt)
}
