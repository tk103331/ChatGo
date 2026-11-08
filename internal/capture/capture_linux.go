//go:build linux

package capture

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// linuxCapture reads the X11 PRIMARY selection, which is where a desktop
// puts the text the user just selected, and falls back to the clipboard
// elsewhere.
type linuxCapture struct {
	clipboardCapture
}

func platformCapture() Capture { return linuxCapture{} }

// Permission reports availability: an X11 build needs one of the selection
// tools, which the user's desktop usually already has.
func (linuxCapture) Permission() Permission {
	if linuxToolAvailable() {
		return Permission{State: PermissionGranted}
	}
	return Permission{State: PermissionUnsupported}
}

func linuxToolAvailable() bool {
	for _, name := range []string{"xclip", "xsel", "wl-paste"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

// Selection reads the primary selection through the usual X11 tools. Wayland
// compositors do not let another application read a selection unless they
// speak a data-control protocol, so wl-clipboard is tried first there.
func (linuxCapture) Selection() (Selection, error) {
	if !mygo.App.IsReady() {
		return Selection{}, ErrUnsupported
	}
	text := linuxPrimary()
	if !AcceptText(text, 1) {
		return Selection{}, ErrNoSelection
	}
	return Selection{Text: text, Source: SourcePrimary}, nil
}

// linuxPrimary asks the first installed tool for the primary selection. A
// missing tool or a failed read just moves on to the next one.
func linuxPrimary() string {
	tools := [][]string{
		{"xclip", "-o", "-selection", "primary"},
		{"xsel", "--primary", "--output"},
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		tools = append([][]string{{"wl-paste", "--primary", "--no-newline"}}, tools...)
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool[0]); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		out, err := exec.CommandContext(ctx, tool[0], tool[1:]...).Output()
		cancel()
		if err != nil {
			continue
		}
		if text := strings.TrimRight(string(out), "\x00"); text != "" {
			return text
		}
	}
	return ""
}
