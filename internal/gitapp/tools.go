package gitapp

import (
	"fmt"
	"strings"
)

// Param is one tool parameter.
type Param struct {
	Desc     string
	Required bool
}

// Tool is one operation exposed to the Agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// System is the app's chat system prompt.
const System = "你是 Git 助手。你可以通过 git_status/git_diff/git_log/git_add/git_commit/git_branch/git_stash 查看与操作项目仓库，帮助用户完成版本控制任务。执行破坏性操作前先向用户确认。"

// Tools returns the operations the Agent may run against the host's repo.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "git_status",
			Desc: "查看仓库当前分支与工作区变更",
			Run: func(map[string]any) (string, error) {
				return cl.status()
			},
		},
		{
			Name:   "git_diff",
			Desc:   "查看某个文件或全部变更的 diff",
			Params: map[string]Param{"path": {Desc: "文件路径，留空查看全部"}},
			Run: func(args map[string]any) (string, error) {
				return cl.diff(str(args, "path")), nil
			},
		},
		{
			Name: "git_log",
			Desc: "查看最近的提交记录",
			Run: func(map[string]any) (string, error) {
				return cl.log(), nil
			},
		},
		{
			Name:   "git_add",
			Desc:   "把文件添加到暂存区",
			Params: map[string]Param{"path": {Desc: "文件路径，留空暂存全部"}},
			Run: func(args map[string]any) (string, error) {
				return cl.add(str(args, "path"))
			},
		},
		{
			Name:   "git_commit",
			Desc:   "用给定信息提交暂存区的变更",
			Params: map[string]Param{"message": {Desc: "提交信息", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.commitMessage(str(args, "message"))
			},
		},
		{
			Name: "git_branch",
			Desc: "列出本地与远程分支，当前分支以 * 标记",
			Run: func(map[string]any) (string, error) {
				return cl.branchList(), nil
			},
		},
		{
			Name: "git_stash",
			Desc: "列出或操作贮藏；action 为 list/apply/pop/drop/save",
			Params: map[string]Param{
				"action": {Desc: "list、apply、pop、drop 或 save", Required: true},
				"ref":    {Desc: "贮藏引用，如 stash@{0}"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.stash(str(args, "action"), str(args, "ref"))
			},
		},
	}
}

func (cl *Client) root() string { return cl.host.Root() }

func (cl *Client) status() (string, error) {
	root := cl.root()
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

func (cl *Client) diff(path string) string {
	root := cl.root()
	args := []string{"diff", "--no-color"}
	if p := strings.TrimSpace(path); p != "" {
		args = append(args, "--", p)
	}
	out := gitOut(root, args...)
	if strings.TrimSpace(out) == "" && strings.TrimSpace(path) != "" {
		out = gitOut(root, "diff", "--no-color", "--cached", "--", path)
	}
	if strings.TrimSpace(out) == "" {
		return "(无差异)"
	}
	return out
}

func (cl *Client) log() string {
	if out := gitOut(cl.root(), "log", "--oneline", "-n", "20"); strings.TrimSpace(out) != "" {
		return out
	}
	return "(没有提交记录)"
}

func (cl *Client) add(path string) (string, error) {
	root := cl.root()
	var args []string
	if strings.TrimSpace(path) == "" {
		args = []string{"add", "-A"}
	} else {
		args = []string{"add", "--", path}
	}
	if out, err := gitRun(root, args...); err != nil {
		return out, err
	}
	if strings.TrimSpace(path) == "" {
		return "已暂存全部变更", nil
	}
	return "已暂存 " + path, nil
}

func (cl *Client) commitMessage(message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("提交信息不能为空")
	}
	out, err := gitRun(cl.root(), "commit", "-m", message)
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(out) == "" {
		return "提交完成", nil
	}
	return out, nil
}

func (cl *Client) branchList() string {
	if out := gitOut(cl.root(), "branch", "-a"); strings.TrimSpace(out) != "" {
		return out
	}
	return "(没有分支)"
}

func (cl *Client) stash(action, ref string) (string, error) {
	root := cl.root()
	switch action {
	case "list", "":
		if out := gitOut(root, "stash", "list"); strings.TrimSpace(out) != "" {
			return out, nil
		}
		return "(没有贮藏)", nil
	case "save":
		if out, err := gitRun(root, "stash", "push", "--include-untracked"); err != nil {
			return out, err
		}
		return "已贮藏当前修改", nil
	case "apply", "pop", "drop":
		if strings.TrimSpace(ref) == "" {
			return "", fmt.Errorf("需要提供贮藏引用")
		}
		out, err := gitRun(root, "stash", action, ref)
		if err != nil {
			return out, err
		}
		if strings.TrimSpace(out) == "" {
			return "已执行 stash " + action + " " + ref, nil
		}
		return out, nil
	default:
		return "", fmt.Errorf("不支持的 action: %s", action)
	}
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
