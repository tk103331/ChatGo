package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// Creating a working tree through the panel puts it under the worktree root
// on a branch of its own, locking it is reflected in the list, and removing
// it takes the directory with it.
func TestWorktreeLifecycleThroughPanel(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()

	want := defaultWorktreePath(dir, "sandbox")
	cw.addWorktree("", "sandbox", true)
	cw.waitFor("the new tree to be listed", func() bool {
		_, ok := cw.worktreeAt(want)
		return !cw.wt.busy && ok
	})

	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("the tree was not created at %q: %v", want, err)
	}
	if !filepath.IsAbs(want) {
		t.Fatalf("the worktree path %q must be absolute", want)
	}
	w, _ := cw.worktreeAt(want)
	if w.Branch != "sandbox" {
		t.Errorf("branch %q, want sandbox", w.Branch)
	}
	if w.Main {
		t.Error("a created tree is never the main one")
	}
	// The form clears itself only after the tree is actually created.
	if cw.wt.formOpen || cw.wt.branch != "" {
		t.Errorf("the form was not reset: open=%v branch=%q", cw.wt.formOpen, cw.wt.branch)
	}

	cw.setWorktreeLock(want, true)
	cw.waitFor("the lock to show up", func() bool {
		w, ok := cw.worktreeAt(want)
		return ok && w.Locked
	})
	cw.setWorktreeLock(want, false)
	cw.waitFor("the lock to be released", func() bool {
		w, ok := cw.worktreeAt(want)
		return ok && !w.Locked
	})

	cw.removeWorktree(want, true)
	cw.waitFor("the tree to be gone", func() bool {
		_, ok := cw.worktreeAt(want)
		return !ok
	})
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("the directory survived the removal: %v", err)
	}
}

// A branch already checked out elsewhere is refused before git is run, and
// nothing is created.
func TestAddWorktreeRefusesTakenBranch(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()
	main, ok := cw.worktreeAt(dir)
	if !ok || main.Branch == "" {
		t.Fatalf("the repository's own tree should hold a branch: %+v", main)
	}

	cw.addWorktree("copy", main.Branch, false)
	if path := defaultWorktreePath(dir, "copy"); dirExists(path) {
		t.Fatalf("a tree was created at %q for an already checked-out branch", path)
	}
	if n := len(cw.wt.trees); n != 1 {
		t.Fatalf("the list changed to %d trees", n)
	}
}

// Creating a branch that already exists is refused, since git would too.
func TestAddWorktreeRefusesExistingBranchAsNew(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()
	if out, err := gitRun(dir, "branch", "later"); err != nil {
		t.Fatalf("branch: %v: %s", err, out)
	}
	cw.loadWorktrees()
	cw.waitWorktrees()

	cw.addWorktree("later", "later", true)
	if path := defaultWorktreePath(dir, "later"); dirExists(path) {
		t.Fatalf("a tree was created at %q for an existing branch", path)
	}
}

// A branch that does not exist cannot be checked out into a new tree.
func TestAddWorktreeRefusesUnknownBranch(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()

	cw.addWorktree("ghost", "ghost", false)
	if path := defaultWorktreePath(dir, "ghost"); dirExists(path) {
		t.Fatalf("a tree was created at %q for an unknown branch", path)
	}
}

// Pruning drops the record of a tree whose directory was deleted outside the
// app, which is the state the list marks as prunable.
func TestPruneWorktreesDropsStaleRecord(t *testing.T) {
	dir := newTestRepo(t)
	cw := worktreeWindow(t, dir)
	cw.resetWorktreeState(dir)
	cw.waitWorktrees()

	gone := filepath.Join(filepath.Dir(dir), "wt-stale")
	if out, err := gitRun(dir, "worktree", "add", "--detach", gone); err != nil {
		t.Fatalf("worktree add: %v: %s", err, out)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	cw.loadWorktrees()
	cw.waitFor("the stale record to be seen as prunable", func() bool {
		w, ok := cw.worktreeAt(gone)
		return ok && w.Prunable
	})

	cw.pruneWorktrees()
	cw.waitFor("the stale record to be pruned", func() bool {
		_, ok := cw.worktreeAt(gone)
		return !ok
	})
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
