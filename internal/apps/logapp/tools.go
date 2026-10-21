package logapp

import (
	"encoding/json"
	"fmt"
	"strings"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
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

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "open_log",
			Desc: "打开一个日志文件。mode 为 tail（只读最后 N 行，默认）或 full（完整加载，超大文件自动截断）",
			Params: map[string]Param{
				"path":  {Desc: "相对工作目录的日志文件路径", Required: true},
				"mode":  {Desc: "tail 或 full，默认 tail"},
				"lines": {Desc: "tail 模式读取的行数，默认 500"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolOpen(strArg(args, "path"), strArg(args, "mode"), strArg(args, "lines"))
			},
		},
		{
			Name: "read_log",
			Desc: "读取当前已加载的日志行，可按级别过滤、限制返回行数",
			Params: map[string]Param{
				"path":      {Desc: "日志文件路径，留空用当前标签"},
				"level":     {Desc: "过滤级别：debug/info/warn/error，留空返回全部"},
				"max_lines": {Desc: "最多返回行数，默认 200"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRead(strArg(args, "path"), strArg(args, "level"), strArg(args, "max_lines"))
			},
		},
		{
			Name: "follow_log",
			Desc: "开关跟踪模式：持续读取日志文件新增内容",
			Params: map[string]Param{
				"path": {Desc: "日志文件路径，留空用当前标签"},
				"on":   {Desc: "true 开启跟踪，false 关闭", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolFollow(strArg(args, "path"), strArg(args, "on"))
			},
		},
		{
			Name: "reload_log",
			Desc: "重新加载日志文件",
			Params: map[string]Param{
				"path":  {Desc: "日志文件路径，留空用当前标签"},
				"mode":  {Desc: "tail 或 full，留空沿用上次模式"},
				"lines": {Desc: "tail 模式行数"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolReload(strArg(args, "path"), strArg(args, "mode"), strArg(args, "lines"))
			},
		},
		{
			Name: "list_tabs",
			Desc: "列出当前打开的日志标签（id、路径、行数、是否跟踪）",
			Run: func(map[string]any) (string, error) {
				return cl.toolListTabs()
			},
		},
	}
}

// toolOpen opens a log file in a tab with the requested mode.
func (cl *Client) toolOpen(path, mode, lines string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("缺少必填参数 path")
	}
	n := parseTailN(lines)
	if strings.EqualFold(mode, "full") {
		n = maxLines
	}
	id := cl.Open(path)
	lf := cl.file(id)
	if lf == nil {
		return "", fmt.Errorf("打开 %s 失败", path)
	}
	if strings.EqualFold(mode, "full") {
		cl.loadFullFile(lf)
	} else {
		cl.loadTailFile(lf, n)
	}
	if lf.err != "" {
		return "", fmt.Errorf("加载 %s 失败: %s", path, lf.err)
	}
	cl.host.Post(func() {
		if cl.openHandler != nil {
			cl.openHandler(path)
		}
		cl.host.Invalidate()
	})
	lf2 := cl.file(id)
	return fmt.Sprintf("已打开 %s，%d 行", path, len(lf2.lines)), nil
}

// toolRead returns loaded lines, optionally filtered by level.
func (cl *Client) toolRead(path, level, maxLinesArg string) (string, error) {
	lf := cl.fileByPathOrActive(path)
	if lf == nil {
		return "", fmt.Errorf("未找到日志标签")
	}
	want := levelOf(level)
	maxN := 200
	if n := parseTailN(maxLinesArg); n > 0 && maxLinesArg != "" {
		maxN = n
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	var picked []mcode.LogLine
	for _, ln := range lf.lines {
		if want >= 0 && ln.Level != want {
			continue
		}
		picked = append(picked, ln)
	}
	total := len(picked)
	start := 0
	if len(picked) > maxN {
		start = len(picked) - maxN
	}
	picked = picked[start:]
	var b strings.Builder
	fmt.Fprintf(&b, "(共 %d 行，返回最后 %d 行)\n", total, len(picked))
	for _, ln := range picked {
		b.WriteString(ln.Text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolFollow starts or stops following a log file.
func (cl *Client) toolFollow(path, on string) (string, error) {
	lf := cl.fileByPathOrActive(path)
	if lf == nil {
		return "", fmt.Errorf("未找到日志标签")
	}
	enable := strings.EqualFold(on, "true")
	cl.mu.Lock()
	if enable {
		cl.startFollowLocked(lf)
	} else {
		cl.stopFollowLocked(lf)
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
	if enable {
		return "已开启跟踪 " + lf.path, nil
	}
	return "已关闭跟踪 " + lf.path, nil
}

// toolReload reloads a log file with the requested mode.
func (cl *Client) toolReload(path, mode, lines string) (string, error) {
	lf := cl.fileByPathOrActive(path)
	if lf == nil {
		return "", fmt.Errorf("未找到日志标签")
	}
	if strings.EqualFold(mode, "full") {
		cl.loadFullFile(lf)
	} else if n := parseTailN(lines); lines != "" {
		cl.loadTailFile(lf, n)
	} else {
		cl.reloadTail(lf)
	}
	lf2 := cl.file(cl.tabs.Active)
	if lf2 != nil && lf2.err != "" {
		return "", fmt.Errorf("重新加载失败: %s", lf2.err)
	}
	return fmt.Sprintf("已重新加载 %s，%d 行", lf.path, len(lf.lines)), nil
}

// toolListTabs lists the open log tabs.
func (cl *Client) toolListTabs() (string, error) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if len(cl.tabs.Items) == 0 {
		return "(没有打开的日志)", nil
	}
	var b strings.Builder
	for _, id := range cl.tabs.Items {
		lf := cl.files[id]
		if lf == nil {
			continue
		}
		name := "未命名"
		if lf.path != "" {
			name = lf.path
		}
		mark := ""
		if lf.follow {
			mark += "[跟踪中]"
		}
		if id == cl.tabs.Active {
			mark += "[当前]"
		}
		fmt.Fprintf(&b, "- %s: %s (%d 行)%s\n", id, name, len(lf.lines), mark)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// fileByPathOrActive resolves a logFile by path or the active tab.
func (cl *Client) fileByPathOrActive(path string) *logFile {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if path != "" {
		if lf, ok := cl.files[path]; ok {
			return lf
		}
	}
	return cl.files[cl.tabs.Active]
}

// levelOf maps a level name to a LogLevel, or -1 for no filter.
func levelOf(name string) mcode.LogLevel {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return mcode.LogDebug
	case "info":
		return mcode.LogInfo
	case "warn", "warning":
		return mcode.LogWarn
	case "error", "err":
		return mcode.LogError
	}
	return -1
}

// strArg reads a string argument.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), "\"")
		}
	}
	return ""
}
