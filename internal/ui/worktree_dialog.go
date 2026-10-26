// The conversation list's per-project Worktree dialog: creating a working
// tree of the project and opening one of its existing trees as the workspace.
// It is shown by the project header's git-fork button, and uses the same
// addWorktreeIn operation as the Worktree tab, only pointed at the project.
package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// workspaceIsProject reports whether a workspace is a Git repository, so its
// header carries the worktree button.
func (cw *ChatWindow) workspaceIsProject(ws string) bool {
	return cw.convGitProject(ws) != nil
}

// projectWorktreeDialog is the create-or-open worktree dialog of a project:
// a branch name makes a new tree, and the list below opens one that exists.
func (cw *ChatWindow) projectWorktreeDialog(c *mgo.Context, ws string, open *bool) {
	if !*open {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	branch := mgo.Local(c.Root(), "wtf:branch:"+ws, func() string { return "" })
	name := mgo.Local(c.Root(), "wtf:name:"+ws, func() string { return "" })
	newBranch := mgo.Local(c.Root(), "wtf:new:"+ws, func() bool { return false })
	kit.Dialog(c, open, kit.DialogOptions{
		Title: "Worktree", Width: 460,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				*open = false
			}
			if kit.Button(c, "创建", kit.ButtonOptions{
				Variant: kit.Primary, Icon: icons.Must("git-fork"),
				Disabled: strings.TrimSpace(*branch) == "",
			}).Clicked() {
				cw.addWorktreeIn(ws, *name, *branch, *newBranch)
				*branch, *name, *newBranch = "", "", false
				*open = false
			}
		},
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			mgo.Text(c, "新建工作树").FontWeight(600).Shrink(0)
			in := kit.TextInput(c, branch, kit.TextInputOptions{
				Label: "分支", Placeholder: "分支名",
			})
			in.Element.FillWidth()
			dir := kit.TextInput(c, name, kit.TextInputOptions{
				Label: "目录名", Placeholder: "留空则用分支名",
			})
			dir.Element.FillWidth()
			nb := *newBranch
			if mgo.Checkbox(c, &nb, "创建新分支").Changed() {
				*newBranch = nb
			}
			if target := cw.worktreeDialogTarget(ws, *name, *branch); target != "" {
				mgo.Text(c, "将创建于 "+target).Font(mtheme.MonoFont).FontSize(caption).
					TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(target)
			}
			mgo.Text(c, "打开已有工作树").FontWeight(600).Shrink(0)
			cw.worktreeOpenList(c, ws, open)
		})
	})
}

// worktreeDialogTarget is where the dialog would create its tree, or "" when
// no branch has been typed yet.
func (cw *ChatWindow) worktreeDialogTarget(ws, name, branch string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(branch)
	}
	if name == "" {
		return ""
	}
	return defaultWorktreePath(ws, name)
}

// worktreeOpenList lists a project's working trees, each with a button that
// opens it as the workspace.
func (cw *ChatWindow) worktreeOpenList(c *mgo.Context, ws string, dialogOpen *bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	p := cw.convGitProject(ws)
	if p == nil {
		mgo.Text(c, "正在读取项目的工作树…").FontSize(caption).TextColor(k.TextMuted)
		return
	}
	for _, path := range p.Worktrees {
		label := worktreeLabel(path)
		if path == p.Main {
			label = "主工作树 · " + label
		}
		mgo.Row(c).FillWidth().Padding(u, u*2).Gap(u).
			AlignItems(mgo.Center).Radius(mtheme.ControlRadius).Children(func() {
			icon := "git-fork"
			if path == p.Main {
				icon = "folder"
			}
			mgo.Icon(c, icons.Must(icon)).Size(14, 14).TextColor(k.Accent).Shrink(0)
			mgo.Text(c, label).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
			openBtn := kit.Button(c, "设为工作区", kit.ButtonOptions{
				Variant: kit.Secondary, Disabled: cw.workspace == path,
			})
			if openBtn.Clicked() {
				*dialogOpen = false
				cw.openWorktreeWorkspace(path)
			}
		})
	}
}
