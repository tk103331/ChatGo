// Package filemanagerapp is the Apps menu's file manager: a FileExplorer
// over the workspace with preview, rename, delete and Agent tools.
package filemanagerapp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mfiles "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/files"
)

// maxPreviewBytes caps a file's preview content.
const maxPreviewBytes = 1 << 20

// insideRoot reports whether p stays inside root after resolving.
func insideRoot(root, p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径 %s 超出工作目录", p)
	}
	return abs, nil
}

// readDir lists one directory as file entries.
func readDir(dir string, showHidden bool) ([]mfiles.FileEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]mfiles.FileEntry, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, mfiles.FileEntry{
			Path:     filepath.Join(dir, name),
			Name:     name,
			Dir:      e.IsDir(),
			Size:     info.Size(),
			Modified: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// listDirs returns a directory's subfolders.
func listDirs(dir string, showHidden bool) []string {
	entries, err := readDir(dir, showHidden)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.Dir {
			out = append(out, e.Path)
		}
	}
	return out
}

// readText reads a text file for preview.
func readText(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s 是目录", filepath.Base(path))
	}
	if info.Size() > maxPreviewBytes {
		return fmt.Sprintf("(文件过大 %s，仅显示前 1MB)", humanBytes(info.Size())), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if isBinary(data) {
		return "(二进制文件，无法预览)", nil
	}
	return string(data), nil
}

// isBinary reports whether data looks binary.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// writeText writes text to a file.
func writeText(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// makeDir creates a directory.
func makeDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// renamePath renames or moves a path.
func renamePath(oldPath, newPath string) error {
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

// removePath deletes a file or directory tree.
func removePath(path string) error {
	return os.RemoveAll(path)
}

// searchFiles walks a tree for names matching a substring.
func searchFiles(root, pattern string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	needle := strings.ToLower(pattern)
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if len(out) >= limit {
			return filepath.SkipAll
		}
		if p == root {
			return nil
		}
		if strings.Contains(strings.ToLower(info.Name()), needle) {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	return out, err
}

// humanBytes formats a byte count for display.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// formatTime renders a modification time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}
