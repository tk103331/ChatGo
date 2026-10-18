// Package markdownapp is the Apps menu's Markdown editor: open documents in
// tabs, edit and save them, and let the Agent read and write files.
package markdownapp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/apps/filetools"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 Markdown 文档助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files 读写与查找项目工作目录中的文件，通过 list_tabs 查看打开的文档标签，帮助用户起草和修改文档。"

// doc is one open document.
type doc struct {
	path        string
	content     string
	saved       string
	loaded      bool
	mode        int
	splitW      float32
	jumpStart   int
	jumpEnd     int
	jumpSel     bool
	previewJump int
	err         string
}

// Edit modes of the Markdown editor.
const (
	modeSource = iota
	modePreview
	modeSplit
)

// Client is one Markdown editor bound to a Host.
type Client struct {
	host    Host
	mu      sync.Mutex
	tabs    appkit.Tabs
	docs    map[string]*doc
	ctxAll  bool
	ctxTabs []string
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns an editor driving the given host.
func New(h Host) *Client {
	return &Client{host: h, docs: map[string]*doc{}}
}

// SetContext stores the chat's selected context tabs; all picks every tab.
func (cl *Client) SetContext(all bool, ids []string) {
	cl.ctxAll = all
	cl.ctxTabs = ids
}

// Title labels a tab with its file name.
func (cl *Client) Title(id string) string {
	if d := cl.doc(id); d != nil && d.path != "" {
		return filepath.Base(d.path)
	}
	return "未命名"
}

// NewTab opens an empty document.
func (cl *Client) NewTab() string { return cl.Open("") }

// Open opens a workspace-relative path in a tab, reusing an existing tab.
func (cl *Client) Open(path string) string {
	key := path
	if key == "" {
		key = "untitled-" + cl.host.NewID()
	}
	cl.mu.Lock()
	if _, ok := cl.docs[key]; !ok {
		d := &doc{path: path, splitW: 260, previewJump: -1}
		if path != "" {
			if data, err := os.ReadFile(cl.abs(path)); err == nil {
				d.content, d.saved, d.loaded = string(data), string(data), true
			} else {
				d.err = err.Error()
			}
		}
		cl.docs[key] = d
		cl.tabs.Add(key)
	} else {
		cl.tabs.Active = key
	}
	cl.mu.Unlock()
	return key
}

// Close drops a tab.
func (cl *Client) Close(id string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for i, key := range cl.tabs.Items {
		if key == id {
			cl.tabs.Close(i)
			delete(cl.docs, key)
			return
		}
	}
}

// Tabs exposes the tab list for an embedding tab bar.
func (cl *Client) Tabs() *appkit.Tabs { return &cl.tabs }

// View draws the active document.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	d := cl.doc(cl.tabs.Active)
	if d == nil {
		mgo.Column(c).Fill().Center().Children(func() {
			mgo.Text(c, "打开一个 Markdown 文件开始编辑").FontSize(caption).TextColor(k.TextMuted)
		})
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "打开…", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cl.openViaDialog()
			}
			saveVariant := minput.Secondary
			if d.content != d.saved {
				saveVariant = minput.Primary
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: saveVariant}).Clicked() {
				cl.save(d)
			}
			for _, m := range []struct {
				mode  int
				label string
			}{{modeSource, "源码"}, {modePreview, "预览"}, {modeSplit, "分屏"}} {
				m := m
				variant := minput.Ghost
				if d.mode == m.mode {
					variant = minput.Secondary
				}
				if minput.Button(c, m.label, minput.ButtonOptions{Variant: variant, Label: m.label}).Clicked() {
					d.mode = m.mode
				}
			}
			mgo.Box(c).Grow(1)
		})
		if d.err != "" {
			mgo.Text(c, d.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Row(c).Grow(1).MinWidth(0).MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
			cl.outlinePanel(c, d, k, u, caption)
			mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
				Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
				switch d.mode {
				case modePreview:
					cl.previewArea(c, d)
				case modeSplit:
					mgo.Split(c, &d.splitW, func() { cl.sourceArea(c, d, u) }, func() { cl.previewArea(c, d) }).
						Grow(1).MinWidth(0).MinHeight(0)
				default:
					cl.sourceArea(c, d, u)
				}
			})
		})
		lines := 0
		if d.content != "" {
			lines = strings.Count(d.content, "\n") + 1
		}
		mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Padding(u, 0, 0, 0).Children(func() {
			mgo.Text(c, d.path).FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			mgo.Text(c, fmt.Sprintf("%d 行 · %d 字符", lines, len([]rune(d.content)))).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if d.content != d.saved {
				mgo.Text(c, "未保存").FontSize(caption).TextColor(k.AccentText).Shrink(0)
			}
		})
	})
}

