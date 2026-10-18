// Package terminalapp is the Apps menu's terminal: shell tabs rooted in the
// project, plus a bounded command runner for the Agent.
package terminalapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是终端应用助手。你可以通过 run_command 在项目工作目录中执行 shell 命令，把结果反馈给用户。命令有白名单限制。"

// shellTab is one open shell.
type shellTab struct {
	id    string
	title string
	term  *term.Terminal
	err   string
}

// Client is one terminal app bound to a Host.
type Client struct {
	host Host
	tabs appkit.Tabs
	byID map[string]*shellTab
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a terminal app driving the given host.
func New(h Host) *Client {
	return &Client{host: h, byID: map[string]*shellTab{}}
}

// Title labels the active tab for a tab bar.
func (cl *Client) Title(id string) string {
	tb := cl.byID[id]
	if tb == nil {
		return "Shell"
	}
	switch {
	case tb.title != "":
		return tb.title
	case tb.err != "":
		return "错误"
	default:
		return "Shell"
	}
}

// NewTab starts a shell and returns its id.
func (cl *Client) NewTab() string {
	dir := cl.host.Workspace()
	if dir == "" {
		dir = cl.host.Root()
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	tb := &shellTab{id: cl.host.NewID()}
	cl.byID[tb.id] = tb
	cl.tabs.Add(tb.id)
	go cl.start(tb, dir)
	return tb.id
}

// start builds one terminal off the UI thread.
func (cl *Client) start(tb *shellTab, dir string) {
	t, err := term.New(term.Options{
		Dir:     dir,
		OnTitle: func(title string) { cl.host.Post(func() { tb.title = title; cl.host.Invalidate() }) },
		OnExit:  func(int) { cl.host.Post(cl.host.Invalidate) },
	})
	cl.host.Post(func() {
		if err != nil {
			tb.err = err.Error()
			cl.host.Invalidate()
			return
		}
		tb.term = t
		cl.host.Invalidate()
	})
}

// Close stops a tab's shell and drops it.
func (cl *Client) Close(id string) {
	if tb := cl.byID[id]; tb != nil && tb.term != nil {
		_ = tb.term.Close()
	}
	delete(cl.byID, id)
	for i, key := range cl.tabs.Items {
		if key == id {
			cl.tabs.Close(i)
			return
		}
	}
}

// View draws the active shell.
func (cl *Client) View(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	tb := cl.byID[cl.tabs.Active]
	mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		switch {
		case tb == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
		case tb.err != "":
			mgo.Text(c, "终端不可用："+tb.err).Padding(u * 3).FontSize(caption).TextColor(k.Danger)
		case tb.term == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
		default:
			term.View(c, tb.term).Fill()
		}
	})
}

// Tabs exposes the tab list for an embedding tab bar.
func (cl *Client) Tabs() *appkit.Tabs { return &cl.tabs }

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name:   "run_command",
			Desc:   "在工作目录中执行一条 shell 命令并返回输出",
			Params: map[string]Param{"command": {Desc: "要执行的 shell 命令", Required: true}},
			Run: func(args map[string]any) (string, error) {
				return cl.runCommand(str(args, "command"))
			},
		},
		{
			Name: "list_terminals",
			Desc: "列出当前打开的终端标签页标题",
			Run: func(map[string]any) (string, error) {
				if len(cl.tabs.Items) == 0 {
					return "(没有打开的终端)", nil
				}
				var b strings.Builder
				for _, id := range cl.tabs.Items {
					fmt.Fprintf(&b, "- %s\n", cl.Title(id))
				}
				return strings.TrimRight(b.String(), "\n"), nil
			},
		},
	}
}

// runCommand runs a shell command in the workspace with a bounded timeout.
func (cl *Client) runCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	if root := cl.host.Root(); root != "" {
		c.Dir = root
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

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
