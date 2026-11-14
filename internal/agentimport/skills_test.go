package agentimport

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkill creates a minimal skill directory.
func writeSkill(t *testing.T, dir, name, body string) string {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestSearchDirsPrefersChatGo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dirs := SearchDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected the two home directories, got %v", dirs)
	}
	if dirs[0] != filepath.Join(home, ".chatgo", "skills") {
		t.Fatalf("~/.chatgo/skills must come first, got %v", dirs)
	}
	if dirs[1] != filepath.Join(home, ".agents", "skills") {
		t.Fatalf("~/.agents/skills must come second, got %v", dirs)
	}
	got, err := HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dirs[0] {
		t.Fatalf("HomeDir should be the primary search dir, got %q", got)
	}
}

func TestImportDirCopiesTreeAndSkipsExisting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := t.TempDir()
	skill := writeSkill(t, src, "demo", "# demo\n\nbody")
	if err := os.MkdirAll(filepath.Join(skill, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "references", "a.md"), []byte("ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Hidden files are an agent's own metadata and must not be copied.
	if err := os.WriteFile(filepath.Join(skill, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := ImportDir(skill, "demo", false)
	if err != nil || !ok {
		t.Fatalf("import failed: %v %v", err, ok)
	}
	dest, _ := HomeDir()
	if data, err := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); err != nil || string(data) != "# demo\n\nbody" {
		t.Fatalf("SKILL.md not copied: %v %q", err, data)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "demo", "references", "a.md")); err != nil || string(data) != "ref" {
		t.Fatalf("nested file not copied: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo", ".DS_Store")); err == nil {
		t.Fatal("hidden files must not be copied")
	}

	// A second run leaves the local copy alone.
	if err := os.WriteFile(filepath.Join(dest, "demo", "SKILL.md"), []byte("# edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = ImportDir(skill, "demo", false)
	if err != nil || ok {
		t.Fatalf("an existing skill must be skipped, got %v %v", ok, err)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); string(data) != "# edited" {
		t.Fatalf("the local edit was overwritten: %q", data)
	}

	// Overwrite replaces the copy when asked.
	ok, err = ImportDir(skill, "demo", true)
	if err != nil || !ok {
		t.Fatalf("overwrite import failed: %v %v", err, ok)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); string(data) != "# demo\n\nbody" {
		t.Fatalf("the overwrite did not restore the source: %q", data)
	}
}
