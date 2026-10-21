package dockerapp

import (
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// maxFileBytes caps a file's readable content.
const maxFileBytes = 1 << 20

// loadFiles lists a container directory and records its children.
func (cl *Client) loadFiles(id, path string) {
	cl.mu.Lock()
	if cl.st.filesLoaded[path] {
		cl.mu.Unlock()
		return
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
	go func() {
		entries, err := listDir(id, path)
		cl.mu.Lock()
		if err != nil {
			cl.st.filesErr = shortErr(err)
		} else {
			cl.st.files[path] = entries
			cl.st.filesLoaded[path] = true
			for _, e := range entries {
				if e.IsDir {
					cl.st.filesDir[joinPath(path, e.Name)] = true
				}
			}
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
			cl.st.filesErr = shortErr(err)
		} else {
			cl.st.fileContent = content
			cl.st.filesErr = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// fileEntries returns a loaded directory's entries.
func (cl *Client) fileEntries(path string) ([]fileEntry, bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	e, ok := cl.st.files[path]
	return e, ok
}

// fileIsDir reports whether a path is a directory.
func (cl *Client) fileIsDir(path string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.st.filesDir[path]
}

// fileLoaded reports whether a directory's children were loaded.
func (cl *Client) fileLoaded(path string) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.st.filesLoaded[path]
}

// fileLabel renders a path's tree label.
func (cl *Client) fileLabel(path string) string {
	if path == "/" {
		return "/"
	}
	name := filepath.Base(path)
	if cl.fileIsDir(path) {
		return name + "/"
	}
	return name
}

// fileSize returns a file's size, or 0 for directories.
func (cl *Client) fileSize(path string) int64 {
	parent := filepath.Dir(path)
	if parent == "." || parent == "" {
		parent = "/"
	}
	entries, ok := cl.fileEntries(parent)
	if !ok {
		return 0
	}
	name := filepath.Base(path)
	for _, e := range entries {
		if e.Name == name {
			return e.Size
		}
	}
	return 0
}

// filesPane draws the container file tree and the selected file's content.
func (cl *Client) filesPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "文件").FontSize(caption).FontWeight(600).Shrink(0)
			if st.filesErr != "" {
				mgo.Text(c, st.filesErr).FontSize(caption).TextColor(k.Danger).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			} else {
				mgo.Text(c, "双击文件查看内容").FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.resetFiles(st.detail)
			}
		})
		body := func() {
			cl.fileTree(c, st, k, u, caption)
		}
		if st.fileViewPath != "" {
			mgo.Split(c, &cl.st.splitFilesH, body, func() {
				cl.fileView(c, st, k, u, caption)
			}).Fill()
			return
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(body)
	})
}

// fileTree draws the lazy directory tree.
func (cl *Client) fileTree(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	detail := st.detail
	v := data.Tree(c, &cl.st.filesTree, data.TreeOptions[string]{
		Roots: []string{"/"},
		Children: func(path string) []string {
			entries, ok := cl.fileEntries(path)
			if !ok {
				return nil
			}
			out := make([]string, 0, len(entries))
			for _, e := range entries {
				out = append(out, joinPath(path, e.Name))
			}
			return out
		},
		Label: func(path string) string { return cl.fileLabel(path) },
		ItemStatus: func(path string) data.DataStatus {
			if cl.fileIsDir(path) && !cl.fileLoaded(path) {
				return data.DataUnloaded
			}
			return data.DataReady
		},
		Load: func(path string) {
			if path == "/" || cl.fileIsDir(path) {
				cl.loadFiles(detail, path)
			}
		},
		Empty: "空目录",
	})
	if v.Changed() || v.Submitted() {
		if sel, ok := cl.st.filesTree.Selected(); ok {
			if cl.fileIsDir(sel) {
				cl.st.filesTree.SetOpen(sel, true)
				cl.loadFiles(detail, sel)
			} else {
				cl.viewFile(detail, sel)
			}
		}
	}
}

// fileView draws the selected file's content.
func (cl *Client) fileView(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(k.Border).Children(func() {
			mgo.Text(c, st.fileViewPath).FontSize(caption).Font(mtheme.MonoFont).FontWeight(600).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			mgo.Text(c, humanSize(cl.fileSize(st.fileViewPath))).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if mcore.IconAction(c, icons.Must("x"), "关闭文件").Clicked() {
				cl.mu.Lock()
				cl.st.fileViewPath = ""
				cl.st.fileContent = ""
				cl.mu.Unlock()
				cl.host.Invalidate()
			}
		})
		mgo.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			mgo.Box(c).FillWidth().Padding(u*2).Children(func() {
				if st.fileContent == "" {
					mgo.Text(c, "(空文件或加载中)").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				mgo.Text(c, st.fileContent).FontSize(caption).Font(mtheme.MonoFont).TextColor(k.Text)
			})
		})
	})
}

// resetFiles clears the tree cache so it reloads.
func (cl *Client) resetFiles(id string) {
	cl.mu.Lock()
	cl.st.files = map[string][]fileEntry{}
	cl.st.filesLoaded = map[string]bool{}
	cl.st.filesDir = map[string]bool{}
	cl.st.filesErr = ""
	cl.st.filesTree = data.TreeState[string]{}
	cl.st.fileViewPath = ""
	cl.st.fileContent = ""
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// joinPath joins a container path to a name, keeping a leading slash.
func joinPath(dir, name string) string {
	if dir == "/" || dir == "" {
		return "/" + strings.TrimPrefix(name, "/")
	}
	return strings.TrimSuffix(dir, "/") + "/" + strings.TrimPrefix(name, "/")
}