// sourceArea draws the editing text area, applying a pending outline jump.
func (cl *Client) sourceArea(c *mgo.Context, d *doc, u float32) {
	ta := mgo.TextAreaBase(c, &d.content).Fill().Padding(u * 2).
		Placeholder("# 标题\n\n开始编写 Markdown…")
	if d.jumpSel {
		ta = ta.SetTextSelection(d.jumpStart, d.jumpEnd)
		d.jumpSel = false
	}
}

// previewArea draws the rendered Markdown, split into sections so an
// outline jump can scroll the matching section into view.
func (cl *Client) previewArea(c *mgo.Context, d *doc) {
	u := mcore.Density(c).Unit()
	hs := parseHeadings(d.content)
	secs := splitSections(d.content, hs)
	var target mgo.Element
	found := false
	mgo.Scroll(c).Fill().Children(func() {
		mgo.Column(c).FillWidth().Gap(u*3).Padding(u*3, u*5).Children(func() {
			for _, s := range secs {
				el := mchat.MarkdownView(c, s.text).Element.FillWidth()
				if d.previewJump >= 0 && s.heading == d.previewJump {
					target = el
					found = true
				}
			}
		})
	})
	if found && d.previewJump >= 0 {
		target.ScrollIntoView()
		d.previewJump = -1
	}
}

// outlinePanel draws the document's heading list; clicking one selects its
// line in the source (switching to split when only previewing).
func (cl *Client) outlinePanel(c *mgo.Context, d *doc, k mtheme.Tokens, u, caption float32) {
	hs := parseHeadings(d.content)
	mgo.Column(c).Width(150).Shrink(0).FillHeight().
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).Children(func() {
		mgo.Text(c, "大纲").FontSize(caption).TextColor(k.TextMuted).Padding(u*2, u*2, u, u*2)
		if len(hs) == 0 {
			mgo.Text(c, "无标题").FontSize(caption).TextColor(k.TextMuted).Padding(0, u*2)
			return
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().Children(func() {
				for i, h := range hs {
					h := h
					b := mgo.ButtonBase(c).Key(fmt.Sprintf("outline-%s-%d", d.path, i)).FillWidth().Padding(u, u*2).Children(func() {
						mgo.Row(c).FillWidth().Gap(u * 2).Children(func() {
							mgo.Box(c).Width(float32(h.level-1) * u * 2).Shrink(0)
							mgo.Text(c, h.text).FontSize(caption).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						})
					})
					if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					if b.Clicked() {
						cl.jumpToHeading(d, i, h)
					}
				}
			})
		})
	})
}

// jumpToHeading schedules a source selection and/or a preview scroll at the
// heading's line.
func (cl *Client) jumpToHeading(d *doc, idx int, h heading) {
	d.jumpStart, d.jumpEnd = h.start, h.end
	d.jumpSel = d.mode != modePreview
	d.previewJump = -1
	if d.mode != modeSource {
		d.previewJump = idx
	}
	cl.host.Invalidate()
}

// heading is one ATX heading of a document, with its rune and byte offsets.
type heading struct {
	level  int
	text   string
	start  int
	end    int
	byteAt int
}

// parseHeadings lists the ATX headings outside fenced code blocks.
func parseHeadings(content string) []heading {
	var out []heading
	inCode := false
	runeOff, byteOff := 0, 0
	for _, row := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(row), "```") {
			inCode = !inCode
		}
		if !inCode {
			if h, ok := atxHeading(row); ok {
				lineRunes := utf8.RuneCountInString(row)
				out = append(out, heading{level: h.level, text: h.text, start: runeOff, end: runeOff + lineRunes, byteAt: byteOff})
			}
		}
		runeOff += utf8.RuneCountInString(row) + 1
		byteOff += len(row) + 1
	}
	return out
}

// section is a slice of Markdown starting at a heading, or the preamble.
type section struct {
	heading int
	text    string
}

