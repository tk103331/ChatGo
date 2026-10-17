package gitapp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	mygo "github.com/egoist/mygo"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	mgit "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/git"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Pages of the client's left list.
const (
	PageChanges = iota
	PageHistory
	PageBranches
	PageTags
	PageRepo
)

// tab identifies one object in the viewer.
type tab struct {
	kind string
	key  string
}

// viewerState is one viewer tab's loaded diff.
type viewerState struct {
	from, to string
	viewer   mgit.DiffViewerState
	raw      string
	loaded   bool

	// commit view
	files      []mgit.ChangedFile
	stat       [2]int
	picked     int
	closedDirs map[string]bool
	fileView   mgit.DiffViewerState
}

// state is the client's retained model and widget state.
type state struct {
	repo     string
	root     string
	pinned   bool
	branch   string
	detached bool

	branches  []mgit.Branch
	changes   []mgit.ChangedFile
	commits   []mgit.Commit
	stashes   []mgit.Stash
	tags      []mgit.GitTag
	stat      [2]int
	conflicts []string
	ahead     int
	behind    int

	page    int
	tabs    []tab
	active  int
	dirty   bool
	loading bool
	loaded  bool

	branchList data.ListState[string]
	stashList  data.ListState[string]
	tagList    data.ListState[string]
	history    mgit.CommitGraphState
	changesUI  mgit.ChangesListState
	commit     mgit.CommitMessage
	commitBusy bool

	splitLeft   float32
	commitFileW float32
	sideBySide  bool

	pickRoot string
	recent   []string

	changeQuery string
	commitQuery string
	branchQuery string
	tagQuery    string
	authorQuery string
	timeQuery   string

	viewers    map[tab]*viewerState
	blames     map[tab]*mgit.BlameViewState
	blameCache map[tab][]mgit.BlameLine
}

// Client is one Git client bound to a Host.
type Client struct {
	host Host
	st   *state
}

// New returns a client driving the given host.
func New(h Host) *Client {
	return &Client{host: h, st: &state{
		splitLeft:   340,
		commitFileW: 240,
		viewers:     map[tab]*viewerState{},
		blames:      map[tab]*mgit.BlameViewState{},
		blameCache:  map[tab][]mgit.BlameLine{},
	}}
}

// View renders the client: header, list, viewer and details.
func (cl *Client) View(c *mgo.Context) {
	g := cl.st
	if g.root == "" {
		g.root = cl.host.Root()
		g.dirty = true
	}
	if !g.pinned && g.root != cl.host.Root() {
		g.root = cl.host.Root()
		g.repo = ""
		g.loaded = false
		g.dirty = true
	}
	if g.dirty {
		cl.reload()
	}
	k := mcore.Tokens(c)
	if g.loading && !g.loaded {
		mgo.Column(c).Fill().Center().Children(func() {
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 18, Label: "读取仓库"})
			mgo.Text(c, "正在读取 Git 信息…").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		})
		return
	}
	if g.repo == "" {
		cl.emptyRepo(c)
		return
	}
	mgo.Column(c).Fill().MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
		cl.header(c, g)
		mgo.Split(c, &g.splitLeft, func() { cl.listPane(c, g) }, func() { cl.viewerPane(c, g) }).Fill()
	})
}

// recentMax caps the recent repository list.
const recentMax = 10

// repoSelector shows the current repository in a dropdown of recent ones,
// with a button beside it to browse for another directory.
func (cl *Client) repoSelector(c *mgo.Context, g *state, u, caption float32, k mtheme.Tokens) {
	opts := make([]minput.SelectOption[string], 0, len(g.recent)+1)
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		opts = append(opts, minput.SelectOption[string]{Value: p, Label: repoLabel(p)})
	}
	add(g.root)
	for _, p := range g.recent {
		add(p)
	}
	if minput.Select(c, &g.pickRoot, opts, minput.SelectOptions{
		Label: "仓库目录", Placeholder: "选择仓库", Empty: "没有最近的仓库",
	}).Changed() {
		cl.SwitchRoot(g.pickRoot)
	}
	if minput.Button(c, "打开", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("folder")}).Clicked() {
		cl.browseRoot(g)
	}
}

