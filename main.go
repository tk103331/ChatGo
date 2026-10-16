package main

import (
	"chatgo/internal/config"
	"chatgo/internal/shellenv"
	"chatgo/internal/ui"
	"fmt"
	"log"

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
