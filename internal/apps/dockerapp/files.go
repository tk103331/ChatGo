package dockerapp

import (
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// maxFileBytes caps a file's readable content.
const maxFileBytes = 1 << 20

// loadFiles lists a container directory.
func (cl *Client) loadFiles(id, path string) {
	cl.mu.Lock()
	if cl.st.filesLoading {
		cl.mu.Unlock()
		return
	}
	cl.st.filesLoading = true
	cl.st.filesPath = path
	cl.mu.Unlock()
	go func() {
		entries, err := listDir(id, path)
		cl.mu.Lock()
		cl.st.filesLoading = false
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.files = entries
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// viewFile reads a container file's content.
func (cl *Client) viewFile(id, path string) {
	cl.mu.Lock()
	cl.st.fileViewPath = path
	cl.st.fileContent = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
	go func() {
		content, err := readFile(id, path, maxFileBytes)
		cl.mu.Lock()
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.fileContent = content
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// filesPane draws the file browser for the container.
func (cl *Client) filesPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			if st.filesPath != "/" && st.filesPath != "" {
				if mcore.IconAction(c, icons.Must("chevron-up"), "上一级").Clicked() {
					parent := filepath.Dir(strings.TrimSuffix(st.filesPath, "/"))
					if parent == "." || parent == "" {
						parent = "/"
					}
					cl.loadFiles(st.detail, parent)
				}
			}
			mgo.Text(c, st.filesPath).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.loadFiles(st.detail, st.filesPath)
			}
		})
		if st.filesLoading {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
				mgo.Text(c, "读取目录中…").FontSize(caption).TextColor(k.TextMuted)
			})
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).MinWidth(0).Children(func() {
			mgo.Column(c).FillWidth().Padding(u, u*2).Gap(2).Children(func() {
					for _, e := range st.files {
					e := e
					name := e.Name
					icon := "file"
					col := k.TextMuted
					if e.IsDir {
						icon = "folder"
						col = k.Accent
						name += "/"
					}
					b := mgo.ButtonBase(c).Column().Key("fl:"+e.Name).
						FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).
						Radius(mtheme.ControlRadius).Padding(u, u).Children(func() {
						mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(col).Shrink(0)
						mgo.Text(c, name).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						if !e.IsDir {
							mgo.Text(c, humanSize(e.Size)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
						}
						mgo.Text(c, e.ModTime).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
					})
					if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					if b.Clicked() {
						if e.IsDir {
							cl.loadFiles(st.detail, joinPath(st.filesPath, e.Name))
						} else {
							cl.viewFile(st.detail, joinPath(st.filesPath, e.Name))
						}
					}
				}
			})
		})
		if st.fileViewPath != "" {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
				BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(k.Border).Children(func() {
				mgo.Text(c, st.fileViewPath).FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if mcore.IconAction(c, icons.Must("x"), "关闭文件").Clicked() {
					cl.mu.Lock()
					cl.st.fileViewPath = ""
					cl.st.fileContent = ""
					cl.mu.Unlock()
					cl.host.Invalidate()
				}
			})
			mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
				Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Padding(u*2).Children(func() {
				if st.fileContent == "" {
					mgo.Text(c, "(空文件或加载中)").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				mgo.Text(c, st.fileContent).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
			})
		}
	})
}

// joinPath joins a container path to a name, keeping a leading slash.
func joinPath(dir, name string) string {
	if dir == "/" || dir == "" {
		return "/" + strings.TrimPrefix(name, "/")
	}
	return strings.TrimSuffix(dir, "/") + "/" + strings.TrimPrefix(name, "/")
}
