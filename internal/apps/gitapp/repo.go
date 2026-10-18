package gitapp

import (
	"os"
	"strconv"
	"strings"

	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
)

// repoModel is one snapshot of a repository read from disk.
type repoModel struct {
	inside        bool
	branch        string
	detached      bool
	branches      []mgit.Branch
	changes       []mgit.ChangedFile
	commits       []mgit.Commit
	stashes       []mgit.Stash
	tags          []mgit.GitTag
	stat          [2]int
	conflicts     []string
	ahead, behind int
}

// readModel reads a repository; safe off the UI thread.
func readModel(root string) repoModel {
	var m repoModel
	if root == "" || gitOut(root, "rev-parse", "--is-inside-work-tree") != "true" {
		return m
	}
	m.inside = true
	m.branch = strings.TrimSpace(gitOut(root, "rev-parse", "--abbrev-ref", "HEAD"))
	m.detached = m.branch == "HEAD"
	m.branches = parseBranches(root, m.branch)
	m.changes = parseChanges(root)
	m.commits = parseCommits(root, 80)
	m.stashes = parseStashes(root)
	m.tags = parseTags(root)
	m.stat = parseStat(root)
	m.conflicts = parseConflicts(root)
	if up := gitOut(root, "rev-parse", "--abbrev-ref", "@{upstream}"); up != "" {
		m.ahead, m.behind = aheadBehind(root, "HEAD", up)
	}
	return m
}

// reload refreshes the client's model in the background.
func (cl *Client) reload() {
	g := cl.st
	if g.loading {
		return
	}
	g.loading, g.dirty = true, false
	root := g.root
	go func() {
		m := readModel(root)
		cl.host.Post(func() { cl.applyModel(g, root, m) })
	}()
}

// applyModel installs a snapshot and redraws.
func (cl *Client) applyModel(g *state, root string, m repoModel) {
	g.loading, g.loaded = false, true
	if !m.inside {
		g.repo = ""
		cl.host.Invalidate()
		return
	}
	g.repo, g.root = root, root
	if g.pickRoot == "" {
		g.pickRoot = root
	}
	cl.pushRecent(root)
	g.branch, g.detached = m.branch, m.detached
	g.branches, g.changes, g.commits = m.branches, m.changes, m.commits
	g.stashes, g.tags = m.stashes, m.tags
	g.stat, g.conflicts = m.stat, m.conflicts
	g.ahead, g.behind = m.ahead, m.behind
	cl.host.Invalidate()
}

// Refresh marks the model stale so the next frame reloads it.
func (cl *Client) Refresh() { cl.st.dirty = true }

// runAction runs a git command and reloads on success.
func (cl *Client) runAction(g *state, okMsg string, args ...string) {
	root := g.root
	go func() {
		out, err := gitRun(root, args...)
		cl.host.Post(func() {
			if err != nil {
				cl.host.Toast(strings.TrimSpace(out), SeverityError)
				return
			}
			if okMsg != "" {
				cl.host.Toast(okMsg, SeveritySuccess)
			}
			g.dirty = true
			cl.host.Invalidate()
		})
	}()
}

// commit commits the staged changes.
func (cl *Client) commit(g *state, amend bool) {
	message := mgit.CommitMessage{Title: strings.TrimSpace(g.commit.Title), Body: strings.TrimSpace(g.commit.Body)}
	title := message.Title
	g.commitBusy = true
	root := g.root
	go func() {
		out, err := gitRun(root, commitArgs(message, amend)...)
		cl.host.Post(func() {
			g.commitBusy = false
			if err != nil {
				cl.host.Toast(strings.TrimSpace(out), SeverityError)
			} else {
				_ = title
				g.commit = mgit.CommitMessage{}
				cl.host.Toast("提交完成", SeveritySuccess)
			}
			g.dirty = true
			cl.host.Invalidate()
		})
	}()
}

func commitArgs(m mgit.CommitMessage, amend bool) []string {
	args := []string{"commit"}
	if amend {
		args = append(args, "--amend")
	}
	args = append(args, "-m", m.Title)
	if m.Body != "" {
		args = append(args, "-m", m.Body)
	}
	return args
}

// checkout switches branches, asking first when the tree is dirty.
func (cl *Client) checkout(g *state, name string) {
	if name == "" || name == g.branch {
		return
	}
	if len(g.changes) > 0 {
		cl.host.Confirm("切换分支", "工作区有未提交的修改，切换可能失败或丢失。确定切换到「"+name+"」？", func() {
			cl.runAction(g, "已切换到 "+name, "checkout", name)
		})
		return
	}
	cl.runAction(g, "已切换到 "+name, "checkout", name)
}

