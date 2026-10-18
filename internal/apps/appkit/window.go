package appkit

import (
	"log"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Window describes a standalone app window.
type Window struct {
	Title  string
	Width  int
	Height int
	Body   func(c *mgo.Context, rt *Standalone)
}

// Run opens the app in its own window and blocks until it closes.
func Run(win Window, rt *Standalone) {
	mygo.App.WhenReady(func() {
		var w *mygo.Window
		w = mygo.NewWindow(mygo.WindowOptions{
			Title:         win.Title,
			Width:         win.Width,
			Height:        win.Height,
			MinWidth:      720,
			MinHeight:     480,
			TitleBarStyle: mygo.TitleBarHiddenInset,
			Content: mgo.View(func(c *mgo.Context) {
				mcore.Use(c, mcore.Settings{Mode: mcore.System})
				Column(c, w, win, rt)
			}),
		})
		rt.Attach(w, w.Update, w.Update)
		w.Show()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// Column lays out a standalone app with its own title bar and status line.
func Column(c *mgo.Context, w *mygo.Window, win Window, rt *Standalone) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Children(func() {
		bar := c.TitleBar()
		mgo.Row(c).FillWidth().Height(44).Shrink(0).
			Padding(0, bar.Right+u*2, 0, bar.Left+u*2).DragWindow().
			AlignItems(mgo.Center).Gap(u).Background(k.Background).Children(func() {
			mgo.Text(c, win.Title).Font(mtheme.SerifFont).FontWeight(600).
				FontSize(mcore.FontSize(c, mtheme.BodySize)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() { win.Body(c, rt) })
		if toasts := rt.Toasts(); len(toasts) > 0 {
			last := toasts[len(toasts)-1]
			col := k.TextMuted
			if last.Sev == mcore.SeverityError {
				col = k.Danger
			}
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).
				BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(k.Border).Children(func() {
				mgo.Text(c, last.Msg).FontSize(caption).TextColor(col).SingleLine().Ellipsis("…")
			})
		}
	})
}
