//go:build !darwin

package capture

import (
	"strings"

	"github.com/egoist/mygo"
)

// clipboardCapture is the capture available where the system has no
// supported way to read another application's selection: the panel falls
// back to the clipboard the user filled with a copy.
type clipboardCapture struct{}

func (clipboardCapture) Permission() Permission {
	return Permission{State: PermissionUnsupported}
}

func (clipboardCapture) RequestPermission() {}

func (clipboardCapture) Selection() (Selection, error) { return Selection{}, ErrUnsupported }

func (clipboardCapture) Watch(func(Selection)) (stop func()) { return func() {} }

func (clipboardCapture) WatchDismiss(func()) (stop func()) { return func() {} }

// ClipboardText returns the plain text on the clipboard.
func (clipboardCapture) ClipboardText() string {
	if !mygo.App.IsReady() {
		return ""
	}
	return strings.TrimRight(mygo.Clipboard.ReadText(), "\x00")
}

// CopySelection needs a platform that can synthesize a copy; the generic
// capture cannot.
func (clipboardCapture) CopySelection() (string, bool) { return "", false }

// Directory reads a folder the user copied, as a file URL or a path.
func (c clipboardCapture) Directory() (string, error) {
	text := c.ClipboardText()
	if text == "" {
		return "", ErrNoDirectory
	}
	if path, ok := ParseFileURL(text); ok && ExistingDir(path) {
		return path, nil
	}
	// A file manager may put several paths on the clipboard, one per line.
	for _, line := range strings.Split(text, "\n") {
		if path, ok := ParseFileURL(line); ok && ExistingDir(path) {
			return path, nil
		}
	}
	return "", ErrNoDirectory
}
