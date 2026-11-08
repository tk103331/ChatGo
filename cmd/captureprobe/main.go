// Command captureprobe reports what the system selection capture sees.
//
// It is the M0 companion for the quick panel: run it, select some text in
// another application, and it prints which application answered, how long
// the selection is and where its box is. Nothing is sent anywhere, and the
// text itself is only printed with -text.
//
//	CHATGO_CAPTURE_DEBUG=1 go run ./cmd/captureprobe -text
//
// Quit with Ctrl+C (or Cmd+Q when the menu bar is present).
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"chatgo/internal/capture"

	"github.com/egoist/mygo"
)

func main() {
	showText := flag.Bool("text", false, "print the selected text as well")
	check := flag.Bool("check", false, "print the permission state and exit")
	flag.Parse()

	grabber := capture.New()
	if *check {
		perm := grabber.Permission()
		fmt.Printf("permission: %s\n", permissionName(perm.State))
		if perm.SettingsURL != "" {
			fmt.Printf("open: %s\n", perm.SettingsURL)
		}
		return
	}
	// Keep the process alive without a window: global monitors need only the
	// application loop.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() {
		perm := grabber.Permission()
		fmt.Printf("permission: %s\n", permissionName(perm.State))
		if perm.State == capture.PermissionDenied {
			fmt.Println("grant Accessibility to this binary, then run it again")
			if perm.SettingsURL != "" {
				fmt.Printf("open: %s\n", perm.SettingsURL)
			}
		}
		if perm.State == capture.PermissionUnsupported {
			fmt.Println("this platform has no system selection capture; copy text and use the app's clipboard fallback")
		}
		stop := grabber.Watch(func(sel capture.Selection) {
			fmt.Printf("%s app=%q bundle=%q rect=%+v runes=%d source=%s\n",
				time.Now().Format("15:04:05"), sel.App, sel.BundleID, sel.Rect,
				len([]rune(sel.Text)), sourceName(sel.Source))
			if *showText {
				fmt.Printf("  text: %q\n", sel.Text)
			}
		})
		defer stop()
		fmt.Println("select text in another application to see a line; Ctrl+C quits")
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func permissionName(state capture.PermissionState) string {
	switch state {
	case capture.PermissionGranted:
		return "granted"
	case capture.PermissionDenied:
		return "denied"
	case capture.PermissionUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

func sourceName(source capture.Source) string {
	switch source {
	case capture.SourceAX:
		return "accessibility"
	case capture.SourceClipboard:
		return "clipboard"
	case capture.SourcePrimary:
		return "primary"
	default:
		return "unknown"
	}
}
