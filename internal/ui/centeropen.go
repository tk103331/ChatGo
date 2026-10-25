// Center tab openers: the actions that create or focus a center tab, shared
// by the new-tab menu, the file tree, the Git panel and the agent tools.
package ui

import (
	"os"
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// openPreview opens a file in the center area, or focuses the tab already
// showing it. Images decode to an image tab, everything else to source.
func (cw *ChatWindow) openPreview(path string) {
	cw.openPreviewAt(path, 0)
}

// openPreviewAt opens a file and scrolls to a one-based line, 0 for the top.
func (cw *ChatWindow) openPreviewAt(path string, line int) {
	id := "file:" + path
	if tab := cw.focusCenterTab(id); tab != nil {
		if line > 0 {
			tab.GotoLine = line
		}
		return
	}
	t := &centerTab{ID: id, Path: path, Title: titleOfPath(path)}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		data, err := os.ReadFile(path)
		switch {
		case err != nil:
			t.Kind, t.Body = centerText, "读取失败："+err.Error()
		default:
			if bmp, err := mgo.DecodeBitmap(data); err == nil {
				t.Kind, t.Bmp = centerImage, bmp
			} else {
				t.Kind, t.Body = centerText, "无法解码图片："+err.Error()
			}
		}
	case ".svg":
		if data, err := os.ReadFile(path); err == nil {
			if svg, err := mgo.ParseSVG(data); err == nil {
				t.Kind, t.Svg = centerImage, svg
			}
		}
		if t.Kind == "" {
			t.Kind, t.Body = centerText, "无法解码 SVG"
		}
	default:
		t.Kind, t.Lang = centerText, langOfExt(ext)
		data, err := os.ReadFile(path)
		switch {
		case err != nil:
			t.Body = "读取失败：" + err.Error()
		case len(data) > 2<<20:
			t.Body = string(data[:2<<20]) + "\n\n…（文件过大，已截断）"
		default:
			t.Body = string(data)
		}
	}
	t.GotoLine = line
	cw.openCenterTab(cw.center.focus, t)
}

// openGitDiff opens a changed file's working-tree diff in the center area.
func (cw *ChatWindow) openGitDiff(rel string) {
	ws := cw.workspace
	id := "diff:" + rel
	if t := cw.focusCenterTab(id); t != nil {
		return
	}
	tab := cw.openCenterTab(cw.center.focus, &centerTab{
		ID: id, Path: "diff:" + rel, Title: titleOfPath(rel) + " (diff)", Kind: centerDiff,
		Lang: langOfExt(strings.ToLower(filepath.Ext(rel))),
	})
	go func() {
		old := gitOut(ws, "show", "HEAD:"+rel)
		newContent := ""
		if data, err := os.ReadFile(filepath.Join(ws, rel)); err == nil {
			newContent = string(data)
		}
		cw.postMain(func() {
			tab.From, tab.To = old, newContent
			cw.invalidate()
		})
	}()
}

// openBlame opens a file's line-by-line history in the center area.
func (cw *ChatWindow) openBlame(rel string) {
	ws := cw.workspace
	id := "blame:" + rel
	if t := cw.focusCenterTab(id); t != nil {
		return
	}
	tab := cw.openCenterTab(cw.center.focus, &centerTab{
		ID: id, Path: "blame:" + rel, Title: titleOfPath(rel) + " (blame)", Kind: centerBlame,
		Lang: langOfExt(strings.ToLower(filepath.Ext(rel))),
	})
	go func() {
		lines := gitBlameLines(ws, rel)
		cw.postMain(func() {
			tab.blame = lines
			cw.invalidate()
		})
	}()
}

// openCommitDiff opens a commit's message and per-file diffs in the center
// area, titled by the short hash.
func (cw *ChatWindow) openCommitDiff(hash string) {
	ws := cw.workspace
	id := "commit:" + hash
	if t := cw.focusCenterTab(id); t != nil {
		return
	}
	tab := cw.openCenterTab(cw.center.focus, &centerTab{
		ID: id, Path: id, Title: shortHash(hash) + " (diff)", Kind: centerCommit,
	})
	go func() {
		info := loadCommit(ws, hash)
		cw.postMain(func() {
			tab.Commit = info
			cw.invalidate()
		})
	}()
}

// openTerminalTab opens a shell as a center tab in the given pane.
func (cw *ChatWindow) openTerminalTab(pane int) {
	cw.openTerminalTabIn(pane, "")
}

