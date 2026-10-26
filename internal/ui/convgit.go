// ConvGit caches a short git summary per workspace, so the conversation list
// can show, under each conversation, the branch its workspace has checked out
// and how many files are changed. The summaries are read in the background
// and cached: the list may hold dozens of conversations, and running git for
// every one of them on the drawing thread would stall the interface.
package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"chatgo/internal/ui/kit"
)

// Project is one repository's working trees grouped together. Its identity is
// the main working tree's path, and every linked tree of the repository joins
// it, so a conversation of any of those trees belongs to this one project.
type Project struct {
	// Main is the main working tree's path, the project's identity.
	Main string
	// Worktrees lists every working tree of the repository, main first.
	Worktrees []string
	// Branches are the repository's local branches, for picking one to check
	// out in a new working tree.
	Branches []string
	// TreeBranches maps each working tree path to the branch it has checked
	// out, "" when detached, so the dialog can tell a taken branch from a
	// free one.
	TreeBranches map[string]string
}

// convGitInfo is one workspace's git summary for a conversation row.
type convGitInfo struct {
	Branch  string
	Changes int
	// Project is the workspace's repository project, so the conversation
	// list can group every tree of one repository together.
	Project *Project
	// Modified, Added, Deleted and Untracked break the change count down by
	// status, for the hover card's analysis line.
	Modified, Added, Deleted, Untracked int
	// DiffAdded and DiffDeleted are the added and deleted line counts of the
	// working tree against HEAD, shown colored on the conversation row.
	DiffAdded, DiffDeleted int
}

// convGitState is the per-workspace summary cache and its in-flight loads.
type convGitState struct {
	mu sync.Mutex
	// info holds a loaded summary per workspace path.
	info map[string]*convGitInfo
	// loading marks the workspaces whose summary is being read, so the list
	// does not start a second load for one still in flight.
	loading map[string]bool
}

func newConvGitState() convGitState {
	return convGitState{info: map[string]*convGitInfo{}, loading: map[string]bool{}}
}

// ensureConvGit starts a background load of ws's summary unless one is
// already cached or running.
func (cw *ChatWindow) ensureConvGit(ws string) {
	if ws == "" {
		return
	}
	cw.convGit.mu.Lock()
	if cw.convGit.info == nil {
		cw.convGit.info = map[string]*convGitInfo{}
	}
	if cw.convGit.loading == nil {
		cw.convGit.loading = map[string]bool{}
	}
	if cw.convGit.info[ws] != nil || cw.convGit.loading[ws] {
		cw.convGit.mu.Unlock()
		return
	}
	cw.convGit.loading[ws] = true
	cw.convGit.mu.Unlock()
	go cw.loadConvGit(ws)
}

// loadConvGit reads ws's summary and stores it on the main thread.
func (cw *ChatWindow) loadConvGit(ws string) {
	info := readConvGit(ws)
	cw.postMain(func() {
		cw.convGit.mu.Lock()
		if info != nil {
			cw.convGit.info[ws] = info
		}
		delete(cw.convGit.loading, ws)
		cw.convGit.mu.Unlock()
		cw.invalidate()
	})
}

// readConvGit builds a workspace's git summary, or nil when it is not inside
// a repository.
func readConvGit(ws string) *convGitInfo {
	if gitOut(ws, "rev-parse", "--is-inside-work-tree") != "true" {
		return nil
	}
	branch := gitOut(ws, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		if h := gitOut(ws, "rev-parse", "--short", "HEAD"); h != "" {
			branch = h + " (detached)"
		}
	}
	if branch == "" {
		return nil
	}
	// The worktree list heads with the main tree, so it gives the project's
	// identity, every tree that belongs to it, and the branch each tree has
	// checked out.
	ts := gitWorktrees(ws)
	if len(ts) == 0 {
		return nil
	}
	project := &Project{Main: ts[0].Path, Branches: gitBranchNames(ws), TreeBranches: map[string]string{}}
	for _, t := range ts {
		project.Worktrees = append(project.Worktrees, t.Path)
		project.TreeBranches[t.Path] = t.Branch
	}
	stat := gitDiffStat(ws)
	var modified, added, deleted, untracked int
	files := gitChangedFiles(ws)
	for _, f := range files {
		switch f.Status {
		case kit.GitAdded:
			added++
		case kit.GitDeleted:
			deleted++
		case kit.GitUntracked:
			untracked++
		default:
			modified++
		}
	}
	// git diff --numstat only counts tracked files, so untracked ones are
	// added by their own line count: a new file contributes its lines to +N.
	for _, f := range files {
		if f.Status != kit.GitUntracked {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(ws, f.Path)); err == nil && len(data) > 0 {
			stat[0] += bytes.Count(data, []byte("\n"))
		}
	}
	return &convGitInfo{
		Branch: branch, Changes: len(files), Project: project,
		Modified: modified, Added: added, Deleted: deleted, Untracked: untracked,
		DiffAdded: stat[0], DiffDeleted: stat[1],
	}
}

