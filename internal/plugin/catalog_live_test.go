package plugin

// The end-to-end install path against the real release: the catalog index
// the app embeds, the URL the manager builds and the zip the plugins
// release serves. Skipped without network so a CI sandbox still passes.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"
)

// TestCatalogIndexAgainstLiveRelease walks the whole chain offline-first:
// every index entry names an id the download URL accepts, and one full
// round trip — download, hash check, install, catalog registration —
// works against the published release.
func TestCatalogIndexAgainstLiveRelease(t *testing.T) {
	entries := Catalog()
	if len(entries) < 80 {
		t.Fatalf("the embedded index should hold the catalog, got %d entries", len(entries))
	}
	// Every entry's id is URL- and filename-safe and its hash is a sha256.
	for _, e := range entries {
		if !catalogIDSafe(e.ID) {
			t.Fatalf("%q is not a safe id", e.ID)
		}
		if len(e.SHA256) != 64 {
			t.Fatalf("%q has a malformed hash %q", e.ID, e.SHA256)
		}
	}

	// The live round trip needs the network; skip when it is unreachable.
	if !netUp(t, CatalogDownloadURL(entries[0])) {
		t.Skip("the plugins release is unreachable; skipping the live round trip")
	}
	// Pick a real entry and install it through the production URL builder.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := newTestConfig()
	m := NewManager(cfg)
	var entry CatalogEntry
	for _, e := range entries {
		if e.ID == "provider-deepseek" {
			entry = e
			break
		}
	}
	if entry.ID == "" {
		t.Fatal("the catalog lacks provider-deepseek to install")
	}
	// The default catalogURLFor builds the release URL; keep it.
	manifest, err := m.InstallCatalog(context.Background(), entry, true)
	if err != nil {
		t.Fatalf("install from the live release failed: %v", err)
	}
	if manifest.ID != entry.ID {
		t.Fatalf("the wrong plugin installed: %+v", manifest)
	}
	// The plugin is loaded and offers its provider preset.
	if got := m.ProviderCatalog(); len(got) != 1 || got[0].Name != "DeepSeek" {
		t.Fatalf("the installed plugin's catalog did not register: %+v", got)
	}
	// The source records the release it came from.
	if info, ok := m.Info(entry.ID); !ok || info.Source.Kind != "zip" ||
		!strings.Contains(info.Source.URL, "releases/download/plugins/") {
		t.Fatalf("the install source should name the release: %+v", info.Source)
	}
}

// netUp reports whether a URL answers within a short wait. The release
// download answers with a redirect, so following to the CDN counts; the
// wait is generous because the CDN can be slow from some networks.
func netUp(t *testing.T, url string) bool {
	t.Helper()
	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// newTestConfig is a bare config for the manager.
func newTestConfig() *config.Config { return &config.Config{} }
