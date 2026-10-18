package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithinRejectsEscape(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../x", "/etc/passwd", "a/../../x"} {
		if _, err := Within(root, rel); err == nil {
			t.Fatalf("escape %q should be rejected", rel)
		}
	}
	if _, err := Within(root, "ok/.."); err != nil {
		t.Fatalf("clean path should be allowed: %v", err)
	}
}

func TestWriteReadAppend(t *testing.T) {
	root := t.TempDir()
	if err := Write(root, "sub/a.txt", "hi"); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, "sub/a.txt")
	if err != nil || got != "hi" {
		t.Fatalf("read: %q %v", got, err)
	}
	if err := Append(root, "sub/a.txt", "!"); err != nil {
		t.Fatal(err)
	}
	got, _ = Read(root, "sub/a.txt")
	if got != "hi!" {
		t.Fatalf("append: %q", got)
	}
}

func TestListSorted(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644)
	out, err := List(root, "")
	if err != nil || out != "a.txt\nsub/" {
		t.Fatalf("list: %q %v", out, err)
	}
	os.MkdirAll(filepath.Join(root, "empty"), 0o755)
	if out, _ := List(root, "empty"); out != "(空目录)" {
		t.Fatalf("empty dir: %q", out)
	}
}

func TestMkdirRenameDelete(t *testing.T) {
	root := t.TempDir()
	if err := Mkdir(root, "d1/d2"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "d1/d2")); err != nil || !info.IsDir() {
		t.Fatal("mkdir failed")
	}
	os.WriteFile(filepath.Join(root, "old.txt"), []byte("x"), 0o644)
	if err := Rename(root, "old.txt", "d1/new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d1/new.txt")); err != nil {
		t.Fatal("rename failed")
	}
	if err := Delete(root, "d1/new.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d1/new.txt")); !os.IsNotExist(err) {
		t.Fatal("delete failed")
	}
	if err := Delete(root, "d1"); err == nil {
		t.Fatal("directory delete should fail")
	}
}

func TestSearch(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.go"), []byte("func Hello() {}\n"), 0o644)
	os.WriteFile(filepath.Join(root, "b.txt"), []byte("hello world\nHELLO again\n"), 0o644)
	os.MkdirAll(filepath.Join(root, ".git"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "x"), []byte("hello in git\n"), 0o644)
	out, err := Search(root, "", "hello", SearchOpts{CaseInsensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "b.txt:1:") || !strings.Contains(out, "b.txt:2:") {
		t.Fatalf("search miss: %q", out)
	}
	if strings.Contains(out, ".git") {
		t.Fatalf("search must skip .git: %q", out)
	}
	if !strings.Contains(out, "a.go:1:") {
		t.Fatalf("ci match: %q", out)
	}
	out, _ = Search(root, "", "hello", SearchOpts{CaseInsensitive: true, MaxLines: 2})
	if strings.Count(out, "\n") >= 3 {
		t.Fatalf("max lines: %q", out)
	}
	out, err = Search(root, "", "zzz", SearchOpts{})
	if err != nil || out != "(无匹配)" {
		t.Fatalf("no match: %q %v", out, err)
	}
	out, _ = Search(root, "b.txt", "^hello", SearchOpts{Regexp: true})
	if !strings.Contains(out, "b.txt:1:") {
		t.Fatalf("regexp: %q", out)
	}
}
