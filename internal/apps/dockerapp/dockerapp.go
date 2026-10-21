package dockerapp

import (
	"fmt"
	"sync"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 Docker 管理助手。你可以通过 docker_list 列出容器/镜像/卷/网络，通过 docker_action 对容器执行 start/stop/restart/pause/unpause/remove，通过 docker_logs 查看容器日志，通过 docker_run 运行新容器，通过 docker_exec 在容器内执行命令，通过 docker_prune 清理未使用资源，通过 docker_info 查看系统信息。执行删除、清理等破坏性操作前先向用户确认。"

// Pages of the Docker manager.
const (
	pageContainers = iota
	pageImages
	pageVolumes
	pageNetworks
	pageSystem
)

const logTailDefault = 200

// state is the client's retained model.
type state struct {
	page       int
	containers []Container
	images     []Image
	volumes    []Volume
	networks   []Network
	sysInfo    string
	diskUsage  string
	loading    bool
	err        string
	lastLoad   time.Time

	logFor      string
	logLines    []mcode.LogLine
	logState    mcode.LogViewerState
	logFollow   bool
	logTailText string
	logLoading  bool
	logStop     func()

	splitLogW float32
}

// Client is one Docker manager bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	st   *state
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a Docker manager driving the given host.
func New(h Host) *Client {
	return &Client{host: h, st: &state{splitLogW: 300, logTailText: fmt.Sprintf("%d", logTailDefault)}}
}

// View renders the Docker manager: page tabs above a content area.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	st := cl.snapshot()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			pages := []struct {
				id    int
				label string
				icon  string
			}{
				{pageContainers, "容器", "square"},
				{pageImages, "镜像", "image"},
				{pageVolumes, "卷", "archive"},
				{pageNetworks, "网络", "share-2"},
				{pageSystem, "系统", "settings"},
			}
			for _, p := range pages {
				p := p
				variant := minput.Ghost
				if st.page == p.id {
					variant = minput.Secondary
				}
				if minput.Button(c, p.label, minput.ButtonOptions{Variant: variant, Icon: icons.Must(p.icon), Label: p.label}).Clicked() {
					cl.setPage(p.id)
				}
			}
			mgo.Box(c).Grow(1)
			if st.err != "" {
				mgo.Text(c, st.err).FontSize(caption).TextColor(k.Danger).MaxWidth(300).SingleLine().Ellipsis("…")
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.Refresh()
			}
		})
		if st.loading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "加载中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			switch st.page {
			case pageContainers:
				cl.containersView(c, st)
			case pageImages:
				cl.imagesView(c, st)
			case pageVolumes:
				cl.volumesView(c, st)
			case pageNetworks:
				cl.networksView(c, st)
			case pageSystem:
				cl.systemView(c, st)
			}
		})
	})
}

// snapshot returns a copy of the state for rendering.
func (cl *Client) snapshot() *state {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cp := *cl.st
	return &cp
}

// setPage switches the active page and loads its data.
func (cl *Client) setPage(p int) {
	cl.mu.Lock()
	cl.st.page = p
	cl.st.err = ""
	cl.mu.Unlock()
	cl.loadPage(p)
}

// Refresh reloads the current page's data.
func (cl *Client) Refresh() {
	cl.mu.Lock()
	p := cl.st.page
	cl.mu.Unlock()
	cl.loadPage(p)
}

// containersView draws the container list with per-row actions and an
// optional log panel below.
func (cl *Client) containersView(c *mgo.Context, st *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if len(st.containers) == 0 {
		cl.emptyView(c, "没有容器")
		return
	}
	mgo.Column(c).Fill().Children(func() {
		if st.logFor != "" {
			mgo.Split(c, &cl.st.splitLogW, func() {
				cl.containerList(c, st, k, u, caption)
			}, func() {
				cl.logPanel(c, st, k, u, caption)
			}).Column().Grow(1).MinHeight(0)
			return
		}
		cl.containerList(c, st, k, u, caption)
	})
}

// containerList draws the scrollable container rows.
func (cl *Client) containerList(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u, u*2).Gap(u).Children(func() {
			for _, ct := range st.containers {
				ct := ct
				mgo.Row(c).FillWidth().Shrink(0).Gap(u*2).AlignItems(mgo.Center).
					Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u, u*2).Children(func() {
					statusCol := k.Success
					if ct.State != "running" {
						statusCol = k.TextMuted
					}
					if ct.State == "paused" {
						statusCol = k.Warning
					}
					mgo.Box(c).Width(8).Height(8).Shrink(0).Radius(4).Background(statusCol)
					mgo.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						mgo.Text(c, ct.Name()).FontWeight(600).FontSize(caption).SingleLine().Ellipsis("…")
						mgo.Text(c, ct.Image).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
					})
					if ct.Ports != "" {
						mgo.Text(c, ct.Ports).FontSize(caption).TextColor(k.TextMuted).MaxWidth(200).SingleLine().Ellipsis("…")
					}
					mgo.Text(c, ct.Status).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
					cl.rowActions(c, ct.State, ct.ShortID(), u)
				})
			}
		})
	})
}

