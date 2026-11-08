package main

import (
	"chatgo/internal/config"
	"chatgo/internal/shellenv"
	"chatgo/internal/ui"
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/plugins/updater/native"
)

func main() {
	shellenv.AugmentPath()
	mygo.Use(native.Plugin)

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

	// A second launch (`chatgo .` from a terminal, or `open chatgo://…`)
	// hands its arguments to the instance already running.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.OnSecondInstance(func(args []string, wd string) {
		cw.HandleLaunchArgs(args, wd)
	})
	mygo.App.OnOpenURL(func(raw string) {
		cw.HandleQuickURL(raw)
	})
	if err := mygo.App.RegisterURLScheme("chatgo"); err != nil {
		log.Printf("chatgo:// scheme: %v", err)
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
		cw.StartUpdateChecker()
		cw.Show()
		// A command line directory or prompt, such as `chatgo .`, opens the
		// quick panel over the main window.
		if wd, err := os.Getwd(); err == nil {
			cw.HandleLaunchArgs(os.Args[1:], wd)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
