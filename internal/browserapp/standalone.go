package browserapp

import (
	"chatgo/internal/appkit"

	mgo "github.com/egoist/mygo/ui"
)

// Standalone opens the browser in its own window.
func Standalone(root string) {
	rt := &appkit.Standalone{Dir: root, Lang: appkit.DefaultLanguage}
	cl := New(rt)
	appkit.Run(appkit.Window{
		Title:  "浏览器",
		Width:  1200,
		Height: 800,
		Body: func(c *mgo.Context, _ *appkit.Standalone) {
			if cl.Tabs().Active == "" {
				cl.NewTab("")
			}
			cl.View(c)
		},
	}, rt)
}