// browseRoot asks the system for a directory, then switches to it.
func (cl *Client) browseRoot(g *state) {
	parent := cl.host.NativeWindow()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "选择 Git 仓库",
			ButtonLabel: "打开",
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		cl.host.Post(func() { cl.SwitchRoot(paths[0]) })
	}()
}

// SwitchRoot points the client at another directory and reloads.
func (cl *Client) SwitchRoot(dir string) {
	cl.st.pinned = true
	cl.st.root = dir
	cl.st.pickRoot = dir
	cl.pushRecent(dir)
	cl.st.repo = ""
	cl.st.loaded = false
	cl.st.dirty = true
	cl.host.Invalidate()
}

// pushRecent records a directory as the most recent repository.
func (cl *Client) pushRecent(dir string) {
	if dir == "" {
		return
	}
	out := []string{dir}
	for _, p := range cl.st.recent {
		if p != dir && len(out) < recentMax {
			out = append(out, p)
		}
	}
	cl.st.recent = out
}

// repoLabel names a repository by its folder and parent folder.
func repoLabel(p string) string {
	base := filepath.Base(p)
	parent := filepath.Base(filepath.Dir(p))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return base
	}
	return parent + "/" + base
}

// shortPath keeps the last n segments of a path for display.
func shortPath(p string, n int) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) <= n {
		return p
	}
	return "…/" + strings.Join(parts[len(parts)-n:], "/")
}

// emptyRepo explains how to start a repository.
func (cl *Client) emptyRepo(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Center().Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("git-branch")).Size(32, 32).TextColor(k.TextMuted)
		mgo.Text(c, "当前项目不是 Git 仓库").FontWeight(600)
		mgo.Text(c, cl.host.Root()).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u).Children(func() {
			if minput.Button(c, "初始化仓库", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if out, err := gitRun(cl.host.Root(), "init"); err != nil {
					cl.host.Toast(out, SeverityError)
				} else {
					cl.host.Toast("仓库已初始化", SeveritySuccess)
					cl.st.dirty = true
				}
			}
			if minput.Button(c, "刷新", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cl.st.dirty = true
			}
		})
	})
}

// header shows the repository, branch and global actions.
func (cl *Client) header(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u*2).AlignItems(mgo.Center).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		cl.repoSelector(c, g, u, caption, k)
		mgo.Icon(c, icons.Must("git-branch")).Size(16, 16).TextColor(k.AccentText)
		sel := mgit.BranchSelector(c, &g.branch, g.branches, mgit.BranchSelectorOptions{AllowCreate: true})
		if sel.Changed() {
			cl.checkout(g, g.branch)
		}
		if name, ok := sel.Created(); ok {
			cl.runAction(g, "已创建并切换到 "+name, "checkout", "-b", name)
		}
		if g.detached {
			mgo.Text(c, "已分离 HEAD").FontSize(caption).TextColor(k.Warning)
		}
		mgo.Text(c, fmt.Sprintf("↑%d ↓%d", g.ahead, g.behind)).
			FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		mgo.Spacer(c)
		if len(g.conflicts) > 0 {
			mgo.Text(c, fmt.Sprintf("%d 个冲突", len(g.conflicts))).FontSize(caption).TextColor(k.Danger)
		}
		if minput.Button(c, "拉取", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("download")}).Clicked() {
			cl.runAction(g, "拉取完成", "pull", "--ff-only")
		}
		if minput.Button(c, "推送", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("upload")}).Clicked() {
			cl.push(g)
		}
		if mcore.IconAction(c, icons.Must("refresh-cw"), "刷新").Clicked() {
			g.dirty = true
		}
	})
}

