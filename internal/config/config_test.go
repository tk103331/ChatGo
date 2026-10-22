package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestShowTrayIconDefaultsOn(t *testing.T) {
	path := writeTempConfig(t, "current_provider: \"\"\n")
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShowTrayIcon == nil {
		t.Fatal("ShowTrayIcon should be set to true by default")
	}
	if !*cfg.ShowTrayIcon {
		t.Error("ShowTrayIcon should default to true")
	}
}

func TestShowTrayIconExplicitOff(t *testing.T) {
	path := writeTempConfig(t, "show_tray_icon: false\n")
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShowTrayIcon == nil || *cfg.ShowTrayIcon {
		t.Errorf("explicit false should be kept, got %v", cfg.ShowTrayIcon)
	}
}

func TestShowTrayIconExplicitOn(t *testing.T) {
	path := writeTempConfig(t, "show_tray_icon: true\n")
	cfg, err := loadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ShowTrayIcon == nil || !*cfg.ShowTrayIcon {
		t.Error("explicit true should be kept")
	}
}
