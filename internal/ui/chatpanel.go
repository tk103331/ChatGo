package ui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// previewTab is one open file or diff in the preview area.
type previewTab struct {
	Path  string
	Title string
	Kind  string // text, image, diff, fdiff, blame
	Body  string
	From  string
	To    string
	Lang  string
	Bmp   *mgo.Bitmap
	Svg   *mgo.SVG
	blame []kit.BlameLine
	view  kit.CodeViewerState
	dview kit.DiffViewerState
	bview kit.BlameViewState
	// Rendered shows a Markdown file as rendered preview rather than source.
	Rendered bool
	// Web is the web view of a Kind "web" tab.
	Web *webTab
	// Commit is a commit diff tab's message, files own their diff views.
	Commit *commitInfo
}

// commitInfo is what a commit diff tab shows above its files.
type commitInfo struct {
	Hash    string
	Author  string
	When    time.Time
	Subject string
	Body    string
	Files   []commitFile
}

// commitFile is one changed file of a commit, with both sides to diff.
type commitFile struct {
	Path     string
	Lang     string
	From, To string
	view     kit.DiffViewerState
}

// sourcePanel shows the workspace file tree or the Git changes, in its own column.
func (cw *ChatWindow) sourcePanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if cw.gitFor != cw.workspace {
		cw.gitFor = cw.workspace
		cw.gitLoaded = false
		cw.loadGit(cw.workspace)
	}
	if cw.treeW == 0 {
		cw.treeW = 300
	}
	segs := []kit.Segment{{Label: "文件"}}
	if cw.gitRepo {
		segs = append(segs, kit.Segment{Label: "Git"})
	}
	if cw.rightTab >= len(segs) {
		cw.rightTab = 0
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Background(k.Surface).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			seg := kit.SegmentedControl(c, &cw.rightTab, segs,
				kit.SegmentedControlOptions{Label: "面板", Equal: true})
			seg.Grow(1)
			if mcore.IconAction(c, icons.Must("x"), "关闭面板").Clicked() {
				cw.rightPanel = false
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() { cw.sourceView(c) })
	})
}

// previewPanel shows the opened files in their own column.
func (cw *ChatWindow) previewPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if cw.previewW == 0 {
		cw.previewW = 320
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Background(k.Surface).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, "预览").FontWeight(600).Grow(1)
			if t := cw.activePreviewTab(); t != nil && t.Kind == "text" && isMarkdownFile(t.Path) {
				label := "预览"
				if t.Rendered {
					label = "源码"
				}
				if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("eye")}).Clicked() {
					t.Rendered = !t.Rendered
				}
			}
			if t := cw.activePreviewTab(); t != nil && t.Kind == "text" {
				if kit.Button(c, "追溯", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("history")}).Clicked() {
					cw.openBlame(t.Path)
				}
			}
			if mcore.IconAction(c, icons.Must("globe"), "新建 WebView 标签").Tooltip("新建 WebView 标签").Clicked() {
				cw.newWebPreview("", "")
			}
			if len(cw.previewTabs) > 1 {
				if kit.Button(c, "全部关闭", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					cw.previewTabs = nil
					cw.previewSel = -1
				}
			}
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() { cw.previewView(c) })
	})
}

// sourceView is the file tree or the Git changes list.
func (cw *ChatWindow) sourceView(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		if cw.rightTab == 1 && cw.gitRepo {
			cw.gitPanel(c)
			return
		}
		mgo.Text(c, cw.workspace).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…").
			Padding(0, u)
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			cw.fsTree(c, cw.workspace)
		})
	})
}

// fsTree shows a directory's entries, folders first and expandable.
func (cw *ChatWindow) fsTree(c *mgo.Context, dir string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	entries, err := os.ReadDir(dir)
	if err != nil {
		mgo.Text(c, "无法读取目录："+err.Error()).TextColor(k.Danger).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})
	for n, e := range entries {
		if n >= 500 {
			break
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		if e.IsDir() {
			switch name {
			case "node_modules", "vendor", "dist", "build", "target":
				continue
			}
			open := mgo.Local(c.Root(), "fs:"+path, func() bool { return false })
			cr := kit.Collapsible(c, open, kit.CollapsibleOptions{Title: name}, func() {
				mgo.Column(c).Margin(0, 0, 0, u*2).Children(func() {
					cw.fsTree(c, path)
				})
			})
			cw.treeContextMenu(c, cr.Element, path, true)
			continue
		}
		cw.fsFileRow(c, path, func() { cw.openPreview(path) })
	}
}

