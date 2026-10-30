package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/jssandbox"
	"chatgo/pkg/models"

	"github.com/cloudwego/eino/schema"
)

// builtinToolParams returns the JSON-schema parameters a builtin tool accepts
// from the model.
func builtinToolParams(toolType string) map[string]*schema.ParameterInfo {
	switch toolType {
	case "httprequest":
		return map[string]*schema.ParameterInfo{
			"url":     {Type: schema.String, Desc: "请求 URL", Required: true},
			"method":  {Type: schema.String, Desc: "HTTP 方法，默认 GET"},
			"headers": {Type: schema.String, Desc: "请求头，JSON 对象字符串，可选"},
			"body":    {Type: schema.String, Desc: "请求体，可选"},
		}
	case "commandline":
		return map[string]*schema.ParameterInfo{
			"command": {Type: schema.String, Desc: "要执行的 shell 命令", Required: true},
		}
	case "filesystem":
		return map[string]*schema.ParameterInfo{
			"action":  {Type: schema.String, Desc: "操作：read（读）/ write（写）/ list（列出目录）", Required: true},
			"path":    {Type: schema.String, Desc: "相对工作目录的路径", Required: true},
			"content": {Type: schema.String, Desc: "写入的文本内容，action=write 时必填"},
			"offset":  {Type: schema.Integer, Desc: "读取起始行，从 0 开始，可选"},
			"limit":   {Type: schema.Integer, Desc: "读取行数，可选，默认 200"},
		}
	case "jssandbox":
		return map[string]*schema.ParameterInfo{
			"code":     {Type: schema.String, Desc: "JavaScript/TypeScript 代码，需 export 一个 run(input) 函数", Required: true},
			"input":    {Type: schema.String, Desc: "传入 run 函数的 JSON 输入，可选"},
			"function": {Type: schema.String, Desc: "要调用的导出函数名，默认 run"},
		}
	case "webview":
		return webviewParams()
	case "knowledge_search":
		return map[string]*schema.ParameterInfo{
			"query":          {Type: schema.String, Desc: "检索关键词或问题", Required: true},
			"knowledge_base": {Type: schema.String, Desc: "知识库名称，留空检索全部知识库"},
			"top_k":          {Type: schema.Integer, Desc: "返回片段数，默认 5，最多 20"},
		}
	case "update_plan":
		return map[string]*schema.ParameterInfo{
			"entries": {Type: schema.Array, Desc: "完整任务列表，每次提交全部条目：content（任务描述）、priority（high/medium/low）、status（pending/in_progress/completed）", Required: true},
		}
	case "ask_question":
		return map[string]*schema.ParameterInfo{
			"questions": {Type: schema.Array, Desc: "问题列表，每项含 question（问题文本）和 options（可选回答列表，留空则只有输入框）。一次问多个问题会用分页展示", Required: true},
		}
	}
	return map[string]*schema.ParameterInfo{}
}

