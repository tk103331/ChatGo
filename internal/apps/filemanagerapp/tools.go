package filemanagerapp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// Param is one tool parameter.
type Param struct {
	Desc     string
	Required bool
}

// Tool is one operation exposed to the Agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "fs_list",
			Desc: "列出工作区内的目录内容（目录在前），path 省略为当前目录",
			Params: map[string]Param{
				"path": {Desc: "目录路径，省略为当前目录"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolList(strArg(args, "path"))
			},
		},
		{
			Name: "fs_read",
			Desc: "读取工作区内的文本文件内容",
			Params: map[string]Param{
				"path": {Desc: "文件路径", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRead(strArg(args, "path"))
			},
		},
		{
			Name: "fs_write",
			Desc: "写入文本文件（不存在则创建，覆盖已有内容前先确认）",
			Params: map[string]Param{
				"path":    {Desc: "文件路径", Required: true},
				"content": {Desc: "文件内容", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolWrite(strArg(args, "path"), strArg(args, "content"))
			},
		},
		{
			Name: "fs_mkdir",
			Desc: "新建目录",
			Params: map[string]Param{
				"path": {Desc: "目录路径", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolMkdir(strArg(args, "path"))
			},
		},
		{
			Name: "fs_rename",
			Desc: "重命名文件或目录（也可用于移动）",
			Params: map[string]Param{
				"path":     {Desc: "原路径", Required: true},
				"new_path": {Desc: "新路径", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRename(strArg(args, "path"), strArg(args, "new_path"))
			},
		},
		{
			Name: "fs_remove",
			Desc: "删除文件或目录（目录会递归删除，破坏性操作，先确认）",
			Params: map[string]Param{
				"path": {Desc: "路径", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRemove(strArg(args, "path"))
			},
		},
		{
			Name: "fs_search",
			Desc: "按名称在工作区内搜索文件",
			Params: map[string]Param{
				"pattern": {Desc: "名称包含的字符串", Required: true},
				"path":    {Desc: "搜索起点目录，省略为工作区根目录"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolSearch(strArg(args, "pattern"), strArg(args, "path"))
			},
		},
	}
}

// resolve resolves a tool path inside the workspace.
func (cl *Client) resolve(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		cl.mu.Lock()
		cur := cl.currentDir(cl.st)
		cl.mu.Unlock()
		return cur, nil
	}
	if !filepath.IsAbs(path) {
		cl.mu.Lock()
		root := cl.st.root
		cl.mu.Unlock()
		path = filepath.Join(root, path)
	}
	return insideRoot(cl.rootPath(), path)
}

// rootPath returns the workspace root.
func (cl *Client) rootPath() string {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.st.root
}

// toolList lists a directory.
func (cl *Client) toolList(path string) (string, error) {
	dir, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	entries, err := readDir(dir, true)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(空目录)", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s（%d 项）\n", dir, len(entries))
	for _, e := range entries {
		if e.Dir {
			fmt.Fprintf(&b, "- %s/\n", e.Name)
		} else {
			fmt.Fprintf(&b, "- %s  %s\n", e.Name, humanBytes(e.Size))
		}
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolRead reads a file.
func (cl *Client) toolRead(path string) (string, error) {
	abs, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	text, err := readText(abs)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() { cl.openPreview(abs) })
	return text, nil
}

// toolWrite writes a file.
func (cl *Client) toolWrite(path, content string) (string, error) {
	abs, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	if err := writeText(abs, content); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast("已写入 "+filepath.Base(abs), mcore.SeveritySuccess)
		cl.refresh()
	})
	return fmt.Sprintf("已写入 %s（%d 字节）", abs, len(content)), nil
}

// toolMkdir creates a directory.
func (cl *Client) toolMkdir(path string) (string, error) {
	abs, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	if err := makeDir(abs); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast("已创建 "+filepath.Base(abs), mcore.SeveritySuccess)
		cl.refresh()
	})
	return "已创建目录 " + abs, nil
}

// toolRename renames a path.
func (cl *Client) toolRename(oldPath, newPath string) (string, error) {
	from, err := cl.resolve(oldPath)
	if err != nil {
		return "", err
	}
	to, err := cl.resolve(newPath)
	if err != nil {
		return "", err
	}
	if err := renamePath(from, to); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast("已重命名", mcore.SeveritySuccess)
		cl.refresh()
	})
	return fmt.Sprintf("已将 %s 重命名为 %s", from, to), nil
}

// toolRemove deletes a path.
func (cl *Client) toolRemove(path string) (string, error) {
	abs, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	if err := removePath(abs); err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast("已删除 "+filepath.Base(abs), mcore.SeveritySuccess)
		cl.refresh()
	})
	return "已删除 " + abs, nil
}

// toolSearch searches names under a directory.
func (cl *Client) toolSearch(pattern, path string) (string, error) {
	dir, err := cl.resolve(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("缺少必填参数 pattern")
	}
	hits, err := searchFiles(dir, pattern, 200)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "(没有匹配的文件)", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "找到 %d 项：\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "- %s\n", h)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// strArg reads a string argument.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), "\"")
		}
	}
	return ""
}
