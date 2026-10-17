package appkit

import (
	"testing"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

func TestTabsAddClose(t *testing.T) {
	var tabs Tabs
	tabs.Add("a")
	tabs.Add("b")
	tabs.Add("c")
	if tabs.Active != "c" || len(tabs.Items) != 3 {
		t.Fatalf("add: %+v", tabs)
	}
	tabs.Close(2)
	if tabs.Active != "b" {
		t.Fatalf("closing active should select previous, got %q", tabs.Active)
	}
	tabs.Close(0)
	if tabs.Active != "b" || len(tabs.Items) != 1 {
		t.Fatalf("closing inactive should keep active: %+v", tabs)
	}
	tabs.Close(0)
	if tabs.Active != "" || len(tabs.Items) != 0 {
		t.Fatalf("closing last should empty: %+v", tabs)
	}
	tabs.Close(5)
}

func TestDefaultLanguage(t *testing.T) {
	cases := map[string]string{".go": "go", ".ts": "typescript", ".md": "markdown", ".zzz": ""}
	for ext, want := range cases {
		if got := DefaultLanguage(ext); got != want {
			t.Fatalf("%s: want %q, got %q", ext, want, got)
		}
	}
}

func TestStandaloneHost(t *testing.T) {
	rt := &Standalone{Dir: "/tmp", Lang: DefaultLanguage}
	if rt.Root() != "/tmp" || rt.Workspace() != "/tmp" {
		t.Fatal("root/workspace mismatch")
	}
	ran := false
	rt.Post(func() { ran = true })
	if !ran {
		t.Fatal("Post should run inline without a window")
	}
	rt.Toast("hi", mcore.SeveritySuccess)
	if got := rt.Toasts(); len(got) != 1 || got[0].Msg != "hi" {
		t.Fatalf("toasts: %+v", got)
	}
	rt.ClearToasts()
	if len(rt.Toasts()) != 0 {
		t.Fatal("clear failed")
	}
	if rt.Language(".go") != "go" {
		t.Fatal("language mapping failed")
	}
	if rt.NewID() == rt.NewID() {
		t.Fatal("ids should be unique")
	}
}
