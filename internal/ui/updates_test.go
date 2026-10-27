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

// mygo.Updater.Check reports an up-to-date app as a nil update with no
// error, which must not be dereferenced: doing so crashed every launch of
// a packaged build, since an installed app is usually on the latest version.
func TestApplyUpdateWithNoUpdateAvailable(t *testing.T) {
	cw := testWindow()
	cw.dispatch = func(fn func()) { fn() }
	cw.applyUpdate(nil)
	if cw.updateVer != "" {
		t.Fatalf("a nil update set updateVer to %q", cw.updateVer)
	}
}

// An update the user has since installed stops being reported, which clears
// the tray prompt left over from it.
func TestApplyUpdateClearsInstalledUpdate(t *testing.T) {
	cw := testWindow()
	cw.dispatch = func(fn func()) { fn() }
	cw.applyUpdate(&mygo.Update{Version: "9.9.9"})
	if cw.updateVer != "9.9.9" {
		t.Fatalf("updateVer is %q, want 9.9.9", cw.updateVer)
	}
	cw.applyUpdate(nil)
	if cw.updateVer != "" {
		t.Fatalf("updateVer is %q after the update went away, want empty", cw.updateVer)
	}
	for _, label := range menuLabels(cw.buildTrayMenu().Items()) {
		if strings.HasPrefix(label, "发现新版本") {
			t.Fatalf("the tray menu still prompts for an installed update: %q", label)
		}
	}
}