func (cw *ChatWindow) fsFileRow(c *mgo.Context, path string, open func()) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).Row().FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).Radius(mtheme.ControlRadius).Label(path).
		Tooltip("双击预览")
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	isHTML := isHTMLFile(path)
	b.Children(func() {
		icon := "file-text"
		if isHTML {
			icon = "globe"
		}
		mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, filepath.Base(path)).SingleLine().Grow(1).MinWidth(0)
		if isHTML && mcore.IconAction(c, icons.Must("eye"), "在 WebView 中预览").Tooltip("在 WebView 中预览").Clicked() {
			cw.openWebPreview(path)
		}
	})
	cw.treeContextMenu(c, b, path, false)
	if b.DoubleClicked() {
		open()
	}
}

// isMarkdownFile reports whether path is a Markdown document.
func isMarkdownFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// isHTMLFile reports whether path is a web page the preview can render.
func isHTMLFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// addWorkspaceContext turns a workspace path into a composer mention.
func (cw *ChatWindow) addWorkspaceContext(path string) {
	rel := cw.workspaceRel(path)
	cw.insertContextMention(kit.ContextItem{ID: rel, Label: rel, Kind: kit.ContextFile})
}

func (cw *ChatWindow) gitPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if cw.gitSplitH == 0 {
		cw.gitSplitH = 240
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(u).Children(func() {
		if !cw.gitLoaded {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "加载中"})
				mgo.Text(c, "正在读取 Git 信息…").TextColor(k.TextMuted).FontSize(caption)
			})
			return
		}
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			sel := kit.BranchSelector(c, &cw.gitBranch, cw.gitBranches, kit.BranchSelectorOptions{AllowCreate: true})
			if sel.Changed() {
				cw.checkoutBranch(cw.gitBranch)
			}
			if name, ok := sel.Created(); ok {
				cw.createBranch(name)
			}
			mgo.Spacer(c)
			if kit.Button(c, "刷新", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.loadGit(cw.workspace)
			}
		})
		mgo.SplitVertical(c, &cw.gitSplitH, func() {
			mgo.Column(c).Fill().MinHeight(0).Gap(u).Children(func() {
				mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
					mgo.Text(c, "变更").FontWeight(600).Grow(1)
					kit.DiffStat(c, cw.gitStat[0], cw.gitStat[1], kit.DiffStatOptions{})
				})
				changes := kit.ChangesList(c, &cw.gitChangesList, cw.gitChanges, kit.ChangesListOptions{})
				changes.Element.Grow(1).MinHeight(0)
				if action, item, ok := changes.Action(); ok {
					cw.gitChangesAction(action, item)
				}
				if changes.Submitted() {
					if i := cw.gitChangesList.Selected(); i >= 0 && i < len(cw.gitChanges) {
						cw.openGitDiff(cw.gitChanges[i].Path)
					}
				}
			})
		}, func() {
			mgo.Column(c).Fill().MinHeight(0).Gap(u).Children(func() {
				input := kit.CommitInput(c, &cw.gitCommit, kit.CommitInputOptions{Busy: cw.gitCommitBusy})
				committed, amended := input.Committed(), input.Amended()
				if committed || amended {
					cw.commitPanel(kit.CommitMessage{Title: cw.gitCommit.Title, Body: cw.gitCommit.Body}, amended)
					cw.gitCommitBusy = true
				} else if cw.gitCommitBusy {
					cw.gitCommitBusy = false
				}
				mgo.Text(c, "最近提交").FontWeight(600).Shrink(0)
				graph := kit.CommitGraph(c, &cw.gitGraph, cw.gitCommits, kit.CommitGraphOptions{})
				graph.Element.Grow(1).MinHeight(0)
				if graph.Submitted() {
					if hash, ok := cw.gitGraph.List.Selected(); ok && hash != "" {
						cw.openCommitDiff(hash)
					}
				}
			})
		}).Grow(1).MinHeight(0)
	})
}

