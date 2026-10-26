// The Worktree tab: a repository's linked working trees, with the actions
// that create, open, lock and remove them. Its data lives in
// worktree_state.go.
package ui

import (
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// openWorktreeTab opens the repository's working trees as a center tab, at
// most one at a time. Whether the workspace is a repository is decided by the
// tab itself once its list loads, so a first open is never refused.
func (cw *ChatWindow) openWorktreeTab(pane int) {
	if t := cw.focusCenterTab("worktree"); t != nil {
		return
	}
	cw.openCenterTab(pane, &centerTab{
		ID: "worktree", Kind: centerWorktree, Title: "Worktree",
	})
	cw.resetWorktreeState(cw.worktreeRepoDir())
}

// worktreeRepoDir is the repository the Worktree tab manages. Working trees
// are listed per repository, not per directory, so opening the tab from a
// working tree shows the same list as opening it from the main one — but the
// workspace path is what the user picked, and git resolves the rest.
func (cw *ChatWindow) worktreeRepoDir() string {
	return cw.fileRoot()
}

// worktreeBody is the Worktree tab: the toolbar, the create form and the list
// of working trees.
func (cw *ChatWindow) worktreeBody(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.resetWorktreeState(cw.worktreeRepoDir())
	if !cw.wt.loaded {
		cw.centerLoading(c)
		return
	}
	if !cw.wt.repo {
		mgo.Column(c).Fill().Center().Gap(u).Children(func() {
			mgo.Icon(c, icons.Must("git-fork")).Size(32, 32).TextColor(k.TextMuted)
			mgo.Text(c, "当前工作区不是 Git 仓库").TextColor(k.TextMuted).FontSize(caption)
		})
		return
	}
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		cw.worktreeBar(c)
		if cw.wt.formOpen {
			cw.worktreeForm(c)
		}
		cw.worktreeSearch(c)
		cw.worktreeList(c)
	})
}

// worktreeBar is the tab's header: how many trees there are, and the actions
// that create one, clean up stale records and reload the list.
func (cw *ChatWindow) worktreeBar(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Gap(u).Children(func() {
		mgo.Icon(c, icons.Must("git-fork")).Size(16, 16).TextColor(k.Accent).Shrink(0)
		mgo.Text(c, itoa(len(cw.wt.trees))+" 个工作树").FontWeight(600).Shrink(0)
		mgo.Text(c, worktreeRoot()).Font(mtheme.MonoFont).FontSize(caption).
			TextColor(k.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).
			Tooltip("新建的工作树放在这个目录下")
		add := kit.Button(c, "新建工作树", kit.ButtonOptions{
			Variant: kit.Primary, Icon: icons.Must("plus"), Disabled: cw.wt.busy,
		})
		if add.Clicked() {
			cw.wt.formOpen = !cw.wt.formOpen
		}
		if kit.Button(c, "清理", kit.ButtonOptions{
			Variant: kit.Secondary, Icon: icons.Must("circle-x"),
		}).Tooltip("清理目录已不存在的工作树记录").Clicked() {
			cw.pruneWorktrees()
		}
		if kit.Button(c, "刷新", kit.ButtonOptions{
			Variant: kit.Ghost, Icon: icons.Must("refresh-cw"),
		}).Clicked() {
			cw.loadWorktrees()
		}
	})
}

// worktreeForm creates a working tree: a branch, whether to create it, and
// the directory name under the worktree root.
func (cw *ChatWindow) worktreeForm(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).FillWidth().Shrink(0).Gap(u).Padding(u*2).Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).Children(func() {
		mgo.Text(c, "新建工作树").FontWeight(600).Shrink(0)
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.End).Children(func() {
			// The branch is picked from a searchable dropdown that can also
			// create a new branch, so a name is never mistyped.
			sel := kit.BranchSelector(c, &cw.wt.branch, cw.wtBranchList(),
				kit.BranchSelectorOptions{AllowCreate: true, Disabled: cw.wt.busy, Key: "wtform"})
			if sel.Changed() {
				cw.wt.newBranch = false
			}
			if name, ok := sel.Created(); ok {
				cw.wt.branch = name
				cw.wt.newBranch = true
			}
			sel.Element.Grow(1).MinWidth(0)
			name := kit.TextInput(c, &cw.wt.name, kit.TextInputOptions{
				Label: "目录名", Placeholder: "留空则用分支名", Disabled: cw.wt.busy,
			})
			name.Element.Grow(1).MinWidth(0)
		})
		if target := cw.worktreeFormTarget(); target != "" {
			mgo.Text(c, "将创建于 "+target).Font(mtheme.MonoFont).FontSize(caption).
				TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(target)
		}
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Spacer(c)
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.wt.formOpen = false
			}
			create := kit.Button(c, "创建", kit.ButtonOptions{
				Variant: kit.Primary, Loading: cw.wt.busy,
				Disabled: cw.wt.busy || strings.TrimSpace(cw.wt.branch) == "",
			})
			if create.Clicked() {
				cw.addWorktree(cw.wt.name, cw.wt.branch, cw.wt.newBranch)
			}
		})
	})
}

