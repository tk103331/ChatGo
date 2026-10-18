package ui

import (
	"bytes"
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
	"chatgo/internal/ui/common"
	"chatgo/internal/jssandbox"

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
		return map[string]*schema.ParameterInfo{
			"action":   {Type: schema.String, Desc: "操作：open（打开网址）/ url（获取当前网址）/ title（获取页面标题）/ content（获取文本内容）/ html（获取HTML）/ click（点击元素）/ fill（填写表单）/ submit（提交表单）/ eval（执行JS）", Required: true},
			"url":      {Type: schema.String, Desc: "要打开的网址，action=open 时必填"},
			"selector": {Type: schema.String, Desc: "CSS 选择器，定位网页元素（content/html/click/fill/submit 时使用，content/html 可选）"},
			"value":    {Type: schema.String, Desc: "填写的值，action=fill 时必填"},
			"code":     {Type: schema.String, Desc: "JavaScript 代码，action=eval 时必填"},
		}
	case "deepseeksearch":
		return map[string]*schema.ParameterInfo{
			"action":       {Type: schema.String, Desc: "操作：search（搜索网页）/ fetch（抓取网页为 Markdown）", Required: true},
			"query":        {Type: schema.String, Desc: "搜索词，action=search 时必填，支持 site:/-site:/-/\"...\"/lang:/after:/before: 等搜索语法"},
			"url":          {Type: schema.String, Desc: "要抓取的网页地址，action=fetch 时必填"},
			"count":        {Type: schema.Integer, Desc: "返回条数，1～50，默认 10，action=search 时可选"},
			"ori_question": {Type: schema.String, Desc: "触发搜索的用户原始问题，action=search 时可选"},
			"need_content": {Type: schema.Boolean, Desc: "是否返回网页正文，默认 false，action=search 时可选"},
		}
	}
	return map[string]*schema.ParameterInfo{}
}

// builtinToolHandler returns the executor for a builtin tool, reading its
// runtime settings from the tool config.
func (cw *ChatWindow) builtinToolHandler(tool *config.BuiltinTool, root string) func(context.Context, string) (string, error) {
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
			out, err := runCommand(ctx, in.Command, allowed, root)
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
		timeout := configSeconds(tool.Config["timeout"], 30)
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				Action   string `json:"action"`
				URL      string `json:"url"`
				Selector string `json:"selector"`
				Value    string `json:"value"`
				Code     string `json:"code"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			return cw.webviewTool(ctx, in.Action, in.URL, in.Selector, in.Value, in.Code, timeout)
		}
	case "deepseeksearch":
		apiKey := strings.TrimSpace(tool.Config["api_key"])
		baseURL := strings.TrimSpace(tool.Config["base_url"])
		if baseURL == "" {
			baseURL = "https://api.deepseek.com"
		}
		defCount := 10
		if n, err := strconv.Atoi(strings.TrimSpace(tool.Config["count"])); err == nil && n > 0 {
			defCount = n
		}
		return func(ctx context.Context, args string) (string, error) {
			var in struct {
				Action      string `json:"action"`
				Query       string `json:"query"`
				URL         string `json:"url"`
				Count       int    `json:"count"`
				OriQuestion string `json:"ori_question"`
				NeedContent bool   `json:"need_content"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			return deepSeekTool(ctx, apiKey, baseURL, defCount, in.Action, in.Query, in.URL, in.Count, in.OriQuestion, in.NeedContent)
		}
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

// deepSeekSearchResult is one hit from the DeepSeek search API.
type deepSeekSearchResult struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	Content     string  `json:"content"`
	SiteName    *string `json:"site_name"`
	Date        *string `json:"date"`
}

// deepSeekFetchResponse is the payload the fetch API returns.
type deepSeekFetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// deepSeekTool runs the DeepSeek search or fetch API.
func deepSeekTool(ctx context.Context, apiKey, baseURL string, defCount int, action, query, url string, count int, oriQuestion string, needContent bool) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("deepseeksearch 需要 api_key")
	}
	action = strings.ToLower(strings.TrimSpace(action))
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var endpoint string
	var payload []byte
	var err error
	switch action {
	case "search", "":
		if strings.TrimSpace(query) == "" {
			return "", fmt.Errorf("action=search 需要参数 query")
		}
		if count <= 0 {
			count = defCount
		}
		payload, err = json.Marshal(map[string]any{
			"query":        query,
			"count":        count,
			"need_content": needContent,
		})
		if err != nil {
			return "", err
		}
		endpoint = baseURL + "/v1/tools/search"
	case "fetch":
		if strings.TrimSpace(url) == "" {
			return "", fmt.Errorf("action=fetch 需要参数 url")
		}
		payload, err = json.Marshal(map[string]any{"url": url})
		if err != nil {
			return "", err
		}
		endpoint = baseURL + "/v1/tools/fetch"
	default:
		return "", fmt.Errorf("未知 action %q（应为 search/fetch）", action)
	}
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepseek API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if action == "fetch" {
		var fr deepSeekFetchResponse
		if err := json.Unmarshal(data, &fr); err != nil {
			return "", fmt.Errorf("解析响应失败: %w", err)
		}
		return fr.Title + "\n\n" + fr.Content, nil
	}
	var sr struct {
		Results []deepSeekSearchResult `json:"results"`
	}
	if err := json.Unmarshal(data, &sr); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	if len(sr.Results) == 0 {
		return "无搜索结果", nil
	}
	var sb strings.Builder
	for i, r := range sr.Results {
		fmt.Fprintf(&sb, "## %d. %s\n", i+1, r.Title)
		fmt.Fprintf(&sb, "URL: %s\n", r.URL)
		if r.SiteName != nil {
			fmt.Fprintf(&sb, "来源: %s\n", *r.SiteName)
		}
		if r.Date != nil {
			fmt.Fprintf(&sb, "日期: %s\n", *r.Date)
		}
		fmt.Fprintf(&sb, "相关性: %.1f/3\n", r.RerankScore)
		if r.Summary != "" {
			fmt.Fprintf(&sb, "摘要: %s\n", r.Summary)
		} else if r.Snippet != "" {
			fmt.Fprintf(&sb, "摘要: %s\n", r.Snippet)
		}
		if r.Content != "" {
			fmt.Fprintf(&sb, "正文: %s\n", r.Content)
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

// runCommand runs a shell command when its program is in the allow list.
func runCommand(ctx context.Context, command string, allowed []string, dir string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	first := command
	if i := strings.IndexAny(first, " \t"); i >= 0 {
		first = first[:i]
	}
	if len(allowed) > 0 && !common.ContainsStr(allowed, "*") && !common.ContainsStr(allowed, filepath.Base(first)) {
		return "", fmt.Errorf("命令 %q 不在允许列表（%s）", first, strings.Join(allowed, ", "))
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