// gitChangesAction stages or unstages a file, or all files.
func (cw *ChatWindow) gitChangesAction(action, item string) {
	var args []string
	switch action {
	case "stage":
		args = []string{"add", "--", item}
	case "unstage":
		args = []string{"restore", "--staged", "--", item}
	case "stage_all":
		args = []string{"add", "-A"}
	case "unstage_all":
		args = []string{"reset"}
	default:
		return
	}
	if out, err := gitRun(cw.workspace, args...); err != nil {
		cw.toast(out, mcore.SeverityError)
	}
	cw.loadGit(cw.workspace)
}

// commitPanel commits the staged changes, optionally amending the last commit.
func (cw *ChatWindow) commitPanel(message kit.CommitMessage, amend bool) {
	title := strings.TrimSpace(message.Title)
	if title == "" {
		cw.toast("提交信息不能为空", mcore.SeverityError)
		return
	}
	args := []string{"-m", title}
	if body := strings.TrimSpace(message.Body); body != "" {
		args = append(args, "-m", body)
	}
	if amend {
		args = append([]string{"--amend"}, args...)
	}
	if out, err := gitRun(cw.workspace, append([]string{"commit"}, args...)...); err != nil {
		cw.toast(out, mcore.SeverityError)
		return
	}
	cw.gitCommit = kit.CommitMessage{}
	cw.loadGit(cw.workspace)
}

func gitChangePath(line string) string {
	if len(line) < 4 {
		return ""
	}
	p := strings.TrimSpace(line[2:])
	if i := strings.LastIndex(p, " -> "); i >= 0 {
		p = p[i+4:]
	}
	return strings.Trim(p, "\"")
}

// previewView is the multi-tab file preview.
func (cw *ChatWindow) previewView(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if len(cw.previewTabs) == 0 {
		mgo.Column(c).Fill().Center().Children(func() {
			mgo.Text(c, "双击左侧文件预览").TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		})
		return
	}
	if cw.previewSel < 0 || cw.previewSel >= len(cw.previewTabs) {
		cw.previewSel = 0
	}
	closeIdx := -1
	mgo.Column(c).FillWidth().Shrink(0).MinWidth(0).Children(func() {
		mgo.ScrollHorizontal(c).Shrink(0).MinWidth(0).Children(func() {
			mgo.Row(c).Gap(u).Children(func() {
				for i := range cw.previewTabs {
					if cw.previewTab(c, i) {
						closeIdx = i
					}
				}
			})
		})
	})
	if closeIdx >= 0 {
		cw.closePreview(closeIdx)
	}
	if len(cw.previewTabs) == 0 {
		mgo.Text(c, "双击左侧文件预览").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		return
	}
	if cw.previewSel < 0 || cw.previewSel >= len(cw.previewTabs) {
		cw.previewSel = 0
	}
	tab := &cw.previewTabs[cw.previewSel]
	if tab.Web != nil {
		cw.webToolbar(c, tab.Web)
		mgo.Box(c).Grow(1).MinHeight(0).Children(func() {
			if tab.Web.URL == webLandingURL {
				cw.webLanding(c, tab.Web)
				return
			}
			mgo.WebView(c, tab.Web.view).Grow(1).MinHeight(0)
		})
		return
	}
	isMD := strings.HasSuffix(strings.ToLower(tab.Path), ".md")
	if tab.Kind == "text" && tab.Bmp == nil && tab.Svg == nil && (!isMD || !tab.Rendered) {
		kit.CodeViewer(c, tab.Body, &tab.view, kit.CodeViewerOptions{Language: tab.Lang, Path: tab.Path}).
			Grow(1).MinHeight(0)
		return
	}
	if tab.Kind == "commit" && tab.Commit != nil {
		cw.commitDiffView(c, tab.Commit)
		return
	}
	if tab.Kind == "fdiff" {
		k := mcore.Tokens(c)
		u := mcore.Density(c).Unit()
		caption := mcore.FontSize(c, mtheme.CaptionSize)
		unified := mgo.Local(c.Root(), "fdiff-unified:"+tab.Path, func() bool { return false })
		mgo.Column(c).FillWidth().Grow(1).MinHeight(0).Gap(u).Children(func() {
			mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("file-text")).Size(13, 13).
					TextColor(k.TextMuted).Shrink(0)
				rel := strings.TrimPrefix(tab.Path, "diff:")
				mgo.Text(c, rel).Font(mtheme.MonoFont).FontSize(caption).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				label := "Inline"
				if *unified {
					label = "Split"
				}
				if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("columns-2")}).Clicked() {
					*unified = !*unified
				}
			})
			mgo.Box(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
				kit.DiffViewer(c, &tab.dview, tab.From, tab.To, kit.DiffViewerOptions{
					Language: tab.Lang, Unified: *unified,
				}).Fill()
			})
		})
		return
	}
	if tab.Kind == "blame" {
		kit.BlameView(c, &tab.bview, tab.blame, kit.BlameViewOptions{Language: tab.Lang}).
			Grow(1).MinHeight(0)
		return
	}
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		cw.previewContent(c, *tab)
	})
}

