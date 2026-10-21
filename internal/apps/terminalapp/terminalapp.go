// Package terminalapp is the Apps menu's terminal: shell tabs rooted in the
// project, plus a bounded command runner for the Agent.
package terminalapp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/apps/appkit"
	"chatgo/internal/apps/filetools"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// System is the app's chat system prompt.
const System = "你是终端应用助手。你可以通过 run_command（命令有白名单限制）在工作目录中执行 shell 命令，通过 list_tabs 查看打开的终端标签，通过 send_keys 向上下文终端标签发送按键、read_screen 读取终端内容，把结果反馈给用户。"

const (
	defaultTimeout = 60
	maxTimeout     = 300
	maxOutput      = 64 << 10
)

// commandWhitelist is the only command prefix run_command accepts.
var commandWhitelist = map[string]bool{
	"ls": true, "pwd": true, "echo": true, "cat": true, "head": true,
	"tail": true, "wc": true, "grep": true, "find": true, "git": true,
	"mkdir": true, "date": true, "uname": true, "du": true, "df": true,
	"ps": true, "curl": true, "node": true, "python3": true, "go": true, "make": true,
}

// shellTab is one open shell.
type shellTab struct {
	id     string
	title  string
	term   *term.Terminal
	err    string
	exited bool
}

// Client is one terminal app bound to a Host.
type Client struct {
	host    Host
	tabs    appkit.Tabs
	byID    map[string]*shellTab
	ctxAll  bool
	ctxTabs []string
}

// Host is the capability set the app needs from its shell.
type Host = appkit.Host

// New returns a terminal app driving the given host.
func New(h Host) *Client {
	return &Client{host: h, byID: map[string]*shellTab{}}
}

// SetContext stores the chat's selected context tabs; all picks every tab.
func (cl *Client) SetContext(all bool, ids []string) {
	cl.ctxAll = all
	cl.ctxTabs = ids
}

// Title returns a tab's short title: the working directory's name.
func (cl *Client) Title(id string) string {
	tb := cl.byID[id]
	if tb == nil {
		return "Shell"
	}
	if tb.term != nil {
		if dir := tb.term.Dir(); dir != "" {
			base := filepath.Base(dir)
			if base != "" && base != "/" && base != "." && base != "~" {
				return base
			}
		}
	}
	if tb.err != "" {
		return "错误"
	}
	return "Shell"
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
		OnExit: func(int) {
			cl.host.Post(func() { tb.exited = true; cl.host.Invalidate() })
		},
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
	mgo.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		switch {
		case tb == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
		case tb.err != "":
			mgo.Text(c, "终端不可用："+tb.err).Padding(u * 3).FontSize(caption).TextColor(k.Danger)
		case tb.term == nil:
			mgo.Text(c, "正在启动终端…").Padding(u * 3).FontSize(caption).TextColor(k.TextMuted)
		default:
			mgo.Row(c).FillWidth().Shrink(0).Padding(u, u*2).Gap(u).AlignItems(mgo.Center).
				BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
				if minput.Button(c, "清屏", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
					tb.term.Send([]byte("\x0c"))
				}
				if minput.Button(c, "中断", minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
					tb.term.Send([]byte("\x03"))
				}
				mgo.Box(c).Grow(1)
				if dir := tb.term.Dir(); dir != "" {
					mgo.Text(c, dir).FontSize(caption).TextColor(k.TextMuted).
						Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				}
				status := "运行中"
				if tb.exited {
					status = "已退出"
				}
				mgo.Text(c, status).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			})
			mgo.Box(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
				term.View(c, tb.term).Fill()
			})
		}
	})
}

