# 丰富 terminalapp / markdownapp / codeeditorapp 功能 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为三个 Apps 菜单应用（终端 / Markdown 编辑器 / 代码编辑器）扩充 Agent 工具面与 UI 交互，并抽取共享文件工具包消除重复。

**Architecture:** 新建 `internal/apps/filetools` 共享文件操作包（全部路径经 `Within` 越权保护）；两个编辑器复用该包并新增文件类工具；终端补白名单/工作目录/超时与输入输出工具；三者各自补 UI 能力（预览、查找替换、脏标记、清屏/中断等）。

**Tech Stack:** Go 1.27.1+、MyGo v0.5.5（`github.com/egoist/mygo/ui`、`plugins/terminal`）、MujicaUI（`core`/`input`/`chat`/`theme`）、`filepath`/`regexp` 标准库。

## Global Constraints

- Go 1.27.1+；改动后必须 `go build ./... && go vet ./... && go test ./...` 通过。
- 不改动 `internal/llm`、`internal/mcp`、`internal/ui/apps_adapt.go` 的 wrap 结构（工具数量自动扩展）。
- 所有读写项目文件的路径都必须经 `filetools.Within(root, rel)` 校验，拒绝越权。
- 破坏性操作（delete_file / rename_file）执行前必须经 `host.Confirm`（异步，Post 到 UI 线程）。
- 工具布尔参数一律为字符串 `"true"` / `"false"`（appTool 参数均为 String 类型）。
- 命令输出截断：文件读取 1MB（`filetools.Read`），终端命令输出 64KB。
- 每任务可独立构建与测试。

---

## 阶段 1：filetools 共享包 + 两个编辑器全部工具

### Task 1: 共享包 internal/apps/filetools

**Files:**
- Create: `internal/apps/filetools/filetools.go`
- Create: `internal/apps/filetools/filetools_test.go`

**Interfaces:**
- Produces: `Within(root, rel string) (string, error)`；`Read(root, rel) (string, error)`；`Write(root, rel, content) error`；`Append(root, rel, content) error`；`List(root, rel) (string, error)`；`Mkdir(root, rel) error`；`Rename(root, old, new) error`；`Delete(root, rel) error`；`Search(root, rel, pattern string, opts SearchOpts) (string, error)`；`type SearchOpts struct { Regexp, CaseInsensitive bool; MaxLines int }`。

- [ ] **Step 1: 写失败测试** `internal/apps/filetools/filetools_test.go`

```go
package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithinRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../x", "/etc/passwd", "a/../../x"} {
		if _, err := Within(root, rel); err == nil {
			t.Fatalf("escape %q should be rejected", rel)
		}
	}
	if _, err := Within(root, "ok/.."); err != nil {
		t.Fatalf("clean path should be allowed: %v", err)
	}
}

func TestWriteReadAppend(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "sub/a.txt", "hi"); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, "sub/a.txt")
	if err != nil || got != "hi" {
		t.Fatalf("read: %q %v", got, err)
	}
	if err := Append(root, "sub/a.txt", "!"); err != nil {
		t.Fatal(err)
	}
	got, _ = Read(root, "sub/a.txt")
	if got != "hi!" {
		t.Fatalf("append: %q", got)
	}
}

func TestListSorted(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644)
	out, err := List(root, "")
	if err != nil || out != "a.txt\nsub/" {
		t.Fatalf("list: %q %v", out, err)
	}
	os.MkdirAll(filepath.Join(root, "empty"), 0o755)
	if out, _ := List(root, "empty"); out != "(空目录)" {
		t.Fatalf("empty dir: %q", out)
	}
}

func TestMkdirRenameDelete(t *testing.T) {
	root := t.TempDir()
	if err := Mkdir(root, "d1/d2"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "d1/d2")); err != nil || !info.IsDir() {
		t.Fatal("mkdir failed")
	}
	os.WriteFile(filepath.Join(root, "old.txt"), []byte("x"), 0o644)
	if err := Rename(root, "old.txt", "d1/new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d1/new.txt")); err != nil {
		t.Fatal("rename failed")
	}
	if err := Delete(root, "d1/new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d1/new.txt")); !os.IsNotExist(err) {
		t.Fatal("delete failed")
	}
	if err := Delete(root, "d1"); err == nil {
		t.Fatal("directory delete should fail")
	}
}

func TestSearch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("func Hello() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("hello world\nHELLO again\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "x"), []byte("hello in git\n"), 0o644)
	out, err := Search(root, "", "hello", SearchOpts{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "b.txt:1:") || !strings.Contains(out, "b.txt:2:") {
		t.Fatalf("search miss: %q", out)
	}
	if strings.Contains(out, ".git") {
		t.Fatalf("search must skip .git: %q", out)
	}
	if !strings.Contains(out, "a.go:1:") {
		t.Fatalf("ci match: %q", out)
	}
	out, _ = Search(root, "", "hello", SearchOpts{CaseInsensitive: true, MaxLines: 2})
	if strings.Count(out, "\n") >= 3 {
		t.Fatalf("max lines: %q", out)
	}
	out, err = Search(root, "", "zzz", SearchOpts{})
	if err != nil || out != "(无匹配)" {
		t.Fatalf("no match: %q %v", out, err)
	}
	out, _ = Search(root, "b.txt", "^hello", SearchOpts{Regexp: true})
	if !strings.Contains(out, "b.txt:1:") {
		t.Fatalf("regexp: %q", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/apps/filetools/ -v`
