package agentimport

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ImportResult reports what one import run did.
type ImportResult struct {
	Imported []string
	Skipped  []string
	Failed   []string
	Errors   []string
}

// maxSkillFileBytes bounds a single copied file, so a stray large asset in a
// skill directory cannot fill the disk unnoticed.
const maxSkillFileBytes = 16 << 20

// Import copies the named skills of a source into ~/.chatgo/skills. A skill
// whose directory already exists is skipped unless overwrite is set, so an
// import never silently replaces a local edit.
func Import(src Source, names []string, overwrite bool) (ImportResult, error) {
	var result ImportResult
	dest, err := HomeDir()
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return result, err
	}
	wanted := make(map[string]bool, len(names))
	for _, n := range names {
		wanted[n] = true
	}
	for _, found := range Discover(src) {
		if len(wanted) > 0 && !wanted[found.Name] {
			continue
		}
		target := filepath.Join(dest, found.Name)
		if _, err := os.Stat(target); err == nil {
			if !overwrite {
				result.Skipped = append(result.Skipped, found.Name)
				continue
			}
			if err := os.RemoveAll(target); err != nil {
				result.Failed = append(result.Failed, found.Name)
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", found.Name, err))
				continue
			}
		}
		if err := copyTree(found.Dir, target); err != nil {
			// A partial copy would look like a broken skill, so remove it.
			_ = os.RemoveAll(target)
			result.Failed = append(result.Failed, found.Name)
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", found.Name, err))
			continue
		}
		result.Imported = append(result.Imported, found.Name)
	}
	return result, nil
}

// copyTree copies a skill directory, skipping hidden entries and symlinks: a
// skill is plain files, and following links could escape the source tree.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSkillFileBytes {
			return fmt.Errorf("%s 超过 %d MB", rel, maxSkillFileBytes>>20)
		}
		return copyFile(path, filepath.Join(dst, rel), info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Summary describes an import run for the status line.
func (r ImportResult) Summary() string {
	if len(r.Imported) == 0 && len(r.Skipped) == 0 && len(r.Failed) == 0 {
		return "没有可导入的 Skill"
	}
	parts := []string{fmt.Sprintf("导入 %d 个", len(r.Imported))}
	if len(r.Skipped) > 0 {
		parts = append(parts, fmt.Sprintf("已存在跳过 %d 个", len(r.Skipped)))
	}
	if len(r.Failed) > 0 {
		msg := fmt.Sprintf("失败 %d 个", len(r.Failed))
		if len(r.Errors) > 0 {
			msg += "（" + r.Errors[0] + "）"
		}
		parts = append(parts, msg)
	}
	return strings.Join(parts, "，")
}