// listPane shows the page switcher and the active page.
func (cl *Client) listPane(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Background(k.Surface).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u / 2).Children(func() {
			for _, p := range []struct {
				id    int
				label string
				icon  string
			}{
				{PageChanges, "变更", "git-commit-horizontal"},
				{PageHistory, "历史", "history"},
				{PageBranches, "分支", "git-branch"},
				{PageTags, "标签", "tag"},
				{PageRepo, "仓库", "archive"},
			} {
				p := p
				variant := minput.Ghost
				if g.page == p.id {
					variant = minput.Secondary
				}
				if minput.Button(c, p.label, minput.ButtonOptions{Variant: variant, Label: p.label, Icon: icons.Must(p.icon)}).Clicked() {
					g.page = p.id
				}
			}
		})
		switch g.page {
		case PageHistory:
			cl.historyPane(c, g)
		case PageBranches:
			cl.branchesPane(c, g)
		case PageTags:
			cl.tagsPane(c, g)
		case PageRepo:
			cl.repoPane(c, g)
		default:
			cl.changesPane(c, g)
		}
	})
}

// changesPane lists changed files and the commit input.
func (cl *Client) changesPane(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(u).Padding(u * 2).Children(func() {
		mgo.Row(c).AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, "变更").FontWeight(600).Grow(1)
			mgit.DiffStat(c, g.stat[0], g.stat[1], mgit.DiffStatOptions{})
		})
		filterBox(c, &g.changeQuery, "按文件名过滤")
		visible := filterChanges(g.changes, g.changeQuery)
		if len(visible) == 0 && len(g.changes) > 0 {
			emptyHint(c, "没有匹配的文件")
		}
		changes := mgit.ChangesList(c, &g.changesUI, visible, mgit.ChangesListOptions{})
		changes.Element.Grow(1).MinHeight(0)
		if action, item, ok := changes.Action(); ok {
			cl.changesAction(g, action, item)
		}
		if changes.Submitted() {
			if i := g.changesUI.Selected(); i >= 0 && i < len(visible) {
				cl.openDiff(g, "work", visible[i].Path)
			}
		}
		if len(g.changes) > 0 {
			if minput.Button(c, "全部暂存", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cl.changesAction(g, "stage_all", "")
			}
		}
		mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
		input := mgit.CommitInput(c, &g.commit, mgit.CommitInputOptions{Busy: g.commitBusy})
		if committed, amended := input.Committed(), input.Amended(); committed || amended {
			cl.commit(g, amended)
		}
		if s := strings.TrimSpace(g.commit.Title); s != "" {
			mgo.Text(c, "提交说明："+s).FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…")
		}
	})
}

// historyPane shows the commit graph.
func (cl *Client) historyPane(c *mgo.Context, g *state) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u).Children(func() {
		mgo.Text(c, "最近提交").FontWeight(600)
		filterBox(c, &g.commitQuery, "按提交信息或哈希过滤")
		filterBox(c, &g.authorQuery, "提交人")
		filterBox(c, &g.timeQuery, "时间，如 2026-10-01 或 7d")
		visible := filterCommits(g.commits, g.commitQuery, g.authorQuery, g.timeQuery)
		if len(visible) == 0 && len(g.commits) > 0 {
			emptyHint(c, "没有匹配的提交")
		}
		graph := mgit.CommitGraph(c, &g.history, visible, mgit.CommitGraphOptions{TimeFormat: "01-02 15:04"})
		graph.Element.Grow(1).MinHeight(0)
		if graph.Submitted() {
			if hash, ok := g.history.List.Selected(); ok && hash != "" {
				cl.openDiff(g, "commit", hash)
			}
		}
	})
}

// branchesPane lists local and remote branches.
func (cl *Client) branchesPane(c *mgo.Context, g *state) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u).Children(func() {
		mgo.Text(c, "分支").FontWeight(600)
		filterBox(c, &g.branchQuery, "按分支名过滤")
		visible := filterBranches(g.branches, g.branchQuery)
		if len(visible) == 0 && len(g.branches) > 0 {
			emptyHint(c, "没有匹配的分支")
		}
		list := mgit.BranchList(c, &g.branchList, visible, mgit.BranchListOptions{
			Actions: []mgit.GitAction{
				{ID: "checkout", Label: "检出", Icon: "check"},
				{ID: "merge", Label: "合并到当前分支", Icon: "git-merge"},
				{ID: "delete", Label: "删除分支", Icon: "trash-2"},
			},
		})
		list.Element.Grow(1).MinHeight(0)
		if action, item, ok := list.Action(); ok {
			cl.branchAction(g, action, item)
		}
		if list.Submitted() {
			if name, ok := g.branchList.Selected(); ok {
				cl.checkout(g, name)
			}
		}
	})
}

