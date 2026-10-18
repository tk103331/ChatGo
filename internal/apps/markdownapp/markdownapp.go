// Package markdownapp is the Apps menu's Markdown editor: open documents in
// tabs, edit and save them, and let the Agent read and write files.
package markdownapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 Markdown 文档助手。你可以通过 read_file/write_file/list_files 读写项目工作目录中的文件，帮助用户起草和修改文档。"

// doc is one open document.
type doc struct {
	path    string
	content string
	loaded  bool
	err     string
}

// Client is one Markdown editor bound to a Host.
type Client struct {
	host Host
	mu   sync.Mutex
	tabs appkit.Tabs
	docs map[string]*doc
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns an editor driving the given host.
func New(h Host) *Client {
	return &Client{host: h, docs: map[string]*doc{}}
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
		d := &doc{path: path}
		if path != "" {
			if data, err := os.ReadFile(cl.abs(path)); err == nil {
				d.content, d.loaded = string(data), true
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
			in := minput.TextInput(c, &d.path, minput.TextInputOptions{
				Label: "路径", Placeholder: "相对工作目录，如 README.md",
			})
			in.Element.Grow(1)
			if minput.Button(c, "打开", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cl.load(d)
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cl.save(d)
			}
		})
		if d.err != "" {
			mgo.Text(c, d.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			mgo.TextAreaBase(c, &d.content).Fill().Padding(u * 2).
				Placeholder("# 标题\n\n开始编写 Markdown…")
		})
	})
}

// load reads the document's path into its content.
func (cl *Client) load(d *doc) {
	if strings.TrimSpace(d.path) == "" {
		d.err = "请填写路径"
		return
	}
	data, err := os.ReadFile(cl.abs(d.path))
	if err != nil {
		d.err = err.Error()
		return
	}
	d.content, d.loaded, d.err = string(data), true, ""
}

// save writes the document's content back to its path.
func (cl *Client) save(d *doc) {
	if strings.TrimSpace(d.path) == "" {
		d.err = "请填写路径"
		return
	}
	abs := cl.abs(d.path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		d.err = err.Error()
		return
	}
	if err := os.WriteFile(abs, []byte(d.content), 0o644); err != nil {
		d.err = err.Error()
		return
	}
	d.loaded, d.err = true, ""
	cl.host.Toast("已保存 "+d.path, mcore.SeveritySuccess)
}

func (cl *Client) doc(id string) *doc {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.docs[id]
}

// abs joins a workspace-relative path to the host root.
func (cl *Client) abs(rel string) string {
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
	}
}

// readFile reads a file under the root for the Agent.
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
	if len(data) > 1<<20 {
		data = data[:1<<20]
	}
	return string(data), nil
}

// writeFile writes a file under the root and syncs an open tab.
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
		if d, ok := cl.docs[rel]; ok {
			d.content, d.loaded, d.err = content, true, ""
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	return "已写入 " + rel, nil
}

// listFiles lists a directory under the root for the Agent.
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

// within joins rel to root, rejecting paths that escape it.
func within(root, rel string) (string, error) {
	abs := filepath.Join(root, rel)
	clean := filepath.Clean(abs)
	base := filepath.Clean(root)
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越权: %s", rel)
	}
	return clean, nil
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