// wtBranchList turns the Worktree tab's branch names into the selector's
// branch list.
func (cw *ChatWindow) wtBranchList() []kit.Branch {
	return branchList(cw.wt.branches)
}

// worktreeFormTarget is where the form would create its working tree, or ""
// when no branch has been typed yet.
func (cw *ChatWindow) worktreeFormTarget() string {
	name := strings.TrimSpace(cw.wt.name)
	if name == "" {
		name = strings.TrimSpace(cw.wt.branch)
	}
	if name == "" {
		return ""
	}
	return defaultWorktreePath(cw.wt.dir, name)
}

// worktreeSearch filters the list by path or branch.
func (cw *ChatWindow) worktreeSearch(c *mgo.Context) {
	if len(cw.wt.trees) < 2 {
		return
	}
	in := kit.SearchInput(c, &cw.wt.query, kit.SearchInputOptions{
		Placeholder: "搜索工作树路径或分支", Label: "搜索工作树",
	})
	in.Element.FillWidth().Shrink(0)
}

// worktreeList draws the filtered working trees, one card each.
func (cw *ChatWindow) worktreeList(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	trees := cw.worktreeVisible()
	if len(trees) == 0 {
		mgo.Column(c).Fill().Center().Gap(u).Children(func() {
			mgo.Icon(c, icons.Must("git-fork")).Size(28, 28).TextColor(k.TextMuted)
			hint := "没有工作树"
			if strings.TrimSpace(cw.wt.query) != "" {
				hint = "没有匹配的工作树"
			}
			mgo.Text(c, hint).TextColor(k.TextMuted).FontSize(caption)
		})
		return
	}
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Gap(u).Children(func() {
			for _, w := range trees {
				cw.worktreeCard(c, w)
			}
		})
	})
}

// worktreeCard is one working tree: what it has checked out, where it lives,
// and the actions that apply to it.
func (cw *ChatWindow) worktreeCard(c *mgo.Context, w worktreeInfo) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	current := cw.workspace == w.Path
	card := mgo.Column(c).FillWidth().Shrink(0).Gap(u).Padding(u * 2).
		Background(k.Surface).Radius(mtheme.CardRadius)
	border := k.Border
	if current {
		border = k.Accent
	}
	card.Border(mtheme.BorderWidth, border)
	card.Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			icon := "git-branch"
			if w.Detached || w.Bare {
				icon = "git-fork"
			}
			mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(k.Accent).Shrink(0)
			mgo.Text(c, w.Title()).FontWeight(600).SingleLine().Ellipsis("…").
				Shrink(1).MinWidth(0)
			cw.worktreeTags(c, w, current)
			mgo.Spacer(c)
			if w.Head != "" {
				mgo.Text(c, shortHash(w.Head)).Font(mtheme.MonoFont).FontSize(caption).
					TextColor(k.TextMuted).Shrink(0)
			}
		})
		mgo.Text(c, w.Path).Font(mtheme.MonoFont).FontSize(caption).
			TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(w.Path)
		if w.Locked && strings.TrimSpace(w.Reason) != "" {
			mgo.Text(c, "锁定原因："+w.Reason).FontSize(caption).
				TextColor(k.TextMuted).MaxLines(2)
		}
		if w.Prunable {
			mgo.Text(c, "已失效："+w.Prune).FontSize(caption).
				TextColor(k.Warning).MaxLines(2)
		}
		cw.worktreeActions(c, w, current)
	})
}

// worktreeTags marks a working tree's nature: the main one, the current
// workspace, a detached HEAD, a lock or a stale record.
func (cw *ChatWindow) worktreeTags(c *mgo.Context, w worktreeInfo, current bool) {
	add := func(text string, tone mgo.Color) {
		u := mcore.Density(c).Unit()
		mgo.Text(c, text).FontSize(mcore.FontSize(c, mtheme.CaptionSize-1)).
			TextColor(tone).Padding(0, u/2).Radius(mtheme.ControlRadius).
			Border(mtheme.BorderWidth, tone).SingleLine().Shrink(0)
	}
	k := mcore.Tokens(c)
	if w.Main {
		add("主工作树", k.TextMuted)
	}
	if current {
		add("当前工作区", k.Accent)
	}
	if w.Bare {
		add("bare", k.TextMuted)
	}
	if w.Detached {
		add("detached", k.Warning)
	}
	if w.Locked {
		add("已锁定", k.TextMuted)
	}
	if w.Prunable {
		add("可清理", k.Warning)
	}
}