// rowActions draws the action buttons for one container.
func (cl *Client) rowActions(c *mgo.Context, state, id string, u float32) {
	iconBtn := func(icon, label string) bool {
		return mcore.IconAction(c, icons.Must(icon), label).Tooltip(label).Clicked()
	}
	switch state {
	case "running":
		if iconBtn("square", "停止") {
			cl.doAction("stop", id)
		}
		if iconBtn("pause", "暂停") {
			cl.doAction("pause", id)
		}
		if iconBtn("refresh-cw", "重启") {
			cl.doAction("restart", id)
		}
	case "paused":
		if iconBtn("play", "继续") {
			cl.doAction("unpause", id)
		}
		if iconBtn("square", "停止") {
			cl.doAction("stop", id)
		}
	default:
		if iconBtn("play", "启动") {
			cl.doAction("start", id)
		}
	}
	if iconBtn("list", "日志") {
		cl.openLogs(id)
	}
	if iconBtn("trash-2", "删除") {
		cl.confirmAction("删除容器", "确定删除容器 "+id+" 吗？", "remove", id)
	}
}

// logPanel draws the log viewer for the selected container.
func (cl *Client) logPanel(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().Background(k.Background).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			mgo.Text(c, "日志: "+st.logFor).FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
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
			if mcore.IconAction(c, icons.Must("x"), "关闭日志").Clicked() {
				cl.closeLogs()
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

// imagesView draws the image list with remove actions.
func (cl *Client) imagesView(c *mgo.Context, st *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if len(st.images) == 0 {
		cl.emptyView(c, "没有镜像")
		return
	}
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u, u*2).Gap(u).Children(func() {
			for _, im := range st.images {
				im := im
				mgo.Row(c).FillWidth().Shrink(0).Gap(u*2).AlignItems(mgo.Center).
					Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u, u*2).Children(func() {
					mgo.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						mgo.Text(c, im.Tag()).FontWeight(600).FontSize(caption).SingleLine().Ellipsis("…")
						mgo.Text(c, im.ShortID()).FontSize(caption).TextColor(k.TextMuted)
					})
					mgo.Text(c, humanSize(im.Size)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
					if mcore.IconAction(c, icons.Must("trash-2"), "删除镜像").Clicked() {
						cl.confirmAction("删除镜像", "确定删除镜像 "+im.Tag()+" 吗？", "rmi", im.ShortID())
					}
				})
			}
		})
	})
}

// volumesView draws the volume list with remove actions.
func (cl *Client) volumesView(c *mgo.Context, st *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if len(st.volumes) == 0 {
		cl.emptyView(c, "没有卷")
		return
	}
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u, u*2).Gap(u).Children(func() {
			for _, v := range st.volumes {
				v := v
				mgo.Row(c).FillWidth().Shrink(0).Gap(u*2).AlignItems(mgo.Center).
					Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u, u*2).Children(func() {
					mgo.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						mgo.Text(c, v.Name).FontWeight(600).FontSize(caption).SingleLine().Ellipsis("…")
						mgo.Text(c, v.Driver).FontSize(caption).TextColor(k.TextMuted)
					})
					if mcore.IconAction(c, icons.Must("trash-2"), "删除卷").Clicked() {
						cl.confirmAction("删除卷", "确定删除卷 "+v.Name+" 吗？", "volume-rm", v.Name)
					}
				})
			}
		})
	})
}

// networksView draws the network list with remove actions.
func (cl *Client) networksView(c *mgo.Context, st *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if len(st.networks) == 0 {
		cl.emptyView(c, "没有网络")
		return
	}
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u, u*2).Gap(u).Children(func() {
			for _, n := range st.networks {
				n := n
				mgo.Row(c).FillWidth().Shrink(0).Gap(u*2).AlignItems(mgo.Center).
					Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u, u*2).Children(func() {
					mgo.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						mgo.Text(c, n.Name).FontWeight(600).FontSize(caption).SingleLine().Ellipsis("…")
						mgo.Text(c, n.Driver+" · "+n.Scope).FontSize(caption).TextColor(k.TextMuted)
					})
					if mcore.IconAction(c, icons.Must("trash-2"), "删除网络").Clicked() {
						cl.confirmAction("删除网络", "确定删除网络 "+n.Name+" 吗？", "network-rm", n.Name)
					}
				})
			}
		})
	})
}

// systemView draws the system info and disk usage.
func (cl *Client) systemView(c *mgo.Context, st *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Padding(u*2, u*3).Gap(u*2).Children(func() {
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "系统信息").FontWeight(600).FontSize(caption).Grow(1)
				if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("trash-2"), Label: "清理未使用资源"}).Clicked() {
					cl.confirmAction("清理资源", "确定执行 docker system prune 吗？这将删除所有未使用的镜像、容器、网络和构建缓存。", "prune-system", "")
				}
			})
			if st.sysInfo != "" {
				mgo.Box(c).FillWidth().Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u*2).Children(func() {
					mgo.Text(c, st.sysInfo).FontSize(caption).Font(mtheme.MonoFont).
						TextColor(k.Text)
				})
			}
			if st.diskUsage != "" {
				mgo.Text(c, "磁盘使用").FontWeight(600).FontSize(caption)
				mgo.Box(c).FillWidth().Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
					Radius(mtheme.ControlRadius).Padding(u*2).Children(func() {
					mgo.Text(c, st.diskUsage).FontSize(caption).Font(mtheme.MonoFont).
						TextColor(k.Text)
				})
			}
		})
	})
}

// emptyView draws a centered placeholder.
func (cl *Client) emptyView(c *mgo.Context, msg string) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().Center().Children(func() {
		mgo.Text(c, msg).FontSize(caption).TextColor(k.TextMuted)
	})
}