// viewerPane shows the open diff and blame tabs.
func (cl *Client) viewerPane(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if len(g.tabs) == 0 {
		mgo.Column(c).Fill().Center().Children(func() {
			mgo.Text(c, "选择一项以查看差异").FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		})
		return
	}
	if g.active < 0 || g.active >= len(g.tabs) {
		g.active = 0
	}
	cur := g.tabs[g.active]
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			closing := -1
			for i, t := range g.tabs {
				i, t := i, t
				variant := minput.Ghost
				if i == g.active {
					variant = minput.Secondary
				}
				if minput.Button(c, t.key, minput.ButtonOptions{Variant: variant, Label: t.key}).Clicked() {
					g.active = i
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭标签").Clicked() {
					closing = i
				}
			}
			if closing >= 0 {
				g.tabs = append(g.tabs[:closing], g.tabs[closing+1:]...)
				if g.active >= len(g.tabs) {
					g.active = max(0, len(g.tabs)-1)
				}
			}
			mgo.Spacer(c)
			cl.splitToggle(c, g, cur)
		})
		mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
			if cur.kind == "blame" {
				cl.blameView(c, g, cur)
				return
			}
			cl.diffView(c, g, cur)
		})
	})
}

// commitView shows a commit as a file list beside the chosen file's diff.
func (cl *Client) commitView(c *mgo.Context, g *state, t tab, v *viewerState) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, fmt.Sprintf("%d 个文件", len(v.files))).FontSize(caption).TextColor(k.TextMuted).Grow(1)
			mgit.DiffStat(c, v.stat[0], v.stat[1], mgit.DiffStatOptions{})
		})
		mgo.Split(c, &g.commitFileW, func() {
			mgo.Scroll(c).Fill().MinWidth(0).Children(func() {
				mgo.Column(c).FillWidth().Gap(0).Children(func() {
					if len(v.files) == 0 {
						mgo.Text(c, "该提交没有文件改动").FontSize(caption).TextColor(k.TextMuted).Padding(u * 2)
						return
					}
					for _, row := range buildTree(v.files, v.closedDirs) {
						row := row
						if row.dir {
							cl.treeDirRow(c, g, v, row, u, caption, k)
							continue
						}
						cl.treeFileRow(c, g, t, v, row, u, caption, k)
					}
				})
			})
		}, func() {
			mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Padding(u, u*2, u*2, u*2).Children(func() {
				if v.picked < 0 || v.picked >= len(v.files) {
					mgo.Column(c).Fill().Center().Children(func() {
						mgo.Text(c, "选择左侧文件查看差异").FontSize(caption).TextColor(k.TextMuted)
					})
					return
				}
				mgit.DiffViewer(c, &v.fileView, v.from, v.to, mgit.DiffViewerOptions{
					Language:   cl.host.Language(extOf(v.files[v.picked].Path)),
					SideBySide: g.sideBySide,
				}).Fill()
			})
		}).Fill()
	})
}

// treeRow is one line of the file tree: a folder or a file.
type treeRow struct {
	name  string
	path  string
	index int
	depth int
	dir   bool
	count int
}

// treeNode nests changed files by directory.
type treeNode struct {
	name  string
	path  string
	files []int
	dirs  []*treeNode
}

// buildTree groups changed files by directory. Folders start open; closed
// names the ones the user collapsed.
func buildTree(files []mgit.ChangedFile, open map[string]bool) []treeRow {
	root := &treeNode{}
	for i, f := range files {
		parts := strings.Split(f.Path, "/")
		cur := root
		for d := 0; d < len(parts)-1; d++ {
			path := strings.Join(parts[:d+1], "/")
			next := cur.child(parts[d], path)
			cur = next
		}
		cur.files = append(cur.files, i)
	}
	sortNode(root)
	var out []treeRow
	walkNode(root, 0, false, open, files, &out)
	return out
}

