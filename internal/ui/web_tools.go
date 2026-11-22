package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"chatgo/internal/webui"
)

// fileReadLimit bounds what one file preview carries. A phone should not pull
// a whole build artifact over the relay.
const fileReadLimit = 1 << 20

// commandOutputLimit bounds one terminal command's returned output.
const commandOutputLimit = 256 << 10

// workspaceDir resolves dir to a real path inside one of the desktop's known
// workspaces. An empty dir means the first workspace.
func (r webRuntime) workspaceDir(dir string) (string, error) {
	cw := r.cw
	spaces := onMain(cw, func() []string { return cw.workspaces() })
	if strings.TrimSpace(dir) == "" {
		if len(spaces) == 0 {
			return "", fmt.Errorf("还没有可用项目")
		}
		dir = spaces[0]
	}
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("目录不存在：%s", dir)
	}
	for _, space := range spaces {
		root, err := filepath.Abs(space)
		if err != nil {
			continue
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if real == root || strings.HasPrefix(real, root+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("目录不在已知项目内：%s", dir)
}

// workspaceFile resolves path to a real file inside a workspace.
func (r webRuntime) workspaceFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("请选择文件")
	}
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("文件不存在：%s", path)
	}
	if _, err := r.workspaceDir(filepath.Dir(real)); err != nil {
		return "", err
	}
	return real, nil
}

// Files lists one workspace directory.
func (r webRuntime) Files(dir string) ([]webui.FileEntry, error) {
	root, err := r.workspaceDir(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("无法读取目录：%w", err)
	}
	out := make([]webui.FileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, webui.FileEntry{
			Name: entry.Name(), Path: filepath.Join(root, entry.Name()),
			Dir: entry.IsDir(), Size: info.Size(), Modified: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReadFile reads one text file, bounded.
func (r webRuntime) ReadFile(path string) (webui.FileContent, error) {
	abs, err := r.workspaceFile(path)
	if err != nil {
		return webui.FileContent{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return webui.FileContent{}, err
	}
	if info.IsDir() {
		return webui.FileContent{}, fmt.Errorf("这是一个目录：%s", filepath.Base(abs))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return webui.FileContent{}, err
	}
	out := webui.FileContent{Path: abs, Name: filepath.Base(abs), Size: int64(len(data))}
	if len(data) > fileReadLimit {
		data, out.Truncated = data[:fileReadLimit], true
	}
	if !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		out.Binary = true
		return out, nil
	}
	out.Content = string(data)
	return out, nil
}

// GitStatus reports a workspace's branch and changes.
func (r webRuntime) GitStatus(dir string) (webui.GitStatusInfo, error) {
	root, err := r.workspaceDir(dir)
	if err != nil {
		return webui.GitStatusInfo{}, err
	}
	if gitOut(root, "rev-parse", "--is-inside-work-tree") != "true" {
		return webui.GitStatusInfo{}, fmt.Errorf("当前目录不是 Git 仓库")
	}
	branch := gitOut(root, "rev-parse", "--abbrev-ref", "HEAD")
	if branch == "" {
		branch = "HEAD"
	}
	out := webui.GitStatusInfo{Branch: branch}
	for _, line := range splitRaw(gitOut(root, "status", "--porcelain")) {
		if len(line) < 4 {
			continue
		}
		status, path := line[:2], strings.TrimSpace(line[3:])
		if i := strings.Index(path, " -> "); i >= 0 {
			path = path[i+4:]
		}
		out.Changes = append(out.Changes, webui.GitChange{
			Path: path, Status: status,
			Staged: status[0] != ' ' && status[0] != '?',
		})
	}
	return out, nil
}

// GitDiff returns a path's diff, or the whole tree's, including an untracked
// file's contents so a new file is not an empty page.
func (r webRuntime) GitDiff(dir, path string) (string, error) {
	root, err := r.workspaceDir(dir)
	if err != nil {
		return "", err
	}
	args := []string{"diff", "--no-color"}
	cleanPath := strings.TrimSpace(path)
	if cleanPath != "" {
		args = append(args, "--", cleanPath)
	}
	out := gitOut(root, args...)
	if strings.TrimSpace(out) == "" && cleanPath != "" {
		out = gitOut(root, "diff", "--no-color", "--cached", "--", cleanPath)
	}
	if strings.TrimSpace(out) == "" && cleanPath != "" {
		abs, err := r.workspaceFile(filepath.Join(root, cleanPath))
		if err == nil {
			if data, err := os.ReadFile(abs); err == nil {
				if len(data) > fileReadLimit {
					data = data[:fileReadLimit]
				}
				if utf8.Valid(data) {
					out = "未跟踪文件：\n" + string(data)
				}
			}
		}
	}
	if strings.TrimSpace(out) == "" {
		out = "(没有差异)"
	}
	return out, nil
}

// GitLog lists recent commits.
func (r webRuntime) GitLog(dir string) ([]webui.GitCommitInfo, error) {
	root, err := r.workspaceDir(dir)
	if err != nil {
		return nil, err
	}
	out := gitOut(root, "log", "--pretty=format:%h%x09%an%x09%ad%x09%s", "--date=short", "-n", "20")
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var commits []webui.GitCommitInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, webui.GitCommitInfo{
			Hash: parts[0], Author: parts[1], Date: parts[2], Subject: parts[3],
		})
	}
	return commits, nil
}

// GitStage stages a path, or every change when path is empty.
func (r webRuntime) GitStage(dir, path string) error {
	root, err := r.workspaceDir(dir)
	if err != nil {
		return err
	}
	args := []string{"add", "--"}
	if strings.TrimSpace(path) == "" {
		args = []string{"add", "-A"}
	} else {
		args = append(args, strings.TrimSpace(path))
	}
	msg, err := gitRun(root, args...)
	if err != nil {
		return fmt.Errorf("%s", firstNonEmptyString(msg, err.Error()))
	}
	return nil
}

// GitCommit commits the staged changes.
func (r webRuntime) GitCommit(dir, message string) error {
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("提交信息不能为空")
	}
	root, err := r.workspaceDir(dir)
	if err != nil {
		return err
	}
	msg, err := gitRun(root, "commit", "-m", strings.TrimSpace(message))
	if err != nil {
		return fmt.Errorf("%s", firstNonEmptyString(msg, err.Error()))
	}
	return nil
}

