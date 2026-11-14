package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// FetchCatalogIndex downloads the release's index, validates every id and
// rejects an empty or malformed one.
func TestFetchCatalogIndex(t *testing.T) {
	good := []CatalogEntry{
		{ID: "provider-openai", Name: "OpenAI", Version: "1.0.0", SHA256: "ab"},
		{ID: "acp-cline", Name: "Cline", Version: "1.0.1", SHA256: "cd"},
	}
	body, _ := json.Marshal(good)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	saved := CatalogBase
	t.Cleanup(func() { CatalogBase = saved })
	CatalogBase = srv.URL

	got, err := FetchCatalogIndex(context.Background())
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(got) != 2 || got[1].ID != "acp-cline" || got[1].Version != "1.0.1" {
		t.Fatalf("unexpected entries: %+v", got)
	}
}

// A malformed or unsafe index is rejected whole: one bad id would poison
// the download URLs the market builds from the list.
func TestFetchCatalogIndexRejectsBadPayloads(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"not json", "nope"},
		{"empty list", "[]"},
		{"unsafe id", `[{"id":"../evil","version":"1"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			saved := CatalogBase
			t.Cleanup(func() { CatalogBase = saved })
			CatalogBase = srv.URL
			if _, err := FetchCatalogIndex(context.Background()); err == nil {
				t.Fatalf("%q must be rejected", tc.name)
			}
		})
	}
}

// PackCatalog is incremental: an unchanged plugin keeps its entry and zip,
// a bumped version repacks exactly one asset under its versioned name.
func TestPackCatalogIncremental(t *testing.T) {
	// Build a tiny catalog source: two plugins, one at 1.0.0.
	src := t.TempDir()
	write := func(id, version string) {
		dir := filepath.Join(src, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := map[string]any{"id": id, "name": id, "version": version}
		data, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte("// stub\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("provider-a", "1.0.0")
	write("provider-b", "1.0.0")
	out := t.TempDir()

	first, err := PackCatalog(src, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("the first pack should build both, got %d", len(first))
	}
	firstZips := dirZipCount(t, out)
	if firstZips != 2 {
		t.Fatalf("the first pack should write two zips, got %d", firstZips)
	}

	// Unchanged: nothing new is packed.
	second, err := PackCatalog(src, out, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || dirZipCount(t, out) != firstZips {
		t.Fatal("an unchanged catalog must not pack anything")
	}
	for _, e := range second {
		if e.Version != "1.0.0" {
			t.Fatalf("the kept entry changed: %+v", e)
		}
	}

	// Bump one: exactly one new zip, under its versioned name; the old
	// version's zip stays.
	write("provider-b", "1.1.0")
	third, err := PackCatalog(src, out, second)
	if err != nil {
		t.Fatal(err)
	}
	if dirZipCount(t, out) != firstZips+1 {
		t.Fatalf("a bumped plugin should add exactly one zip, got %d", dirZipCount(t, out))
	}
	if _, err := os.Stat(filepath.Join(out, "plugin-provider-b-1.1.0.zip")); err != nil {
		t.Fatalf("the versioned zip is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "plugin-provider-b-1.0.0.zip")); err != nil {
		t.Fatalf("the old version's zip must stay for older apps: %v", err)
	}
	for _, e := range third {
		if e.ID == "provider-b" && e.Version != "1.1.0" {
			t.Fatalf("the bumped entry did not update: %+v", e)
		}
	}
}

// dirZipCount counts the zips a pack directory holds.
func dirZipCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".zip" {
			n++
		}
	}
	return n
}
