package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// previewTab is one open file or diff in the preview area.
type previewTab struct {
	Path  string
	Title string
	Kind  string // text, image, diff
	Body  string
	Lang  string
	Bmp   *mgo.Bitmap
	Svg   *mgo.SVG
	view  code.CodeViewerState
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
	segs := []mnavigation.Segment{{Label: "文件"}}
	if cw.gitRepo {
		segs = append(segs, mnavigation.Segment{Label: "Git"})
	}
	if cw.rightTab >= len(segs) {
		cw.rightTab = 0
	}
	mgo.Column(c).Width(cw.treeW).Shrink(0).MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Background(k.Surface).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			seg := mnavigation.SegmentedControl(c, &cw.rightTab, segs,
				mnavigation.SegmentedControlOptions{Label: "面板", Equal: true})
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
			if len(cw.previewTabs) > 1 {
				if minput.Button(c, "全部关闭", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
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
			data.Collapsible(c, open, data.CollapsibleOptions{Title: name}, func() {
				mgo.Column(c).Margin(0, 0, 0, u*2).Children(func() {
					cw.fsTree(c, path)
				})
			})
			continue
		}
		fsFileRow(c, path, func() { cw.openPreview(path) })
	}
}

func fsFileRow(c *mgo.Context, path string, open func()) {
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
	if b.DoubleClicked() {
		open()
	}
}

func (cw *ChatWindow) gitPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(u).Children(func() {
		if !cw.gitLoaded {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "加载中"})
				mgo.Text(c, "正在读取 Git 信息…").TextColor(k.TextMuted).FontSize(caption)
			})
			return
		}
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "分支："+cw.gitBranch).FontWeight(600).Grow(1).SingleLine()
			if minput.Button(c, "刷新", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cw.loadGit(cw.workspace)
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Gap(u).Children(func() {
			mgo.Text(c, "变更 ("+strconv.Itoa(len(cw.gitChanges))+")").FontWeight(600).Shrink(0)
			if len(cw.gitChanges) == 0 {
				mgo.Text(c, "工作区干净").TextColor(k.TextMuted).FontSize(caption)
			}
			for _, ch := range cw.gitChanges {
				rel := gitChangePath(ch)
				if rel == "" {
					continue
				}
				gitChangeRow(c, ch, func() { cw.openGitDiff(rel) })
			}
			mgo.Text(c, "最近提交").FontWeight(600).Shrink(0)
			for _, l := range cw.gitLog {
				mgo.Text(c, l).Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted).
					SingleLine().Ellipsis("…")
			}
		})
	})
}

func gitChangeRow(c *mgo.Context, line string, open func()) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	b := mgo.ButtonBase(c).Row().FillWidth().MinHeight(mcore.Density(c).ControlHeight()).
		Padding(u, u*2).AlignItems(mgo.Center).Justify(mgo.Start).Radius(mtheme.ControlRadius).
		Label(line).Tooltip("双击查看 Diff")
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Text(c, line).Font(mtheme.MonoFont).FontSize(caption).SingleLine().Ellipsis("…")
	})
	if b.DoubleClicked() {
		open()
	}
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
		code.CodeViewer(c, splitRaw(tab.Body), &tab.view, code.CodeViewerOptions{Language: tab.Lang}).
			Element.Grow(1).MinHeight(0)
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
			mchat.MarkdownView(c, tab.Body)
		} else {
			mchat.CodeBlock(c, tab.Body, mchat.CodeBlockOptions{Language: tab.Lang, LineNumbers: true})
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
			fg = k.AccentText
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

// openGitDiff loads a changed file's diff into the preview area.
func (cw *ChatWindow) openGitDiff(rel string) {
	ws := cw.workspace
	path := "diff:" + rel
	go func() {
		out := gitOut(ws, "diff", "--no-color", "--", rel)
		if strings.TrimSpace(out) == "" {
			out = gitOut(ws, "diff", "--no-color", "--cached", "--", rel)
		}
		if strings.TrimSpace(out) == "" {
			if data, err := os.ReadFile(filepath.Join(ws, rel)); err == nil {
				var b strings.Builder
				for _, l := range splitRaw(string(data)) {
					b.WriteString("+")
					b.WriteString(l)
					b.WriteString("\n")
				}
				out = b.String()
			}
		}
		body := out
		cw.postMain(func() {
			for i := range cw.previewTabs {
				if cw.previewTabs[i].Path == path {
					cw.previewTabs[i].Body = body
					cw.previewSel = i
					return
				}
			}
			cw.previewTabs = append(cw.previewTabs, previewTab{
				Path: path, Title: rel + " (diff)", Kind: "diff", Body: body,
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
			branch  string
			changes []string
			log     []string
		)
		if repo {
			branch = gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
			changes = splitRaw(gitOut(dir, "status", "--porcelain"))
			log = splitRaw(gitOut(dir, "log", "--oneline", "-n", "20"))
		}
		cw.postMain(func() {
			cw.gitRepo, cw.gitBranch, cw.gitChanges, cw.gitLog, cw.gitLoaded = repo, branch, changes, log, true
			cw.invalidate()
		})
	}()
}

func gitOut(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func splitRaw(s string) []string {
	s = strings.TrimRight(s, "\n")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
