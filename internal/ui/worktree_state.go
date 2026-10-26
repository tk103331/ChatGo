// Worktree state: reading a repository's linked working trees. The actions
// that change them live in worktree_ops.go and the view that draws them in
// worktree.go, the way the Git graph keeps gitgraph_state.go and gitgraph.go
// apart.
package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// worktreeInfo is one linked working tree of a repository, as git worktree
// list reports it.
type worktreeInfo struct {
	// Path is the working tree's directory.
	Path string
	// Head is the commit it has checked out.
	Head string
	// Branch is the short branch name, empty on a detached HEAD.
	Branch string
	// Main marks the repository's own working tree, which cannot be removed.
	Main bool
	// Bare marks a bare repository, which has no files to open.
	Bare bool
	// Detached marks a working tree on no branch.
	Detached bool
	// Locked marks a working tree git refuses to prune; Reason is the note
	// given when it was locked, which is often empty.
	Locked bool
	Reason string
	// Prunable marks a working tree whose directory git can no longer find,
	// with Prune explaining why.
	Prunable bool
	Prune    string
}

// Title names a worktree in the list: its branch, or its short hash when
// detached.
func (w worktreeInfo) Title() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Branch != "":
		return w.Branch
	case w.Head != "":
		return shortHash(w.Head) + " (detached)"
	default:
		return filepath.Base(w.Path)
	}
}

// worktreeState is the Worktree tab's own view of a repository: the trees it
// found, the form that creates one, and the filter over the list.
type worktreeState struct {
	// dir is the repository the state was loaded for.
	dir string
	// trees are the working trees found, the main one first as git lists it.
	trees []worktreeInfo
	// branches are the repository's local branches, offered when creating a
	// working tree from an existing branch.
	branches []string
	// repo reports whether dir is inside a Git repository at all.
	repo   bool
	loaded bool

	// query filters the list by path or branch.
	query string

	// formOpen shows the create form; name is the directory name under the
	// worktree root and branch the branch to check out or create.
	formOpen bool
	name     string
	branch   string
	// newBranch creates branch rather than checking out an existing one.
	newBranch bool
	// busy blocks a second create while one runs.
	busy bool
}

// worktreeRoot is where ChatGo puts the working trees it creates:
// ~/.chatgo/worktrees. Keeping them outside the repository means they never
// show up in its status, and one place holds every tree the app made.
func worktreeRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "chatgo-worktrees")
	}
	return filepath.Join(home, ".chatgo", "worktrees")
}

// defaultWorktreePath is where a working tree named name of the repository at
// repo is created: ~/.chatgo/worktrees/<repo>/<name>, so trees of different
// repositories with the same branch name never collide.
func defaultWorktreePath(repo, name string) string {
	return filepath.Join(worktreeRoot(), worktreeSlug(filepath.Base(repo)), worktreeSlug(name))
}

// worktreeSlug makes a name safe as one path element: a branch like
// feature/login would otherwise create a nested directory.
func worktreeSlug(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == os.PathSeparator:
			b.WriteByte('-')
		case r < 0x20 || r == 0x7f:
			// Control characters have no place in a path.
		default:
			b.WriteRune(r)
		}
	}
	slug := strings.Trim(b.String(), ".-")
	if slug == "" {
		return "worktree"
	}
	return slug
}

// gitWorktrees reads a repository's linked working trees.
//
// The porcelain form prints one record per tree as `key value` fields, with
// valueless keys (bare, detached) standing alone and an empty field ending the
// record. It is read with -z, as git's own documentation recommends: with
// newline-terminated fields git C-quotes any lock reason holding non-ASCII,
// quotes or backslashes, and a path may itself contain a newline. Should -z be
// unavailable the newline form is unquoted instead.
func gitWorktrees(dir string) []worktreeInfo {
	if out := gitOut(dir, "worktree", "list", "--porcelain", "-z"); strings.Contains(out, "\x00") {
		return parseWorktrees(out, "\x00", false)
	}
	return parseWorktrees(gitOut(dir, "worktree", "list", "--porcelain"), "\n", true)
}

