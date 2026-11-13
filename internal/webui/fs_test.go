package webui

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// The fs listing walks the host's directories for the workspace picker: only
// subdirectories, sorted, with the parent for stepping back up.
func TestHandleFSListsDirectories(t *testing.T) {
	s, _ := testServer(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := serve(t, s, http.MethodGet, "/api/fs?dir="+root, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Dir     string   `json:"dir"`
		Parent  string   `json:"parent"`
		Entries []FSItem `json:"entries"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Dir != root {
		t.Fatalf("dir = %q, want %q", resp.Dir, root)
	}
	if len(resp.Entries) != 2 || resp.Entries[0].Name != "a" || resp.Entries[1].Name != "b" {
		t.Fatalf("entries = %+v (files must stay out)", resp.Entries)
	}
	if resp.Parent == "" || resp.Parent == root {
		t.Fatalf("parent = %q", resp.Parent)
	}
	// Walking into a subdirectory answers with its own listing.
	w = serve(t, s, http.MethodGet, "/api/fs?dir="+filepath.Join(root, "a"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("subdir status %d", w.Code)
	}
	// A missing directory is reported, not a panic.
	w = serve(t, s, http.MethodGet, "/api/fs?dir="+filepath.Join(root, "gone"), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing dir status %d", w.Code)
	}
	// A file is not a directory.
	w = serve(t, s, http.MethodGet, "/api/fs?dir="+filepath.Join(root, "file.txt"), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("file status %d", w.Code)
	}
}

// An empty dir starts the walk at the host user's home, so the first screen
// of the picker is never blank.
func TestHandleFSDefaultsToHome(t *testing.T) {
	s, _ := testServer(t)
	w := serve(t, s, http.MethodGet, "/api/fs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Dir string `json:"dir"`
	}
	if err := jsonUnmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if resp.Dir != home {
		t.Fatalf("dir = %q, want home %q", resp.Dir, home)
	}
}
