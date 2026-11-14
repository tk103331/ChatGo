package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"chatgo/internal/config"
)

// packOneForTest zips one catalog source plugin and serves it as a
// release asset, returning the entry the index would carry and its URL.
func packOneForTest(t *testing.T, srcDir, id string) (CatalogEntry, string) {
	t.Helper()
	buf, sum, err := zipPluginDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	url := srv.URL + "/plugin-" + id + ".zip"
	return CatalogEntry{ID: id, Name: id, Version: "1.0.0", SHA256: sum}, url
}

// InstallCatalog downloads the entry's zip, checks its hash and installs
// it enabled; a swapped asset fails the check instead of running.
func TestInstallCatalogDownloadsAndVerifies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// One real catalog source plugin, packed and served like the release.
	src := filepath.Join("..", "..", "plugins", "catalog", "provider-openai")
	if _, err := os.Stat(src); err != nil {
		t.Skip("the catalog sources are not present")
	}
	cfg := &config.Config{}
	m := NewManager(cfg)
	entry, url := packOneForTest(t, src, "provider-openai")

	saved := catalogURLFor
	t.Cleanup(func() { catalogURLFor = saved })
	catalogURLFor = func(CatalogEntry) string { return url }

	manifest, err := m.InstallCatalog(context.Background(), entry, true)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if manifest.ID != "provider-openai" {
		t.Fatalf("the wrong plugin installed: %+v", manifest)
	}
	// The plugin is loaded and its catalog entry is served.
	if len(m.ProviderCatalog()) != 1 {
		t.Fatalf("the installed plugin's catalog did not register: %+v", m.ProviderCatalog())
	}
	if inst, ok := m.Info("provider-openai"); !ok || inst.Status != StatusLoaded {
		t.Fatalf("the plugin should be loaded: %+v", inst)
	}
	// The files landed under the plugin root, not any builtin directory.
	if _, err := os.Stat(filepath.Join(m.root, "provider-openai", "plugin.json")); err != nil {
		t.Fatalf("the plugin files did not land: %v", err)
	}

	// A swapped asset fails the hash check before anything installs.
	entry2, _ := packOneForTest(t, src, "provider-openai")
	entry2.SHA256 = "deadbeef"
	if _, err := m.InstallCatalog(context.Background(), entry2, true); err == nil {
		t.Fatal("a hash mismatch must fail the install")
	}
}

// catalogIDSafe rejects the path escapes an entry id could carry into the
// download URL or the install target.
func TestCatalogIDSafe(t *testing.T) {
	for _, bad := range []string{"", "../evil", "a/b", "..", `a\b`} {
		if catalogIDSafe(bad) {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	for _, ok := range []string{"provider-openai", "acp-cline", "importer-cursor-config"} {
		if !catalogIDSafe(ok) {
			t.Fatalf("%q should be accepted", ok)
		}
	}
}