// child returns the directory node, creating it when missing.
func (n *treeNode) child(name, path string) *treeNode {
	for _, d := range n.dirs {
		if d.path == path {
			return d
		}
	}
	d := &treeNode{name: name, path: path}
	n.dirs = append(n.dirs, d)
	return d
}

// sortNode orders folders before files, each by name.
func sortNode(n *treeNode) {
	sort.Slice(n.dirs, func(i, j int) bool { return n.dirs[i].name < n.dirs[j].name })
	for _, d := range n.dirs {
		sortNode(d)
	}
}

// walkNode flattens the tree, skipping the folders the user collapsed.
func walkNode(n *treeNode, depth int, isDir bool, open map[string]bool, files []mgit.ChangedFile, out *[]treeRow) {
	for _, d := range n.dirs {
		*out = append(*out, treeRow{
			name: d.name, path: d.path, depth: depth, dir: true, count: countFiles(d),
		})
		if !open[d.path] {
			walkNode(d, depth+1, true, open, files, out)
		}
	}
	for _, i := range n.files {
		*out = append(*out, treeRow{name: files[i].Path[strings.LastIndex(files[i].Path, "/")+1:], path: files[i].Path, index: i, depth: depth})
	}
}

// countFiles counts the files under a directory node.
func countFiles(n *treeNode) int {
	n2 := len(n.files)
	for _, d := range n.dirs {
		n2 += countFiles(d)
	}
	return n2
}

// treeDirRow draws a folder row that toggles its contents.
func (cl *Client) treeDirRow(c *mgo.Context, g *state, v *viewerState, row treeRow, u, caption float32, k mtheme.Tokens) {
	b := mgo.ButtonBase(c).Column().Key("cd:"+row.path).FillWidth().
		Padding(u, u*2+float32(row.depth)*u*2).AlignItems(mgo.Start).
		Radius(mtheme.ControlRadius).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			icon := "chevron-down"
			if v.closedDirs[row.path] {
				icon = "chevron-right"
			}
			mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
			mgo.Icon(c, icons.Must("folder")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, row.name).FontSize(caption).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			mgo.Text(c, fmt.Sprintf("%d", row.count)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})
	})
	clicked := b.Clicked()
	if clicked {
		if v.closedDirs[row.path] {
			delete(v.closedDirs, row.path)
		} else {
			v.closedDirs[row.path] = true
		}
	}
}

// treeFileRow draws a file row and opens it when clicked.
func (cl *Client) treeFileRow(c *mgo.Context, g *state, t tab, v *viewerState, row treeRow, u, caption float32, k mtheme.Tokens) {
	f := v.files[row.index]
	b := mgo.ButtonBase(c).Column().Key(fmt.Sprintf("cf:%d:%s", row.index, f.Path)).
		FillWidth().Padding(u, u*2+float32(row.depth)*u*2).AlignItems(mgo.Start).
		Radius(mtheme.ControlRadius)
	if row.index == v.picked {
		b.Background(k.Selection)
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgit.GitStatusBadge(c, f.Status, mgit.GitStatusBadgeOptions{})
			mgo.Text(c, row.name).Font(mtheme.MonoFont).FontSize(caption).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		})
	})
	if b.Clicked() {
		v.picked = row.index
		v.from, v.to = "", ""
		v.fileView = mgit.DiffViewerState{}
		go cl.loadCommitFile(g, t, v, f.Path)
	}
}

// loadCommitFile loads one file's two sides from a commit.
func (cl *Client) loadCommitFile(g *state, t tab, v *viewerState, path string) {
	root := g.root
	oldPath := commitOldPath(root, t.key, path)
	from := gitOut(root, "show", t.key+"^:"+oldPath)
	to := gitOut(root, "show", t.key+":"+path)
	cl.host.Post(func() {
		if path != v.files[v.picked].Path {
			return
		}
		v.from, v.to = from, to
		cl.host.Invalidate()
	})
}

// splitToggle switches a text diff between unified and side-by-side.
func (cl *Client) splitToggle(c *mgo.Context, g *state, cur tab) {
	if cur.kind == "commit" || cur.kind == "range" || cur.kind == "blame" {
		return
	}
	label := "并排"
	variant := minput.Ghost
	if g.sideBySide {
		variant = minput.Secondary
	}
	if minput.Button(c, label, minput.ButtonOptions{Variant: variant, Label: label}).Clicked() {
		g.sideBySide = !g.sideBySide
	}
}