Expected: 编译失败，`filetools` 包不存在。

- [ ] **Step 3: 实现** `internal/apps/filetools/filetools.go`

```go
// Package filetools holds the file operations the Markdown and code editors
// share, every one of them confined to a workspace root.
package filetools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaxRead = 1 << 20

// Within joins rel to root, rejecting any path that escapes root.
func Within(root, rel string) (string, error) {
	base := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(base, rel))
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越权: %s", rel)
	}
	return abs, nil
}

// Read returns a file's text, capped at MaxRead.
func Read(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if len(data) > MaxRead {
		data = data[:MaxRead]
	}
	return string(data), nil
}

// Write overwrites a file, creating its parent directories.
func Write(root, rel, content string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// Append adds content to a file's end, creating parent directories.
func Append(root, rel, content string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// List names a directory's entries, sorted, directories suffixed with "/".
func List(root, rel string) (string, error) {
	abs, err := Within(root, rel)
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

// Mkdir creates a directory and its parents.
func Mkdir(root, rel string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

// Rename moves a file (or directory), creating the destination's parents.
func Rename(root, old, new string) error {
	if strings.TrimSpace(old) == "" || strings.TrimSpace(new) == "" {
		return fmt.Errorf("缺少路径")
	}
	from, err := Within(root, old)
	if err != nil {
		return err
	}
	to, err := Within(root, new)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Delete removes a file only; directories are refused.
func Delete(root, rel string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("不能删除目录: %s", rel)
	}
	return os.Remove(abs)
}

// SearchOpts tune a Search.
type SearchOpts struct {
	Regexp          bool
	CaseInsensitive bool
	MaxLines        int
}

// Search recursively greps text under root for pattern, skipping VCS and
// dependency directories, emitting "rel:line: text" rows up to MaxLines.
func Search(root, rel, pattern string, opts SearchOpts) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("缺少搜索内容")
	}
	if opts.MaxLines <= 0 {
		opts.MaxLines = 100
	}
	re, err := compilePattern(pattern, opts.Regexp, opts.CaseInsensitive)
	if err != nil {
		return "", err
	}
	abs, err := Within(root, rel)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	lines := 0
	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != abs && skippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		for i, row := range strings.Split(string(data), "\n") {
			if lines >= opts.MaxLines {
				return filepath.SkipAll
			}
			if !re.MatchString(row) {
				continue
			}
			row = strings.TrimRight(row, "\r")
			if lines > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s:%d: %s", relPath, i+1, row)
			lines++
		}
		return nil
	})
	if lines == 0 {
		return "(无匹配)", nil
	}
	return b.String(), nil
}

func compilePattern(pattern string, useRegexp, ci bool) (*regexp.Regexp, error) {
	if ci {
		pattern = "(?i:" + pattern + ")"
	}
	if !useRegexp {
		pattern = regexp.QuoteMeta(pattern)
	}
	return regexp.Compile(pattern)
}

func skippedDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor":
		return true
	}
	return false
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/apps/filetools/ -v`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/apps/filetools/
git commit -m "feat(filetools): 新增共享文件操作包，含越权保护与递归搜索"
```

### Task 2: markdownapp 工具面（3 → 8）

**Files:**
- Modify: `internal/apps/markdownapp/markdownapp.go`
- Modify: `internal/apps/markdownapp/markdownapp_test.go`

**Interfaces:**
- Consumes: `filetools.Read/Write/Append/List/Mkdir/Rename/Delete/Search`, `filetools.SearchOpts`, `filetools.Within`（Task 1）。
- Produces: 工具集 `read_file` `write_file` `append_file` `list_files` `rename_file` `delete_file` `create_directory` `search_files`；保留方法名 `readFile/writeFile/listFiles`（测试依赖），新增 `appendFile/mkdir/remove/rename/search`、`syncDoc(rel string)`。

- [ ] **Step 1: 写失败测试**（追加到 `markdownapp_test.go`；该文件需新增 import `"strings"`）

```go
func TestNewFileTools(t *testing.T) {
	dir := t.TempDir()
	h := &host{dir: dir}
	cl := New(h)
	if _, err := cl.appendFile("log.md", "# t\n"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "log.md"))
	if string(data) != "# t\n" {
		t.Fatalf("append: %q", data)
	}
	if _, err := cl.mkdir("sub/dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/dir")); err != nil {
		t.Fatal("mkdir failed")
	}
	out, err := cl.search("t", "", "false", "false")
	if err != nil || !strings.Contains(out, "log.md:1:") {
		t.Fatalf("search: %q %v", out, err)
	}
	if _, err := cl.search("", "", "false", "false"); err == nil {
		t.Fatal("empty pattern should fail")
	}
}