// previewTab draws one preview tab and reports whether its close was clicked.
func (cw *ChatWindow) previewTab(c *mgo.Context, i int) bool {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	tab := cw.previewTabs[i]
	if tab.Web != nil {
		cw.previewTabs[i].Title = tab.Web.Title
		tab = cw.previewTabs[i]
	}
	closing := false
	b := mgo.ButtonBase(c).Row().Shrink(0).MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Radius(mtheme.ControlRadius).Label(tab.Title)
	if i == cw.previewSel {
		b.Background(k.Selection)
	}
	b.Children(func() {
		if icon := previewTabIcon(tab); icon != "" {
			mgo.Icon(c, icons.Must(icon)).Size(13, 13).TextColor(k.TextMuted).Shrink(0)
		}
		mgo.Text(c, tab.Title).SingleLine()
		if mcore.IconAction(c, icons.Must("x"), "关闭预览").Clicked() {
			closing = true
		}
	})
	if b.Clicked() && i != cw.previewSel {
		cw.previewSel = i
	}
	return closing
}

func previewTabIcon(tab previewTab) string {
	switch tab.Kind {
	case "image":
		return "image"
	case "fdiff", "commit":
		return "git-branch"
	case "blame":
		return "history"
	case "web":
		return "globe"
	case "text":
		if isMarkdownFile(tab.Path) {
			return "eye"
		}
		return "file-text"
	}
	return ""
}

func (cw *ChatWindow) previewContent(c *mgo.Context, tab previewTab) {
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	switch tab.Kind {
	case "image":
		switch {
		case tab.Bmp != nil:
			mgo.Image(c, tab.Bmp).Fit(mgo.Contain)
		case tab.Svg != nil:
			mgo.Image(c, tab.Svg).Fit(mgo.Contain)
		}
	case "diff":
		renderDiff(c, tab.Body)
	default:
		if isMarkdownFile(tab.Path) {
			kit.MarkdownPreview(c, tab.Body)
		} else {
			kit.CodeBlock(c, tab.Body, kit.CodeBlockOptions{Language: tab.Lang})
		}
		_ = caption
	}
}

// renderDiff shows a unified diff with colored line bands.
func renderDiff(c *mgo.Context, body string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if strings.TrimSpace(body) == "" {
		mgo.Text(c, "无变更内容").TextColor(k.TextMuted).FontSize(caption)
		return
	}
	for _, line := range strings.Split(body, "\n") {
		fg, bg := k.Text, mgo.Color{}
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			fg = k.TextMuted
		case strings.HasPrefix(line, "@@"):
			fg = k.Accent
		case strings.HasPrefix(line, "+"):
			fg, bg = k.Success, k.Success.Alpha(0.12)
		case strings.HasPrefix(line, "-"):
			fg, bg = k.Danger, k.Danger.Alpha(0.12)
		}
		row := mgo.Row(c).FillWidth().Padding(0, u).MinHeight(caption * 1.7)
		if bg.A > 0 {
			row.Background(bg).Radius(2)
		}
		row.Children(func() {
			mgo.Text(c, line).Font(mtheme.MonoFont).FontSize(caption).TextColor(fg)
		})
	}
}

