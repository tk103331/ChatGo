// Package codeeditorapp is the Apps menu's code editor: open source files in
// tabs with MyGo's editor, save, and let the Agent read and write.
package codeeditorapp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/apps/filetools"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是代码编辑器助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files/open_file 读写与查找项目中的源文件，通过 list_tabs 查看打开的代码标签，帮助用户查看、修改与生成代码。"

const maxFile = 2 << 20

// file is one open source file and its MyGo editor buffer.
type file struct {
	path   string
	err    string
	buf    *editor.Buffer
	base   *editor.Buffer
	st     *editor.State
	wrap   bool
	lineNo bool
}

// Client is one code editor bound to a Host.
type Client struct {
	host        Host
	mu          sync.Mutex
	tabs        appkit.Tabs
	docs        map[string]*file
	ctxAll      bool
	ctxTabs     []string
	openHandler func(path string)
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns an editor driving the given host.
func New(h Host) *Client {
	return &Client{host: h, docs: map[string]*file{}}
}

// SetOpenHandler registers a callback run when a file is opened into a new
// tab, so the shell can mirror it into its own tab bar.
func (cl *Client) SetOpenHandler(fn func(path string)) { cl.openHandler = fn }

// SetContext stores the chat's selected context tabs; all picks every tab.
func (cl *Client) SetContext(all bool, ids []string) {
	cl.ctxAll = all
	cl.ctxTabs = ids
}

// Title labels a tab with its file name.
func (cl *Client) Title(id string) string {
	if f := cl.file(id); f != nil && f.path != "" {
		return filepath.Base(f.path)
	}
	return "未命名"
}

// NewTab opens an empty buffer.
func (cl *Client) NewTab() string { return cl.Open("") }

// Open opens a workspace-relative path in a tab, reusing an existing tab.
func (cl *Client) Open(path string) string {
	key := path
	if key == "" {
		key = "buffer-" + cl.host.NewID()
	}
	cl.mu.Lock()
	if _, ok := cl.docs[key]; !ok {
		f := &file{path: path, lineNo: true}
		text := ""
		if path != "" {
			if data, err := os.ReadFile(cl.abs(path)); err == nil {
				text = truncate(string(data))
			} else {
				f.err = err.Error()
			}
		}
		lang := editor.LanguageFor(path)
		f.buf = editor.NewBuffer(text)
		f.buf.SetLanguage(lang)
		f.base = editor.NewBuffer(text)
		f.base.SetLanguage(lang)
		f.st = &editor.State{Buffer: f.buf, DiffBase: f.base}
		cl.docs[key] = f
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

// View draws the active file with MyGo's editor.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	f := cl.file(cl.tabs.Active)
	if f == nil {
		mgo.Column(c).Fill().Center().Children(func() {
			mgo.Text(c, "打开一个源文件开始编辑").FontSize(caption).TextColor(k.TextMuted)
		})
		return
	}
	dirty := f.buf.Modified()
	f.st.SoftWrap = f.wrap
	if f.lineNo {
		f.st.LineNumbers = editor.LineNumbersOn
	} else {
		f.st.LineNumbers = editor.LineNumbersOff
	}
	iconBtn := func(icon, label string) bool {
		return minput.Button(c, "", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must(icon), Label: label}).Clicked()
	}
	toggleBtn := func(icon, label string, on bool) bool {
		variant := minput.Ghost
		if on {
			variant = minput.Secondary
		}
		return minput.Button(c, "", minput.ButtonOptions{Variant: variant, Icon: icons.Must(icon), Label: label}).Clicked()
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			if iconBtn("folder", "打开") {
				cl.openViaDialog()
			}
			saveVariant := minput.Ghost
			if dirty {
				saveVariant = minput.Primary
			}
			if minput.Button(c, "", minput.ButtonOptions{Variant: saveVariant, Icon: icons.Must("download"), Label: "保存"}).Clicked() {
				cl.save(f)
			}
			mgo.Box(c).Width(1).Height(18).Shrink(0).Background(k.ControlBorder)
			if iconBtn("search", "查找") {
				cl.runCmd(f, "find")
			}
			if iconBtn("message-square", "注释") {
				cl.runCmd(f, "toggleComment")
			}
			if iconBtn("chevron-left", "减少缩进") {
				cl.runCmd(f, "outdent")
			}
			if iconBtn("chevron-right", "增加缩进") {
				cl.runCmd(f, "indent")
			}
			if iconBtn("chevrons-up-down", "折叠全部") {
				cl.runCmd(f, "foldAll")
			}
			if iconBtn("arrow-up-down", "跳转到行") {
				cl.runCmd(f, "goToLine")
			}
			if toggleBtn("wrap-text", "软换行", f.wrap) {
				f.wrap = !f.wrap
			}
			if toggleBtn("list", "行号", f.lineNo) {
				f.lineNo = !f.lineNo
			}
			if toggleBtn("lock", "只读", f.st.ReadOnly) {
				f.st.ReadOnly = !f.st.ReadOnly
			}
			mgo.Box(c).Grow(1)
			if f.path != "" {
				mgo.Text(c, f.path).FontSize(caption).TextColor(k.TextMuted).Shrink(0).SingleLine().Ellipsis("…")
			}
		})
		if f.err != "" {
			mgo.Text(c, f.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			editor.View(c, f.st).Grow(1).FillWidth().AutoFocus()
		})
		pos := f.st.CursorPos()
		lang := "Plain Text"
		if l := f.buf.Language(); l != nil {
			lang = l.Name
		}
		mgo.Row(c).FillWidth().Shrink(0).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, fmt.Sprintf("行 %d, 列 %d", pos.Line+1, pos.Col+1)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, fmt.Sprintf("%d 行 · %d 字符", f.buf.LineCount(), f.buf.Len())).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, lang).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if n, cur := f.st.FindMatches(); n > 0 {
				mgo.Text(c, fmt.Sprintf("%d/%d", cur+1, n)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
			mgo.Box(c).Grow(1)
			if dirty {
				mgo.Text(c, "未保存").FontSize(caption).TextColor(k.AccentText).Shrink(0)
			}
		})
	})
}