func TestDeleteConfirm(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	os.WriteFile(filepath.Join(dir, "x.md"), []byte("x"), 0o644)
	if _, err := cl.remove("x.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.md")); !os.IsNotExist(err) {
		t.Fatal("confirm should delete the file")
	}
}

func TestRenameConfirm(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644)
	if _, err := cl.rename("a.md", "b.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.md")); err != nil {
		t.Fatal("rename failed")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/apps/markdownapp/ -run 'TestNewFileTools|TestDeleteConfirm|TestRenameConfirm'`
Expected: 编译失败，方法不存在。

- [ ] **Step 3: 实现工具方法**（`markdownapp.go`）

新增 import：`"chatgo/internal/apps/filetools"`。删除本地 `within` 函数与 `os.ReadFile` 读取逻辑，把 `readFile/writeFile/listFiles` 改为调用 filetools（保持方法签名不变，`writeFile` 仍同步打开的标签，逻辑不变）：

```go
func (cl *Client) readFile(rel string) (string, error) {
	return filetools.Read(cl.host.Root(), rel)
}

func (cl *Client) writeFile(rel, content string) (string, error) {
	if err := filetools.Write(cl.host.Root(), rel, content); err != nil {
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

func (cl *Client) listFiles(rel string) (string, error) {
	return filetools.List(cl.host.Root(), rel)
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

// syncDoc refreshes an open tab's buffer from disk.
func (cl *Client) syncDoc(rel string) {
	cl.host.Post(func() {
		cl.mu.Lock()
		if d, ok := cl.docs[rel]; ok {
			if data, err := os.ReadFile(filepath.Join(cl.host.Root(), rel)); err == nil {
				d.content, d.loaded, d.err = string(data), true, ""
			}
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
}
```

在 `Tools()` 中追加 5 个工具条目：

```go
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
```

更新 `System` 常量：

```go
const System = "你是 Markdown 文档助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files 读写与查找项目工作目录中的文件，帮助用户起草和修改文档。"
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/apps/markdownapp/`
Expected: 既有 + 新增测试全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/apps/markdownapp/
git commit -m "feat(markdownapp): 新增文件工具并复用共享 filetools"
```

### Task 3: codeeditorapp 工具面（3 → 9）

**Files:**
- Modify: `internal/apps/codeeditorapp/codeeditorapp.go`
- Modify: `internal/apps/codeeditorapp/codeeditorapp_test.go`

**Interfaces:**
- Consumes: `filetools`（Task 1）。
- Produces: 工具集 `read_file` `write_file` `append_file` `list_files` `rename_file` `delete_file` `create_directory` `search_files` `open_file`；方法 `appendFile/mkdir/remove/rename/search/openTool/syncDoc`。删除本地 `within` 函数（改走 `filetools.Within`），保留本地 `truncate/maxFile` 供 UI 打开文件用。

- [ ] **Step 1: 写失败测试**（追加到 `codeeditorapp_test.go`；该文件需新增 import `"strings"`。除文件名与内容外，`TestNewFileTools/TestDeleteConfirm/TestRenameConfirm` 与 Task 2 相同，另加 open_file 用例）

```go
func TestOpenFileTool(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	out, err := cl.openTool("main.go")
	if err != nil || !strings.Contains(out, "main.go") {
		t.Fatalf("open: %q %v", out, err)
	}
	if cl.Tabs().Active != "main.go" {
		t.Fatalf("open should activate tab: %q", cl.Tabs().Active)
	}
	if _, err := cl.openTool("../../x"); err == nil {
		t.Fatal("escape should fail")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/apps/codeeditorapp/ -run 'TestNewFileTools|TestDeleteConfirm|TestRenameConfirm|TestOpenFileTool'`
Expected: 编译失败，方法不存在。

- [ ] **Step 3: 实现**（`codeeditorapp.go`）

- 新增 import `"chatgo/internal/apps/filetools"`；删除本地 `within` 函数。
- `readFile/writeFile/listFiles` 改为调用 filetools（方法签名不变；`writeFile` 同步标签逻辑不变；`readFile` 不再用本地 `truncate`，改由 filetools 1MB 截断）。
- 新增 `appendFile/mkdir/remove/rename/search/syncDoc`（同 Task 2 实现，Toast 文案不变）。
- 新增 `openTool`：

```go
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
```

- `Tools()` 追加 6 个工具条目：与 Task 2 相同的 `append_file`/`create_directory`/`rename_file`/`delete_file`/`search_files`（Params 与 Run 同 markdownapp，调用 codeeditorapp 自身的方法），另加：

```go
{
	Name:   "open_file",
	Desc:   "在工作区中打开一个文件标签供用户查看",
	Params: map[string]Param{"path": {Desc: "相对工作目录的路径", Required: true}},
	Run: func(args map[string]any) (string, error) {
		return cl.openTool(str(args, "path"))
	},
},
```

更新 `System` 常量：

```go
const System = "你是代码编辑器助手。你可以通过 read_file/write_file/append_file/list_files/rename_file/delete_file/create_directory/search_files/open_file 读写与查找项目中的源文件，帮助用户查看、修改与生成代码。"
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/apps/codeeditorapp/`
Expected: 既有 + 新增测试全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/apps/codeeditorapp/
git commit -m "feat(codeeditorapp): 新增文件工具与 open_file 并复用共享 filetools"
```

## 阶段 2：编辑器 UI 增强

### Task 4: markdownapp UI — 预览、脏标记、状态栏

**Files:**
- Modify: `internal/apps/markdownapp/markdownapp.go`

**Interfaces:**
- Consumes: Task 2 的 `doc` 结构；`mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"`。
- Produces: `doc` 新增字段 `saved string; preview bool`；`View` 内新增预览开关、脏标记保存按钮、底部状态栏。

- [ ] **Step 1: 加字段并维护 saved**（`markdownapp.go`）

```go
type doc struct {
	path    string
	content string
	saved   string
	loaded  bool
	preview bool
	err     string
}
```

在 `Open(path != "")` 读到文件后与 `load`/`save` 成功处设置 `d.saved = d.content`；`writeFile`/`syncDoc` 同步标签内容时同样刷新 `d.saved = d.content`。

- [ ] **Step 2: 实现 View 改造**

新增 import `mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"`。在 `View` 的工具栏行里，`保存` 按钮改为按脏标记选 variant，并追加 `预览` 按钮：

```go
dirty := d.content != d.saved
saveVariant := minput.Secondary
if dirty {
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
```

编辑区 body 的默认分支改为按 `d.preview` 二选一：

```go
switch {
case d.preview:
	mgo.Scroll(c).Fill().Children(func() {
		mchat.MarkdownView(c, d.content).Element.FillWidth()
	})
default:
	mgo.TextAreaBase(c, &d.content).Fill().Padding(u * 2).
		Placeholder("# 标题\n\n开始编写 Markdown…")
}
```

编辑区 Box 之后追加状态栏：

```go
lines := 0
if d.content != "" {
	lines = strings.Count(d.content, "\n") + 1
}
chars := len([]rune(d.content))
mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Padding(u, 0, 0, 0).Children(func() {
	mgo.Text(c, d.path).FontSize(caption).TextColor(k.TextMuted).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
	mgo.Text(c, fmt.Sprintf("%d 行 · %d 字符", lines, chars)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
	if dirty {
		mgo.Text(c, "未保存").FontSize(caption).TextColor(k.AccentText).Shrink(0)
	}
})
```

（`新建` 文档按钮已由 App 标签栏的「新建」提供，应用内不再重复添加。）

- [ ] **Step 3: 构建验证**

Run: `go build ./... && go vet ./internal/apps/markdownapp/`
Expected: 通过。`save` 成功后 `d.saved = d.content`，脏标记随之消失。

- [ ] **Step 4: 提交**

```bash
git add internal/apps/markdownapp/
git commit -m "feat(markdownapp): 新增预览、脏标记与状态栏"
```

### Task 5: codeeditorapp UI — 查找替换、状态栏、脏标记

**Files:**
- Modify: `internal/apps/codeeditorapp/codeeditorapp.go`
- Create: `internal/apps/codeeditorapp/findreplace.go`
- Create: `internal/apps/codeeditorapp/findreplace_test.go`

**Interfaces:**
- Produces: 包级纯函数 `findNext(content, needle string, from int, ci bool) int`、`findPrev(content, needle string, from int, ci bool) int`、`replaceAll(content, find, repl string, ci bool) string`；`file` 新增字段 `saved string; find, repl string; findPos int`。

- [ ] **Step 1: 写失败测试** `internal/apps/codeeditorapp/findreplace_test.go`

```go
package codeeditorapp

import "testing"

func TestFindNext(t *testing.T) {
	if got := findNext("aXbxc", "x", 0, false); got != 3 {
		t.Fatalf("findNext: %d", got)
	}
	if got := findNext("aXbxc", "x", 4, false); got != -1 {
		t.Fatalf("findNext from end: %d", got)
	}
	if got := findNext("aXbxc", "x", 0, true); got != 1 {
		t.Fatalf("findNext ci: %d", got)
	}
	if got := findNext("abc", "", 0, false); got != -1 {
		t.Fatalf("empty needle: %d", got)
	}
}

func TestFindPrev(t *testing.T) {
	if got := findPrev("axbxc", "x", 4, false); got != 3 {
		t.Fatalf("findPrev: %d", got)
	}
	if got := findPrev("axbxc", "x", 2, false); got != 1 {
		t.Fatalf("findPrev earlier: %d", got)
	}
	if got := findPrev("axbxc", "x", 0, false); got != -1 {
		t.Fatalf("findPrev at 0: %d", got)
	}
}

func TestReplaceAll(t *testing.T) {
	if got := replaceAll("aXbxc", "x", "y", false); got != "aXbyc" {
		t.Fatalf("replace: %q", got)
	}
	if got := replaceAll("aXbxc", "x", "y", true); got != "aybyc" {
		t.Fatalf("replace ci: %q", got)
	}
	if got := replaceAll("abc", "", "y", false); got != "abc" {
		t.Fatalf("empty find: %q", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/apps/codeeditorapp/ -run 'TestFindNext|TestFindPrev|TestReplaceAll'`
Expected: 编译失败，函数未定义。

- [ ] **Step 3: 实现** `internal/apps/codeeditorapp/findreplace.go`

```go
package codeeditorapp

import "strings"

// findNext returns the index of the next occurrence of needle at or after
// from, or -1; ci lowers both sides for case-insensitive matching.
func findNext(content, needle string, from int, ci bool) int {
	if needle == "" || from < 0 || from > len(content) {
		return -1
	}
	hay := content
	if ci {
		hay, needle = strings.ToLower(content), strings.ToLower(needle)
	}
	i := strings.Index(hay[from:], needle)
	if i < 0 {
		return -1
	}
	return from + i
}

// findPrev returns the index of the last occurrence of needle before from.
func findPrev(content, needle string, from int, ci bool) int {
	if needle == "" || from <= 0 {
		return -1
	}
	hay := content
	if ci {
		hay, needle = strings.ToLower(content), strings.ToLower(needle)
	}
	end := from
	if end > len(hay) {
		end = len(hay)
	}
	return strings.LastIndex(hay[:end], needle)
}

// replaceAll replaces every occurrence of find in content, unchanged when
// find is empty; ci makes the matching case-insensitive.
func replaceAll(content, find, repl string, ci bool) string {
	if find == "" {
		return content
	}
	if !ci {
		return strings.ReplaceAll(content, find, repl)
	}
	lower, needle := strings.ToLower(content), strings.ToLower(find)
	var b strings.Builder
	from := 0
	for {
		i := strings.Index(lower[from:], needle)
		if i < 0 {
			b.WriteString(content[from:])
			break
		}
		i += from
		b.WriteString(content[from:i])
		b.WriteString(repl)
		from = i + len(find)
	}
	return b.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/apps/codeeditorapp/ -run 'TestFindNext|TestFindPrev|TestReplaceAll'`
Expected: PASS。

- [ ] **Step 5: UI 接线**（`codeeditorapp.go`）

`file` 结构新增字段：

```go
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
```

在 `Open(path != "")` 读到文件后与 `load`/`save` 成功处设置 `f.saved = f.content`；`writeFile` 同步标签时同样刷新。

在路径工具栏行后、编辑区 Box 前插入查找/替换条：

```go
mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
	in := minput.TextInput(c, &f.find, minput.TextInputOptions{Label: "查找", Placeholder: "查找内容"})
	in.Element.Grow(1)
	re := minput.TextInput(c, &f.repl, minput.TextInputOptions{Label: "替换", Placeholder: "替换为"})
	re.Element.Grow(1)
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
```

编辑区 Box 之后追加状态栏（同 Task 4 样式，`dirty := f.content != f.saved`；预览+语法模式时显示 `f.view.Line()+1` 作为当前行，否则只显示总行数）：

```go
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
	if dirty {
		mgo.Text(c, "未保存").FontSize(caption).TextColor(k.AccentText).Shrink(0)
	}
})
```

`保存` 按钮按 `dirty` 选 variant（Primary 高亮）。

- [ ] **Step 6: 构建与测试**

Run: `go build ./... && go test ./internal/apps/codeeditorapp/`
Expected: 全部通过。

- [ ] **Step 7: 提交**

```bash
git add internal/apps/codeeditorapp/
git commit -m "feat(codeeditorapp): 新增查找替换条、状态栏与脏标记"
```

## 阶段 3：终端增强

### Task 6: terminalapp 工具面 — 白名单、workdir、timeout、send_keys、read_screen

**Files:**
- Modify: `internal/apps/terminalapp/terminalapp.go`
- Modify: `internal/apps/terminalapp/terminalapp_test.go`

**Interfaces:**
- Consumes: `filetools.Within`（Task 1）；`term.Terminal.Send/Text/Close`。
- Produces: `runCommandWith(command, workdir string, timeoutSec int) (string, error)`（保留 `runCommand(command)` 为薄包装）；`sendKeys(text string) (string, error)`；`readScreen() (string, error)`；`activeTerm() *term.Terminal`；`commandWhitelist map[string]bool`；包常量 `defaultTimeout/maxTimeout/maxOutput`。

- [ ] **Step 1: 写失败测试**（追加到 `terminalapp_test.go`）

```go
func TestRunCommandWhitelist(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	if _, err := cl.runCommand("evil_thing -x"); err == nil {
		t.Fatal("non-whitelisted command should fail")
	}
	out, err := cl.runCommand("echo hi")
	if err != nil || !strings.Contains(out, "hi") {
		t.Fatalf("whitelisted command: %q %v", out, err)
	}
}

func TestRunCommandWorkdir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	cl := New(&host{dir: dir})
	out, err := cl.runCommandWith("pwd", "sub", 0)
	if err != nil || !strings.Contains(out, "sub") {
		t.Fatalf("workdir: %q %v", out, err)
	}
	if _, err := cl.runCommandWith("pwd", "../../etc", 0); err == nil {
		t.Fatal("workdir escape should fail")
	}
}

func TestSendKeysReadScreenNoTab(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	if _, err := cl.sendKeys("ls"); err == nil {
		t.Fatal("send_keys without a tab should fail")
	}
	if _, err := cl.readScreen(); err == nil {
		t.Fatal("read_screen without a tab should fail")
	}
}

func TestToolCount(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	if len(cl.Tools()) != 4 {
		t.Fatalf("want 4 tools, got %d", len(cl.Tools()))
	}
}
```

注意：既有 `TestTools` 的断言需同步改为 `len(cl.Tools()) != 4`（工具顺序为 `run_command`、`list_terminals`、`send_keys`、`read_screen`，`cl.Tools()[1]` 仍是 `list_terminals`，其断言保持不变）。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/apps/terminalapp/ -run 'TestRunCommandWhitelist|TestRunCommandWorkdir|TestSendKeysReadScreenNoTab|TestToolCount'`
Expected: 编译失败，方法不存在；`TestTools` 因工具数断言 2 → 4 失败。

- [ ] **Step 3: 实现**（`terminalapp.go`）

新增 import：`"chatgo/internal/apps/filetools"`。

包级常量与白名单：

```go
const (
	defaultTimeout = 60
	maxTimeout     = 300
	maxOutput      = 64 << 10
)

var commandWhitelist = map[string]bool{
	"ls": true, "pwd": true, "echo": true, "cat": true, "head": true,
	"tail": true, "wc": true, "grep": true, "find": true, "git": true,
	"mkdir": true, "date": true, "uname": true, "du": true, "df": true,
	"ps": true, "curl": true, "node": true, "python3": true, "go": true, "make": true,
}
```

`runCommand` 改为薄包装，新增 `runCommandWith`：

```go
// runCommand runs a shell command in the workspace with a bounded timeout.
func (cl *Client) runCommand(command string) (string, error) {
	return cl.runCommandWith(command, "", 0)
}

func (cl *Client) runCommandWith(command, workdir string, timeoutSec int) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	name := firstWord(command)
	if !commandWhitelist[name] {
		return "", fmt.Errorf("命令 %q 不在白名单内", name)
	}
	dir := cl.host.Root()
	if strings.TrimSpace(workdir) != "" {
		abs, err := filetools.Within(cl.host.Root(), workdir)
		if err != nil {
			return "", err
		}
		dir = abs
	}
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeout
	}
	if timeoutSec > maxTimeout {
		timeoutSec = maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		c.Dir = dir
	}
	out, err := c.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if len(result) > maxOutput {
		result = result[:maxOutput] + "\n(输出已截断)"
	}
	if err != nil {
		return result + "\n" + err.Error(), nil
	}
	if result == "" {
		result = "(无输出)"
	}
	return result, nil
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}
	return s
}
```

新增 `activeTerm` / `sendKeys` / `readScreen`：

```go
func (cl *Client) activeTerm() *term.Terminal {
	if tb := cl.byID[cl.tabs.Active]; tb != nil {
		return tb.term
	}
	return nil
}

func (cl *Client) sendKeys(text string) (string, error) {
	t := cl.activeTerm()
	if t == nil {
		return "", fmt.Errorf("没有打开的终端")
	}
	t.Send([]byte(text))
	return "已发送到终端", nil
}

func (cl *Client) readScreen() (string, error) {
	t := cl.activeTerm()
	if t == nil {
		return "", fmt.Errorf("没有打开的终端")
	}
	out := strings.TrimSpace(t.Text())
	if out == "" {
		return "(空屏幕)", nil
	}
	return out, nil
}
```

`Tools()` 里 `run_command` 的 Run 改为：

```go
Run: func(args map[string]any) (string, error) {
	return cl.runCommandWith(str(args, "command"), str(args, "workdir"), intFrom(args, "timeout"))
},
```

并追加 `Params`：`"command"`（必填）、`"workdir"`（可选）、`"timeout"`（可选）。新增 `intFrom`：

```go
func intFrom(args map[string]any, key string) int {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err == nil {
				return n
			}
		}
	}
	return 0
}
```

（`strconv` 需加入 import。）`Tools()` 追加 `send_keys`（参数 `text` 必填）与 `read_screen`（无参数）。更新 `System`：

```go
const System = "你是终端应用助手。你可以通过 run_command（命令有白名单限制）在工作目录中执行 shell 命令，通过 send_keys 向终端发送按键、read_screen 读取终端内容，把结果反馈给用户。"
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/apps/terminalapp/`
Expected: 全部 PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/apps/terminalapp/
git commit -m "feat(terminalapp): run_command 支持白名单/工作目录/超时，新增 send_keys 与 read_screen"
```

### Task 7: terminalapp UI — 清屏、中断、状态栏

**Files:**
- Modify: `internal/apps/terminalapp/terminalapp.go`

**Interfaces:**
- Consumes: Task 6 的 `shellTab`；`term.Terminal.Send/Text/Dir`。
- Produces: `shellTab` 新增 `exited bool`；`View` 内工具栏行（清屏/中断按钮 + 状态文本）。

- [ ] **Step 1: 加 exited 字段并维护**（`terminalapp.go`）

```go
type shellTab struct {
	id     string
	title  string
	term   *term.Terminal
	err    string
	exited bool
}
```

`start` 的 `OnExit` 改为：

```go
OnExit: func(int) {
	cl.host.Post(func() { tb.exited = true; cl.host.Invalidate() })
},
```

- [ ] **Step 2: 实现 View 工具栏**

新增 import `minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"`。把 `View` 的整体结构从单个 `mgo.Box` 改为 `mgo.Column`，终端可用时先画工具栏再画终端：

```go
tb := cl.byID[cl.tabs.Active]
mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
	switch {
	case tb == nil:
		mgo.Text(c, "正在启动终端…").Padding(u*3).FontSize(caption).TextColor(k.TextMuted)
	case tb.err != "":
		mgo.Text(c, "终端不可用："+tb.err).Padding(u*3).FontSize(caption).TextColor(k.Danger)
	case tb.term == nil:
		mgo.Text(c, "正在启动终端…").Padding(u*3).FontSize(caption).TextColor(k.TextMuted)
	default:
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			if minput.Button(c, "清屏", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				tb.term.Send([]byte("\x0c"))
			}
			if minput.Button(c, "中断", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				tb.term.Send([]byte("\x03"))
			}
			mgo.Box(c).Grow(1)
			if dir := tb.term.Dir(); dir != "" {
				mgo.Text(c, dir).FontSize(caption).TextColor(k.TextMuted).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			}
			status := "运行中"
			if tb.exited {
				status = "已退出"
			}
			mgo.Text(c, status).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			term.View(c, tb.term).Fill()
		})
	}
})
```

（新建标签由 App 标签栏的「新建」提供，见 `internal/ui/apps.go` 的 `addAppTab`，应用内不重复。）

- [ ] **Step 3: 构建验证**

Run: `go build ./... && go vet ./internal/apps/terminalapp/`
Expected: 通过。

- [ ] **Step 4: 提交**

```bash
git add internal/apps/terminalapp/
git commit -m "feat(terminalapp): 新增清屏/中断工具栏与进程状态显示"
```
