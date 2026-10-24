package agentimport

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDiscoverNeedsSkillFile(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "good", "# good")
	if err := os.MkdirAll(filepath.Join(dir, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loose.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := Discover(Source{Name: "x", Dirs: []string{dir}})
	if len(found) != 1 || found[0].Name != "good" {
		t.Fatalf("only a directory with SKILL.md is a skill, got %+v", found)
	}
}

func TestDiscoverDeduplicatesAcrossDirs(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeSkill(t, a, "dup", "# from a")
	writeSkill(t, b, "dup", "# from b")
	writeSkill(t, b, "only-b", "# b")
	found := Discover(Source{Name: "x", Dirs: []string{a, b}})
	if len(found) != 2 {
		t.Fatalf("expected 2 skills, got %+v", found)
	}
	for _, f := range found {
		if f.Name == "dup" && f.Dir != filepath.Join(a, "dup") {
			t.Fatalf("the first directory should win, got %q", f.Dir)
		}
	}
}

func TestImportCopiesTreeAndSkipsExisting(t *testing.T) {
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

	source := Source{Name: "Claude Code", Dirs: []string{src}}
	res, err := Import(source, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || res.Imported[0] != "demo" {
		t.Fatalf("unexpected result %+v", res)
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
	res, err = Import(source, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 0 || len(res.Skipped) != 1 {
		t.Fatalf("an existing skill must be skipped, got %+v", res)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); string(data) != "# edited" {
		t.Fatalf("the local edit was overwritten: %q", data)
	}

	// Overwrite replaces it.
	res, err = Import(source, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 {
		t.Fatalf("overwrite should import, got %+v", res)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); string(data) != "# demo\n\nbody" {
		t.Fatalf("overwrite did not replace the file: %q", data)
	}
}

func TestImportSelectsNamedSkillsOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := t.TempDir()
	writeSkill(t, src, "wanted", "# a")
	writeSkill(t, src, "other", "# b")

	res, err := Import(Source{Name: "x", Dirs: []string{src}}, []string{"wanted"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Imported) != 1 || res.Imported[0] != "wanted" {
		t.Fatalf("only the named skill should be imported, got %+v", res)
	}
	dest, _ := HomeDir()
	if _, err := os.Stat(filepath.Join(dest, "other")); err == nil {
		t.Fatal("an unnamed skill must not be imported")
	}
}

func TestAvailableHidesEmptySources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Only Claude Code has a skill, so only it is offered.
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "demo", "# demo")
	got := Available("")
	if len(got) != 1 {
		t.Fatalf("expected one usable source, got %+v", got)
	}
	if got[0].Source.Name != "Claude Code" || len(got[0].Skills) != 1 {
		t.Fatalf("unexpected source %+v", got[0])
	}
}

func TestSummaryDescribesRun(t *testing.T) {
	if got := (ImportResult{}).Summary(); got != "没有可导入的 Skill" {
		t.Fatalf("empty summary %q", got)
	}
	r := ImportResult{Imported: []string{"a"}, Skipped: []string{"b"}, Failed: []string{"c"}, Errors: []string{"c: boom"}}
	got := r.Summary()
	for _, want := range []string{"导入 1 个", "跳过 1 个", "失败 1 个", "c: boom"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q missing %q", got, want)
		}
	}
}

// TestImportRejectsPathEscape checks a crafted skill name cannot write outside
// ~/.chatgo/skills: Discover only yields directory entries, so a name with
// separators or dots never reaches the copy.
func TestImportRejectsPathEscape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := t.TempDir()
	writeSkill(t, src, "ok", "# ok")

	// Asking for names that try to escape must import nothing.
	for _, name := range []string{"../evil", "../../evil", "/etc/passwd", "..", "."} {
		res, err := Import(Source{Name: "x", Dirs: []string{src}}, []string{name}, true)
		if err != nil {
			t.Fatalf("name %q errored: %v", name, err)
		}
		if len(res.Imported) != 0 {
			t.Fatalf("name %q imported %v", name, res.Imported)
		}
	}
	// Nothing was written above the destination.
	if _, err := os.Stat(filepath.Join(home, ".chatgo", "evil")); err == nil {
		t.Fatal("a path escape wrote outside the skills directory")
	}
	if _, err := os.Stat(filepath.Join(home, "evil")); err == nil {
		t.Fatal("a path escape wrote into the home directory")
	}
}

// TestImportSkipsSymlinks keeps a symlinked file in a source skill from being
// followed, which could copy data from outside the source tree.
func TestImportSkipsSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := t.TempDir()
	skill := writeSkill(t, src, "demo", "# demo")

	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(skill, "leak.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := Import(Source{Name: "x", Dirs: []string{src}}, nil, false); err != nil {
		t.Fatal(err)
	}
	dest, _ := HomeDir()
	if _, err := os.Stat(filepath.Join(dest, "demo", "leak.md")); err == nil {
		t.Fatal("a symlink must not be copied")
	}
	if data, err := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md")); err != nil || string(data) != "# demo" {
		t.Fatalf("the real file should still be copied: %v %q", err, data)
	}
}