// splitSections cuts content at each heading so preview blocks line up with
// the outline.
func splitSections(content string, hs []heading) []section {
	if len(hs) == 0 {
		return []section{{heading: -1, text: content}}
	}
	var out []section
	if hs[0].byteAt > 0 {
		out = append(out, section{heading: -1, text: content[:hs[0].byteAt]})
	}
	for i, h := range hs {
		end := len(content)
		if i+1 < len(hs) {
			end = hs[i+1].byteAt
		}
		out = append(out, section{heading: i, text: content[h.byteAt:end]})
	}
	return out
}

func atxHeading(row string) (heading, bool) {
	trimmed := strings.TrimLeft(row, " \t")
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(trimmed) || trimmed[n] != ' ' {
		return heading{}, false
	}
	return heading{level: n, text: strings.TrimSpace(trimmed[n:])}, true
}

// save writes the document's content back to its path.
func (cl *Client) save(d *doc) {
	path := d.path
	if strings.TrimSpace(path) == "" {
		if picked := cl.host.SaveFileDialog("untitled.md"); picked != "" {
			path = cl.relOf(picked)
			d.path = path
		}
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	abs := cl.abs(path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		d.err = err.Error()
		return
	}
	if err := os.WriteFile(abs, []byte(d.content), 0o644); err != nil {
		d.err = err.Error()
		return
	}
	d.saved, d.loaded, d.err = d.content, true, ""
	cl.host.Toast("已保存 "+path, mcore.SeveritySuccess)
}

// openViaDialog picks a file with the native dialog and opens it in a tab.
func (cl *Client) openViaDialog() {
	go func() {
		picked := cl.host.OpenFileDialog(cl.host.Root())
		if picked == "" {
			return
		}
		rel := cl.relOf(picked)
		cl.host.Post(func() {
			cl.Open(rel)
			cl.host.Invalidate()
		})
	}()
}

// relOf converts an absolute path to a workspace-relative one when inside
// the root, keeping the absolute path otherwise.
func (cl *Client) relOf(abs string) string {
	if root := cl.host.Root(); root != "" {
		if r, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return abs
}

func (cl *Client) doc(id string) *doc {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.docs[id]
}

// abs joins a workspace-relative path to the host root, or keeps an
// absolute one.
func (cl *Client) abs(rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(cl.host.Root(), rel)
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name:   "read_file",
			Desc:   "读取工作目录中某个文件的文本内容",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.readFile(str(args, "path"))
			},
		},
		{
			Name:   "write_file",
			Desc:   "把文本写入工作目录中的文件（覆盖），并同步到已打开的编辑器标签",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}, "content": {Desc: "要写入的完整文本内容", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.writeFile(str(args, "path"), str(args, "content"))
			},
		},
		{
			Name:   "list_files",
			Desc:   "列出工作目录中的文件与子目录",
			Params: map[string]Param{"dir": {Desc: "相对工作目录的子目录，可留空"}},
			Run: func(args map[string]any) (string, error) {
				return cl.listFiles(str(args, "dir"))
			},
		},
		{
			Name:   "append_file",
			Desc:   "把文本追加到工作目录中的文件末尾，并同步到已打开的编辑器标签",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}, "content": {Desc: "要追加的文本", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.appendFile(str(args, "path"), str(args, "content"))
			},
		},
		{
			Name:   "create_directory",
			Desc:   "创建工作目录下的目录（含父目录）",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.mkdir(str(args, "path"))
			},
		},
		{
			Name:   "rename_file",
			Desc:   "重命名或移动工作目录中的文件（需用户确认）",
			Params: map[string]Param{"path": {Desc: "原路径", Required: true}, "new_path": {Desc: "新路径", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.rename(str(args, "path"), str(args, "new_path"))
			},
		},
		{
			Name:   "delete_file",
			Desc:   "删除工作目录中的文件（需用户确认，目录不可删）",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.remove(str(args, "path"))
			},
		},
		{
			Name: "search_files",
			Desc: "递归搜索工作目录中的文本，输出 路径:行号: 内容",
			Params: map[string]Param{
				"pattern":          {Desc: "搜索内容", Required: true},
				"dir":              {Desc: "起始目录，可留空"},
				"regexp":           {Desc: "是否正则，true/false"},
				"case_insensitive": {Desc: "是否忽略大小写，true/false"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.search(str(args, "pattern"), str(args, "dir"), str(args, "regexp"), str(args, "case_insensitive"))
			},
		},
		{
			Name: "list_tabs",
			Desc: "列出当前打开的文档标签（id、路径；未保存或选中的标注）",
			Run: func(map[string]any) (string, error) {
				if len(cl.tabs.Items) == 0 {
					return "(没有打开的文档)", nil
				}
				var b strings.Builder
				for _, id := range cl.tabs.Items {
					d := cl.doc(id)
					name := "未命名"
					if d != nil && d.path != "" {
						name = d.path
					}
					var mark string
					if d != nil && d.content != d.saved {
						mark += "[未保存]"
					}
					if cl.ctxAll || slices.Contains(cl.ctxTabs, id) {
						mark += "[上下文]"
					}
					fmt.Fprintf(&b, "- %s: %s%s\n", id, name, mark)
				}
				return strings.TrimRight(b.String(), "\n"), nil
			},
		},
	}
}

