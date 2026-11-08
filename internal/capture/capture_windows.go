//go:build windows

package capture

import (
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/egoist/mygo"
)

type windowsCapture struct {
	clipboardCapture
}

func platformCapture() Capture { return windowsCapture{} }

var (
	user32         = windows.NewLazySystemDLL("user32.dll")
	procKeybdEvent = user32.NewProc("keybd_event")
)

const (
	vkControl      = 0x11
	vkC            = 0x43
	keyeventfKeyUp = 0x0002
)

// CopySelection sends Ctrl+C to the frontmost window and returns the text
// that landed on the clipboard. When the clipboard held only text, the old
// text is put back afterwards; rich content is left replaced rather than
// silently lost.
func (windowsCapture) CopySelection() (string, bool) {
	if !mygo.App.IsReady() {
		return "", false
	}
	oldText := strings.TrimRight(mygo.Clipboard.ReadText(), "\x00")
	oldFormats := mygo.Clipboard.AvailableFormats()
	pressCtrlC()

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
		text := strings.TrimRight(mygo.Clipboard.ReadText(), "\x00")
		if text == "" || text == oldText {
			continue
		}
		if textOnlyClipboard(oldFormats) {
			if oldText == "" {
				mygo.Clipboard.Clear()
			} else {
				mygo.Clipboard.WriteText(oldText)
			}
		}
		return text, true
	}
	return "", false
}

// pressCtrlC synthesizes the copy shortcut. keybd_event is deprecated but
// still delivered, and avoids the larger SendInput structure on purpose: the
// feature is opt-in and only needs a plain Ctrl+C.
func pressCtrlC() {
	procKeybdEvent.Call(vkControl, 0, 0, 0)
	procKeybdEvent.Call(vkC, 0, 0, 0)
	procKeybdEvent.Call(vkC, 0, keyeventfKeyUp, 0)
	procKeybdEvent.Call(vkControl, 0, keyeventfKeyUp, 0)
}