// openTerminalTabIn is openTerminalTab with an explicit starting directory.
func (cw *ChatWindow) openTerminalTabIn(pane int, dir string) {
	tb := cw.newTermTab(dir)
	cw.openCenterTab(pane, &centerTab{
		ID: cw.newTabID("term"), Kind: centerTerm, Title: "Shell", Term: tb,
	})
	go cw.startTerm(tb)
}

// openWebTabIn opens a blank web view as a center tab in the given pane.
func (cw *ChatWindow) openWebTabIn(pane int) {
	if _, err := cw.newWebTabIn(pane, "", ""); err != nil {
		cw.toast(err.Error(), mcore.SeverityError)
	}
}

// openGitGraphTab opens the repository's branches and commit graph as a
// center tab, at most one at a time.
// openGitGraphTab opens the repository's history as a center tab, at most one
// at a time. Whether the workspace is a repository is decided by the tab
// itself once its history loads, so a first open is never refused.
func (cw *ChatWindow) openGitGraphTab(pane int) {
	if t := cw.focusCenterTab("gitgraph"); t != nil {
		return
	}
	cw.openCenterTab(pane, &centerTab{
		ID: "gitgraph", Kind: centerGitGraph, Title: "GitGraph",
	})
	// Only the tab's own state is prepared here: opening the graph must not
	// reload or reset what the Git panel is showing.
	cw.resetGraphState(cw.workspace)
}

// openEditor opens a file for editing in the center area, or focuses the
// editor tab already holding it.
func (cw *ChatWindow) openEditor(path string) {
	id := "editor:" + path
	if t := cw.focusCenterTab(id); t != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		cw.toast("读取失败："+err.Error(), mcore.SeverityError)
		return
	}
	buf := editor.NewBuffer(string(data))
	buf.SetLanguage(editorLanguageOf(path))
	cw.openCenterTab(cw.center.focus, &centerTab{
		ID: id, Kind: centerEditor, Path: path, Title: titleOfPath(path),
		Lang: langOfExt(strings.ToLower(filepath.Ext(path))),
		// A zero Font is MyGo's own monospaced default, which is what a
		// code editor wants.
		Edit: &editorTab{
			st:    editor.State{Buffer: buf},
			saved: buf.Version(),
		},
	})
}

// editorLanguageOf resolves a file's editor language by its name.
func editorLanguageOf(path string) *editor.Language {
	return editor.LanguageFor(path)
}

// centerEditorBody draws an editor tab: its path, a save action and the
// writable editor.
func (cw *ChatWindow) centerEditorBody(c *mgo.Context, tab *centerTab) {
	if tab.Edit == nil {
		cw.centerLoading(c)
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, icons.Must("pencil")).Size(13, 13).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, cw.workspaceRel(tab.Path)).Font(mtheme.MonoFont).FontSize(caption).
				SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if tab.dirty() {
				mgo.Text(c, "未保存").FontSize(caption).TextColor(k.Accent).Shrink(0)
			}
			save := kit.Button(c, "保存", kit.ButtonOptions{
				Variant: kit.Primary, Icon: icons.Must("save"), Disabled: !tab.dirty(),
			})
			if save.Clicked() {
				cw.saveEditorTab(tab)
			}
		})
		ed := editor.View(c, &tab.Edit.st)
		ed.Grow(1).MinHeight(0).Label(tab.Title)
		if ed.Shortcut(mgo.Cmd, mgo.KeyS) {
			cw.saveEditorTab(tab)
		}
	})
}

// saveEditorTab writes an editor tab's buffer to disk.
func (cw *ChatWindow) saveEditorTab(tab *centerTab) {
	if tab.Edit == nil || tab.Edit.st.Buffer == nil {
		return
	}
	body := tab.Edit.st.Buffer.String()
	if err := os.WriteFile(tab.Path, []byte(body), 0o644); err != nil {
		cw.toast("保存失败："+err.Error(), mcore.SeverityError)
		return
	}
	tab.Edit.saved = tab.Edit.st.Buffer.Version()
	cw.toast("已保存 "+titleOfPath(tab.Path), mcore.SeveritySuccess)
	// A source tab of the same file is now stale; refresh what it shows.
	cw.refreshTextTabs(tab.Path, body)
	if cw.gitRepo {
		cw.loadGit(cw.workspace)
	}
}

// refreshTextTabs updates the open source tabs of a file after it is saved.
func (cw *ChatWindow) refreshTextTabs(path, body string) {
	for _, t := range cw.centerTabsOfKind(centerText) {
		if t.Path == path {
			t.Body = body
		}
	}
}