// readFile reads a file under the root for the Agent.
func (cl *Client) readFile(rel string) (string, error) {
	return filetools.Read(cl.host.Root(), rel)
}

// writeFile writes a file under the root and syncs an open tab.
func (cl *Client) writeFile(rel, content string) (string, error) {
	if err := filetools.Write(cl.host.Root(), rel, content); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		if d, ok := cl.docs[rel]; ok {
			d.content, d.saved, d.loaded, d.err = content, content, true, ""
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	return "已写入 " + rel, nil
}

// appendFile appends text under the root and refreshes an open tab.
func (cl *Client) appendFile(rel, content string) (string, error) {
	if err := filetools.Append(cl.host.Root(), rel, content); err != nil {
		return "", err
	}
	cl.syncDoc(rel)
	return "已追加到 " + rel, nil
}

// mkdir creates a directory under the root.
func (cl *Client) mkdir(rel string) (string, error) {
	if err := filetools.Mkdir(cl.host.Root(), rel); err != nil {
		return "", err
	}
	return "已创建目录 " + rel, nil
}

// remove deletes a file under the root after the user confirms.
func (cl *Client) remove(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	cl.host.Post(func() {
		cl.host.Confirm("删除文件", "确定删除 "+rel+" 吗？", func() {
			if err := filetools.Delete(cl.host.Root(), rel); err != nil {
				cl.host.Toast("删除失败："+err.Error(), mcore.SeverityError)
				return
			}
			cl.host.Toast("已删除 "+rel, mcore.SeveritySuccess)
		})
	})
	return "已请求用户确认删除 " + rel, nil
}

// rename renames a file under the root after the user confirms.
func (cl *Client) rename(old, newPath string) (string, error) {
	if strings.TrimSpace(old) == "" || strings.TrimSpace(newPath) == "" {
		return "", fmt.Errorf("缺少必填参数 path/new_path")
	}
	cl.host.Post(func() {
		cl.host.Confirm("重命名文件", "确定把 "+old+" 重命名为 "+newPath+" 吗？", func() {
			if err := filetools.Rename(cl.host.Root(), old, newPath); err != nil {
				cl.host.Toast("重命名失败："+err.Error(), mcore.SeverityError)
				return
			}
			cl.host.Toast("已重命名 "+old+" → "+newPath, mcore.SeveritySuccess)
		})
	})
	return "已请求用户确认重命名 " + old, nil
}

// search greps text under the root for the Agent.
func (cl *Client) search(pattern, dir, regexp, ci string) (string, error) {
	return filetools.Search(cl.host.Root(), dir, pattern, filetools.SearchOpts{
		Regexp:          strings.EqualFold(regexp, "true"),
		CaseInsensitive: strings.EqualFold(ci, "true"),
	})
}

// syncDoc refreshes an open tab's buffer from disk.
func (cl *Client) syncDoc(rel string) {
	cl.host.Post(func() {
		cl.mu.Lock()
		if d, ok := cl.docs[rel]; ok {
			if data, err := os.ReadFile(filepath.Join(cl.host.Root(), rel)); err == nil {
				d.content, d.saved, d.loaded, d.err = string(data), string(data), true, ""
			}
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
}

// listFiles lists a directory under the root for the Agent.
func (cl *Client) listFiles(rel string) (string, error) {
	return filetools.List(cl.host.Root(), rel)
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
