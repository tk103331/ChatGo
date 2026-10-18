package markdownapp

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
	id := cl.Open("doc.md")
	if cl.Title(id) != "doc.md" {
		t.Fatalf("title: %q", cl.Title(id))
	}
	if _, err := cl.writeFile("doc.md", "# hello"); err != nil {
		t.Fatal(err)
	}
	if got := cl.Title(id); got != "doc.md" {
		t.Fatalf("title after write: %q", got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "doc.md"))
	if err != nil || string(data) != "# hello" {
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

func TestListTabs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# hi"), 0o644)
	cl := New(&host{dir: dir})
	id := cl.Open("doc.md")
	cl.SetContext(false, []string{id})
	if d := cl.doc(id); d != nil {
		d.content = "# changed"
	}
	tools := cl.Tools()
	out, err := tools[len(tools)-1].Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- "+id+": doc.md[未保存][上下文]") {
		t.Fatalf("list_tabs: %q", out)
	}
}

func TestAbsAndRel(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	if got := cl.abs("sub/a.md"); got != filepath.Join(dir, "sub/a.md") {
		t.Fatalf("abs rel: %q", got)
	}
	abs := filepath.Join(dir, "a.md")
	if got := cl.abs(abs); got != abs {
		t.Fatalf("abs absolute: %q", got)
	}
	if got := cl.relOf(abs); got != "a.md" {
		t.Fatalf("relOf inside: %q", got)
	}
	outside := filepath.Join(filepath.Dir(dir), "x.md")
	if got := cl.relOf(outside); got != outside {
		t.Fatalf("relOf outside: %q", got)
	}
}

func TestParseHeadings(t *testing.T) {
	content := "# Title\n\n## Sub\nSome text\n### Deep\n#nospace\n```go\n# code comment\n```\n"
	hs := parseHeadings(content)
	if len(hs) != 3 {
		t.Fatalf("headings: %d %+v", len(hs), hs)
	}
	if hs[0].level != 1 || hs[0].text != "Title" || hs[0].start != 0 {
		t.Fatalf("h1: %+v", hs[0])
	}
	if hs[1].level != 2 || hs[1].text != "Sub" {
		t.Fatalf("h2: %+v", hs[1])
	}
	if hs[2].level != 3 || hs[2].text != "Deep" {
		t.Fatalf("h3: %+v", hs[2])
	}
	if hs[1].start <= hs[0].end || hs[2].start <= hs[1].end {
		t.Fatalf("offsets not increasing: %+v", hs)
	}
}

func TestJumpToHeading(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	id := cl.Open("doc.md")
	d := cl.doc(id)
	d.content = "# A\n## B\n"
	d.mode = modePreview
	hs := parseHeadings(d.content)
	cl.jumpToHeading(d, hs[1])
	if d.mode != modeSplit {
		t.Fatal("jump from preview should switch to split")
	}
	if !d.jumpSel || d.jumpStart != hs[1].start || d.jumpEnd != hs[1].end {
		t.Fatalf("pending selection: %+v", d)
	}
}
