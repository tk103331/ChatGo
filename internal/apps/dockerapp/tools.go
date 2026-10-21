package dockerapp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
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
			Name: "docker_list",
			Desc: "列出 Docker 资源：containers（容器，默认）、images（镜像）、volumes（卷）、networks（网络）",
			Params: map[string]Param{
				"kind": {Desc: "containers/images/volumes/networks，默认 containers"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolList(strArg(args, "kind"))
			},
		},
		{
			Name: "docker_action",
			Desc: "对容器执行操作：start/stop/restart/pause/unpause/remove",
			Params: map[string]Param{
				"action": {Desc: "start/stop/restart/pause/unpause/remove", Required: true},
				"id":     {Desc: "容器 ID 或名称", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolAction(strArg(args, "action"), strArg(args, "id"))
			},
		},
		{
			Name: "docker_logs",
			Desc: "查看容器日志。follow=true 时在界面开启跟踪并立即返回",
			Params: map[string]Param{
				"id":     {Desc: "容器 ID 或名称", Required: true},
				"tail":   {Desc: "读取末尾行数，默认 200"},
				"follow": {Desc: "true 开启跟踪模式"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolLogs(strArg(args, "id"), strArg(args, "tail"), strArg(args, "follow"))
			},
		},
		{
			Name: "docker_run",
			Desc: "从镜像创建并启动一个新容器",
			Params: map[string]Param{
				"image":  {Desc: "镜像名，如 nginx:latest", Required: true},
				"name":   {Desc: "容器名称，可省略"},
				"ports":  {Desc: "端口映射数组，如 [\"8080:80\"]"},
				"detach": {Desc: "true 后台运行（默认），false 则等待结束并返回日志"},
				"cmd":    {Desc: "容器内命令数组，可省略"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolRun(strArg(args, "image"), strArg(args, "name"),
					listArg(args, "ports"), strArg(args, "detach"), listArg(args, "cmd"))
			},
		},
		{
			Name: "docker_exec",
			Desc: "在运行中的容器内执行命令，返回合并输出",
			Params: map[string]Param{
				"id":  {Desc: "容器 ID 或名称", Required: true},
				"cmd": {Desc: "命令数组，如 [\"ls\",\"-la\"]", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolExec(strArg(args, "id"), listArg(args, "cmd"))
			},
		},
		{
			Name: "docker_prune",
			Desc: "清理未使用资源：system/image/container/volume",
			Params: map[string]Param{
				"scope": {Desc: "system/image/container/volume", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolPrune(strArg(args, "scope"))
			},
		},
		{
			Name: "docker_info",
			Desc: "查看 Docker 系统信息（info）或磁盘使用（df）",
			Params: map[string]Param{
				"kind": {Desc: "info 或 df，默认 info"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolInfo(strArg(args, "kind"))
			},
		},
	}
}

// toolList lists a kind of Docker resource.
func (cl *Client) toolList(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "containers", "container":
		rows, err := listContainers()
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "(没有容器)", nil
		}
		var b strings.Builder
		for _, ct := range rows {
			fmt.Fprintf(&b, "- %s  %s  %s  %s  %s\n", ct.ShortID(), ct.Name(), ct.Image, ct.State, ct.Status)
		}
		return strings.TrimRight(b.String(), "\n"), nil
	case "images", "image":
		rows, err := listImages()
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "(没有镜像)", nil
		}
		var b strings.Builder
		for _, im := range rows {
			fmt.Fprintf(&b, "- %s  %s  %s\n", im.ShortID(), im.Tag(), humanSize(im.Size))
		}
		return strings.TrimRight(b.String(), "\n"), nil
	case "volumes", "volume":
		rows, err := listVolumes()
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "(没有卷)", nil
		}
		var b strings.Builder
		for _, v := range rows {
			fmt.Fprintf(&b, "- %s  %s\n", v.Name, v.Driver)
		}
		return strings.TrimRight(b.String(), "\n"), nil
	case "networks", "network":
		rows, err := listNetworks()
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "(没有网络)", nil
		}
		var b strings.Builder
		for _, n := range rows {
			fmt.Fprintf(&b, "- %s  %s  %s  %s\n", n.ShortID(), n.Name, n.Driver, n.Scope)
		}
		return strings.TrimRight(b.String(), "\n"), nil
	}
	return "", fmt.Errorf("未知资源类型 %q", kind)
}

// toolAction performs a container action.
func (cl *Client) toolAction(action, id string) (string, error) {
	if action == "" || id == "" {
		return "", fmt.Errorf("缺少必填参数 action 或 id")
	}
	msg, err := containerAction(action, id)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast(actionName(action)+" "+shortID(id), mcore.SeveritySuccess)
		cl.loadPage(pageContainers)
	})
	return msg + " " + shortID(id), nil
}

// toolLogs reads container logs or starts following them.
func (cl *Client) toolLogs(id, tail, follow string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("缺少必填参数 id")
	}
	n := logTailDefault
	if v, err := strconv.Atoi(strings.TrimSpace(tail)); err == nil && v > 0 {
		n = v
	}
	if strings.EqualFold(follow, "true") {
		cl.openDetail(shortID(id), tabLogs)
		cl.mu.Lock()
		cl.st.logLines = nil
		cl.st.logState = mcodeLogViewerState()
		cl.stopFollowLocked()
		cl.startFollowLocked()
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
		return "已开启跟踪 " + shortID(id) + " 的日志（最近 " + fmt.Sprintf("%d", n) + " 行）", nil
	}
	lines, err := fetchLogs(id, n)
	if err != nil {
		return "", err
	}
	cl.openDetail(shortID(id), tabLogs)
	cl.host.Post(func() {
		cl.mu.Lock()
		cl.st.logLines = toLogLines(lines)
		cl.st.logState = mcodeLogViewerState()
		cl.stopFollowLocked()
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	if len(lines) == 0 {
		return "(没有日志)", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(最后 %d 行)\n", len(lines))
	for _, s := range lines {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolRun creates and starts a new container.
func (cl *Client) toolRun(image, name string, ports []string, detach string, cmd []string) (string, error) {
	if image == "" {
		return "", fmt.Errorf("缺少必填参数 image")
	}
	d := !strings.EqualFold(detach, "false")
	out, err := runContainer(image, name, ports, d, cmd)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast("已运行 "+image, mcore.SeveritySuccess)
		cl.loadPage(pageContainers)
	})
	return out, nil
}

// toolExec runs a command inside a container.
func (cl *Client) toolExec(id string, cmd []string) (string, error) {
	if id == "" || len(cmd) == 0 {
		return "", fmt.Errorf("缺少必填参数 id 或 cmd")
	}
	return execContainer(id, cmd)
}

// toolPrune cleans unused resources.
func (cl *Client) toolPrune(scope string) (string, error) {
	if scope == "" {
		return "", fmt.Errorf("缺少必填参数 scope")
	}
	msg, err := prune(scope)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.host.Toast(msg, mcore.SeveritySuccess)
		cl.Refresh()
	})
	return msg, nil
}

// toolInfo returns system info or disk usage.
func (cl *Client) toolInfo(kind string) (string, error) {
	if strings.EqualFold(kind, "df") {
		return diskUsageText()
	}
	return systemInfoText()
}

// shortID trims an id for display.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
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

// listArg reads a string-array argument.
func listArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
		}
		return strings.Fields(val)
	}
	return nil
}

// mcodeLogViewerState returns a fresh viewer state with follow on.
func mcodeLogViewerState() mcode.LogViewerState {
	return mcode.LogViewerState{}
}