func (cw *ChatWindow) closePreview(i int) {
	if i < 0 || i >= len(cw.previewTabs) {
		return
	}
	if tab := cw.previewTabs[i].Web; tab != nil && tab.view != nil {
		tab.view.Destroy()
	}
	cw.previewTabs = append(cw.previewTabs[:i], cw.previewTabs[i+1:]...)
	if cw.previewSel >= len(cw.previewTabs) {
		cw.previewSel = len(cw.previewTabs) - 1
	}
}

// openPreview opens or focuses a file in the preview area.
func (cw *ChatWindow) openPreview(path string) {
	for i := range cw.previewTabs {
		if cw.previewTabs[i].Path == path && cw.previewTabs[i].Kind != "diff" {
			cw.previewSel = i
			return
		}
	}
	t := previewTab{Path: path, Title: filepath.Base(path)}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
		data, err := os.ReadFile(path)
		if err != nil {
			t.Kind, t.Body = "text", "读取失败："+err.Error()
		} else if bmp, err := mgo.DecodeBitmap(data); err == nil {
			t.Kind, t.Bmp = "image", bmp
		} else {
			t.Kind, t.Body = "text", "无法解码图片："+err.Error()
		}
	case ".svg":
		data, err := os.ReadFile(path)
		if err == nil {
			if svg, err := mgo.ParseSVG(data); err == nil {
				t.Kind, t.Svg = "image", svg
			}
		}
		if t.Kind == "" {
			t.Kind, t.Body = "text", "无法解码 SVG"
		}
	default:
		t.Kind, t.Lang = "text", langOfExt(ext)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Body = "读取失败：" + err.Error()
		} else if len(data) > 2<<20 {
			t.Body = string(data[:2<<20]) + "\n\n…（文件过大，已截断）"
		} else {
			t.Body = string(data)
		}
	}
	cw.previewTabs = append(cw.previewTabs, t)
	cw.previewSel = len(cw.previewTabs) - 1
}

// openGitDiff loads a changed file's working-tree diff into the preview area.
func (cw *ChatWindow) openGitDiff(rel string) {
	ws := cw.workspace
	path := "diff:" + rel
	go func() {
		old := gitOut(ws, "show", "HEAD:"+rel)
		newContent := ""
		if data, err := os.ReadFile(filepath.Join(ws, rel)); err == nil {
			newContent = string(data)
		}
		cw.postMain(func() {
			for i := range cw.previewTabs {
				if cw.previewTabs[i].Path == path {
					cw.previewTabs[i].From = old
					cw.previewTabs[i].To = newContent
					cw.previewSel = i
					return
				}
			}
			cw.previewTabs = append(cw.previewTabs, previewTab{
				Path: path, Title: filepath.Base(rel) + " (diff)", Kind: "fdiff",
				From: old, To: newContent, Lang: langOfExt(strings.ToLower(filepath.Ext(rel))),
			})
			cw.previewSel = len(cw.previewTabs) - 1
		})
	}()
}

// activePreviewTab returns the selected preview tab, if any.
func (cw *ChatWindow) activePreviewTab() *previewTab {
	if cw.previewSel < 0 || cw.previewSel >= len(cw.previewTabs) {
		return nil
	}
	return &cw.previewTabs[cw.previewSel]
}