// RunCommand runs one shell command in a workspace and caps what it returns.
func (r webRuntime) RunCommand(ctx context.Context, dir, command string) webui.CommandResult {
	start := time.Now()
	command = strings.TrimSpace(command)
	if command == "" {
		return webui.CommandResult{Output: "命令为空", ExitCode: 2}
	}
	root, err := r.workspaceDir(dir)
	if err != nil {
		return webui.CommandResult{Output: err.Error(), ExitCode: 1}
	}
	cmd := shellCommandContext(ctx, command, root)
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = commandOutputLimit, commandOutputLimit
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	out := strings.TrimSpace(stdout.String())
	if more := strings.TrimSpace(stderr.String()); more != "" {
		if out != "" {
			out += "\n"
		}
		out += more
	}
	exit := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = 1
			if out != "" {
				out += "\n"
			}
			out += runErr.Error()
		}
	}
	if out == "" {
		out = "(无输出)"
	}
	return webui.CommandResult{
		Output: out, ExitCode: exit, Truncated: stdout.truncated || stderr.truncated,
		Duration: time.Since(start).Milliseconds(),
	}
}

// CreateProject makes a directory under parent, initializes Git when asked,
// and registers it as a workspace.
func (r webRuntime) CreateProject(parent, name string, git bool) (string, error) {
	cw := r.cw
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("项目名称不合法")
	}
	parent = strings.TrimSpace(parent)
	if parent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		parent = home
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("父目录不存在：%s", parent)
	}
	// A phone may create under the user's home or an existing workspace only.
	allowed := false
	if home, err := os.UserHomeDir(); err == nil {
		if root, err := filepath.EvalSymlinks(home); err == nil {
			if real, err := filepath.EvalSymlinks(parent); err == nil {
				allowed = real == root || strings.HasPrefix(real, root+string(filepath.Separator))
			}
		}
	}
	if !allowed {
		spaces := onMain(cw, func() []string { return cw.workspaces() })
		for _, space := range spaces {
			root, err := filepath.EvalSymlinks(space)
			if err != nil {
				continue
			}
			real, err := filepath.EvalSymlinks(parent)
			if err == nil && (real == root || strings.HasPrefix(real, root+string(filepath.Separator))) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return "", fmt.Errorf("只能在用户目录或已有项目内新建项目")
	}
	path := filepath.Join(parent, name)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("目录已存在：%s", path)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		return "", fmt.Errorf("创建项目失败：%w", err)
	}
	if git {
		if msg, err := gitRun(path, "init"); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("初始化 Git 失败：%s", firstNonEmptyString(msg, err.Error()))
		}
	}
	onMain(cw, func() bool { cw.addWorkspace(path); return true })
	return path, nil
}

// cappedBuffer keeps at most limit bytes of a command's output.
type cappedBuffer struct {
	b         strings.Builder
	limit     int
	truncated bool
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	if w.b.Len() >= w.limit {
		w.truncated = true
		return len(p), nil
	}
	room := w.limit - w.b.Len()
	if len(p) > room {
		w.truncated = true
		p = p[:room]
	}
	w.b.Write(p)
	return len(p), nil
}

func (w *cappedBuffer) String() string { return w.b.String() }
