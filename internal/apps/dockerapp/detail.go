package dockerapp

import (
	mgo "github.com/egoist/mygo/ui"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// openDetail opens a container's detail pane on the given tab.
func (cl *Client) openDetail(id string, tab int) {
	cl.mu.Lock()
	if cl.st.detail == id && cl.st.detailTab == tab {
		cl.mu.Unlock()
		return
	}
	cl.st.detail = id
	cl.st.detailTab = tab
	cl.st.logLines = nil
	cl.st.logState = mcode.LogViewerState{}
	cl.stopFollowLocked()
	cl.st.inspectText = ""
	cl.st.inspecting = false
	cl.st.termInput = ""
	cl.st.termErr = ""
	if cl.st.term != nil {
		cl.st.term.Close()
		cl.st.term = nil
	}
	cl.st.filesPath = ""
	cl.st.files = nil
	cl.st.fileContent = ""
	cl.st.fileViewPath = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
	cl.loadDetailTab(tab)
}

// setDetailTab switches the active detail tab, loading it lazily.
func (cl *Client) setDetailTab(tab int) {
	cl.mu.Lock()
	cl.st.detailTab = tab
	cl.mu.Unlock()
	cl.loadDetailTab(tab)
}

// loadDetailTab loads the active detail tab's data on first visit.
func (cl *Client) loadDetailTab(tab int) {
	cl.mu.Lock()
	id := cl.st.detail
	cl.mu.Unlock()
	if id == "" {
		return
	}
	switch tab {
	case tabInfo:
		cl.loadInspect(id)
	case tabLogs:
		cl.loadLogsTab(id)
	case tabTerm:
		cl.openTerm(id)
	case tabFiles:
		cl.loadFiles(id, "/")
	}
}

// loadInspect fetches the container's metadata.
func (cl *Client) loadInspect(id string) {
	cl.mu.Lock()
	if cl.st.inspecting {
		cl.mu.Unlock()
		return
	}
	cl.st.inspecting = true
	cl.st.inspectText = ""
	cl.mu.Unlock()
	go func() {
		text, err := inspectText(id)
		cl.mu.Lock()
		cl.st.inspecting = false
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.inspectText = text
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// loadLogsTab loads the container's recent logs.
func (cl *Client) loadLogsTab(id string) {
	cl.mu.Lock()
	if cl.st.logLoading {
		cl.mu.Unlock()
		return
	}
	cl.st.logLines = nil
	cl.st.logLoading = true
	cl.stopFollowLocked()
	cl.mu.Unlock()
	n := cl.tailN()
	go func() {
		lines, err := fetchLogs(id, n)
		cl.mu.Lock()
		cl.st.logLoading = false
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.logLines = toLogLines(lines)
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// closeDetail closes the detail pane, stopping streams.
func (cl *Client) closeDetail() {
	cl.mu.Lock()
	cl.stopFollowLocked()
	if cl.st.term != nil {
		cl.st.term.Close()
		cl.st.term = nil
	}
	cl.st.detail = ""
	cl.st.logLines = nil
	cl.st.files = nil
	cl.st.fileContent = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// detailPane draws the detail tab bar and the active tab's content.
func (cl *Client) detailPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			tabs := []struct {
				id    int
				label string
				icon  string
			}{
				{tabInfo, "信息", "info"},
				{tabLogs, "日志", "list"},
				{tabTerm, "终端", "terminal"},
				{tabFiles, "文件", "folder"},
			}
			for _, t := range tabs {
				t := t
				variant := minput.Ghost
				if st.detailTab == t.id {
					variant = minput.Secondary
				}
				if minput.Button(c, "", minput.ButtonOptions{Variant: variant, Icon: icons.Must(t.icon), Label: t.label}).Clicked() {
					cl.setDetailTab(t.id)
				}
			}
			mgo.Text(c, "容器 "+st.detail).FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if mcore.IconAction(c, icons.Must("x"), "关闭详情").Clicked() {
				cl.closeDetail()
			}
		})
		switch st.detailTab {
		case tabInfo:
			cl.infoPane(c, st, k, u, caption)
		case tabLogs:
			cl.logPane(c, st, k, u, caption)
		case tabTerm:
			cl.termPane(c, st, k, u, caption)
		case tabFiles:
			cl.filesPane(c, st, k, u, caption)
		}
	})
}

// infoPane draws the container's metadata text.
func (cl *Client) infoPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u*2, u*3).Gap(u*2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "容器信息").FontWeight(600).FontSize(caption).Grow(1)
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
					cl.loadInspect(st.detail)
				}
			})
			if st.inspecting {
				mgo.Row(c).FillWidth().Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
					mgo.Text(c, "加载中…").FontSize(caption).TextColor(k.TextMuted)
				})
			}
			if st.inspectText != "" {
				mgo.Box(c).FillWidth().Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u*2).Children(func() {
					mgo.Text(c, st.inspectText).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
				})
			}
		})
	})
}

// logPane draws the log viewer with toolbar.
func (cl *Client) logPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			minput.TextInput(c, &cl.st.logTailText, minput.TextInputOptions{Placeholder: "行数"}).Element.Width(60).Shrink(0)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("list"), Label: "Tail"}).Clicked() {
				cl.reloadLogs()
			}
			followVariant := minput.Ghost
			if st.logFollow {
				followVariant = minput.Secondary
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: followVariant, Icon: icons.Must("eye"), Label: "跟踪"}).Clicked() {
				cl.toggleLogFollow()
			}
			mgo.Box(c).Grow(1)
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.reloadLogs()
			}
		})
		if st.logLoading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "加载日志中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			mcode.LogViewer(c, cl.st.logLines, &cl.st.logState, mcode.LogViewerOptions{}).Element.Fill()
		})
	})
}
