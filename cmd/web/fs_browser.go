package main

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// loadFS lists one directory of the host's file system for the workspace
// browser. An empty dir starts at the host user's home.
func (a *app) loadFS(dir string) {
	a.fsLoading, a.fsErr = true, ""
	a.win.Update(func() {})
	go func() {
		var resp struct {
			Dir     string   `json:"dir"`
			Parent  string   `json:"parent"`
			Entries []fsItem `json:"entries"`
		}
		err := apiGet("/api/fs?dir="+url.QueryEscape(dir), &resp)
		a.win.Update(func() {
			a.fsLoading = false
			if err != nil {
				a.fsErr = err.Error()
				return
			}
			a.fsDir, a.fsParent, a.fsEntries = resp.Dir, resp.Parent, resp.Entries
		})
	}()
}

// openFSBrowse resets the browser and lists the workspace-to-be's directory,
// so the walk starts where the user was heading.
func (a *app) openFSBrowse() {
	a.fsOpen = true
	start := strings.TrimSpace(a.newCustomDir)
	if start == "" {
		start = a.fsDir
	}
	a.loadFS(start)
}

// fsBrowserButton draws the browse control: a button that opens a popover
// walking the connected host's file system, since a phone cannot see it.
func (a *app) fsBrowserButton(c *ui.Context, t *ui.Theme) {
	b := kit.Button(c, "浏览主机…", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("folder")})
	if b.Clicked() {
		a.openFSBrowse()
	}
	kit.Popover(c, b, &a.fsOpen, kit.PopoverOptions{
		Title: "选择工作区目录", Width: 480, Up: true, MaxHeight: 420,
	}, func() {
		u := uikit.Unit(c)
		caption := uikit.FontSize(c, mtheme.CaptionSize)
		// Current directory, the address line of the walk.
		ui.Text(c, a.fsDir).Font(mtheme.MonoFont).FontSize(caption).
			TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		switch {
		case a.fsLoading:
			ui.Text(c, "正在读取…").FontSize(caption).TextColor(t.TextMuted).Padding(u * 3)
		case a.fsErr != "":
			ui.Text(c, a.fsErr).FontSize(caption).TextColor(t.Danger).MaxLines(2)
			if kit.Button(c, "重试", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
				a.loadFS(a.fsDir)
			}
		default:
			ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(u * 0.5).Children(func() {
					// Up one level, until the file system root.
					if a.fsParent != "" && a.fsParent != a.fsDir {
						row := ui.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
							Padding(u, u*2).Gap(u).AlignItems(ui.Center).Justify(ui.Start).
							Radius(mtheme.ControlRadius).Label("上级目录")
						if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							ui.Icon(c, icons.Must("chevron-left")).Size(u*4.5, u*4.5).
								Shrink(0).TextColor(t.TextMuted)
							ui.Text(c, "上级目录").Grow(1).MinWidth(0)
							ui.Text(c, a.fsParent).FontSize(caption).
								TextColor(t.TextMuted).SingleLine().Ellipsis("…").MaxWidth(240)
						})
						if row.Clicked() {
							a.loadFS(a.fsParent)
						}
					}
					for _, e := range a.fsEntries {
						e := e
						row := ui.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
							Padding(u, u*2).Gap(u).AlignItems(ui.Center).Justify(ui.Start).
							Radius(mtheme.ControlRadius).Label(e.Name)
						if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							ui.Icon(c, icons.Must("folder")).Size(u*4.5, u*4.5).
								Shrink(0).TextColor(t.Accent)
							ui.Text(c, e.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						})
						if row.Clicked() {
							a.loadFS(e.Path)
						}
					}
					if len(a.fsEntries) == 0 && (a.fsParent == "" || a.fsParent == a.fsDir) {
						ui.Text(c, "没有子目录").FontSize(caption).TextColor(t.TextMuted).
							Padding(u * 2)
					}
				})
			})
			// Choose the directory being looked at.
			if kit.Button(c, "选择此目录", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				a.newCustomDir = a.fsDir
				a.fsOpen = false
			}
		}
	})
}
