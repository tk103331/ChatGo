package browserapp

import (
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

func TestListTabs(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	id := cl.NewTab("https://example.com")
	cl.SetContext(false, []string{id})
	tools := cl.Tools()
	if tools[0].Name != "list_tabs" {
		t.Fatalf("first tool should be list_tabs: %s", tools[0].Name)
	}
	out, err := tools[0].Run(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "- "+id+": https://example.com（加载中）[上下文]") {
		t.Fatalf("list_tabs: %q", out)
	}
}

func TestTargetPagesFallback(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	cl.byID["a"] = &page{id: "a", view: nil}
	cl.byID["b"] = &page{id: "b", view: nil}
	cl.tabs.Add("a")
	cl.tabs.Add("b")
	// No usable views: targetPages filters them out.
	if got := cl.targetPages(); len(got) != 0 {
		t.Fatalf("no usable pages: %v", got)
	}
	// Empty selection falls back to the active tab id.
	if ids := cl.targetIDs(); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("fallback: %v", ids)
	}
}
