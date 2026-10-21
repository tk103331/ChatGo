package appkit

import (
	"log"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Window describes a standalone app window.
type Window struct {
	Title  string
	Width  int
	Height int
	Body   func(c *mgo.Context, rt *Standalone)
	// Tools are the operations exposed to the window's chat agent; when
	// empty the window has no chat pane.
	Tools []Tool
	// System is the chat agent's system prompt.
	System string
}

// Run opens the app in its own window and blocks until it closes.
func Run(win Window, rt *Standalone) {
	rt.tools = win.Tools
	rt.system = win.System
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
				mcore.Use(c, mcore.Settings{Mode: mcore.System, Density: mtheme.Compact})
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

// Column lays out a standalone app with its own title bar, an optional chat
// pane on the right, and a status line.
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
			if rt.HasChat() {
				icon := "message-square"
				label := "隐藏对话"
				if rt.chatHidden {
					icon = "panel-left"
					label = "显示对话"
				}
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must(icon), Label: label}).Clicked() {
					rt.ToggleChat()
				}
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			if !rt.HasChat() || rt.chatHidden {
				win.Body(c, rt)
				return
			}
			winW, _ := c.Size()
			if rt.bodyW == 0 {
				rt.bodyW = winW - 340
				if rt.bodyW < 380 {
					rt.bodyW = 380
				}
			}
			mgo.Split(c, &rt.bodyW, func() {
				mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Children(func() { win.Body(c, rt) })
			}, func() {
				mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
					mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(k.Background).
						BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(k.Border).
						Children(func() { rt.ChatView(c) })
				})
			}).Grow(1).MinHeight(0)
		})
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
