package ui

import (
	"bytes"
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
			if t := cw.activePreviewTab(); t != nil && t.Kind == "text" {
				if kit.Button(c, "追溯", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("history")}).Clicked() {
					cw.openBlame(t.Path)
				}
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
	b.Children(func() {
		mgo.Icon(c, icons.Must("file-text")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, filepath.Base(path)).SingleLine()
	})
	cw.treeContextMenu(c, b, path, false)
	if b.DoubleClicked() {
		open()
	}
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
	if tab.Kind == "text" && tab.Bmp == nil && tab.Svg == nil && !strings.HasSuffix(strings.ToLower(tab.Path), ".md") {
		kit.CodeViewer(c, tab.Body, &tab.view, kit.CodeViewerOptions{Language: tab.Lang, Path: tab.Path}).
			Grow(1).MinHeight(0)
		return
	}
	if tab.Kind == "fdiff" {
		kit.DiffViewer(c, &tab.dview, tab.From, tab.To, kit.DiffViewerOptions{Language: tab.Lang}).
			Grow(1).MinHeight(0)
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
	closing := false
	b := mgo.ButtonBase(c).Row().Shrink(0).MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).Gap(u).AlignItems(mgo.Center).Radius(mtheme.ControlRadius).Label(tab.Title)
	if i == cw.previewSel {
		b.Background(k.Selection)
	}
	b.Children(func() {
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
		if strings.HasSuffix(strings.ToLower(tab.Path), ".md") {
			kit.MarkdownView(c, tab.Body)
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
				Path: path, Title: rel + " (diff)", Kind: "fdiff",
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

// openCommitDiff loads a commit's diff into the preview area.
func (cw *ChatWindow) openCommitDiff(hash string) {
	ws := cw.workspace
	path := "commit:" + hash
	go func() {
		body := gitOut(ws, "show", "--no-color", hash)
		cw.postMain(func() {
			for i := range cw.previewTabs {
				if cw.previewTabs[i].Path == path {
					cw.previewTabs[i].Body = body
					cw.previewSel = i
					return
				}
			}
			cw.previewTabs = append(cw.previewTabs, previewTab{
				Path: path, Title: hash + " (diff)", Kind: "diff", Body: body,
			})
			cw.previewSel = len(cw.previewTabs) - 1
		})
	}()
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
