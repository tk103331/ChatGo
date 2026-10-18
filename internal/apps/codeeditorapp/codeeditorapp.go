// Package codeeditorapp is the Apps menu's code editor: open source files in
// tabs, edit them with syntax preview, save, and let the Agent read and write.
package codeeditorapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是代码编辑器助手。你可以通过 read_file/write_file/list_files 读写项目中的源文件，帮助用户查看、修改与生成代码。"

const maxFile = 2 << 20

// file is one open source file.
type file struct {
	path    string
	content string
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
				f.content = truncate(string(data))
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
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
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
	f.content, f.err = truncate(string(data)), ""
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
	f.err = ""
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
	}
}

func (cl *Client) readFile(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	abs, err := within(cl.host.Root(), rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return truncate(string(data)), nil
}

func (cl *Client) writeFile(rel, content string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	abs, err := within(cl.host.Root(), rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		if f, ok := cl.docs[rel]; ok {
			f.content, f.err = content, ""
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	return "已写入 " + rel, nil
}

func (cl *Client) listFiles(rel string) (string, error) {
	abs, err := within(cl.host.Root(), rel)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(空目录)", nil
	}
	return strings.Join(names, "\n"), nil
}

func truncate(s string) string {
	if len(s) > maxFile {
		return s[:maxFile]
	}
	return s
}

func within(root, rel string) (string, error) {
	base := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(base, rel))
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越权: %s", rel)
	}
	return abs, nil
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
