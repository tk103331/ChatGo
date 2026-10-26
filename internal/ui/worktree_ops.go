// Worktree operations: creating, removing, locking and pruning a
// repository's linked working trees, and pointing the app at one. Each runs
// git in the background and reloads the list from the main thread, so a slow
// repository never blocks a frame.
package ui

import (
	"os"
	"path/filepath"
	"strings"

	mcore "chatgo/internal/ui/uikit"
)

// addWorktree creates a working tree of the Worktree tab's repository. With
// newBranch it creates branch at HEAD, otherwise it checks the existing
// branch out.
func (cw *ChatWindow) addWorktree(name, branch string, newBranch bool) {
	cw.addWorktreeIn(cw.wt.dir, name, branch, newBranch)
}

// addWorktreeIn creates a working tree of the repository at dir, whatever
// panel asked for it. The checks read the repository itself, so a dialog
// outside the Worktree tab does not depend on that tab's state.
func (cw *ChatWindow) addWorktreeIn(dir, name, branch string, newBranch bool) {
	if dir == "" {
		return
	}
	name, branch = strings.TrimSpace(name), strings.TrimSpace(branch)
	if branch == "" {
		cw.toast("请填写分支名", mcore.SeverityError)
		return
	}
	// An empty name follows the branch, which is what one usually wants.
	if name == "" {
		name = branch
	}
	path := defaultWorktreePath(dir, name)
	if _, err := os.Stat(path); err == nil {
		cw.toast("目录已存在："+path, mcore.SeverityError)
		return
	}
	branches := gitBranchNames(dir)
	if !newBranch && !containsStr(branches, branch) {
		cw.toast("分支不存在："+branch, mcore.SeverityError)
		return
	}
	if newBranch && containsStr(branches, branch) {
		cw.toast("分支已存在："+branch, mcore.SeverityError)
		return
	}
	if !newBranch && gitBranchCheckedOut(dir, branch) {
		cw.toast("分支已被其他工作树签出："+branch, mcore.SeverityError)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		cw.toast("创建目录失败："+err.Error(), mcore.SeverityError)
		return
	}
	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "-b", branch, path)
	} else {
		args = append(args, path, branch)
	}
	go func() {
		out, err := gitRun(dir, args...)
		cw.postMain(func() {
			if err != nil {
				cw.toast(worktreeError(out, err), mcore.SeverityError)
				cw.invalidate()
				return
			}
			cw.wt.formOpen, cw.wt.name, cw.wt.branch = false, "", ""
			cw.wt.newBranch = false
			cw.toast("已创建工作树 "+path, mcore.SeveritySuccess)
			// The new tree changes both the Worktree tab and the conversation
			// list's project, so both reload.
			cw.loadWorktrees()
			cw.bumpConvGit(dir)
		})
	}()
}

// gitBranchNames lists a repository's local branches.
func gitBranchNames(dir string) []string {
	var names []string
	for _, b := range gitLocalBranches(dir, "") {
		names = append(names, b.Name)
	}
	return names
}

// gitBranchCheckedOut reports whether a branch is checked out in some working
// tree of the repository, which git refuses a second time.
func gitBranchCheckedOut(dir, branch string) bool {
	for _, t := range gitWorktrees(dir) {
		if t.Branch != "" && t.Branch == branch {
			return true
		}
	}
	return false
}

// removeWorktree deletes a working tree. Force also discards the changes it
// still holds, which git otherwise refuses.
func (cw *ChatWindow) removeWorktree(path string, force bool) {
	dir := cw.wt.dir
	if dir == "" || path == "" {
		return
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	go func() {
		out, err := gitRun(dir, args...)
		cw.postMain(func() {
			if err != nil {
				cw.toast(worktreeError(out, err), mcore.SeverityError)
				cw.invalidate()
				return
			}
			cw.forgetWorktreeWorkspace(path)
			cw.toast("已删除工作树 "+filepath.Base(path), mcore.SeveritySuccess)
			cw.loadWorktrees()
		})
	}()
}

// pruneWorktrees drops the administrative records of working trees whose
// directories are gone.
func (cw *ChatWindow) pruneWorktrees() {
	dir := cw.wt.dir
	if dir == "" {
		return
	}
	go func() {
		out, err := gitRun(dir, "worktree", "prune", "-v")
		cw.postMain(func() {
			if err != nil {
				cw.toast(worktreeError(out, err), mcore.SeverityError)
				cw.invalidate()
				return
			}
			note := "没有需要清理的工作树"
			if strings.TrimSpace(out) != "" {
				note = out
			}
			cw.toast(note, mcore.SeveritySuccess)
			cw.loadWorktrees()
		})
	}()
}

// setWorktreeLock locks or unlocks a working tree, which decides whether
// prune may remove it.
func (cw *ChatWindow) setWorktreeLock(path string, lock bool) {
	dir := cw.wt.dir
	if dir == "" || path == "" {
		return
	}
	args := []string{"worktree", "unlock", path}
	if lock {
		args = []string{"worktree", "lock", path}
	}
	go func() {
		out, err := gitRun(dir, args...)
		cw.postMain(func() {
			if err != nil {
				cw.toast(worktreeError(out, err), mcore.SeverityError)
				cw.invalidate()
				return
			}
			cw.loadWorktrees()
		})
	}()
}

// worktreeError is the message a failed git worktree command shows: its own
// stderr when it wrote one, else the exit error.
func worktreeError(out string, err error) string {
	if strings.TrimSpace(out) != "" {
		return out
	}
	return err.Error()
}

// openWorktreeWorkspace makes a working tree the current workspace, so its
// sessions, tabs, terminals and tools all run in that directory.
//
// Each workspace keeps its own center tabs, so switching stashes the ones the
// old workspace had and restores the new one's — which for a tree opened the
// first time is none at all. The Worktree tab is therefore reopened in the
// restored state, or the action would appear to close the very panel it was
// pressed in.
func (cw *ChatWindow) openWorktreeWorkspace(path string) {
	if path == "" {
		return
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		cw.toast("工作树目录不可用："+path, mcore.SeverityError)
		return
	}
	reopen := cw.focusCenterTab("worktree") != nil
	pane := cw.center.focus
	cw.switchCenterState(path)
	cw.addWorkspace(path)
	// Load the tree's summary now, so the conversation list groups it under
	// the project's main tree instead of showing a lone group of its own.
	cw.ensureConvGit(path)
	if reopen {
		cw.openWorktreeTab(pane)
	}
	cw.refreshGitPanel(path)
	cw.invalidate()
}

// refreshGitPanel points the Git panel at dir and reloads it. Setting gitFor
// keeps the panel from loading the same repository a second time on the next
// frame, which is where it notices a workspace change by itself.
func (cw *ChatWindow) refreshGitPanel(dir string) {
	cw.gitFor = dir
	cw.loadGit(dir)
}

// forgetWorktreeWorkspace drops a removed working tree from the workspace
// list, and moves off it when it was the current one, so the app never points
// at a directory git has just deleted.
func (cw *ChatWindow) forgetWorktreeWorkspace(path string) {
	cw.removeWorkspace(path)
	if cw.workspace != path {
		return
	}
	fallback := cw.wt.dir
	if fallback == "" || fallback == path {
		fallback, _ = os.Getwd()
	}
	reopen := cw.focusCenterTab("worktree") != nil
	pane := cw.center.focus
	cw.switchCenterState(fallback)
	cw.workspace = fallback
	if reopen {
		cw.openWorktreeTab(pane)
	}
	cw.refreshGitPanel(fallback)
}