// parseWorktrees reads porcelain records whose fields are separated by sep,
// unquoting the values git may have C-quoted when asked.
func parseWorktrees(out, sep string, quoted bool) []worktreeInfo {
	if strings.TrimSpace(out) == "" {
		return nil
	}
	var (
		trees []worktreeInfo
		cur   worktreeInfo
		open  bool
	)
	flush := func() {
		if open && cur.Path != "" {
			// git lists the main working tree first, so the first record is
			// the repository's own.
			cur.Main = len(trees) == 0
			trees = append(trees, cur)
		}
		cur, open = worktreeInfo{}, false
	}
	for _, field := range strings.Split(out, sep) {
		if sep == "\n" {
			field = strings.TrimRight(field, "\r")
		}
		if strings.TrimSpace(field) == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(field, " ")
		if quoted {
			value = unquoteGitValue(value)
		}
		switch key {
		case "worktree":
			// A new record begins; the previous one is complete even when the
			// output lacked its empty separator.
			flush()
			cur.Path, open = value, true
		case "HEAD":
			cur.Head = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "locked":
			cur.Locked, cur.Reason = true, value
		case "prunable":
			cur.Prunable, cur.Prune = true, value
		}
	}
	flush()
	return trees
}

// unquoteGitValue undoes the C-style quoting git applies to a value holding
// non-ASCII, a quote or a backslash. Go's string literal syntax covers the
// same escapes, so an unquotable value is one git did not quote.
func unquoteGitValue(value string) string {
	if !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) || len(value) < 2 {
		return value
	}
	if out, err := strconv.Unquote(value); err == nil {
		return out
	}
	return value
}

// resetWorktreeState points the Worktree tab at dir, reloading when it
// changed.
func (cw *ChatWindow) resetWorktreeState(dir string) {
	if cw.wt.dir == dir {
		return
	}
	cw.wt = worktreeState{dir: dir}
	cw.loadWorktrees()
}

// loadWorktrees reads the repository's working trees in the background, so a
// slow repository never blocks a frame.
func (cw *ChatWindow) loadWorktrees() {
	dir := cw.wt.dir
	if dir == "" {
		return
	}
	cw.wt.loaded = false
	go func() {
		repo := gitOut(dir, "rev-parse", "--is-inside-work-tree") == "true"
		var (
			trees    []worktreeInfo
			branches []string
		)
		if repo {
			trees = gitWorktrees(dir)
			for _, b := range gitLocalBranches(dir, "") {
				branches = append(branches, b.Name)
			}
		}
		cw.postMain(func() {
			if cw.wt.dir != dir {
				return
			}
			cw.wt.repo, cw.wt.trees, cw.wt.branches = repo, trees, branches
			cw.wt.loaded = true
			cw.invalidate()
		})
	}()
}

// worktreeVisible is the list the tab shows, after filtering by path or
// branch.
func (cw *ChatWindow) worktreeVisible() []worktreeInfo {
	q := strings.ToLower(strings.TrimSpace(cw.wt.query))
	if q == "" {
		return cw.wt.trees
	}
	out := make([]worktreeInfo, 0, len(cw.wt.trees))
	for _, w := range cw.wt.trees {
		if strings.Contains(strings.ToLower(w.Path), q) ||
			strings.Contains(strings.ToLower(w.Branch), q) {
			out = append(out, w)
		}
	}
	return out
}

// worktreeBranchTaken reports whether a branch is already checked out in some
// working tree, which is what git refuses a second time.
func (cw *ChatWindow) worktreeBranchTaken(branch string) bool {
	for _, w := range cw.wt.trees {
		if w.Branch != "" && w.Branch == branch {
			return true
		}
	}
	return false
}