// openBlame loads a file's line-by-line history into the preview area.
func (cw *ChatWindow) openBlame(rel string) {
	ws := cw.workspace
	path := "blame:" + rel
	go func() {
		lines := gitBlameLines(ws, rel)
		cw.postMain(func() {
			for i := range cw.previewTabs {
				if cw.previewTabs[i].Path == path {
					cw.previewTabs[i].blame = lines
					cw.previewSel = i
					return
				}
			}
			cw.previewTabs = append(cw.previewTabs, previewTab{
				Path: path, Title: filepath.Base(rel) + " (blame)", Kind: "blame",
				blame: lines, Lang: langOfExt(strings.ToLower(filepath.Ext(rel))),
			})
			cw.previewSel = len(cw.previewTabs) - 1
		})
	}()
}

// gitBlameLines parses git blame --line-porcelain into per-line commits.
func gitBlameLines(dir, rel string) []kit.BlameLine {
	out := gitOut(dir, "blame", "--line-porcelain", "--", rel)
	var lines []kit.BlameLine
	var cur kit.BlameLine
	flush := func() {
		if cur.Hash != "" {
			lines = append(lines, cur)
		}
		cur = kit.BlameLine{}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case isHexHash(line):
			cur.Hash = line[:40]
		case strings.HasPrefix(line, "author "):
			cur.Author = line[len("author "):]
		case strings.HasPrefix(line, "author-time "):
			sec, _ := strconv.ParseInt(line[len("author-time "):], 10, 64)
			cur.When = time.Unix(sec, 0)
		case strings.HasPrefix(line, "summary "):
			cur.Summary = line[len("summary "):]
		case strings.HasPrefix(line, "\t"):
			cur.Text = line[1:]
			flush()
		}
	}
	return lines
}

func isHexHash(line string) bool {
	if len(line) < 40 || (len(line) > 40 && line[40] != ' ') {
		return false
	}
	for _, ch := range line[:40] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// checkoutBranch switches to a branch and reloads the Git state.
func (cw *ChatWindow) checkoutBranch(name string) {
	if out, err := gitRun(cw.workspace, "checkout", name); err != nil {
		cw.toast(out, mcore.SeverityError)
	}
	cw.loadGit(cw.workspace)
}

// createBranch creates and checks out a new branch.
func (cw *ChatWindow) createBranch(name string) {
	if out, err := gitRun(cw.workspace, "checkout", "-b", name); err != nil {
		cw.toast(out, mcore.SeverityError)
	}
	cw.loadGit(cw.workspace)
}

// openCommitDiff loads a commit's message and per-file diffs into the preview
// area, titled by the short hash.
func (cw *ChatWindow) openCommitDiff(hash string) {
	ws := cw.workspace
	path := "commit:" + hash
	short := shortHash(hash)
	go func() {
		info := loadCommit(ws, hash)
		cw.postMain(func() {
			for i := range cw.previewTabs {
				if cw.previewTabs[i].Path == path {
					cw.previewTabs[i].Commit = info
					cw.previewTabs[i].Title = short + " (diff)"
					cw.previewSel = i
					return
				}
			}
			cw.previewTabs = append(cw.previewTabs, previewTab{
				Path: path, Title: short + " (diff)", Kind: "commit", Commit: info,
			})
			cw.previewSel = len(cw.previewTabs) - 1
		})
	}()
}

// shortHash is the seven-character abbreviation a commit diff is titled by.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// loadCommit reads a commit's message and the both sides of every file it
// changed: each file's parent version and its version at the commit.
func loadCommit(dir, hash string) *commitInfo {
	meta := gitOut(dir, "show", "-s", "--pretty=format:%H%x09%an%x09%at%x09%s%x09%b", hash)
	info := &commitInfo{Hash: hash}
	parts := strings.SplitN(meta, "\t", 5)
	if len(parts) >= 4 {
		info.Hash, info.Author, info.Subject = parts[0], parts[1], parts[3]
		if sec, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
			info.When = time.Unix(sec, 0)
		}
		if len(parts) == 5 {
			info.Body = strings.TrimSpace(parts[4])
		}
	}
	parents := strings.Fields(gitOut(dir, "rev-list", "--parents", "-n", "1", hash))
	parent := ""
	if len(parents) > 1 {
		parent = parents[1]
	}
	for _, line := range splitRaw(gitOut(dir, "show", "--name-status", "--pretty=format:", hash)) {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status, rel := fields[0], fields[len(fields)-1]
		file := commitFile{Path: rel, Lang: langOfExt(strings.ToLower(filepath.Ext(rel)))}
		if !strings.HasPrefix(status, "A") && parent != "" {
			file.From = gitOut(dir, "show", parent+":"+rel)
		}
		if !strings.HasPrefix(status, "D") {
			file.To = gitOut(dir, "show", hash+":"+rel)
		}
		info.Files = append(info.Files, file)
	}
	return info
}

func langOfExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hpp":
		return "cpp"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh", ".bash", ".zsh":
		return "bash"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sql":
		return "sql"
	case ".toml":
		return "toml"
	case ".md", ".markdown":
		return "markdown"
	case ".xml":
		return "xml"
	}
	return ""
}

// loadGit reads the workspace repository state in the background.
func (cw *ChatWindow) loadGit(dir string) {
	cw.gitLoaded = false
	go func() {
		repo := gitOut(dir, "rev-parse", "--is-inside-work-tree") == "true"
		var (
			branch   string
			branches []kit.Branch
			changes  []kit.ChangedFile
			commits  []kit.Commit
			stat     [2]int
		)
		if repo {
			branch = gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
			branches = gitLocalBranches(dir, branch)
			changes = gitChangedFiles(dir)
			commits = gitCommitHistory(dir, 30)
			stat = gitDiffStat(dir)
		}
		cw.postMain(func() {
			cw.gitRepo, cw.gitBranch, cw.gitBranches = repo, branch, branches
			cw.gitChanges, cw.gitCommits, cw.gitStat, cw.gitLoaded = changes, commits, stat, true
			cw.invalidate()
		})
	}()
}

func gitLocalBranches(dir, current string) []kit.Branch {
	var branches []kit.Branch
	for _, line := range splitRaw(gitOut(dir, "for-each-ref", "--format=%(refname:short)%09%(upstream:short)", "refs/heads")) {
		parts := strings.SplitN(line, "\t", 2)
		b := kit.Branch{Name: parts[0], Current: parts[0] == current}
		if len(parts) > 1 {
			b.Upstream = parts[1]
		}
		branches = append(branches, b)
	}
	return branches
}

// gitChangedFiles parses git status --porcelain into changed files.
func gitChangedFiles(dir string) []kit.ChangedFile {
	var files []kit.ChangedFile
	for _, line := range splitRaw(gitOut(dir, "status", "--porcelain")) {
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		p := strings.TrimSpace(line[2:])
		if i := strings.LastIndex(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		p = strings.Trim(p, "\"")
		if p == "" {
			continue
		}
		status, staged := gitPorcelainStatus(x, y)
		files = append(files, kit.ChangedFile{Path: p, Status: status, Staged: staged})
	}
	return files
}

func gitPorcelainStatus(x, y byte) (kit.GitStatus, bool) {
	if x == '?' {
		return kit.GitUntracked, false
	}
	staged := x != ' '
	code := y
	if staged {
		code = x
	}
	return gitStatusCode(code), staged
}

func gitStatusCode(code byte) kit.GitStatus {
	switch code {
	case 'A':
		return kit.GitAdded
	case 'D':
		return kit.GitDeleted
	case 'R':
		return kit.GitRenamed
	case 'C':
		return kit.GitCopied
	case '?':
		return kit.GitUntracked
	default:
		return kit.GitModified
	}
}

// gitCommitHistory reads the last n commits, newest first.
func gitCommitHistory(dir string, n int) []kit.Commit {
	out := gitOut(dir, "log", "--pretty=format:%H%x09%P%x09%an%x09%at%x09%s", "-n", strconv.Itoa(n))
	var commits []kit.Commit
	for _, line := range splitRaw(out) {
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			continue
		}
		sec, _ := strconv.ParseInt(parts[3], 10, 64)
		var parents []string
		if parts[1] != "" {
			parents = strings.Fields(parts[1])
		}
		commits = append(commits, kit.Commit{
			Hash: parts[0], Parents: parents, Author: parts[2], When: time.Unix(sec, 0), Subject: parts[4],
		})
	}
	return commits
}

