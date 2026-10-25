package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// filePickState is the file chooser the Editor tab opens with: the directory
// it is listing, and what to do with the file it picks.
type filePickState struct {
	open bool
	dir  string
	// picked holds a file chosen this frame, opened at the start of the next.
	picked string
	// action receives the chosen file's absolute path.
	action func(string)
}

// askFile opens the file chooser at dir, or at the workspace when dir is
// empty.
func (cw *ChatWindow) askFile(dir string, action func(string)) {
	if dir == "" {
		dir = cw.fileRoot()
	}
	cw.filePick = filePickState{open: true, dir: dir, action: action}
}

// openEditorTabPick lets the user choose a file to edit from the center area,
// starting in the workspace and free to walk anywhere on disk.
func (cw *ChatWindow) openEditorTabPick(pane int) {
	cw.center.focus = pane
	cw.askFile(cw.fileRoot(), func(path string) { cw.openEditor(path) })
}

// filePickDialog shows the pending file chooser.
func (cw *ChatWindow) filePickDialog(c *mgo.Context) {
	// A pick made last frame is opened here, before the dialog is laid out:
	// the content callback below runs while the dialog is built, so acting on
	// its result there would be a frame too early to see it.
	if cw.filePick.open && cw.filePick.picked != "" {
		path := cw.filePick.picked
		action := cw.filePick.action
		cw.filePick = filePickState{}
		if action != nil {
			action(path)
		}
	}
	if !cw.filePick.open {
		return
	}
	kit.Dialog(c, &cw.filePick.open, kit.DialogOptions{
		Title: "选择文件",
		Width: 560,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.filePick = filePickState{}
			}
		},
	}, func() {
		cw.filePickBody(c)
	})
	if !cw.filePick.open {
		cw.filePick = filePickState{}
	}
}

// filePickBody is the chooser's content: a location bar, the current
// directory's entries and a way to a parent or from the filesystem root.
func (cw *ChatWindow) filePickBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Gap(u).Children(func() {
		cw.filePickBar(c)
		dir := cw.filePick.dir
		entries, err := os.ReadDir(dir)
		if err != nil {
			mgo.Text(c, "无法读取目录："+err.Error()).TextColor(k.Danger).FontSize(caption)
			return
		}
		sortDirEntries(entries)
		// The dialog already wraps its content in a scroll view, so the
		// listing needs a fixed height of its own: without it the nested
		// scroll collapses and the entries fall outside the viewport.
		mgo.Box(c).FillWidth().Height(u * 46).Clip().Children(func() {
			mgo.Scroll(c).Fill().Children(func() {
				mgo.Column(c).FillWidth().Gap(2).Children(func() {
					if parent := filepath.Dir(dir); parent != dir {
						if cw.filePickRow(c, "folder", "..", "上级目录").Clicked() {
							cw.filePick.dir = parent
						}
					}
					shown := 0
					for _, e := range entries {
						name := e.Name()
						if strings.HasPrefix(name, ".") {
							continue
						}
						if shown >= 500 {
							break
						}
						shown++
						path := filepath.Join(dir, name)
						if e.IsDir() {
							switch name {
							case "node_modules", "vendor", "dist", "build", "target":
								continue
							}
							if cw.filePickRow(c, "folder", name, path).Clicked() {
								cw.filePick.dir = path
							}
							continue
						}
						if cw.filePickRow(c, "file-text", name, path).DoubleClicked() {
							cw.filePick.picked = path
						}
					}
					if shown == 0 {
						mgo.Text(c, "这个目录是空的").TextColor(k.TextMuted).FontSize(caption)
					}
				})
			})
		})
		mgo.Text(c, "双击文件打开").TextColor(k.TextMuted).FontSize(caption)
	})
}

// filePickBar shows the directory being listed, with a button back to the
// workspace and one to a chosen directory.
func (cw *ChatWindow) filePickBar(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
		mgo.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(k.Accent).Shrink(0)
		mgo.Text(c, cw.filePick.dir).Font(mtheme.MonoFont).FontSize(caption).
			SingleLine().Ellipsis("…").Grow(1).MinWidth(0).Tooltip(cw.filePick.dir)
		if kit.Button(c, "工作区", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
			cw.filePick.dir = cw.fileRoot()
		}
		if kit.Button(c, "选择目录…", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
			cw.pickDirectory("选择要浏览的目录", func(dir string) { cw.filePick.dir = dir })
		}
	})
}

// filePickRow draws one directory entry and returns it for its click tests.
func (cw *ChatWindow) filePickRow(c *mgo.Context, icon, label, tooltip string) mgo.Element {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).Row().FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u/2, u).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
		Radius(mtheme.ControlRadius).Label(label).Tooltip(tooltip)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	color := k.TextMuted
	if icon == "folder" {
		color = k.Accent
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(color).Shrink(0)
		mgo.Text(c, label).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
	})
	return b
}

// sortDirEntries orders a directory the way the file tree does: folders
// first, then names case-insensitively.
func sortDirEntries(entries []os.DirEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
}
