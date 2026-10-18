// Package markdownapp is the Apps menu's Markdown editor: open documents in
// tabs, edit and save them, and let the Agent read and write files.
package markdownapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/apps/filetools"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是 Markdown 文档助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files 读写与查找项目工作目录中的文件，帮助用户起草和修改文档。"

// doc is one open document.
type doc struct {
	path    string
	content string
	saved   string
	loaded  bool
	preview bool
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
			in := minput.TextInput(c, &d.path, minput.TextInputOptions{
				Label: "路径", Placeholder: "相对工作目录，如 README.md",
			})
			in.Element.Grow(1)
			if minput.Button(c, "打开", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cl.load(d)
			}
			saveVariant := minput.Secondary
			if d.content != d.saved {
				saveVariant = minput.Primary
			}
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: saveVariant}).Clicked() {
				cl.save(d)
			}
			previewVariant := minput.Ghost
			if d.preview {
				previewVariant = minput.Secondary
			}
			if minput.Button(c, "预览", minput.ButtonOptions{Variant: previewVariant}).Clicked() {
				d.preview = !d.preview
			}
		})
		if d.err != "" {
			mgo.Text(c, d.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Radius(mtheme.ControlRadius).Clip().Children(func() {
			switch {
			case d.preview:
				mgo.Scroll(c).Fill().Children(func() {
					mchat.MarkdownView(c, d.content).Element.FillWidth()
				})
			default:
				mgo.TextAreaBase(c, &d.content).Fill().Padding(u * 2).
					Placeholder("# 标题\n\n开始编写 Markdown…")
			}
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
	d.content, d.saved, d.loaded, d.err = string(data), string(data), true, ""
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
	d.saved, d.loaded, d.err = d.content, true, ""
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
			Name:   "search_files",
			Desc:   "递归搜索工作目录中的文本，输出 路径:行号: 内容",
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
