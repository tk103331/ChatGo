package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuickPanelDefaults(t *testing.T) {
	var q QuickPanelConfig
	q.Ensure()
	if !q.IsEnabled() || !q.IsBackground() || !q.IsClipboardFallback() {
		t.Fatalf("defaults: enabled=%v background=%v clipboard=%v",
			q.IsEnabled(), q.IsBackground(), q.IsClipboardFallback())
	}
	if got := q.MinSelection(); got != 2 {
		t.Errorf("MinSelection = %d, want 2", got)
	}
}

func TestQuickPanelExplicitFalse(t *testing.T) {
	off := false
	q := QuickPanelConfig{Enabled: &off, Background: &off, ClipboardFallback: &off, MinChars: 5}
	q.Ensure()
	if q.IsEnabled() || q.IsBackground() || q.IsClipboardFallback() {
		t.Errorf("explicit false was overwritten")
	}
	if got := q.MinSelection(); got != 5 {
		t.Errorf("MinSelection = %d, want 5", got)
	}
}

func TestLoadFileQuickPanel(t *testing.T) {
	dir := t.TempDir()

	// A config written before the panel existed gets the defaults.
	old := filepath.Join(dir, "old.yaml")
	if err := os.WriteFile(old, []byte("current_provider: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFile(old)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.QuickPanel.IsEnabled() {
		t.Errorf("missing section did not default to enabled")
	}

	// Values in the file win, including an explicit false.
	path := filepath.Join(dir, "config.yaml")
	body := "quick_panel:\n  enabled: false\n  auto_popup: true\n  min_chars: 3\n  favorites:\n    - agent:Review\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.QuickPanel.IsEnabled() {
		t.Errorf("explicit enabled: false was not kept")
	}
	if !cfg.QuickPanel.AutoPopup {
		t.Errorf("auto_popup was not read")
	}
	if got := cfg.QuickPanel.MinSelection(); got != 3 {
		t.Errorf("MinSelection = %d, want 3", got)
	}
	if len(cfg.QuickPanel.Favorites) != 1 || cfg.QuickPanel.Favorites[0] != "agent:Review" {
		t.Errorf("favorites = %v", cfg.QuickPanel.Favorites)
	}
}
