package terminalapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mygo "github.com/egoist/mygo"
)

type host struct{ dir string }

func (h *host) Root() string                                        { return h.dir }
func (h *host) Workspace() string                                   { return h.dir }
func (h *host) Post(fn func())                                      { fn() }
func (h *host) Invalidate()                                         {}
func (h *host) Toast(string, mcore.Severity)                        {}
func (h *host) Confirm(_ string, _ string, a func())                { a() }
func (h *host) Prompt(_ string, _ string, _ string, a func(string)) { a("") }
func (h *host) OpenFile(string)                                     {}
func (h *host) Language(ext string) string                          { return appkit.DefaultLanguage(ext) }
func (h *host) NewID() string                                       { return "id" }
func (h *host) NativeWindow() *mygo.Window                          { return nil }

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	cl := New(&host{dir: dir})
	out, err := cl.runCommand("echo hello")
	if err != nil || !strings.Contains(out, "hello") {
		t.Fatalf("run: %q %v", out, err)
	}
	if _, err := cl.runCommand("  "); err == nil {
		t.Fatal("empty command should fail")
	}
}

func TestRunCommandUsesRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0o644)
	cl := New(&host{dir: dir})
	out, err := cl.runCommand("ls")
	if err != nil || !strings.Contains(out, "marker.txt") {
		t.Fatalf("command should run in root: %q %v", out, err)
	}
}

func TestTools(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	if len(cl.Tools()) != 2 {
		t.Fatalf("want 2 tools, got %d", len(cl.Tools()))
	}
	out, err := cl.Tools()[1].Run(nil)
	if err != nil || out != "(没有打开的终端)" {
		t.Fatalf("list: %q %v", out, err)
	}
}