// convGitProject is the project a workspace's conversations belong to, or nil
// while its summary is still loading or it is not a repository.
func (cw *ChatWindow) convGitProject(ws string) *Project {
	if ws == "" {
		return nil
	}
	cw.convGit.mu.Lock()
	info := cw.convGit.info[ws]
	cw.convGit.mu.Unlock()
	if info == nil {
		return nil
	}
	return info.Project
}

// convGitGroup is the grouping key a conversation's workspace falls under:
// its project's main working tree, or the workspace itself while its summary
// is still loading or it is not a repository.
func (cw *ChatWindow) convGitGroup(ws string) string {
	if p := cw.convGitProject(ws); p != nil && p.Main != "" {
		return p.Main
	}
	return ws
}

// convGitSubtitle is the branch name of a conversation's workspace, shown on
// its second line; the colored line counts ride beside it.
func (cw *ChatWindow) convGitSubtitle(ws string) string {
	if ws == "" {
		return ""
	}
	cw.convGit.mu.Lock()
	info := cw.convGit.info[ws]
	cw.convGit.mu.Unlock()
	if info == nil {
		return ""
	}
	return info.Branch
}

// convGitDiff is a workspace's added and deleted line counts against HEAD,
// shown colored as "+N -M" on the conversation row.
func (cw *ChatWindow) convGitDiff(ws string) (added, deleted int) {
	if ws == "" {
		return
	}
	cw.convGit.mu.Lock()
	info := cw.convGit.info[ws]
	cw.convGit.mu.Unlock()
	if info == nil {
		return
	}
	return info.DiffAdded, info.DiffDeleted
}

// convGitAnalysis breaks a workspace's changes down by status, for the hover
// card: "3 修改 · 1 新增", or "工作区干净" when nothing is changed.
func (cw *ChatWindow) convGitAnalysis(ws string) string {
	if ws == "" {
		return ""
	}
	cw.convGit.mu.Lock()
	info := cw.convGit.info[ws]
	cw.convGit.mu.Unlock()
	if info == nil {
		return ""
	}
	var parts []string
	if info.Modified > 0 {
		parts = append(parts, fmt.Sprintf("%d 修改", info.Modified))
	}
	if info.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d 新增", info.Added))
	}
	if info.Deleted > 0 {
		parts = append(parts, fmt.Sprintf("%d 删除", info.Deleted))
	}
	if info.Untracked > 0 {
		parts = append(parts, fmt.Sprintf("%d 未跟踪", info.Untracked))
	}
	if len(parts) == 0 {
		return "工作区干净"
	}
	return strings.Join(parts, " · ")
}

// convGitMain reports whether a workspace is the main working tree of its
// project, for the hover card's worktree line.
func (cw *ChatWindow) convGitMain(ws string) bool {
	p := cw.convGitProject(ws)
	return p != nil && p.Main == ws
}

// bumpConvGit refreshes a workspace's cached summary in the background. The
// old value stays on screen until the reload lands, so grouping and the
// project headers never flicker: a caller that deleted the cache would make
// workspaceIsProject report false for a frame, and the worktree dialog it
// just opened would close again.
func (cw *ChatWindow) bumpConvGit(ws string) {
	if ws == "" {
		return
	}
	cw.convGit.mu.Lock()
	if cw.convGit.info == nil {
		cw.convGit.info = map[string]*convGitInfo{}
	}
	if cw.convGit.loading == nil {
		cw.convGit.loading = map[string]bool{}
	}
	if cw.convGit.loading[ws] {
		cw.convGit.mu.Unlock()
		return
	}
	cw.convGit.loading[ws] = true
	cw.convGit.mu.Unlock()
	go cw.loadConvGit(ws)
}
