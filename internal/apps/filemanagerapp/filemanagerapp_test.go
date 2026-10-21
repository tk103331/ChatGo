package filemanagerapp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsideRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := insideRoot(root, filepath.Join(root, "a", "b")); err != nil {
		t.Errorf("inside path rejected: %v", err)
	}
	if _, err := insideRoot(root, filepath.Join(root, "..", "escape")); err == nil {
		t.Error("escaping path should be rejected")
	}
}

func TestReadDirAndSearch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := readDir(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("readDir = %d entries, want 2", len(entries))
	}
	if !entries[0].Dir || entries[0].Name != "sub" {
		t.Errorf("first entry = %+v, want sub dir", entries[0])
	}
	dirs := listDirs(root, true)
	if len(dirs) != 1 {
		t.Errorf("listDirs = %v, want 1", dirs)
	}
	hits, err := searchFiles(root, "hello", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0] != "hello.txt" {
		t.Errorf("searchFiles = %v, want [hello.txt]", hits)
	}
}

func TestReadText(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(textPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := readText(textPath); err != nil || got != "hello" {
		t.Errorf("readText = %q, %v", got, err)
	}
	binPath := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(binPath, []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := readText(binPath); got != "(二进制文件，无法预览)" {
		t.Errorf("readText binary = %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(512); got != "512 B" {
		t.Errorf("humanBytes(512) = %q", got)
	}
	if got := humanBytes(2048); got != "2.0 KB" {
		t.Errorf("humanBytes(2048) = %q", got)
	}
}
