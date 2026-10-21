package codeeditorapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mygo "github.com/egoist/mygo"
)

type host struct {
	dir    string
	toasts []string
}

func (h *host) Root() string                                        { return h.dir }
func (h *host) Workspace() string                                   { return h.dir }
func (h *host) Post(fn func())                                      { fn() }
func (h *host) Invalidate()                                         {}
func (h *host) Toast(msg string, _ mcore.Severity)                  { h.toasts = append(h.toasts, msg) }
func (h *host) Confirm(_ string, _ string, a func())                { a() }
func (h *host) Prompt(_ string, _ string, _ string, a func(string)) { a("") }
func (h *host) OpenFile(string)                                     {}
func (h *host) OpenFileDialog(string) string                        { return "" }
func (h *host) SaveFileDialog(string) string                        { return "" }
func (h *host) Language(ext string) string                          { return appkit.DefaultLanguage(ext) }
func (h *host) NewID() string                                       { return "id" }
func (h *host) NativeWindow() *mygo.Window                          { return nil }

func TestOpenSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	h := &host{dir: dir}
	cl := New(h)
	id := cl.Open("main.go")
	if cl.Title(id) != "main.go" {
		t.Fatalf("title: %q", cl.Title(id))
	}
	if _, err := cl.writeFile("main.go", "package main"); err != nil {
		t.Fatal(err)
	}
	if got := cl.Title(id); got != "main.go" {
		t.Fatalf("title after write: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil || string(data) != "package main" {
		t.Fatalf("file: %q %v", data, err)
	}
	cl.Close(id)
	if cl.Tabs().Active != "" {
		t.Fatal("close should clear active tab")
	}
}

func TestToolRejectsEscape(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	if _, err := cl.readFile("../../etc/passwd"); err == nil {
		t.Fatal("path escape should be rejected")
	}
	if _, err := cl.readFile(""); err == nil {
		t.Fatal("empty path should be rejected")
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("x"), 0o644)
	cl := New(&host{dir: dir})
	out, err := cl.listFiles("")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a.md\nsub/" {
		t.Fatalf("list: %q", out)
	}
}

func TestNewFileTools(t *testing.T) {
	dir := t.TempDir()
	h := &host{dir: dir}
	cl := New(h)
	if _, err := cl.appendFile("log.go", "package log\n"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "log.go"))
	if string(data) != "package log\n" {
		t.Fatalf("append: %q", data)
	}
	if _, err := cl.mkdir("sub/dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub/dir")); err != nil {
		t.Fatal("mkdir failed")
	}
	out, err := cl.search("log", "", "false", "false")
	if err != nil || !strings.Contains(out, "log.go:1:") {
		t.Fatalf("search: %q %v", out, err)
	}
	if _, err := cl.search("", "", "false", "false"); err == nil {
		t.Fatal("empty pattern should fail")
	}
}

func TestDeleteConfirm(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("x"), 0o644)
	if _, err := cl.remove("x.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.go")); !os.IsNotExist(err) {
		t.Fatal("confirm should delete the file")
	}
}

func TestRenameConfirm(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("x"), 0o644)
	if _, err := cl.rename("a.go", "b.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.go")); err != nil {
		t.Fatal("rename failed")
	}
}

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

func TestListTabs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	cl := New(&host{dir: dir})
	id := cl.Open("main.go")
	cl.SetContext(false, []string{id})
	if f := cl.file(id); f != nil {
		f.buf.Set("package changed")
	}
	tools := cl.Tools()
	out, err := tools[len(tools)-1].Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- "+id+": main.go[未保存][上下文]") {
		t.Fatalf("list_tabs: %q", out)
	}
}

func TestAbsAndRel(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	if got := cl.abs("sub/main.go"); got != filepath.Join(dir, "sub/main.go") {
		t.Fatalf("abs rel: %q", got)
	}
	abs := filepath.Join(dir, "main.go")
	if got := cl.abs(abs); got != abs {
		t.Fatalf("abs absolute: %q", got)
	}
	if got := cl.relOf(abs); got != "main.go" {
		t.Fatalf("relOf inside: %q", got)
	}
	outside := filepath.Join(filepath.Dir(dir), "x.go")
	if got := cl.relOf(outside); got != outside {
		t.Fatalf("relOf outside: %q", got)
	}
}

func TestOpenToolRegisters(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)
	cl := New(&host{dir: dir})
	var got []string
	cl.SetOpenHandler(func(p string) { got = append(got, p) })
	if _, err := cl.openTool("main.go"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "main.go" {
		t.Fatalf("handler: %v", got)
	}
	if cl.Tabs().Active != "main.go" {
		t.Fatalf("active: %q", cl.Tabs().Active)
	}
}

func TestTreeLoadAndLabel(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	cl := New(&host{dir: dir})
	if !cl.treeLoaded[treeRoot] {
		t.Fatal("root should be preloaded")
	}
	kids := cl.treeDirs[treeRoot]
	if len(kids) != 2 || kids[0] != "pkg" || kids[1] != "main.go" {
		t.Fatalf("children = %v, want [pkg main.go]", kids)
	}
	if !cl.treeIsDir["pkg"] {
		t.Error("pkg should be a dir")
	}
	if cl.treeIsDir["main.go"] {
		t.Error("main.go should not be a dir")
	}
	if got := cl.treeLabel("main.go"); got != "main.go" {
		t.Errorf("treeLabel = %q", got)
	}
	if got := cl.treeLabel("pkg"); got != "pkg/" {
		t.Errorf("treeLabel dir = %q, want pkg/", got)
	}
}

func TestOpenAndRegisterMirrorsTab(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cl := New(&host{dir: dir})
	var mirrored []string
	cl.SetOpenHandler(func(p string) { mirrored = append(mirrored, p) })
	cl.NewTab()
	cl.openAndRegister("main.go")
	if len(mirrored) != 1 || mirrored[0] != "main.go" {
		t.Fatalf("open handler = %v, want [main.go]", mirrored)
	}
	if cl.Tabs().Active != "main.go" {
		t.Fatalf("active = %q, want main.go", cl.Tabs().Active)
	}
	f := cl.file("main.go")
	if f == nil || f.buf.String() != "package main\n" {
		t.Fatalf("file content not loaded: %+v", f)
	}
}

func TestOpenTreeItemOpensFileAndSkipsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cl := New(&host{dir: dir})
	var mirrored []string
	cl.SetOpenHandler(func(p string) { mirrored = append(mirrored, p) })
	cl.openTreeItem("pkg")
	if len(mirrored) != 0 {
		t.Fatalf("folder should not open a tab: %v", mirrored)
	}
	cl.openTreeItem("build.sh")
	if len(mirrored) != 1 || mirrored[0] != "build.sh" {
		t.Fatalf("file open = %v, want [build.sh]", mirrored)
	}
	if f := cl.file("build.sh"); f == nil || f.buf.String() != "#!/bin/sh\n" {
		t.Fatalf("file not loaded: %+v", f)
	}
}