// runCmd runs a named editor command on the file's state.
func (cl *Client) runCmd(f *file, name string) {
	if cmd, ok := editor.CommandNamed(name); ok {
		f.st.Run(cmd)
		cl.host.Invalidate()
	}
}

// save writes the buffer back to its path.
func (cl *Client) save(f *file) {
	path := f.path
	if strings.TrimSpace(path) == "" {
		if picked := cl.host.SaveFileDialog("untitled.go"); picked != "" {
			path = cl.relOf(picked)
			f.path = path
			f.buf.SetLanguage(editor.LanguageFor(path))
		}
	}
	if strings.TrimSpace(path) == "" {
		return
	}
	abs := cl.abs(path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		f.err = err.Error()
		return
	}
	if err := os.WriteFile(abs, []byte(f.buf.String()), 0o644); err != nil {
		f.err = err.Error()
		return
	}
	f.buf.MarkSaved()
	f.base.Reset(f.buf.String())
	f.err = ""
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
			cl.openAndRegister(rel)
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

func (cl *Client) file(id string) *file {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.docs[id]
}

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
			Name:   "open_file",
			Desc:   "在工作区中打开一个文件标签供用户查看",
			Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.openTool(str(args, "path"))
			},
		},
		{
			Name: "list_tabs",
			Desc: "列出当前打开的代码标签（id、路径；未保存或选中的标注）",
			Run: func(map[string]any) (string, error) {
				if len(cl.tabs.Items) == 0 {
					return "(没有打开的文件)", nil
				}
				var b strings.Builder
				for _, id := range cl.tabs.Items {
					f := cl.file(id)
					name := "未命名"
					mark := ""
					if f != nil {
						if f.path != "" {
							name = f.path
						}
						if f.buf.Modified() {
							mark += "[未保存]"
						}
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

func (cl *Client) readFile(rel string) (string, error) {
	return filetools.Read(cl.host.Root(), rel)
}

func (cl *Client) writeFile(rel, content string) (string, error) {
	if err := filetools.Write(cl.host.Root(), rel, content); err != nil {
		return "", err
	}
	cl.replaceBuffer(rel, content)
	return "已写入 " + rel, nil
}

func (cl *Client) appendFile(rel, content string) (string, error) {
	if err := filetools.Append(cl.host.Root(), rel, content); err != nil {
		return "", err
	}
	cl.syncDoc(rel)
	return "已追加到 " + rel, nil
}

func (cl *Client) mkdir(rel string) (string, error) {
	if err := filetools.Mkdir(cl.host.Root(), rel); err != nil {
		return "", err
	}
	return "已创建目录 " + rel, nil
}

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

func (cl *Client) search(pattern, dir, regexp, ci string) (string, error) {
	return filetools.Search(cl.host.Root(), dir, pattern, filetools.SearchOpts{
		Regexp:          strings.EqualFold(regexp, "true"),
		CaseInsensitive: strings.EqualFold(ci, "true"),
	})
}

func (cl *Client) openTool(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	if _, err := filetools.Within(cl.host.Root(), rel); err != nil {
		return "", err
	}
	cl.openAndRegister(rel)
	return "已在编辑器打开 " + rel, nil
}

// openAndRegister opens a path in a tab and mirrors it into the shell's tab
// bar through the open handler.
func (cl *Client) openAndRegister(rel string) {
	cl.Open(rel)
	if cl.openHandler != nil {
		cl.openHandler(rel)
	}
}

// replaceBuffer sets an open tab's text to content on the UI thread and marks
// it saved, as the Agent just wrote the file.
func (cl *Client) replaceBuffer(rel, content string) {
	cl.host.Post(func() {
		cl.mu.Lock()
		if f, ok := cl.docs[rel]; ok {
			f.buf.Set(content)
			f.buf.MarkSaved()
			f.base.Reset(content)
			f.err = ""
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
}

// syncDoc refreshes an open tab's buffer from disk.
func (cl *Client) syncDoc(rel string) {
	cl.host.Post(func() {
		cl.mu.Lock()
		if f, ok := cl.docs[rel]; ok {
			if data, err := os.ReadFile(filepath.Join(cl.host.Root(), rel)); err == nil {
				text := truncate(string(data))
				f.buf.Set(text)
				f.buf.MarkSaved()
				f.base.Reset(text)
				f.err = ""
			}
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
}

func (cl *Client) listFiles(rel string) (string, error) {
	return filetools.List(cl.host.Root(), rel)
}

func truncate(s string) string {
	if len(s) > maxFile {
		return s[:maxFile]
	}
	return s
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
