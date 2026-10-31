// Command webd serves a local ChatGo web UI.
//
// It reads the same ~/.chatgo config and conversations as the desktop app,
// runs model calls in this process, and serves the MyGo wasm client from
// -web. It is the first step toward remote control: the browser only draws
// and sends input; the agent runs here, on the local machine.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"chatgo/internal/config"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8422", "listen address")
	webDir := flag.String("web", "web", "directory with index.html, app.wasm and wasm_exec.js")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cm, err := models.NewConversationManager()
	if err != nil {
		log.Fatalf("open conversations: %v", err)
	}

	s, err := webui.NewServer(cfg, cm, *webDir)
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	if _, err := os.Stat(filepath.Join(*webDir, "app.wasm")); err != nil {
		log.Printf("warning: %s not found; run ./web/build.sh first", filepath.Join(*webDir, "app.wasm"))
	}

	log.Printf("ChatGo web on http://%s", *addr)
	if provider, model, ok := s.Target(); ok {
		log.Printf("model: %s / %s", provider, model)
	} else {
		log.Printf("model unavailable")
	}
	if err := s.ListenAndServe(*addr); err != nil {
		log.Fatal(err)
	}
}
