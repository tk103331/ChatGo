package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

// gitApp is the App menu's Git client. Each tab is one changed file.
func gitApp() appDef {
	return appDef{
		ID:     "git",
		Title:  "Git 客户端",
		Desc:   "查看变更、对比 diff、暂存与提交，Agent 可代为执行 Git 操作",
		Icon:   "git-branch",
		System: "你是 Git 助手。你可以通过 git_status/git_diff/git_log/git_add/git_commit/git_branch 查看与操作项目仓库，帮助用户完成版本控制任务。",
		Tools: func(cw *ChatWindow) []appTool {
			return []appTool{
				{
					Name:   "git_status",
					Desc:   "查看仓库当前分支与工作区变更",
					Params: map[string]appParam{},
					Run: func(cw *ChatWindow, _ map[string]any) (string, error) {
						return cw.appGitStatus()
					},
				},
				{
					Name: "git_diff",
					Desc: "查看某个文件或全部变更的 diff",
					Params: map[string]appParam{
						"path": {Desc: "文件路径，留空查看全部"},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appGitDiff(argString(args, "path")), nil
					},
				},
				{
					Name:   "git_log",
					Desc:   "查看最近的提交记录",
					Params: map[string]appParam{},
					Run: func(cw *ChatWindow, _ map[string]any) (string, error) {
						return cw.appGitLog(), nil
					},
				},
				{
					Name: "git_add",
					Desc: "把文件添加到暂存区",
					Params: map[string]appParam{
						"path": {Desc: "文件路径，留空暂存全部"},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appGitAdd(argString(args, "path"))
					},
				},
				{
					Name: "git_commit",
					Desc: "用给定信息提交暂存区的变更",
					Params: map[string]appParam{
						"message": {Desc: "提交信息", Required: true},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appGitCommit(argString(args, "message"))
					},
				},
				{
					Name:   "git_branch",
					Desc:   "列出本地分支，当前分支以 * 标记",
					Params: map[string]appParam{},
					Run: func(cw *ChatWindow, _ map[string]any) (string, error) {
						return cw.appGitBranch(), nil
					},
				},
			}
		},
		NewTab: func(cw *ChatWindow) string {
			return cw.newGitFileTab()
		},
		TabLabel: func(cw *ChatWindow, tab string) string {
			if tab == "" {
				return "变更"
			}
			return filepath.Base(tab)
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.appGitBody(c, tab)
		},
	}
}

// newGitFileTab opens a tab for one changed file, or the first change.
func (cw *ChatWindow) newGitFileTab() string {
	changes := splitRaw(gitOut(cw.fileRoot(), "status", "--porcelain"))
	if len(changes) == 0 {
		return ""
	}
	return gitChangePath(changes[0])
}