// diffView renders a loaded diff with the DiffViewer component.
func (cl *Client) diffView(c *mgo.Context, g *state, t tab) {
	v := g.viewers[t]
	if v == nil {
		return
	}
	if !v.loaded {
		mgo.Row(c).Fill().Center().Children(func() {
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "读取差异"})
		})
		return
	}
	if t.kind == "commit" {
		cl.commitView(c, g, t, v)
		return
	}
	if t.kind == "range" {
		mgo.Scroll(c).Fill().Children(func() {
			mgo.Text(c, v.raw).Font(mtheme.MonoFont).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).Padding(mcore.Density(c).Unit() * 2)
		})
		return
	}
	mgit.DiffViewer(c, &v.viewer, v.from, v.to, mgit.DiffViewerOptions{
		Language:   cl.host.Language(extOf(t.key)),
		SideBySide: g.sideBySide,
	}).Fill()
}

// repoPane shows the repository root, stashes, tags and conflicts.
func (cl *Client) repoPane(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Scroll(c).Fill().MinWidth(0).MinHeight(0).Background(k.Surface).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).Children(func() {
		mgo.Column(c).FillWidth().Padding(u * 2).Gap(u * 2).Children(func() {
			mgo.Text(c, "仓库").FontWeight(600)
			cl.repoRow(c, "根目录", g.root)
			cl.repoRow(c, "分支", g.branch)
			cl.repoRow(c, "变更", fmt.Sprintf("%d 个文件", len(g.changes)))
			cl.repoRow(c, "提交", fmt.Sprintf("%d 条", len(g.commits)))
			cl.repoRow(c, "标签", fmt.Sprintf("%d 个", len(g.tags)))
			if g.ahead > 0 || g.behind > 0 {
				cl.repoRow(c, "远端", fmt.Sprintf("领先 %d，落后 %d", g.ahead, g.behind))
			}
			mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Background(k.Border)
			mgo.Row(c).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "暂存").FontWeight(600).Grow(1)
				if len(g.stashes) == 0 {
					mgo.Text(c, "无").FontSize(caption).TextColor(k.TextMuted)
				}
			})
			stash := mgit.StashList(c, &g.stashList, g.stashes, mgit.StashListOptions{})
			stash.Element.MinHeight(320).Grow(1)
			if action, item, ok := stash.Action(); ok {
				cl.stashAction(g, action, item)
			}
			if minput.Button(c, "贮藏当前修改", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("archive")}).Clicked() {
				cl.host.Prompt("贮藏修改", "贮藏说明", "", func(msg string) {
					args := []string{"stash", "push", "--include-untracked"}
					if msg != "" {
						args = append(args, "-m", msg)
					}
					cl.runAction(g, "已贮藏当前修改", args...)
				})
			}
			if len(g.conflicts) > 0 {
				mgo.Text(c, "冲突文件").FontWeight(600).TextColor(k.Danger)
				for _, p := range g.conflicts {
					p := p
					if minput.Button(c, p, minput.ButtonOptions{Variant: minput.Ghost, Label: p}).Clicked() {
						cl.host.OpenFile(filepath.Join(g.root, p))
					}
				}
			}
		})
	})
}

// tagsPane lists tags and creates new ones.
func (cl *Client) tagsPane(c *mgo.Context, g *state) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Background(k.Surface).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).
		Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "标签").FontWeight(600).Grow(1)
			if len(g.tags) == 0 {
				mgo.Text(c, "无").FontSize(caption).TextColor(k.TextMuted)
			}
		})
		filterBox(c, &g.tagQuery, "按标签名过滤")
		visible := filterTags(g.tags, g.tagQuery)
		if len(visible) == 0 && len(g.tags) > 0 {
			emptyHint(c, "没有匹配的标签")
		}
		tags := mgit.TagList(c, &g.tagList, visible, mgit.TagListOptions{})
		tags.Element.Grow(1).MinHeight(0)
		if action, target, ok := tags.Action(); ok && action == "jump" && target != "" {
			cl.openDiff(g, "commit", target)
		}
		if minput.Button(c, "新建标签", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("tag")}).Clicked() {
			cl.host.Prompt("新建标签", "标签名", "", func(name string) {
				if name == "" {
					return
				}
				cl.runAction(g, "已创建标签 "+name, "tag", "-a", name, "-m", name)
			})
		}
	})
}