// push pushes the current branch, setting upstream on first push.
func (cl *Client) push(g *state) {
	args := []string{"push"}
	if gitOut(g.root, "rev-parse", "--abbrev-ref", "@{upstream}") == "" {
		remotes := splitRaw(gitOut(g.root, "remote"))
		if len(remotes) == 0 {
			cl.host.Toast("仓库没有远端，无法推送", SeverityError)
			return
		}
		args = append(args, "-u", remotes[0], g.branch)
	}
	cl.runAction(g, "推送完成", args...)
}

// changesAction stages, unstages or discards a path.
func (cl *Client) changesAction(g *state, action, item string) {
	switch action {
	case "stage":
		cl.runAction(g, "", "add", "--", item)
	case "unstage":
		cl.runAction(g, "", "restore", "--staged", "--", item)
	case "stage_all":
		cl.runAction(g, "", "add", "-A")
	case "unstage_all":
		cl.runAction(g, "", "reset")
	case "discard":
		cl.host.Confirm("放弃修改", "确定丢弃「"+item+"」的未暂存修改？此操作不可撤销。", func() {
			cl.runAction(g, "已放弃修改", "restore", "--", item)
		})
	}
}

// branchAction handles a branch row's action.
func (cl *Client) branchAction(g *state, action, item string) {
	switch action {
	case "checkout":
		if isRemote(item) {
			cl.runAction(g, "已检出 "+item, "checkout", "--track", item)
			return
		}
		cl.checkout(g, item)
	case "merge":
		cl.runAction(g, "已合并 "+item, "merge", "--no-edit", item)
	case "delete":
		cl.host.Confirm("删除分支", "确定删除本地分支「"+item+"」？", func() {
			cl.runAction(g, "已删除 "+item, "branch", "-d", item)
		})
	}
}

// stashAction applies, pops or drops a stash.
func (cl *Client) stashAction(g *state, action, ref string) {
	switch action {
	case "apply":
		cl.runAction(g, "已应用 "+ref, "stash", "apply", ref)
	case "pop":
		cl.runAction(g, "已恢复 "+ref, "stash", "pop", ref)
	case "drop":
		cl.host.Confirm("删除贮藏", "确定删除「"+ref+"」？", func() {
			cl.runAction(g, "已删除 "+ref, "stash", "drop", ref)
		})
	}
}

// isRemote reports whether a branch name came from a remote.
func isRemote(name string) bool {
	if strings.HasPrefix(name, "origin/") {
		return true
	}
	i := strings.Index(name, "/")
	return i > 0 && !strings.HasPrefix(name, "refs/")
}

// parseBranches reads local and remote branches with upstream counts.
func parseBranches(root, current string) []mgit.Branch {
	var out []mgit.Branch
	for _, line := range splitRaw(gitOut(root, "for-each-ref", "--format=%(refname:short)%09%(upstream:short)", "refs/heads")) {
		parts := strings.SplitN(line, "\t", 2)
		b := mgit.Branch{Name: parts[0], Current: parts[0] == current}
		if len(parts) > 1 {
			b.Upstream = parts[1]
		}
		if b.Upstream != "" {
			b.Ahead, b.Behind = aheadBehind(root, b.Name, b.Upstream)
		}
		out = append(out, b)
	}
	for _, name := range splitRaw(gitOut(root, "for-each-ref", "--format=%(refname:short)", "refs/remotes")) {
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		out = append(out, mgit.Branch{Name: name, Remote: true})
	}
	return out
}

// parseStashes reads the stash list.
func parseStashes(root string) []mgit.Stash {
	var out []mgit.Stash
	for _, line := range splitRaw(gitOut(root, "stash", "list", "--format=%gd%x09%gs")) {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		st := mgit.Stash{Ref: parts[0], Message: parts[1]}
		if strings.HasPrefix(parts[1], "On ") {
			rest := strings.TrimPrefix(parts[1], "On ")
			if i := strings.Index(rest, ":"); i > 0 {
				st.Branch = rest[:i]
			}
		}
		if i := strings.LastIndex(parts[1], ": "); i >= 0 {
			st.Message = parts[1][i+2:]
		}
		out = append(out, st)
	}
	return out
}

// parseTags reads tags with their target commit.
func parseTags(root string) []mgit.GitTag {
	var out []mgit.GitTag
	for _, line := range splitRaw(gitOut(root, "for-each-ref", "--format=%(refname:short)%09%(objectname)%09%(contents:subject)", "refs/tags")) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		t := mgit.GitTag{Name: parts[0], Target: parts[1]}
		if len(parts) > 2 {
			t.Annotation = parts[2]
		}
		out = append(out, t)
	}
	return out
}