// builtinToolHandler returns the executor for a builtin tool, reading its
// runtime settings from the tool config.
func (cw *ChatWindow) builtinToolHandler(tool *config.BuiltinTool, root string, env *builtinToolEnv, conv ...*models.Conversation) func(context.Context, string) (string, error) {
	params := builtinToolParams(tool.Type)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	log.Printf("[builtin] %s exposed params=%v", tool.Type, keys)
	switch tool.Type {
	case "httprequest":
		timeout := configSeconds(tool.Config["timeout"], 30)
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				URL     string `json:"url"`
				Method  string `json:"method"`
				Headers string `json:"headers"`
				Body    string `json:"body"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if strings.TrimSpace(in.URL) == "" {
				return "", fmt.Errorf("缺少必填参数 url")
			}
			return httpRequest(ctx, in.Method, in.URL, in.Headers, in.Body, timeout)
		}
	case "commandline":
		allowed := splitList(tool.Config["allowed_commands"])
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				Command string `json:"command"`
			}
			log.Printf("[builtin] commandline args=%q allowed=%v", args, allowed)
			_ = json.Unmarshal([]byte(args), &in)
			if in.Command == "" {
				in.Command = strings.TrimSpace(args)
			}
			cmd := in.Command
			// The allow list gates the command the model asked for, before
			// any rewrite: rtk turns it into "rtk <cmd>", whose first word
			// would not be in the list.
			if err := checkCommandAllowed(cmd, allowed); err != nil {
				return "", err
			}
			if cw.cfg.RTKEnabled {
				if rewritten, blocked := cw.rtkRewrite(cmd); blocked {
					return "RTK 已阻止该命令：" + rewritten, nil
				} else if rewritten != "" {
					cmd = rewritten
				}
			}
			log.Printf("[builtin] commandline command=%q rewritten=%q", in.Command, cmd)
			out, err := runCommand(ctx, cmd, nil, root)
			log.Printf("[builtin] commandline command=%q out=%q err=%v", in.Command, out, err)
			return out, err
		}
	case "filesystem":
		return func(_ context.Context, args string) (string, error) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(args), &raw); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			action := strVal(raw["action"])
			path := strVal(raw["path"])
			if path == "" {
				return "", fmt.Errorf("缺少必填参数 path")
			}
			abs, err := withinDir(root, path)
			if err != nil {
				return "", err
			}
			switch strings.ToLower(strings.TrimSpace(action)) {
			case "read":
				data, err := os.ReadFile(abs)
				if err != nil {
					return "", err
				}
				lines := strings.Split(string(data), "\n")
				offset := intVal(raw["offset"], 0)
				limit := intVal(raw["limit"], 200)
				if offset < 0 {
					offset = 0
				}
				if limit <= 0 {
					limit = 200
				}
				if offset > len(lines) {
					offset = len(lines)
				}
				end := offset + limit
				if end > len(lines) {
					end = len(lines)
				}
				return strings.Join(lines[offset:end], "\n"), nil
			case "write":
				if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
					return "", err
				}
				if err := os.WriteFile(abs, []byte(strVal(raw["content"])), 0o644); err != nil {
					return "", err
				}
				return "已写入 " + abs, nil
			case "list":
				entries, err := os.ReadDir(abs)
				if err != nil {
					return "", err
				}
				var names []string
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name += "/"
					}
					names = append(names, name)
				}
				sort.Strings(names)
				if len(names) == 0 {
					return "(空目录)", nil
				}
				return strings.Join(names, "\n"), nil
			}
			return "", fmt.Errorf("未知 action %q（应为 read/write/list）", action)
		}
	case "jssandbox":
		scfg := jssandbox.ParseConfig(tool.Config)
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				Code     string `json:"code"`
				Input    string `json:"input"`
				Function string `json:"function"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if strings.TrimSpace(in.Code) == "" {
				return "", fmt.Errorf("缺少必填参数 code")
			}
			return jssandbox.Run(ctx, in.Code, in.Input, in.Function, scfg)
		}
	case "webview":
		return cw.webviewTool
	case "knowledge_search":
		return cw.knowledgeSearchTool(tool.Config)
	case "ask_question":
		return cw.askQuestionTool(env, conv...)
	case "update_plan":
		var target *models.Conversation
		if len(conv) > 0 {
			target = conv[0]
		}
		return cw.updatePlanTool(target)
	}
	return func(context.Context, string) (string, error) {
		return "工具 " + tool.Type + " 暂未实现", nil
	}
}

// fileRoot is the directory file tools are confined to: the active workspace,
// or the process working directory when none is set.
func (cw *ChatWindow) fileRoot() string {
	if cw.workspace != "" {
		return cw.workspace
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// withinDir joins rel to root and rejects paths that escape it.
func withinDir(root, rel string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越权: %s", rel)
	}
	return abs, nil
}

// strVal coerces a decoded JSON value to a string.
func strVal(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// intVal coerces a decoded JSON value to an int, returning def otherwise.
func intVal(v any, def int) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
	}
	return def
}