// repoRow shows one labelled repository fact.
func (cl *Client) repoRow(c *mgo.Context, label, value string) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(mcore.Density(c).Unit() * 2).Children(func() {
		mgo.Text(c, label).FontSize(caption).TextColor(k.TextMuted).Width(64).Shrink(0)
		mgo.Text(c, value).FontSize(caption).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
	})
}

// openDiff opens a diff tab and loads it in the background.
func (cl *Client) openDiff(g *state, kind, key string) {
	t := tab{kind: kind, key: key}
	v, ok := g.viewers[t]
	if !ok {
		v = &viewerState{}
		g.viewers[t] = v
	}
	for i, existing := range g.tabs {
		if existing == t {
			g.active = i
			if !v.loaded {
				go cl.loadDiff(g, t, v)
			}
			return
		}
	}
	g.tabs = append(g.tabs, t)
	g.active = len(g.tabs) - 1
	if !v.loaded {
		go cl.loadDiff(g, t, v)
	}
}

// loadDiff fills a viewer tab's two sides off the UI thread.
func (cl *Client) loadDiff(g *state, t tab, v *viewerState) {
	root := g.root
	var from, to string
	switch t.kind {
	case "work":
		from = gitOut(root, "show", "HEAD:"+t.key)
		to = readWorkFile(root, t.key)
	case "staged":
		from = gitOut(root, "show", "HEAD:"+t.key)
		to = gitOut(root, "show", ":"+t.key)
	case "commit":
		v.files, v.stat = parseCommitFiles(root, t.key)
		v.picked = -1
		v.closedDirs = map[string]bool{}
		v.loaded = true
		if len(v.files) > 0 {
			v.picked = 0
			path := v.files[0].Path
			oldPath := commitOldPath(root, t.key, path)
			v.from = gitOut(root, "show", t.key+"^:"+oldPath)
			v.to = gitOut(root, "show", t.key+":"+path)
		}
		cl.host.Post(cl.host.Invalidate)
		return
	case "range":
		v.raw = gitOut(root, "diff", "--no-color", t.key)
		v.loaded = true
		cl.host.Post(cl.host.Invalidate)
		return
	}
	v.from, v.to = from, to
	v.loaded = true
	cl.host.Post(cl.host.Invalidate)
}

// openBlame opens a file's per-line history.
func (cl *Client) openBlame(g *state, path string) {
	t := tab{kind: "blame", key: path}
	if _, ok := g.blames[t]; !ok {
		g.blames[t] = &mgit.BlameViewState{}
		g.tabs = append(g.tabs, t)
		root := g.root
		go func() {
			lines := gitBlameLines(root, path)
			cl.host.Post(func() {
				g.blameCache[t] = lines
				cl.host.Invalidate()
			})
		}()
	}
	for i, existing := range g.tabs {
		if existing == t {
			g.active = i
		}
	}
}

// blameView renders a file's per-line history.
func (cl *Client) blameView(c *mgo.Context, g *state, t tab) {
	lines, ok := g.blameCache[t]
	b := g.blames[t]
	if b == nil {
		return
	}
	if !ok {
		mgo.Row(c).Fill().Center().Children(func() {
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 16, Label: "读取历史"})
		})
		return
	}
	mgit.BlameView(c, b, lines, mgit.BlameViewOptions{Language: cl.host.Language(extOf(t.key))}).Fill()
}

// OpenBlame shows a file's history tab from outside the client.
func (cl *Client) OpenBlame(path string) { cl.openBlame(cl.st, path) }

// SetPage switches the left list to a page.
func (cl *Client) SetPage(page int) { cl.st.page = page }

// InRepo reports whether the host's root is a Git repository.
func (cl *Client) InRepo() bool { return cl.st.repo != "" }