// appGitBody renders the Git work area for a changed file.
func (cw *ChatWindow) appGitBody(c *mgo.Context, tab string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	root := cw.fileRoot()
	branch := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	changes := splitRaw(gitOut(root, "status", "--porcelain"))
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(u * 2).Gap(u * 2).Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "分支："+branch).FontWeight(600).Grow(1).SingleLine()
			if minput.Button(c, "刷新", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
				cw.invalidate()
			}
		})
		mgo.Text(c, fmt.Sprintf("变更 (%d)", len(changes))).FontWeight(600).FontSize(caption).TextColor(k.TextMuted)
		mgo.Scroll(c).MaxHeight(180).Children(func() {
			mgo.Column(c).Gap(0).Children(func() {
				if len(changes) == 0 {
					mgo.Text(c, "工作区干净").FontSize(caption).TextColor(k.TextMuted)
					return
				}
				for _, line := range changes {
					line := line
					p := gitChangePath(line)
					b := mgo.ButtonBase(c).Key("gitchange:"+p).FillWidth().
						MinHeight(mcore.Density(c).ControlHeight()).Padding(u, u*2).
						AlignItems(mgo.Center).Justify(mgo.Start).Radius(mtheme.ControlRadius).Label(line)
					if p == tab {
						b.Background(k.Selection)
					}
					b.Children(func() {
						mgo.Text(c, line).Font(mtheme.MonoFont).FontSize(caption).SingleLine().Ellipsis("…")
					})
					if b.Clicked() {
						cw.appTab["git"] = p
					}
				}
			})
		})
		mgo.Text(c, "Diff："+tab).FontWeight(600).FontSize(caption).TextColor(k.TextMuted)
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			diff := cw.appGitDiff(tab)
			if strings.TrimSpace(diff) == "" {
				mgo.Text(c, "(无差异)").Font(mtheme.MonoFont).FontSize(caption).TextColor(k.TextMuted)
				return
			}
			mgo.Text(c, diff).Font(mtheme.MonoFont).FontSize(caption)
		})
		commitMsg := cw.gitCommitMsg
		mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
			in := minput.TextInput(c, &commitMsg, minput.TextInputOptions{Label: "提交信息", Placeholder: "提交信息"})
			in.Element.Grow(1)
			cw.gitCommitMsg = commitMsg
			if minput.Button(c, "暂存", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				if _, err := cw.appGitAdd(tab); err != nil {
					cw.toast(err.Error(), mcore.SeverityError)
				}
			}
			if minput.Button(c, "提交", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if _, err := cw.appGitCommit(commitMsg); err != nil {
					cw.toast(err.Error(), mcore.SeverityError)
				} else {
					cw.gitCommitMsg = ""
				}
			}
		})
	})
}

// appGitStatus returns the branch and working-tree changes.
func (cw *ChatWindow) appGitStatus() (string, error) {
	root := cw.fileRoot()
	if gitOut(root, "rev-parse", "--is-inside-work-tree") != "true" {
		return "", fmt.Errorf("当前目录不是 Git 仓库")
	}
	branch := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	changes := splitRaw(gitOut(root, "status", "--porcelain"))
	var b strings.Builder
	fmt.Fprintf(&b, "分支：%s\n", branch)
	if len(changes) == 0 {
		b.WriteString("工作区干净")
		return b.String(), nil
	}
	b.WriteString("变更：\n")
	for _, ch := range changes {
		b.WriteString("  " + ch + "\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// appGitDiff returns the diff for a path, or for everything.
func (cw *ChatWindow) appGitDiff(path string) string {
	root := cw.fileRoot()
	args := []string{"diff", "--no-color"}
	if strings.TrimSpace(path) != "" {
		args = append(args, "--", path)
	}
	out := gitOut(root, args...)
	if strings.TrimSpace(out) == "" && strings.TrimSpace(path) != "" {
		out = gitOut(root, "diff", "--no-color", "--cached", "--", path)
	}
	return out
}

// appGitLog returns recent commits.
func (cw *ChatWindow) appGitLog() string {
	out := gitOut(cw.fileRoot(), "log", "--oneline", "-n", "20")
	if strings.TrimSpace(out) == "" {
		return "(没有提交记录)"
	}
	return out
}

// appGitAdd stages a path, or everything when empty.
func (cw *ChatWindow) appGitAdd(path string) (string, error) {
	root := cw.fileRoot()
	args := []string{"add", "--"}
	if strings.TrimSpace(path) == "" {
		args = []string{"add", "-A"}
	} else {
		args = append(args, path)
	}
	if out := gitOut(root, args...); out != "" {
		return out, nil
	}
	cw.invalidate()
	if strings.TrimSpace(path) == "" {
		return "已暂存全部变更", nil
	}
	return "已暂存 " + path, nil
}

// appGitCommit commits the staged changes.
func (cw *ChatWindow) appGitCommit(message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("提交信息不能为空")
	}
	out := gitOut(cw.fileRoot(), "commit", "-m", message)
	cw.invalidate()
	if strings.TrimSpace(out) == "" {
		return "提交完成", nil
	}
	return out, nil
}

// appGitBranch lists local branches.
func (cw *ChatWindow) appGitBranch() string {
	out := gitOut(cw.fileRoot(), "branch")
	if strings.TrimSpace(out) == "" {
		return "(没有分支)"
	}
	return out
}
