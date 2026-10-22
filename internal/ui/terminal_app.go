package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	term "github.com/egoist/mygo/plugins/terminal"
	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// appTermTab is one shell tab of the terminal app.
type appTermTab struct {
	id    string
	title string
	term  *term.Terminal
	err   string
}

// terminalApp is the App menu's shell application.
func terminalApp() appDef {
	return appDef{
		ID:     "terminal",
		Title:  "终端",
		Desc:   "在项目目录中运行命令，命令历史与会话保留在标签页里",
		Icon:   "terminal",
		System: "你是终端应用助手。你可以通过 run_command 在项目工作目录中执行 shell 命令，把结果反馈给用户。命令有白名单限制。",
		Tools: func(cw *ChatWindow) []appTool {
			return []appTool{
				{
					Name: "run_command",
					Desc: "在工作目录中执行一条 shell 命令并返回输出",
					Params: map[string]appParam{
						"command": {Desc: "要执行的 shell 命令", Required: true},
					},
					Run: func(cw *ChatWindow, args map[string]any) (string, error) {
						return cw.appRunCommand(argString(args, "command"))
					},
				},
				{
					Name:   "list_terminals",
					Desc:   "列出当前打开的终端标签页标题",
					Params: map[string]appParam{},
					Run: func(cw *ChatWindow, _ map[string]any) (string, error) {
						cw.ensureAppState()
						tabs := cw.appTermTabs
						if len(tabs) == 0 {
							return "(没有打开的终端)", nil
						}
						var b strings.Builder
						for _, tb := range tabs {
							fmt.Fprintf(&b, "- %s\n", tb.title)
						}
						return strings.TrimRight(b.String(), "\n"), nil
					},
				},
			}
		},
		NewTab: func(cw *ChatWindow) string {
			return cw.newAppTermTab()
		},
		TabLabel: func(cw *ChatWindow, tab string) string {
			for _, tb := range cw.appTermTabs {
				if tb.id == tab {
					if tb.title != "" {
						return tb.title
					}
					if tb.err != "" {
						return "错误"
					}
					return "Shell"
				}
			}
			return "Shell"
		},
		Body: func(cw *ChatWindow, c *mgo.Context, tab string) {
			cw.appTermBody(c, tab)
		},
	}
}

// newAppTermTab starts a shell for the terminal app and returns its id.
func (cw *ChatWindow) newAppTermTab() string {
	tb := &appTermTab{id: cw.newID()}
	cw.appTermTabs = append(cw.appTermTabs, tb)
	go cw.startAppTerm(tb)
	return tb.id
}

// startAppTerm builds one terminal off the main thread, like the bottom panel.
func (cw *ChatWindow) startAppTerm(tb *appTermTab) {
	dir := cw.workspace
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	t, err := term.New(term.Options{
		Dir:     dir,
		OnTitle: func(title string) { cw.postMain(func() { tb.title = title; cw.invalidate() }) },
		OnExit:  func(int) { cw.postMain(func() { cw.invalidate() }) },
	})
	cw.postMain(func() {
		if err != nil {
			tb.err = err.Error()
			return
		}
		if !cw.hasAppTerm(tb) {
			_ = t.Close()
			return
		}
		tb.term = t
	})
}

// hasAppTerm reports whether the terminal tab is still open.
func (cw *ChatWindow) hasAppTerm(tb *appTermTab) bool {
	for _, t := range cw.appTermTabs {
		if t == tb {
			return true
		}
	}
	return false
}

// appTermBody draws the active terminal tab's shell view.
func (cw *ChatWindow) appTermBody(c *mgo.Context, tab string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Box(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		tb := cw.appTermByID(tab)
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

// appTermByID returns the terminal tab with the given id.
func (cw *ChatWindow) appTermByID(id string) *appTermTab {
	for _, tb := range cw.appTermTabs {
		if tb.id == id {
			return tb
		}
	}
	return nil
}

// appRunCommand runs a shell command in the workspace with a bounded timeout.
func (cw *ChatWindow) appRunCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("缺少必填参数 command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	if root := cw.fileRoot(); root != "" {
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
