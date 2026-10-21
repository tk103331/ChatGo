package dockerapp

import (
	mgo "github.com/egoist/mygo/ui"
	term "github.com/egoist/mygo/plugins/terminal"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// openTerm starts an interactive TTY shell for the container.
func (cl *Client) openTerm(id string) {
	cl.mu.Lock()
	if cl.st.term != nil || cl.st.termBusy {
		cl.mu.Unlock()
		return
	}
	cl.st.termBusy = true
	cl.st.termErr = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
	go func() {
		t, err := openDockerTerminal(id, "", func() {
			cl.host.Post(func() { cl.host.Invalidate() })
		})
		cl.host.Post(func() {
			cl.mu.Lock()
			cl.st.termBusy = false
			if err != nil {
				cl.st.termErr = shortErr(err)
			} else {
				cl.st.term = t
			}
			cl.mu.Unlock()
			cl.host.Invalidate()
		})
	}()
}

// closeTerm closes the terminal session.
func (cl *Client) closeTerm() {
	cl.mu.Lock()
	t := cl.st.term
	cl.st.term = nil
	cl.mu.Unlock()
	if t != nil {
		_ = t.Close()
	}
	cl.host.Invalidate()
}

// termPane draws the interactive TTY terminal.
func (cl *Client) termPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			dotCol := k.TextMuted
			stateLabel := "未连接"
			if st.term != nil {
				dotCol = k.Success
				stateLabel = "已连接"
			} else if st.termBusy {
				dotCol = k.Warning
				stateLabel = "连接中…"
			}
			mgo.Box(c).Width(8).Height(8).Shrink(0).Radius(4).Background(dotCol)
			mgo.Text(c, stateLabel).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if st.termErr != "" {
				mgo.Text(c, st.termErr).FontSize(caption).TextColor(k.Danger).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			} else {
				mgo.Text(c, "直接键入，Ctrl+C 中断，Ctrl+D 退出。").FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			}
			if st.term != nil {
				if mcore.IconAction(c, icons.Must("refresh-cw"), "重连").Clicked() {
					cl.closeTerm()
					cl.openTerm(st.detail)
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭终端").Clicked() {
					cl.closeTerm()
				}
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			if st.term == nil {
				if st.termBusy {
					mgo.Text(c, "连接终端中…").FontSize(caption).TextColor(k.TextMuted).Padding(u*2)
				} else {
					mgo.Text(c, "(终端未连接)").FontSize(caption).TextColor(k.TextMuted).Padding(u*2)
				}
				return
			}
			term.View(c, st.term).Fill()
		})
	})
}
