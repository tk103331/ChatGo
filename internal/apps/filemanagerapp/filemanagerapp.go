package filemanagerapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/files"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是文件管理器助手。你可以通过 fs_list 列出目录，通过 fs_read 读取文本文件，通过 fs_write 写入文件，通过 fs_mkdir 新建目录，通过 fs_rename 重命名或移动，通过 fs_remove 删除，通过 fs_search 按名称搜索。所有路径都限制在工作目录内。删除和覆盖写入前先向用户确认。"

// state is the client's retained model.
type state struct {
	root       string
	explorer   files.FileExplorerState
	showHidden bool
	revision   uint64
	cache      map[string][]files.FileEntry
	cacheRev   uint64

	previewPath string
	previewName string
	previewSize int64
	previewText string
	previewErr  string
	previewLoad bool

	err      string
	splitW   float32
	selected int
}

// Client is one file manager bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	st   *state
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a file manager driving the given host.
func New(h Host) *Client {
	root := h.Root()
	if root == "" {
		root, _ = os.Getwd()
	}
	return &Client{host: h, st: &state{
		root:   root,
		cache:  map[string][]files.FileEntry{},
		splitW: 520,
	}}
}

// View renders the file manager: path bar, explorer and preview.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	st := cl.snapshot()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			cur := cl.currentDir(st)
			if cur != "" {
				bar := files.PathBar(c, cur, files.PathBarOptions{RootLabel: "工作区"})
				bar.Element.Grow(1).MinWidth(0)
				if bar.Navigated != "" {
					cl.navigate(bar.Navigated)
				}
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("plus"), Label: "新建文件夹"}).Clicked() {
				cl.promptMkdir()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("pencil"), Label: "重命名"}).Clicked() {
				cl.promptRename()
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("trash-2"), Label: "删除"}).Clicked() {
				cl.confirmRemove()
			}
			hidden := minput.Ghost
			if st.showHidden {
				hidden = minput.Secondary
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: hidden, Icon: icons.Must("eye"), Label: "隐藏文件"}).Clicked() {
				cl.toggleHidden()
			}
			if mcore.IconAction(c, icons.Must("refresh-cw"), "刷新").Clicked() {
				cl.refresh()
			}
		})
		if st.err != "" {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Children(func() {
				mgo.Text(c, st.err).FontSize(caption).TextColor(k.Danger).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			})
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			cl.body(c, st, k, u, caption)
		})
		if st.previewPath != "" || len(st.explorer.Selected()) > 0 {
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).
				BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(k.Border).Children(func() {
				cl.statusLine(c, st, k, caption)
			})
		}
	})
}

// snapshot returns a copy of the state for rendering.
func (cl *Client) snapshot() *state {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cp := *cl.st
	return &cp
}

// body draws the explorer and the preview pane.
func (cl *Client) body(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	explorer := func() {
		cl.explorerPane(c, st, k, u, caption)
	}
	if st.previewPath == "" {
		mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Children(explorer)
		return
	}
	mgo.Split(c, &cl.st.splitW, explorer, func() {
		cl.previewPane(c, st, k, u, caption)
	}).Fill()
}

// explorerPane draws the FileExplorer component.
func (cl *Client) explorerPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	root := st.root
	res := files.FileExplorer(c, &cl.st.explorer, files.FileExplorerOptions{
		Roots:    []string{root},
		Revision: st.revision,
		Folders: func(dir string) []string {
			return listDirs(dir, st.showHidden)
		},
		Entries: func(dir string) []files.FileEntry {
			return cl.cachedEntries(dir, st.showHidden)
		},
	})
	res.Element.FillWidth().Grow(1).MinWidth(0).MinHeight(0)
	if res.Navigated {
		cl.onNavigate(cl.st.explorer.Current)
	}
	if res.Changed {
		cl.onSelect(cl.st.explorer.Selected())
	}
	if res.Opened != nil {
		cl.openPreview(res.Opened.Path)
	}
}

// previewPane draws the file preview.
func (cl *Client) previewPane(c *mgo.Context, st *state, k mtheme.Tokens, u, caption float32) {
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).Children(func() {
			mgo.Text(c, filepath.Base(st.previewPath)).FontSize(caption).FontWeight(600).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			if st.previewSize > 0 {
				mgo.Text(c, humanBytes(st.previewSize)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭预览").Clicked() {
				cl.closePreview()
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Padding(u*2).Children(func() {
			if st.previewErr != "" {
				mgo.Text(c, st.previewErr).FontSize(caption).TextColor(k.Danger)
				return
			}
			status := data.DataReady
			if st.previewLoad {
				status = data.DataLoading
			}
			res := files.FilePreview(c, files.PreviewFile{
				Name: st.previewName,
				Size: st.previewSize,
				Text: st.previewText,
			}, files.FilePreviewOptions{Status: status})
			res.Element.Fill()
			if res.Closed {
				cl.closePreview()
			}
			if res.Opened {
				cl.host.OpenFile(st.previewPath)
			}
			if res.Retried {
				cl.openPreview(st.previewPath)
			}
		})
	})
}

// statusLine draws the selection summary.
func (cl *Client) statusLine(c *mgo.Context, st *state, k mtheme.Tokens, caption float32) {
	sel := st.explorer.Selected()
	msg := ""
	if len(sel) == 1 {
		msg = sel[0]
	} else if len(sel) > 1 {
		msg = fmt.Sprintf("已选择 %d 项", len(sel))
	} else if st.previewPath != "" {
		msg = st.previewPath
	}
	mgo.Text(c, msg).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
}

// cachedEntries returns a directory listing, caching per revision.
func (cl *Client) cachedEntries(dir string, showHidden bool) []files.FileEntry {
	cl.mu.Lock()
	if cl.st.cacheRev != cl.st.revision || cl.st.cache == nil {
		cl.st.cache = map[string][]files.FileEntry{}
		cl.st.cacheRev = cl.st.revision
	}
	if e, ok := cl.st.cache[dir]; ok {
		cl.mu.Unlock()
		return e
	}
	cl.mu.Unlock()
	entries, _ := readDir(dir, showHidden)
	cl.mu.Lock()
	cl.st.cache[dir] = entries
	cl.mu.Unlock()
	return entries
}
