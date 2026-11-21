// Command _pairdev is a throwaway development harness: it connects a desktop
// connector to a local relay and prints a pairing code, so the phone app can
// be exercised end to end without clicking through the desktop GUI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"chatgo/internal/relayclient"
)

func main() {
	relayURL := flag.String("relay", "ws://127.0.0.1:8787", "relay URL")
	desktopID := flag.String("desktop", "d_android_local", "desktop id")
	secret := flag.String("secret", "dev-secret", "desktop secret")
	local := flag.String("local", "http://127.0.0.1:8422", "local webui URL")
	token := flag.String("token", "dev-token", "local webui token")
	flag.Parse()

	conn := relayclient.New(relayclient.Config{
		RelayURL:     *relayURL,
		DesktopID:    *desktopID,
		DesktopName:  "Dev Desktop",
		Secret:       *secret,
		LocalURL:     *local,
		LocalToken:   *token,
		AllowPairing: true,
		Capabilities: map[string]bool{"chat": true},
		OnState:      func(s relayclient.State) { log.Printf("relay state: %s", s) },
		OnDevices: func(d []relayclient.Device) {
			for _, dev := range d {
				log.Printf("paired device: %s %s %s", dev.ID, dev.Name, dev.Platform)
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)

	deadline := time.Now().Add(15 * time.Second)
	for conn.State() != relayclient.StateConnected && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if conn.State() != relayclient.StateConnected {
		log.Fatalf("relay did not connect: %s", conn.State())
	}
	code, expires := conn.NewPairCode(10 * time.Minute)
	fmt.Printf("PAIR_CODE=%s\n", code)
	fmt.Printf("PAIR_LINK=chatgo://pair?v=1&r=%s&d=%s&c=%s\n",
		urlEncode(*relayURL), *desktopID, code)
	fmt.Printf("expires=%s\n", expires.Format(time.RFC3339))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}

func urlEncode(s string) string {
	out := make([]byte, 0, len(s)*3)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		case c == '-' || c == '_' || c == '.' || c == '~':
			out = append(out, c)
		default:
			out = append(out, '%')
			const hex = "0123456789ABCDEF"
			out = append(out, hex[c>>4], hex[c&0xf])
		}
	}
	return string(out)
}
