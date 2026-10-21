package dockerapp

import (
	"time"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// openTerm starts an interactive shell for the container.
func (cl *Client) openTerm(id string) {
	cl.mu.Lock()
	if cl.st.term != nil {
		cl.mu.Unlock()
		return
	}
	cl.st.termBusy = true
	cl.st.termErr = ""
	cl.mu.Unlock()
	go func() {
		tc, err := openTerminal(id, "")
		cl.mu.Lock()
		cl.st.termBusy = false
		if err != nil {
			cl.st.termErr = shortErr(err)
		} else {
			cl.st.term = tc
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
		go func() {
			for {
				cl.host.Invalidate()
				tc := func() *termConn {
					cl.mu.Lock()
					defer cl.mu.Unlock()
					return cl.st.term
				}()
				if tc == nil {
					return
				}
				select {
				case <-tc.done:
					return
				default:
				}
				cl.mu.Lock()
				dead := cl.st.term == nil || cl.st.detail == ""
				cl.mu.Unlock()
				if dead {
					return
				}
				time.Sleep(300 * time.Millisecond)
			}
		}()
	}()
}

// closeTerm closes the terminal session.
func (cl *Client) closeTerm() {
	cl.mu.Lock()
	if cl.st.term != nil {
		cl.st.term.Close()
		cl.st.term = nil
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// sendTerm writes a command line to the terminal.
func (cl *Client) sendTerm() {
	cl.mu.Lock()
	tc := cl.st.term
	line := cl.st.termInput
	cl.mu.Unlock()
	if tc == nil {
		return
	}
	if err := tc.Send(line); err != nil {
		cl.mu.Lock()
		cl.st.termErr = shortErr(err)
		cl.mu.Unlock()
		cl.host.Invalidate()
	}
}

// termPane draws the interactive terminal.
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
				mgo.Text(c, "输入命令，回车执行。每行一条命令。").FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			}
			if st.term != nil {
				if mcore.IconAction(c, icons.Must("x"), "关闭终端").Clicked() {
					cl.closeTerm()
				}
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Padding(u, u*2).Children(func() {
			var lines []string
			if st.term != nil {
				lines = st.term.Lines()
			}
			if len(lines) == 0 {
				if st.termBusy {
					mgo.Text(c, "连接终端中…").FontSize(caption).TextColor(k.TextMuted)
				} else {
					mgo.Text(c, "(终端未连接)").FontSize(caption).TextColor(k.TextMuted)
				}
				return
			}
			mgo.Column(c).Fill().Gap(2).Children(func() {
				for _, ln := range lines {
					mgo.Text(c, ln).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text).Grow(1)
				}
			})
		})
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "$").FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Success).Shrink(0)
			t := minput.TextInput(c, &cl.st.termInput, minput.TextInputOptions{Placeholder: "输入命令"})
			t.Element.Grow(1).MinWidth(0)
			if t.Submitted() {
				cl.sendTerm()
				cl.mu.Lock()
				cl.st.termInput = ""
				cl.mu.Unlock()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("terminal"), Label: "发送"}).Clicked() {
				cl.sendTerm()
				cl.mu.Lock()
				cl.st.termInput = ""
				cl.mu.Unlock()
			}
		})
	})
}
