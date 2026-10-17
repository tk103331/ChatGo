package ui

import (
	"os/exec"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/gitapp"
	"chatgo/pkg/models"
)

func gitTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "a@b.c"},
		{"config", "user.name", "T"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGitClientRenders(t *testing.T) {
	dir := gitTestRepo(t)
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.workspace = dir
	cw.section = sectionApps
	cw.openApp("git")
	for _, page := range []int{gitapp.PageChanges, gitapp.PageHistory, gitapp.PageBranches} {
		cw.gitClient().SetPage(page)
		img := mgo.Render(cw.view, 1200, 800, 1)
		if img == nil {
			t.Fatalf("page %d render failed", page)
		}
	}
}

func TestGitClientEmptyRepo(t *testing.T) {
	cw := testWindow()
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.workspace = t.TempDir()
	cw.section = sectionApps
	cw.openApp("git")
	if img := mgo.Render(cw.view, 1000, 700, 1); img == nil {
		t.Fatal("empty repo render failed")
	}
	if cw.gitClient().InRepo() {
		t.Fatal("non-repo should render the empty state")
	}
}
