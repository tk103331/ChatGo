package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// appFile is one open document of the markdown app: a workspace-relative path
// and its draft text.
type appFile struct {
	path    string
	content string
	loaded  bool
	dirty   bool
	err     string
}

// markdownApp is the App menu's markdown editor.
func markdownApp() appDef {
	return appDef{
		ID:     "markdown",
		Title:  "Markdown 编辑器",
		Desc:   "打开、编辑并保存项目中的 Markdown 文档，Agent 可读写文件",
		Icon:   "file-text",
		System: "你是 Markdown 文档助手。你可以通过 read_file/write_file/list_files 读写项目工作目录中的文件，帮助用户起草和修改文档。",
		Tools: func(cw *ChatWindow) []appTool {
			return []appTool{
				{
					Name: "read_file",
					Desc: "读取工作目录中某个文件的文本内容",
					Params: map[string]appParam{
						"path": {Desc: "相对工作目录的路径", Required: true},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appReadFile(argString(args, "path"))
					},
				},
				{
					Name: "write_file",
					Desc: "把文本写入工作目录中的文件（覆盖），并同步到已打开的编辑器标签",
					Params: map[string]appParam{
						"path":    {Desc: "相对工作目录的路径", Required: true},
						"content": {Desc: "要写入的完整文本内容", Required: true},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appWriteFile(argString(args, "path"), argString(args, "content"))
					},
				},
				{
					Name:   "list_files",
					Desc:   "列出工作目录中的文件与子目录",
					Params: map[string]appParam{"dir": {Desc: "相对工作目录的子目录，可留空"}},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appListFiles(argString(args, "dir"))
					},
				},
			}
		},
		NewTab: func(cw *ChatWindow) string {
			return cw.newAppFileTab("")
		},
		TabLabel: func(cw *ChatWindow, tab string) string {
			if tab == "" {
				return "未命名"
			}
			return filepath.Base(tab)
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.appFileBody(c, tab)
		},
	}
}

// ensureAppFiles prepares the open-document map.
func (cw *ChatWindow) ensureAppFiles() {
	if cw.appFiles == nil {
		cw.appFiles = map[string]*appFile{}
	}
}

// newAppFileTab opens a document tab and returns its key (the path, or a
// generated placeholder for a new unnamed document).
func (cw *ChatWindow) newAppFileTab(path string) string {
	cw.ensureAppFiles()
	key := path
	if key == "" {
		key = "untitled-" + cw.newID()
	}
	if _, ok := cw.appFiles[key]; !ok {
		f := &appFile{path: path}
		if path != "" {
			if data, err := os.ReadFile(cw.appAbs(path)); err == nil {
				f.content = string(data)
				f.loaded = true
			} else {
				f.err = err.Error()
			}
		}
		cw.appFiles[key] = f
	}
	return key
}

// appFileBody renders the markdown editor for a tab.
func (cw *ChatWindow) appFileBody(c *mgo.Context, tab string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.ensureAppFiles()
	f := cw.appFiles[tab]
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			in := kit.TextInput(c, &f.path, kit.TextInputOptions{
				Label: "路径", Placeholder: "相对工作目录，如 README.md",
			})
			in.Element.Grow(1)
			if kit.Button(c, "打开", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.loadAppFile(f)
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveAppFile(f)
			}
		})
		if f.err != "" {
			mgo.Text(c, f.err).FontSize(caption).TextColor(k.Danger)
		}
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			mgo.TextAreaBase(c, &f.content).Fill().Padding(u * 2).
				Placeholder("# 标题\n\n开始编写 Markdown…")
		})
	})
}

// loadAppFile reads the tab's path into its content.
func (cw *ChatWindow) loadAppFile(f *appFile) {
	if strings.TrimSpace(f.path) == "" {
		f.err = "请填写路径"
		return
	}
	data, err := os.ReadFile(cw.appAbs(f.path))
	if err != nil {
		f.err = err.Error()
		return
	}
	f.content = string(data)
	f.loaded, f.err = true, ""
}

// saveAppFile writes the tab's content to its path.
func (cw *ChatWindow) saveAppFile(f *appFile) {
	if strings.TrimSpace(f.path) == "" {
		f.err = "请填写路径"
		return
	}
	abs := cw.appAbs(f.path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		f.err = err.Error()
		return
	}
	if err := os.WriteFile(abs, []byte(f.content), 0o644); err != nil {
		f.err = err.Error()
		return
	}
	f.loaded, f.err = true, ""
	cw.toast("已保存 "+f.path, mcore.SeveritySuccess)
}

// appAbs joins a workspace-relative path to the workspace root.
func (cw *ChatWindow) appAbs(rel string) string {
	return filepath.Join(cw.fileRoot(), rel)
}

// appReadFile reads a file under the workspace for the Agent.
func (cw *ChatWindow) appReadFile(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	abs, err := withinDir(cw.fileRoot(), rel)
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

// appWriteFile writes a file under the workspace and syncs an open tab.
func (cw *ChatWindow) appWriteFile(rel, content string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	abs, err := withinDir(cw.fileRoot(), rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	cw.postMain(func() {
		cw.ensureAppFiles()
		if f, ok := cw.appFiles[rel]; ok {
			f.content, f.loaded, f.err = content, true, ""
		}
	})
	return "已写入 " + rel, nil
}

// appListFiles lists a directory under the workspace for the Agent.
func (cw *ChatWindow) appListFiles(rel string) (string, error) {
	abs, err := withinDir(cw.fileRoot(), rel)
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