// parseConflicts lists files with merge conflicts.
func parseConflicts(root string) []string {
	var out []string
	for _, line := range splitRaw(gitOut(root, "diff", "--name-only", "--diff-filter=U")) {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// parseChanges reads the working tree and index changes.
func parseChanges(root string) []mgit.ChangedFile {
	var files []mgit.ChangedFile
	for _, line := range splitRaw(gitOut(root, "status", "--porcelain")) {
		if len(line) < 3 {
			continue
		}
		x, y := line[0], line[1]
		p := strings.TrimSpace(line[2:])
		if i := strings.LastIndex(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		p = strings.Trim(p, "\"")
		if p == "" {
			continue
		}
		status, staged := porcelainStatus(x, y)
		files = append(files, mgit.ChangedFile{Path: p, Status: status, Staged: staged})
	}
	return files
}

func porcelainStatus(x, y byte) (mgit.GitStatus, bool) {
	if x == '?' {
		return mgit.GitUntracked, false
	}
	staged := x != ' '
	code := y
	if staged {
		code = x
	}
	return gitStatusOf(code), staged
}

func gitStatusOf(code byte) mgit.GitStatus {
	switch code {
	case 'A':
		return mgit.GitAdded
	case 'D':
		return mgit.GitDeleted
	case 'R':
		return mgit.GitRenamed
	case 'C':
		return mgit.GitCopied
	case '?':
		return mgit.GitUntracked
	default:
		return mgit.GitModified
	}
}

// parseStat sums added and deleted lines against HEAD.
func parseStat(root string) [2]int {
	var stat [2]int
	for _, line := range splitRaw(gitOut(root, "diff", "HEAD", "--numstat")) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		add, _ := parseInt(parts[0])
		del, _ := parseInt(parts[1])
		stat[0] += add
		stat[1] += del
	}
	return stat
}

func aheadBehind(root, branch, upstream string) (int, int) {
	parts := strings.Fields(gitOut(root, "rev-list", "--left-right", "--count", branch+"..."+upstream))
	if len(parts) != 2 {
		return 0, 0
	}
	ahead, _ := parseInt(parts[0])
	behind, _ := parseInt(parts[1])
	return ahead, behind
}

// readWorkFile reads a work-tree file, capped so huge files stay responsive.
func readWorkFile(root, rel string) string {
	abs, ok := within(root, rel)
	if !ok {
		return ""
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	if len(data) > 2<<20 {
		return string(data[:2<<20])
	}
	return string(data)
}

// within joins rel to root, rejecting paths that escape it.
func within(root, rel string) (string, bool) {
	abs := root + string(os.PathSeparator) + rel
	if strings.HasPrefix(rel, "/") {
		return "", false
	}
	if strings.Contains(rel, "..") {
		return "", false
	}
	return abs, true
}

func extOf(p string) string {
	if i := strings.LastIndex(p, "."); i >= 0 && !strings.Contains(p[i:], "/") {
		return p[i:]
	}
	return ""
}

func parseInt(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}

// parseCommitFiles lists a commit's changed files and its line totals.
func parseCommitFiles(root, hash string) ([]mgit.ChangedFile, [2]int) {
	var files []mgit.ChangedFile
	var stat [2]int
	for _, line := range splitRaw(gitOut(root, "show", "--format=", "--name-status", hash)) {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || parts[0] == "" {
			continue
		}
		p := parts[len(parts)-1]
		files = append(files, mgit.ChangedFile{Path: p, Status: commitStatus(parts[0][0])})
	}
	for _, line := range splitRaw(gitOut(root, "show", "--format=", "--numstat", hash)) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		add, _ := parseInt(parts[0])
		del, _ := parseInt(parts[1])
		stat[0] += add
		stat[1] += del
	}
	return files, stat
}

// commitStatus maps a name-status letter to a file status.
func commitStatus(c byte) mgit.GitStatus {
	switch c {
	case 'A':
		return mgit.GitAdded
	case 'D':
		return mgit.GitDeleted
	case 'R':
		return mgit.GitRenamed
	case 'C':
		return mgit.GitCopied
	default:
		return mgit.GitModified
	}
}

// commitOldPath resolves the pre-rename path of a file in a commit.
func commitOldPath(root, hash, path string) string {
	for _, line := range splitRaw(gitOut(root, "show", "--format=", "--name-status", hash)) {
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 && parts[0][0] == 'R' && parts[2] == path {
			return parts[1]
		}
	}
	return path
}
