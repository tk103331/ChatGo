package ui

import (
	"testing"

	"chatgo/internal/ui/icons"
)

// TestNavIconsExist guards every icon name the navigation and the room pages
// pass to icons.Must, which panics on a miss: a missing icon would take the
// window down on the frame that first draws it, not at build time.
func TestNavIconsExist(t *testing.T) {
	for _, name := range []string{
		"message-square", "users", "clock", "bot", "brain",
		"file-text", "chart-column", "settings",
		"plus", "x", "folder", "pencil", "play", "ellipsis",
		"chevron-down", "sliders-horizontal", "search", "check",
	} {
		if _, ok := icons.Get(name); !ok {
			t.Errorf("icon %q is referenced by the UI but not embedded under icons/svg", name)
		}
	}
}