// worktreeActions are a working tree's buttons: opening it as the workspace,
// a terminal in it, the lock toggle and the removal. The main working tree
// and a bare repository cannot be removed, which git enforces anyway.
func (cw *ChatWindow) worktreeActions(c *mgo.Context, w worktreeInfo, current bool) {
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Wrap().Children(func() {
		if !w.Bare {
			open := kit.Button(c, "设为工作区", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("folder-git-2"),
				Disabled: current || w.Prunable,
			})
			if open.Clicked() {
				cw.openWorktreeWorkspace(w.Path)
			}
			term := kit.Button(c, "终端", kit.ButtonOptions{
				Variant: kit.Ghost, Icon: icons.Must("terminal"), Disabled: w.Prunable,
			})
			if term.Clicked() {
				cw.openTerminalTabIn(cw.center.focus, w.Path)
			}
		}
		mgo.Spacer(c)
		if !w.Main {
			label, icon := "锁定", "lock"
			if w.Locked {
				label, icon = "解锁", "lock-open"
			}
			if kit.Button(c, label, kit.ButtonOptions{
				Variant: kit.Ghost, Icon: icons.Must(icon),
			}).Clicked() {
				cw.setWorktreeLock(w.Path, !w.Locked)
			}
			remove := kit.Button(c, "删除", kit.ButtonOptions{
				Variant: kit.Ghost, Icon: icons.Must("x"), Disabled: w.Locked,
			})
			if remove.Clicked() {
				cw.askRemoveWorktree(w)
			}
		}
	})
}

// askRemoveWorktree confirms a removal before running it, since a working
// tree holding uncommitted work is deleted outright.
func (cw *ChatWindow) askRemoveWorktree(w worktreeInfo) {
	desc := "确定删除工作树「" + filepath.Base(w.Path) + "」？目录 " + w.Path + " 会被移除。"
	if w.Prunable {
		desc = "确定清理失效的工作树记录「" + filepath.Base(w.Path) + "」？"
	}
	cw.askConfirm("删除工作树", desc, func() {
		// Force covers both a tree with uncommitted changes and a record
		// whose directory git can no longer find.
		cw.removeWorktree(w.Path, true)
	})
}

// worktreeSelectorVisible reports whether the Git panel shows the worktree
// selector. A repository with only its own working tree has nothing to switch
// between, so a plain repository's panel stays as it was.
func (cw *ChatWindow) worktreeSelectorVisible() bool {
	return cw.wt.loaded && cw.wt.repo && len(cw.wt.trees) > 1
}

// worktreeSelector picks the working tree the Git panel points at, switching
// the workspace to it.
//
// It is labelled by directory rather than by branch: the branch selector sits
// right beside it, and on the main working tree both would read the same.
func (cw *ChatWindow) worktreeSelector(c *mgo.Context) {
	cw.resetWorktreeState(cw.worktreeRepoDir())
	if !cw.worktreeSelectorVisible() {
		return
	}
	u := mcore.Density(c).Unit()
	trigger := kit.Button(c, worktreeLabel(cw.workspace), kit.ButtonOptions{
		Variant: kit.Secondary, Icon: icons.Must("git-fork"), Label: "切换工作树",
	})
	trigger.Tooltip("切换工作树")
	open := mgo.Local(trigger, "chatgo.worktree.panel", func() bool { return false })
	if trigger.Clicked() {
		*open = !*open
	}
	kit.Popover(c, trigger, open, kit.PopoverOptions{Title: "工作树", Width: 320}, func() {
		for _, w := range cw.wt.trees {
			w := w
			if w.Bare {
				continue
			}
			current := w.Path == cw.workspace
			label := worktreeLabel(w.Path) + " · " + w.Title()
			menuItem(c, iconLeading(c, "folder-git-2"), label, func() {
				*open = false
				if !current {
					cw.openWorktreeWorkspace(w.Path)
				}
			})
		}
		manage := kit.Button(c, "管理工作树", kit.ButtonOptions{
			Variant: kit.Ghost, Icon: icons.Must("git-fork"),
		})
		manage.AlignSelf(mgo.Start).Margin(u, 0, 0, 0)
		if manage.Clicked() {
			*open = false
			cw.openWorktreeTab(cw.center.focus)
		}
	})
}

// worktreeLabel names a working tree by its directory, which is what tells
// two trees of one repository apart.
func worktreeLabel(path string) string {
	if base := filepath.Base(path); base != "" && base != "." && base != string(filepath.Separator) {
		return base
	}
	return path
}
