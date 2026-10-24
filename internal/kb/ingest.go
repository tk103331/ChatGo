package kb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// textExtensions are the file types imported as text knowledge.
var textExtensions = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".text": true,
	".rst": true, ".adoc": true, ".org": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".ini": true, ".conf": true, ".csv": true, ".log": true,
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".java": true, ".kt": true, ".rs": true, ".c": true, ".h": true, ".cc": true,
	".cpp": true, ".hpp": true, ".cs": true, ".rb": true, ".php": true,
	".sh": true, ".sql": true, ".html": true, ".css": true,
}

const maxFileBytes = 8 << 20

// IngestResult reports what one import did.
type IngestResult struct {
	Added   int
	Updated int
	Skipped int
	Failed  int
	Errors  []string
}

func (r *IngestResult) merge(other IngestResult) {
	r.Added += other.Added
	r.Updated += other.Updated
	r.Skipped += other.Skipped
	r.Failed += other.Failed
	r.Errors = append(r.Errors, other.Errors...)
}

// ImportFiles indexes the listed files. Files whose content hash is unchanged
// are skipped.
func (d *DB) ImportFiles(kbID string, paths []string) (IngestResult, error) {
	var result IngestResult
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if info.IsDir() {
			sub, err := d.importDir(kbID, path)
			result.merge(sub)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			}
			continue
		}
		res, err := d.importFile(kbID, path, info)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		result.merge(res)
	}
	return result, nil
}

// importDir walks a directory and imports every supported text file.
func (d *DB) importDir(kbID, dir string) (IngestResult, error) {
	var result IngestResult
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.Failed++
			return nil
		}
		if entry.IsDir() {
			if path != dir && SkipDirName(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if IndexableFile(entry.Name()) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	for _, path := range files {
		info, statErr := os.Stat(path)
		if statErr != nil {
			result.Failed++
			continue
		}
		res, importErr := d.importFile(kbID, path, info)
		if importErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, importErr))
			continue
		}
		result.merge(res)
	}
	return result, err
}

// importFile indexes one file, skipping it when its content is unchanged.
func (d *DB) importFile(kbID, path string, info fs.FileInfo) (IngestResult, error) {
	var result IngestResult
	abs, err := filepath.Abs(path)
	if err != nil {
		return result, err
	}
	if info.Size() > maxFileBytes {
		return result, fmt.Errorf("文件超过 %d MB", maxFileBytes>>20)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return result, err
	}
	hash := contentHash(data)
	existing, ok, err := d.docByPath(kbID, abs)
	if err != nil {
		return result, err
	}
	if ok && existing.Hash == hash {
		result.Skipped++
		return result, nil
	}
	doc := Document{
		ID:     newID(),
		KBID:   kbID,
		Title:  titleOf(abs, string(data)),
		Source: "file",
		Path:   abs,
		Hash:   hash,
		Size:   info.Size(),
	}
	if ok {
		doc.ID = existing.ID
		doc.CreatedAt = existing.CreatedAt
	}
	if err := d.saveDocument(doc, SplitDocument(string(data))); err != nil {
		return result, err
	}
	if ok {
		result.Updated++
	} else {
		result.Added++
	}
	return result, nil
}

// ImportText indexes pasted text as a document of its own.
func (d *DB) ImportText(kbID, title, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("内容为空")
	}
	if strings.TrimSpace(title) == "" {
		title = "未命名文本"
	}
	doc := Document{
		ID:     newID(),
		KBID:   kbID,
		Title:  title,
		Source: "paste",
		Hash:   contentHash([]byte(text)),
		Size:   int64(len(text)),
	}
	if err := d.saveDocument(doc, SplitDocument(text)); err != nil {
		return "", err
	}
	return doc.ID, nil
}

// Reindex rebuilds every file document of a knowledge base from disk, so
// changed files are picked up and deleted files are dropped.
func (d *DB) Reindex(kbID string) (IngestResult, error) {
	var result IngestResult
	docs, err := d.ListDocuments(kbID)
	if err != nil {
		return result, err
	}
	for _, doc := range docs {
		if doc.Source != "file" {
			continue
		}
		info, statErr := os.Stat(doc.Path)
		if statErr != nil {
			if err := d.DeleteDocument(doc.ID); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", doc.Path, err))
				continue
			}
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: 文件已不存在，已移除", doc.Path))
			continue
		}
		// Drop the stored hash so an unchanged file is re-read anyway: a full
		// reindex is also the way to rebuild after a segmentation change.
		if _, err := d.conn.Exec(`UPDATE doc SET hash='' WHERE id=?`, doc.ID); err != nil {
			return result, err
		}
		res, importErr := d.importFile(kbID, doc.Path, info)
		if importErr != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", doc.Path, importErr))
			continue
		}
		result.merge(res)
	}
	return result, nil
}

// titleOf names a document after its first Markdown heading, or its file name.
func titleOf(path, content string) string {
	for _, p := range splitParagraphs(content) {
		if h, _, ok := markdownHeading(p); ok {
			return h
		}
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// skipDirs are directories never walked when importing: version control,
// dependency trees and build output carry no knowledge worth indexing and
// would otherwise dwarf the real documents.
var skipDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true, ".bzr": true,
	"node_modules": true, "vendor": true, "bower_components": true,
	".venv": true, "venv": true, "__pycache__": true, ".tox": true,
	".idea": true, ".vscode": true, ".vs": true,
	"dist": true, "build": true, "out": true, "target": true,
	".next": true, ".nuxt": true, ".cache": true, ".gradle": true,
	".terraform": true, ".pytest_cache": true, ".mypy_cache": true,
	".ruff_cache": true, ".turbo": true, ".parcel-cache": true,
}

// skipFiles are files that match a text extension but hold no prose: lock
// files and editor or OS metadata.
var skipFiles = map[string]bool{
	".ds_store": true, "thumbs.db": true, "desktop.ini": true,
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"bun.lock": true, "bun.lockb": true, "composer.lock": true,
	"cargo.lock": true, "poetry.lock": true, "pipfile.lock": true,
	"gemfile.lock": true, "go.sum": true, "packages.lock.json": true,
}

// SkipDirName reports whether a directory is skipped while importing: hidden
// directories and the well-known dependency, build and tooling trees.
func SkipDirName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return skipDirs[strings.ToLower(name)]
}

// IndexableFile reports whether a file is imported as text: a supported
// extension, not hidden and not a lock or metadata file.
func IndexableFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	if skipFiles[lower] {
		return false
	}
	if strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css") {
		return false
	}
	return textExtensions[strings.ToLower(filepath.Ext(name))]
}
