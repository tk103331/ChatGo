package gitapp

import (
	"chatgo/internal/appkit"

	mgo "github.com/egoist/mygo/ui"
)

// Standalone opens the Git client in its own window.
func Standalone(root string) {
	rt := &appkit.Standalone{Dir: root, Lang: appkit.DefaultLanguage}
	cl := New(rt)
	appkit.Run(appkit.Window{
		Title:  "Git 客户端",
		Width:  1280,
		Height: 820,
		Body:   func(c *mgo.Context, _ *appkit.Standalone) { cl.View(c) },
	}, rt)
}
