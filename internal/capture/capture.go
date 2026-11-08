// Package capture reads the text the user selected in another application,
// and the directory the file manager has selected, without asking them to
// copy and paste. macOS backs it with the Accessibility API; other platforms
// report the capability as unsupported for now.
//
// The package is deliberately free of UI imports: Watch may call its
// callback from any goroutine, and the caller moves the work onto the main
// thread before touching a window.
package capture

import (
	"errors"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// captureDebug turns on diagnostic logging. Set CHATGO_CAPTURE_DEBUG=1 to see
// which application answered which AX attribute; the text itself is never
// logged.
var captureDebug = os.Getenv("CHATGO_CAPTURE_DEBUG") != ""

func debugf(format string, args ...any) {
	if captureDebug {
		log.Printf("[capture] "+format, args...)
	}
}

// Rect is a rectangle in screen coordinates: DIPs, origin at the top-left
// corner of the primary display, the space mygo.Rectangle uses.
type Rect struct{ X, Y, Width, Height int }

// Size is a width and height in DIPs.
type Size struct{ Width, Height int }

// Source names how a selection was read.
type Source int

const (
	// SourceAX read the selection through the Accessibility API.
	SourceAX Source = iota
	// SourceClipboard took it from the clipboard instead.
	SourceClipboard
	// SourcePrimary read the X11 primary selection.
	SourcePrimary
)

// Selection is text selected in another application.
type Selection struct {
	Text string
	// Rect is the on-screen box of the selection, zero when the application
	// did not report one.
	Rect     Rect
	App      string
	BundleID string
	PID      int
	Source   Source
}

// PermissionState is how the system answers the capture permission.
type PermissionState int

const (
	// PermissionUnsupported means the platform has no system capture.
	PermissionUnsupported PermissionState = iota
	// PermissionDenied means the user has not granted it.
	PermissionDenied
	// PermissionGranted means reads work.
	PermissionGranted
)

// Permission is the state of the system capture permission.
type Permission struct {
	State PermissionState
	// SettingsURL opens the pane where the user grants it.
	SettingsURL string
}

// Errors reported by Capture implementations.
var (
	ErrUnsupported = errors.New("capture: unsupported on this platform")
	ErrNoSelection = errors.New("capture: no selection")
	ErrNoDirectory = errors.New("capture: no directory")
	ErrPermission  = errors.New("capture: permission denied")
)

// Capture reads selections and directories from the system.
type Capture interface {
	// Permission reports whether selections can be read.
	Permission() Permission
	// RequestPermission asks the system to show its permission prompt, when
	// it has one. It does nothing where reads need no permission.
	RequestPermission()
	// Selection reads the text selected in the frontmost application.
	Selection() (Selection, error)
	// Watch calls fn when the user finishes a selection in another
	// application. The callback may run on any goroutine.
	Watch(fn func(Selection)) (stop func())
	// WatchDismiss calls fn when the user presses the mouse outside this
	// application, which is how an inactive bar learns they moved on. The
	// callback runs on the main thread. Call it on the main thread.
	WatchDismiss(fn func()) (stop func())
	// Directory returns the directory selected in the file manager, or the
	// one on the clipboard.
	Directory() (string, error)
	// ClipboardText returns the plain text on the clipboard, which is the
	// fallback for a platform or app whose selection cannot be read.
	ClipboardText() string
	// CopySelection asks the frontmost application to copy its selection and
	// returns what landed on the clipboard. Platforms with a readable
	// selection API report false.
	CopySelection() (string, bool)
}

// New returns the platform Capture. Call it once and keep the result.
func New() Capture { return platformCapture() }

// AcceptText reports whether text is worth showing the panel for: it has at
// least min runes once trimmed.
func AcceptText(text string, min int) bool {
	if min < 1 {
		min = 1
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	return utf8.RuneCountInString(trimmed) >= min
}

// ClipText shortens text to at most max runes, marking the cut with an
// ellipsis. It is applied to the quote the panel sends, not to the text the
// user sees selected in their application.
func ClipText(text string, max int) string {
	if max <= 0 {
		return text
	}
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	return string(r[:max]) + "…"
}

// Quote turns a selection into the block that accompanies the user's
// instruction, one "> " line per source line.
func Quote(text string, max int) string {
	text = strings.TrimSpace(ClipText(text, max))
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = "> " + strings.TrimRight(line, "\r")
	}
	return strings.Join(lines, "\n")
}

// ParseFileURL returns the local path of a file:// URL or of an absolute
// path. It accepts the forms Finder puts on the clipboard and the ones a
// shell passes on the command line.
func ParseFileURL(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimRight(s, "\x00"))
	if s == "" {
		return "", false
	}
	if strings.HasPrefix(s, "file://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", false
		}
		// url.Parse keeps Path decoded, so "%20" is already a space here.
		p := u.Path
		if p == "" {
			return "", false
		}
		if runtime.GOOS == "windows" {
			// file:///C:/dir parses as /C:/dir.
			if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
				p = p[1:]
			}
			if u.Host != "" && u.Host != "localhost" {
				p = `\\` + u.Host + strings.ReplaceAll(p, "/", `\`)
			}
		}
		return filepath.Clean(p), true
	}
	if filepath.IsAbs(s) {
		return filepath.Clean(s), true
	}
	return "", false
}

// textOnlyClipboard reports whether a clipboard holding these formats has
// only text, so restoring a text value cannot lose other content.
func textOnlyClipboard(formats []string) bool {
	for _, f := range formats {
		if !strings.HasPrefix(f, "text/") {
			return false
		}
	}
	return true
}

// ExistingDir reports whether path names a directory that exists.
func ExistingDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// PlaceNear places a box of size next to anchor, preferring below it and
// flipping above when the work area has no room, then clamped inside the
// work area. A zero anchor means "no selection box": the box goes centered,
// a third of the way down, as a launcher does.
func PlaceNear(anchor Rect, size Size, work Rect) (x, y int) {
	const gap = 8
	if size.Width <= 0 || size.Height <= 0 || work.Width <= 0 || work.Height <= 0 {
		return work.X, work.Y
	}
	if size.Width >= work.Width || size.Height >= work.Height {
		return work.X, work.Y
	}
	if anchor.Width <= 0 && anchor.Height <= 0 {
		x = work.X + (work.Width-size.Width)/2
		y = work.Y + work.Height/3
	} else {
		x = anchor.X
		y = anchor.Y + anchor.Height + gap
		if y+size.Height > work.Y+work.Height {
			if above := anchor.Y - gap - size.Height; above >= work.Y {
				y = above
			}
		}
	}
	maxX := work.X + work.Width - size.Width - gap
	if x > maxX {
		x = maxX
	}
	if x < work.X+gap {
		x = work.X + gap
	}
	maxY := work.Y + work.Height - size.Height - gap
	if y > maxY {
		y = maxY
	}
	if y < work.Y+gap {
		y = work.Y + gap
	}
	return x, y
}
