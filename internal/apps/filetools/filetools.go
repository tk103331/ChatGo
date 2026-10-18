// Package filetools holds the file operations the Markdown and code editors
// share, every one of them confined to a workspace root.
package filetools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaxRead = 1 << 20

// Within joins rel to root, rejecting any path that escapes root.
func Within(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("路径必须相对: %s", rel)
	}
	base := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(base, rel))
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("路径越权: %s", rel)
	}
	return abs, nil
}

// Read returns a file's text, capped at MaxRead.
func Read(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if len(data) > MaxRead {
		data = data[:MaxRead]
	}
	return string(data), nil
}

// Write overwrites a file, creating its parent directories.
func Write(root, rel, content string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// Append adds content to a file's end, creating parent directories.
func Append(root, rel, content string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// List names a directory's entries, sorted, directories suffixed with "/".
func List(root, rel string) (string, error) {
	abs, err := Within(root, rel)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "(空目录)", nil
	}
	return strings.Join(names, "\n"), nil
}

// Mkdir creates a directory and its parents.
func Mkdir(root, rel string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

// Rename moves a file (or directory), creating the destination's parents.
func Rename(root, old, new string) error {
	if strings.TrimSpace(old) == "" || strings.TrimSpace(new) == "" {
		return fmt.Errorf("缺少路径")
	}
	from, err := Within(root, old)
	if err != nil {
		return err
	}
	to, err := Within(root, new)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Delete removes a file only; directories are refused.
func Delete(root, rel string) error {
	if strings.TrimSpace(rel) == "" {
		return fmt.Errorf("缺少路径")
	}
	abs, err := Within(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("不能删除目录: %s", rel)
	}
	return os.Remove(abs)
}

// SearchOpts tune a Search.
type SearchOpts struct {
	Regexp          bool
	CaseInsensitive bool
	MaxLines        int
}

// Search recursively greps text under root for pattern, skipping VCS and
// dependency directories, emitting "rel:line: text" rows up to MaxLines.
func Search(root, rel, pattern string, opts SearchOpts) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("缺少搜索内容")
	}
	if opts.MaxLines <= 0 {
		opts.MaxLines = 100
	}
	re, err := compilePattern(pattern, opts.Regexp, opts.CaseInsensitive)
	if err != nil {
		return "", err
	}
	abs, err := Within(root, rel)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	lines := 0
	_ = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != abs && skippedDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		for line, row := range strings.Split(string(data), "\n") {
			if lines >= opts.MaxLines {
				return filepath.SkipAll
			}
			if !re.MatchString(row) {
				continue
			}
			row = strings.TrimRight(row, "\r")
			if lines > 0 {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "%s:%d: %s", relPath, line+1, row)
			lines++
		}
		return nil
	})
	if lines == 0 {
		return "(无匹配)", nil
	}
	return b.String(), nil
}

func compilePattern(pattern string, useRegexp, ci bool) (*regexp.Regexp, error) {
	if !useRegexp {
		pattern = regexp.QuoteMeta(pattern)
	}
	if ci {
		pattern = "(?i:" + pattern + ")"
	}
	return regexp.Compile(pattern)
}

func skippedDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor":
		return true
	}
	return false
}
