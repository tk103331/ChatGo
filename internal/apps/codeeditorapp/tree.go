package codeeditorapp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

// treeRoot is the key of the workspace root.
const treeRoot = "."

// treePanel draws the workspace file tree.
func (cl *Client) treePanel(c *mgo.Context, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Background(k.Background).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "文件").FontSize(caption).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("refresh-cw"), Label: "刷新"}).Clicked() {
				cl.resetTree()
			}
		})
		v := data.Tree(c, &cl.tree, data.TreeOptions[string]{
			Roots: []string{treeRoot},
			Children: func(dir string) []string {
				entries := cl.treeEntries(dir)
				if entries == nil {
					return nil
				}
				out := make([]string, 0, len(entries))
				for _, e := range entries {
					out = append(out, e)
				}
				return out
			},
			Label: func(p string) string { return cl.treeLabel(p) },
			ItemStatus: func(p string) data.DataStatus {
				if cl.treeIsDir[p] && !cl.treeLoaded[p] {
					return data.DataUnloaded
				}
				return data.DataReady
			},
			Load: func(p string) { cl.loadTree(p) },
			Empty: "空目录",
		})
		v.Element.Grow(1).MinHeight(0).MinWidth(0)
		if v.Changed() || v.Submitted() {
			if sel, ok := cl.tree.Selected(); ok {
				cl.openTreeItem(sel)
			}
		}
	})
}

// openTreeItem opens a file tree item, ignoring folders.
func (cl *Client) openTreeItem(key string) {
	if cl.treeIsDir[key] {
		return
	}
	cl.openAndRegister(key)
	cl.host.Invalidate()
}

// treeEntries returns a loaded directory's children, loading the root on
// first use.
func (cl *Client) treeEntries(dir string) []string {
	if !cl.treeLoaded[dir] {
		cl.loadTree(dir)
		return nil
	}
	return cl.treeDirs[dir]
}

// loadTree reads a directory's children off the UI work.
func (cl *Client) loadTree(dir string) {
	if cl.treeLoaded[dir] {
		return
	}
	cl.treeLoaded[dir] = true
	abs := cl.abs(dir)
	entries, err := os.ReadDir(abs)
	if err != nil {
		cl.treeDirs[dir] = nil
		return
	}
	var dirs, fileList []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		child := name
		if dir != treeRoot {
			child = filepath.ToSlash(filepath.Join(dir, name))
		}
		if e.IsDir() {
			cl.treeIsDir[child] = true
			dirs = append(dirs, child)
		} else {
			fileList = append(fileList, child)
		}
	}
	sort.Strings(dirs)
	sort.Strings(fileList)
	cl.treeDirs[dir] = append(dirs, fileList...)
}

// treeLabel renders a tree key as a name.
func (cl *Client) treeLabel(p string) string {
	if p == treeRoot {
		if root := cl.host.Root(); root != "" {
			return filepath.Base(root)
		}
		return "工作区"
	}
	name := filepath.Base(p)
	if cl.treeIsDir[p] {
		return name + "/"
	}
	return name
}

// resetTree drops the cached tree so it reloads.
func (cl *Client) resetTree() {
	cl.tree = data.TreeState[string]{}
	cl.treeDirs = map[string][]string{}
	cl.treeLoaded = map[string]bool{}
	cl.treeIsDir = map[string]bool{}
	cl.loadTree(treeRoot)
	cl.host.Invalidate()
}
