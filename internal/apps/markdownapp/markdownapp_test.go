package markdownapp

import (
	"os"
	"path/filepath"
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
