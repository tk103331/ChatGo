// Package codeeditorapp is the Apps menu's code editor: open source files in
// tabs, edit them with syntax preview, save, and let the Agent read and write.
package codeeditorapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/apps/filetools"

	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是代码编辑器助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files/open_file 读写与查找项目中的源文件，帮助用户查看、修改与生成代码。"

const maxFile = 2 << 20

// file is one open source file.
type file struct {
	path    string
	content string
	saved   string
	find    string
	repl    string
	findPos int
	err     string
	preview bool
	view    code.CodeViewerState
}

// Client is one code editor bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	tabs appkit.Tabs
	docs map[string]*file
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns an editor driving the given host.
func New(h Host) *Client {
	return &Client{host: h, docs: map[string]*file{}}
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
		f := &file{path: path}
		if path != "" {
			if data, err := os.ReadFile(cl.abs(path)); err == nil {
				f.content, f.saved = truncate(string(data)), truncate(string(data))
			} else {
				f.err = err.Error()
			}
		}
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

// View draws the active file.
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
	lang := cl.host.Language(filepath.Ext(f.path))
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			in := minput.TextInput(c, &f.path, minput.TextInputOptions{
				Label: "路径", Placeholder: "相对工作目录，如 main.go",
			})
			in.Element.Grow(1)
			if minput.Button(c, "打开", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cl.load(f)
			}
			saveVariant := minput.Secondary
			if f.content != f.saved {
				saveVariant = minput.Primary
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: saveVariant}).Clicked() {
				cl.save(f)
			}
			variant := minput.Ghost
			if f.preview {
				variant = minput.Secondary
			}
			if minput.Button(c, "预览", minput.ButtonOptions{Variant: variant}).Clicked() {
				f.preview = !f.preview
			}
		})
		mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
			find := minput.TextInput(c, &f.find, minput.TextInputOptions{Label: "查找", Placeholder: "查找内容"})
			find.Element.Grow(1)
			repl := minput.TextInput(c, &f.repl, minput.TextInputOptions{Label: "替换", Placeholder: "替换为"})
			repl.Element.Grow(1)
			if minput.Button(c, "上一个", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				f.findPos = findPrev(f.content, f.find, f.findPos, false)
			}
			if minput.Button(c, "下一个", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				i := findNext(f.content, f.find, f.findPos, false)
				if i < 0 {
					i = findNext(f.content, f.find, 0, false)
				}
				f.findPos = i
			}
			if minput.Button(c, "全部替换", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				before := f.content
				f.content = replaceAll(f.content, f.find, f.repl, false)
				if f.content != before && f.find != "" {
					cl.host.Toast("已全部替换", mcore.SeveritySuccess)
				}
			}
		})
		if f.err != "" {
			mgo.Text(c, f.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			switch {
			case f.preview && lang != "":
				code.CodeViewer(c, strings.Split(f.content, "\n"), &f.view, code.CodeViewerOptions{Language: lang}).Element.Fill()
			case f.preview:
				mgo.Scroll(c).Fill().Children(func() {
					mgo.Text(c, f.content).Font(mtheme.MonoFont).FontSize(caption).Padding(u * 2)
				})
			default:
				mgo.TextAreaBase(c, &f.content).Fill().Padding(u * 2).
					Placeholder("// 开始编写代码…").Font(mtheme.MonoFont)
			}
		})
		lines := 0
		if f.content != "" {
			lines = strings.Count(f.content, "\n") + 1
		}
		mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Padding(u, 0, 0, 0).Children(func() {
			mgo.Text(c, f.path).FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			mgo.Text(c, fmt.Sprintf("%d 行 · %d 字符", lines, len([]rune(f.content)))).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			if f.preview && lang != "" {
				mgo.Text(c, fmt.Sprintf("当前行 %d", f.view.Line()+1)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			}
			if f.content != f.saved {
				mgo.Text(c, "未保存").FontSize(caption).TextColor(k.AccentText).Shrink(0)
			}
		})
	})
}

// load reads the file's path into its buffer.
func (cl *Client) load(f *file) {
	if strings.TrimSpace(f.path) == "" {
		f.err = "请填写路径"
		return
	}
	data, err := os.ReadFile(cl.abs(f.path))
	if err != nil {
		f.err = err.Error()
		return
	}
	f.content, f.saved, f.err = truncate(string(data)), truncate(string(data)), ""
}

// save writes the buffer back to its path.
func (cl *Client) save(f *file) {
	if strings.TrimSpace(f.path) == "" {
		f.err = "请填写路径"
		return
	}
	abs := cl.abs(f.path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		f.err = err.Error()
		return
	}
	if err := os.WriteFile(abs, []byte(f.content), 0o644); err != nil {
		f.err = err.Error()
		return
	}
	f.saved, f.err = f.content, ""
	cl.host.Toast("已保存 "+f.path, mcore.SeveritySuccess)
}

func (cl *Client) file(id string) *file {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.docs[id]
}

func (cl *Client) abs(rel string) string { return filepath.Join(cl.host.Root(), rel) }

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
	}
}

func (cl *Client) readFile(rel string) (string, error) {
	return filetools.Read(cl.host.Root(), rel)
}

func (cl *Client) writeFile(rel, content string) (string, error) {
	if err := filetools.Write(cl.host.Root(), rel, content); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		if f, ok := cl.docs[rel]; ok {
			f.content, f.saved, f.err = content, content, ""
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
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
	id := cl.Open(rel)
	return "已在编辑器打开 " + cl.Title(id), nil
}

// syncDoc refreshes an open tab's buffer from disk.
func (cl *Client) syncDoc(rel string) {
	cl.host.Post(func() {
		cl.mu.Lock()
		if f, ok := cl.docs[rel]; ok {
			if data, err := os.ReadFile(filepath.Join(cl.host.Root(), rel)); err == nil {
				f.content, f.saved, f.err = truncate(string(data)), truncate(string(data)), ""
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
