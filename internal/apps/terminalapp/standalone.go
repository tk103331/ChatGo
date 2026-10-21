package terminalapp

import (
	"chatgo/internal/apps/appkit"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Standalone opens the terminal in its own window.
func Standalone(root string) {
	rt := &appkit.Standalone{Dir: root, Lang: appkit.DefaultLanguage}
	cl := New(rt)
	appkit.Run(appkit.Window{
		Title:  "终端",
		Width:  960,
		Height: 640,
		Body: func(c *mgo.Context, w *appkit.Standalone) {
			if cl.Tabs().Active == "" {
				cl.NewTab()
			}
			k := mcore.Tokens(c)
			u := mcore.Density(c).Unit()
			mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
				mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
					BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
					closing := ""
					for _, tab := range cl.Tabs().Items {
						tab := tab
						variant := minput.Ghost
						if tab == cl.Tabs().Active {
							variant = minput.Secondary
						}
						label := cl.Title(tab)
						if minput.Button(c, label, minput.ButtonOptions{Variant: variant, Label: label}).Clicked() {
							cl.Tabs().Active = tab
							w.Invalidate()
						}
						if mcore.IconAction(c, icons.Must("x"), "关闭标签").Clicked() {
							closing = tab
						}
					}
					mgo.Box(c).Grow(1)
					if minput.Button(c, "新建", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("plus")}).Clicked() {
						cl.NewTab()
						w.Invalidate()
					}
					if closing != "" {
						cl.Close(closing)
						if cl.Tabs().Active == "" {
							cl.NewTab()
						}
						w.Invalidate()
					}
				})
				mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() { cl.View(c) })
			})
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
