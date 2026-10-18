package main

import (
	"chatgo/internal/apps/browserapp"
	"chatgo/internal/apps/codeeditorapp"
	"chatgo/internal/config"
	"chatgo/internal/apps/gitapp"
	"chatgo/internal/apps/markdownapp"
	"chatgo/internal/shellenv"
	"chatgo/internal/apps/terminalapp"
	"chatgo/internal/ui"
	"chatgo/internal/apps/webapp"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

func main() {
	shellenv.AugmentPath()
	mygo.Use(native.Plugin)

	app := flag.String("app", "", "独立打开一个应用窗口: git, terminal, markdown, code, browser, 或 webapp:<id>")
	dir := flag.String("dir", "", "独立打开时的工作目录，默认为当前目录")
	flag.Parse()
	if *app != "" {
		runStandalone(*app, *dir)
		return
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	cw, err := ui.NewChatWindow(cfg)
	if err != nil {
		fmt.Printf("Failed to create chat window: %v\n", err)
		return
	}

	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "ChatGo", Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout},
				updater.MenuItem(),
				mygo.Separator(),
				{Role: mygo.RoleQuit},
			}},
			{Role: mygo.RoleEditMenu},
			{Role: mygo.RoleWindowMenu},
		}))
		cw.Show()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// runStandalone opens one app in its own window, without the chat shell.
func runStandalone(id, dir string) {
	root := dir
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	if strings.HasPrefix(id, "webapp:") {
		name := strings.TrimPrefix(id, "webapp:")
		a, ok := webapp.Find(name)
		if !ok {
			fmt.Printf("未找到 web 应用 %q\n", name)
			os.Exit(1)
		}
		webapp.Standalone(a)
		return
	}
	switch id {
	case "git":
		gitapp.Standalone(root)
	case "terminal":
		terminalapp.Standalone(root)
	case "markdown":
		markdownapp.Standalone(root)
	case "code":
		codeeditorapp.Standalone(root)
	case "browser":
		browserapp.Standalone(root)
	default:
		fmt.Printf("未知应用 %q，可选: git, terminal, markdown, code, browser, webapp:<id>\n", id)
		os.Exit(1)
	}
}
