package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"chatgo/internal/netx"
)

// proxyTestHome points HOME at a temp dir, so LoadConfig and SaveConfig
// never touch the real ~/.chatgo.
func proxyTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// Loading a config applies its proxy to the shared HTTP clients, saving
// applies the change just as immediately, and the setting survives the
// round-trip through the file.
func TestProxyConfigAppliesAndPersists(t *testing.T) {
	t.Cleanup(func() { netx.SetProxy("") })
	home := proxyTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".chatgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".chatgo", "config.yaml"), []byte(`
current_provider: ""
proxy:
  enabled: true
  url: socks5://127.0.0.1:1080
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Proxy.EffectiveProxyURL(); got != "socks5://127.0.0.1:1080" {
		t.Fatalf("EffectiveProxyURL = %q", got)
	}
	if got := netx.ProxyURL(); got != "socks5://127.0.0.1:1080" {
		t.Fatalf("loading should apply the proxy, got %q", got)
	}

	// Turning the proxy off in the config and saving clears the applied
	// proxy without a restart.
	cfg.Proxy.Enabled = false
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := netx.ProxyURL(); got != "" {
		t.Fatalf("saving a disabled proxy should clear it, got %q", got)
	}
	data, err := os.ReadFile(filepath.Join(home, ".chatgo", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "url: socks5://127.0.0.1:1080") {
		t.Fatalf("the address should persist: %s", data)
	}

	// An address that cannot serve as a proxy is never applied.
	cfg.Proxy.Enabled = true
	cfg.Proxy.URL = "ftp://127.0.0.1:21"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if got := netx.ProxyURL(); got != "" {
		t.Fatalf("an invalid address must not become the proxy, got %q", got)
	}
}
