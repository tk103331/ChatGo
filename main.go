package main

import (
	"chatgo/internal/config"
	"chatgo/internal/ui"
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater/native"
)

func main() {
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
		cw.Show()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