// Tabs exposes the tab list for an embedding tab bar.
func (cl *Client) Tabs() *appkit.Tabs { return &cl.tabs }

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "run_command",
			Desc: "在工作目录中执行一条白名单内的 shell 命令并返回输出",
			Params: map[string]Param{
				"command": {Desc: "要执行的 shell 命令", Required: true},
				"workdir": {Desc: "相对根目录的工作目录，可留空"},
				"timeout": {Desc: "超时秒数，1-300，默认 60"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.runCommandWith(str(args, "command"), str(args, "workdir"), intFrom(args, "timeout"))
			},
		},
		{
			Name: "list_tabs",
			Desc: "列出当前打开的终端标签（id、标题、状态；选中的标 [上下文]）",
			Run: func(map[string]any) (string, error) {
				if len(cl.tabs.Items) == 0 {
					return "(没有打开的终端)", nil
				}
				var b strings.Builder
				for _, id := range cl.tabs.Items {
					tb := cl.byID[id]
					status := "启动中"
					if tb != nil && tb.term != nil {
						status = "运行中"
						if tb.exited {
							status = "已退出"
						}
					}
					fmt.Fprintf(&b, "- %s: %s（%s）%s\n", id, cl.Title(id), status, cl.ctxMark(id))
				}
				return strings.TrimRight(b.String(), "\n"), nil
			},
		},
		{
			Name:   "send_keys",
			Desc:   "把文本作为按键发送到指定或上下文终端标签（配合交互式程序）",
			Params: map[string]Param{"text": {Desc: "要发送的文本", Required: true}, "tab": {Desc: "目标标签 id（来自 list_tabs），留空用上下文"}},
			Run: func(args map[string]any) (string, error) {
				return cl.sendKeys(str(args, "text"), str(args, "tab"))
			},
		},
		{
			Name:   "read_screen",
			Desc:   "读取指定或上下文终端标签的屏幕与回滚文本",
			Params: map[string]Param{"tab": {Desc: "目标标签 id（来自 list_tabs），留空用上下文"}},
			Run: func(args map[string]any) (string, error) {
				return cl.readScreen(str(args, "tab"))
			},
		},
	}
}

// ctxMark is "[上下文]" when id is in the chat's context set.
func (cl *Client) ctxMark(id string) string {
	if cl.ctxAll || slices.Contains(cl.ctxTabs, id) {
		return "[上下文]"
	}
	return ""
}

// runCommand runs a shell command in the workspace with a bounded timeout.
func (cl *Client) runCommand(command string) (string, error) {
	return cl.runCommandWith(command, "", 0)
}

// runCommandWith runs a whitelisted shell command in workdir (or the root)
// with a bounded timeout, capping the output.
func (cl *Client) runCommandWith(command, workdir string, timeoutSec int) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	name := firstWord(command)
	if !commandWhitelist[name] {
		return "", fmt.Errorf("命令 %q 不在白名单内", name)
	}
	dir := cl.host.Root()
	if strings.TrimSpace(workdir) != "" {
		abs, err := filetools.Within(cl.host.Root(), workdir)
		if err != nil {
			return "", err
		}
		dir = abs
	}
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeout
	}
	if timeoutSec > maxTimeout {
		timeoutSec = maxTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		c.Dir = dir
	}
	out, err := c.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if len(result) > maxOutput {
		result = result[:maxOutput] + "\n(输出已截断)"
	}
	if err != nil {
		return result + "\n" + err.Error(), nil
	}
	if result == "" {
		result = "(无输出)"
	}
	return result, nil
}

// targetIDs returns the ids the context-selected tabs map to: every tab
// when ctxAll, the picked set when set, else the active tab.
func (cl *Client) targetIDs() []string {
	var ids []string
	switch {
	case cl.ctxAll:
		ids = append(ids, cl.tabs.Items...)
	case len(cl.ctxTabs) > 0:
		ids = append(ids, cl.ctxTabs...)
	default:
		if cl.tabs.Active != "" {
			ids = []string{cl.tabs.Active}
		}
	}
	return ids
}

// targetTerms returns the shells of the context-selected tabs, or of one
// explicit tab id when tab is non-empty.
func (cl *Client) targetTerms(tab string) []*term.Terminal {
	if tab != "" {
		if tb := cl.byID[tab]; tb != nil && tb.term != nil {
			return []*term.Terminal{tb.term}
		}
		return nil
	}
	var out []*term.Terminal
	for _, id := range cl.targetIDs() {
		if tb := cl.byID[id]; tb != nil && tb.term != nil {
			out = append(out, tb.term)
		}
	}
	return out
}

// sendKeys writes text into every target shell.
func (cl *Client) sendKeys(text, tab string) (string, error) {
	terms := cl.targetTerms(tab)
	if len(terms) == 0 {
		return "", fmt.Errorf("没有打开的终端")
	}
	for _, t := range terms {
		t.Send([]byte(text))
	}
	return "已发送到终端", nil
}

// readScreen returns the target screens, one block per tab.
func (cl *Client) readScreen(tab string) (string, error) {
	terms := cl.targetTerms(tab)
	if len(terms) == 0 {
		return "", fmt.Errorf("没有打开的终端")
	}
	var b strings.Builder
	for i, t := range terms {
		if i > 0 {
			b.WriteString("\n\n")
		}
		out := strings.TrimSpace(t.Text())
		if out == "" {
			b.WriteString("(空屏幕)")
		} else {
			b.WriteString(out)
		}
	}
	return b.String(), nil
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}
	return s
}

func intFrom(args map[string]any, key string) int {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
				return n
			}
		}
	}
	return 0
}

func str(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