// gitDiffStat sums added and deleted lines against HEAD.
func gitDiffStat(dir string) [2]int {
	var stat [2]int
	for _, line := range splitRaw(gitOut(dir, "diff", "HEAD", "--numstat")) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		add, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		stat[0] += add
		stat[1] += del
	}
	return stat
}

func gitOut(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitRun(dir string, args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(stderr.String()), err
	}
	return strings.TrimSpace(string(out)), nil
}

func splitRaw(s string) []string {
	s = strings.TrimRight(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// commitDiffView shows a commit's message, then every file it changed.
func (cw *ChatWindow) commitDiffView(c *mgo.Context, info *commitInfo) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	sel := mgo.Local(c.Root(), "commit-sel:"+shortHash(info.Hash), func() int { return 0 })
	if *sel < 0 || *sel >= len(info.Files) {
		*sel = 0
	}
	unified := mgo.Local(c.Root(), "commit-unified:"+shortHash(info.Hash), func() bool { return false })
	mgo.Column(c).FillWidth().Grow(1).MinHeight(0).Gap(u*2).Padding(0, u).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Gap(u).Padding(u*2).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).Children(func() {
			mgo.Text(c, info.Subject).FontWeight(600).Selectable()
			if info.Body != "" {
				mgo.Text(c, info.Body).TextColor(k.TextMuted).FontSize(caption).Selectable()
			}
			meta := shortHash(info.Hash) + " · " + info.Author
			if !info.When.IsZero() {
				meta += " · " + info.When.Format("2006-01-02 15:04")
			}
			mgo.Text(c, meta).TextColor(k.TextMuted).FontSize(caption).Font(mtheme.MonoFont)
		})
		if len(info.Files) == 0 {
			mgo.Text(c, "该提交没有文件变更").TextColor(k.TextMuted).FontSize(caption)
			return
		}
		mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, fmt.Sprintf("%d 个文件变更", len(info.Files))).
				TextColor(k.TextMuted).FontSize(caption).Grow(1)
			label := "Inline"
			if *unified {
				label = "Split"
			}
			if kit.Button(c, label, kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("columns-2")}).Clicked() {
				*unified = !*unified
			}
		})
		mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(220).Children(func() {
			mgo.Column(c).FillWidth().Gap(2).Children(func() {
				for i := range info.Files {
					file := &info.Files[i]
					b := mgo.ButtonBase(c).Row().FillWidth().Shrink(0).
						Padding(u/2, u).Gap(u).AlignItems(mgo.Center).
						Radius(mtheme.ControlRadius)
					if i == *sel {
						b.Background(k.Selection)
					} else if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					b.Children(func() {
						mgo.Icon(c, icons.Must("file-text")).Size(13, 13).
							TextColor(k.TextMuted).Shrink(0)
						mgo.Text(c, file.Path).Font(mtheme.MonoFont).FontSize(caption).
							SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
						mgo.Text(c, fileChangeLabel(file)).FontSize(caption).
							TextColor(k.TextMuted).Shrink(0)
					})
					if b.Clicked() && i != *sel {
						*sel = i
					}
				}
			})
		})
		file := &info.Files[*sel]
		mgo.Box(c).FillWidth().Grow(1).MinHeight(0).Clip().Children(func() {
			kit.DiffViewer(c, &file.view, file.From, file.To, kit.DiffViewerOptions{
				Language: file.Lang, Path: file.Path, Unified: *unified,
			}).Fill()
		})
	})
}

func fileChangeLabel(f *commitFile) string {
	fromLines := strings.Count(f.From, "\n")
	toLines := strings.Count(f.To, "\n")
	switch {
	case f.From == "" && f.To == "":
		return "空变更"
	case f.From == "":
		return fmt.Sprintf("新增 %d 行", toLines)
	case f.To == "":
		return fmt.Sprintf("删除 %d 行", fromLines)
	case fromLines == toLines:
		return fmt.Sprintf("%d 行", fromLines)
	default:
		return fmt.Sprintf("%d → %d 行", fromLines, toLines)
	}
}
