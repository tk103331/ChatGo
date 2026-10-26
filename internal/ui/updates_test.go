package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// A newly found update version changes the tray menu key, so the cached
// menu is rebuilt and the "发现新版本" entry appears.
func TestTrayMenuKeyTracksUpdate(t *testing.T) {
	cw := testWindow()
	before := cw.trayMenuKey()
	cw.updateVer = "9.9.9"
	after := cw.trayMenuKey()
	if before == after {
		t.Fatalf("the tray menu key ignores updateVer %q", cw.updateVer)
	}
}

// The tray menu surfaces a known update as an entry that opens the update
// window, placed ahead of the plain check item.
func TestTrayMenuShowsUpdateEntry(t *testing.T) {
	cw := testWindow()
	cw.updateVer = "9.9.9"
	items := cw.buildTrayMenu().Items()
	var hasUpdate, hasCheck bool
	for _, it := range items {
		if strings.HasPrefix(it.Label, "发现新版本 v9.9.9") {
			hasUpdate = true
		}
		if it.Label == "检查更新…" {
			hasCheck = true
		}
	}
	if !hasUpdate {
		t.Fatalf("the tray menu lacks the update entry; items: %v", menuLabels(items))
	}
	if !hasCheck {
		t.Fatalf("the tray menu lacks the check entry; items: %v", menuLabels(items))
	}
}

func menuLabels(items []*mygo.MenuItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}