// httpRequest performs an HTTP request and returns the status and body.
func httpRequest(ctx context.Context, method, url, headers, body string, timeout time.Duration) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	var rdr io.Reader
	if strings.TrimSpace(body) != "" {
		rdr = strings.NewReader(body)
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, method, url, rdr)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(headers) != "" {
		var h map[string]string
		if err := json.Unmarshal([]byte(headers), &h); err == nil {
			for k, v := range h {
				req.Header.Set(k, v)
			}
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, strings.TrimSpace(string(data))), nil
}

// checkCommandAllowed rejects a command whose program is not in the allow
// list, or passes it when the list is empty, wildcarded, or the program has
// no base name.
func checkCommandAllowed(command string, allowed []string) error {
	if len(allowed) == 0 || containsStr(allowed, "*") {
		return nil
	}
	first := command
	if i := strings.IndexAny(first, " \t"); i >= 0 {
		first = first[:i]
	}
	if !containsStr(allowed, filepath.Base(first)) {
		return fmt.Errorf("命令 %q 不在允许列表（%s）", first, strings.Join(allowed, ", "))
	}
	return nil
}

// runCommand runs a shell command when its program is in the allow list.
func runCommand(ctx context.Context, command string, allowed []string, dir string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	if err := checkCommandAllowed(command, allowed); err != nil {
		return "", err
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c := exec.CommandContext(cctx, "sh", "-c", command)
	if dir != "" {
		c.Dir = dir
	}
	out, err := c.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		return result + "\n" + err.Error(), nil
	}
	if result == "" {
		result = "(无输出)"
	}
	return result, nil
}

// configSeconds parses a seconds config value with a fallback default.
func configSeconds(v string, def int) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return time.Duration(def) * time.Second
}

// splitList splits a comma-separated config value into trimmed items.
func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// updatePlanTool records the agent's task list on the conversation, so the
// plan panel above the composer reflects it. The agent sends the whole list
// each time, matching the ACP rule: a plan update replaces the old one.
func (cw *ChatWindow) updatePlanTool(target *models.Conversation) func(context.Context, string) (string, error) {
	return func(_ context.Context, args string) (string, error) {
		var in struct {
			Entries []struct {
				Content  string `json:"content"`
				Priority string `json:"priority"`
				Status   string `json:"status"`
			} `json:"entries"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
		plan := make([]models.PlanEntry, 0, len(in.Entries))
		for _, e := range in.Entries {
			content := strings.TrimSpace(e.Content)
			if content == "" {
				continue
			}
			status := strings.ToLower(strings.TrimSpace(e.Status))
			switch status {
			case models.PlanPending, models.PlanInProgress, models.PlanCompleted:
			default:
				status = models.PlanPending
			}
			plan = append(plan, models.PlanEntry{
				Content:  content,
				Priority: strings.ToLower(strings.TrimSpace(e.Priority)),
				Status:   status,
			})
		}
		// The tool may have been built before a conversation was open, so the
		// captured target can be nil; resolve the current one at call time.
		done := make(chan struct{})
		var saved bool
		cw.postMain(func() {
			defer close(done)
			conv := target
			if conv == nil {
				conv = cw.cur
			}
			if conv == nil {
				return
			}
			conv.Plan = plan
			_ = cw.convManager.SaveConversation(conv)
			saved = true
			cw.invalidate()
		})
		<-done
		if !saved {
			return "计划已记录（当前上下文不保存计划）", nil
		}
		doneN := 0
		for _, e := range plan {
			if e.Status == models.PlanCompleted {
				doneN++
			}
		}
		return fmt.Sprintf("计划已更新：%d/%d 项完成", doneN, len(plan)), nil
	}
}
