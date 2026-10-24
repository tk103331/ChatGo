package kb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSegmentSplitsHanIntoBigrams(t *testing.T) {
	if got := Segment("知识库检索"); got != "知识 识库 库检 检索" {
		t.Fatalf("han bigrams got %q", got)
	}
	if got := Segment("Hello FTS5 索引"); got != "hello fts5 索引" {
		t.Fatalf("mixed text got %q", got)
	}
	if got := Segment("的 了 在"); got != "" {
		t.Fatalf("stopwords should be dropped, got %q", got)
	}
}

func TestSplitDocumentUsesHeadings(t *testing.T) {
	text := "# 方案\n\n使用 FTS5 全文索引。\n\n## 分词\n\n中文采用二元切分。\n"
	chunks := SplitDocument(text)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Heading != "方案" || strings.Contains(chunks[0].Text, "#") {
		t.Fatalf("first chunk %+v", chunks[0])
	}
	if chunks[1].Heading != "分词" || chunks[1].Text != "中文采用二元切分。" {
		t.Fatalf("second chunk %+v", chunks[1])
	}
}

func TestSplitDocumentSplitsLongParagraph(t *testing.T) {
	long := strings.Repeat("长", maxChunkRunes*2)
	chunks := SplitDocument(long)
	if len(chunks) < 2 {
		t.Fatalf("expected the long paragraph to be split, got %d", len(chunks))
	}
	for _, c := range chunks {
		if runeLen(c.Text) > maxChunkRunes {
			t.Fatalf("chunk of %d runes exceeds the limit", runeLen(c.Text))
		}
	}
}

func TestImportAndSearch(t *testing.T) {
	db := openTestDB(t)
	id, err := db.CreateKnowledgeBase("库", "描述")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	body := "# 本地知识库\n\n使用 FTS5 建立全文索引，支持增量更新。\n\n## 分词\n\n中文按二元切分处理。\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := db.ImportFiles(id, []string{path})
	if err != nil || res.Added != 1 {
		t.Fatalf("import got %+v err=%v", res, err)
	}

	for _, q := range []string{"知识库", "全文索引", "二元切分", "增量更新"} {
		hits, err := db.Search(id, q, 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) == 0 {
			t.Fatalf("query %q returned no hits", q)
		}
	}
	if hits, _ := db.Search(id, "完全无关的词汇xyz", 5); len(hits) != 0 {
		t.Fatalf("unrelated query returned %d hits", len(hits))
	}
}

func TestImportSkipsUnchangedAndUpdatesChanged(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	path := filepath.Join(t.TempDir(), "a.md")
	os.WriteFile(path, []byte("# 标题\n\n第一版内容。\n"), 0o644)

	if res, _ := db.ImportFiles(id, []string{path}); res.Added != 1 {
		t.Fatalf("first import %+v", res)
	}
	if res, _ := db.ImportFiles(id, []string{path}); res.Skipped != 1 {
		t.Fatalf("unchanged file should be skipped, got %+v", res)
	}
	os.WriteFile(path, []byte("# 标题\n\n第二版内容，新增向量检索说明。\n"), 0o644)
	if res, _ := db.ImportFiles(id, []string{path}); res.Updated != 1 {
		t.Fatalf("changed file should be updated, got %+v", res)
	}
	docs, _ := db.ListDocuments(id)
	if len(docs) != 1 {
		t.Fatalf("re-import must replace the document, got %d", len(docs))
	}
	if hits, _ := db.Search(id, "向量检索", 5); len(hits) == 0 {
		t.Fatal("new content is not searchable")
	}
	if hits, _ := db.Search(id, "第一版", 5); len(hits) != 0 {
		t.Fatal("stale chunks were left behind")
	}
}

func TestImportDirectory(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("Alpha 文档"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("Beta 文档"), 0o644)
	os.WriteFile(filepath.Join(dir, "skip.png"), []byte("binary"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "c.md"), []byte("hidden"), 0o644)

	res, err := db.ImportFiles(id, []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 {
		t.Fatalf("expected 2 text files, got %+v", res)
	}
}

func TestDeleteKnowledgeBaseRemovesIndex(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	db.ImportText(id, "标题", "待删除的知识库内容")
	if hits, _ := db.Search("", "待删除", 5); len(hits) == 0 {
		t.Fatal("content should be searchable before delete")
	}
	if err := db.DeleteKnowledgeBase(id); err != nil {
		t.Fatal(err)
	}
	if hits, _ := db.Search("", "待删除", 5); len(hits) != 0 {
		t.Fatalf("index not cleaned, got %d hits", len(hits))
	}
	bases, _ := db.ListKnowledgeBases()
	if len(bases) != 0 {
		t.Fatalf("knowledge base still listed: %+v", bases)
	}
}

func TestSearchScopesToKnowledgeBase(t *testing.T) {
	db := openTestDB(t)
	a, _ := db.CreateKnowledgeBase("A", "")
	b, _ := db.CreateKnowledgeBase("B", "")
	db.ImportText(a, "甲", "只属于甲库的独特内容")
	db.ImportText(b, "乙", "只属于乙库的独特内容")

	if hits, _ := db.Search(a, "甲库", 5); len(hits) != 1 {
		t.Fatalf("scoped search got %d", len(hits))
	}
	if hits, _ := db.Search(a, "乙库", 5); len(hits) != 0 {
		t.Fatalf("search leaked across knowledge bases: %d hits", len(hits))
	}
	if hits, _ := db.Search("", "独特内容", 5); len(hits) != 2 {
		t.Fatalf("global search got %d, want 2", len(hits))
	}
}

func TestReindexDropsMissingFiles(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "gone.md")
	os.WriteFile(path, []byte("# 待移除\n\n内容"), 0o644)
	db.ImportFiles(id, []string{path})
	os.Remove(path)

	if _, err := db.Reindex(id); err != nil {
		t.Fatal(err)
	}
	docs, _ := db.ListDocuments(id)
	if len(docs) != 0 {
		t.Fatalf("missing file should be dropped, got %+v", docs)
	}
}

func TestFormatIncludesSource(t *testing.T) {
	out := Format([]Result{{Doc: "手册", Heading: "安装", Path: "/tmp/a.md", Text: "执行 go build"}}, 0)
	for _, want := range []string{"手册", "安装", "/tmp/a.md", "go build"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatted output missing %q: %s", want, out)
		}
	}
	if Format(nil, 0) == "" {
		t.Fatal("empty results should still explain themselves")
	}
}

func TestIndexableFileAndSkipDir(t *testing.T) {
	for _, name := range []string{"README.md", "main.go", "notes.txt", "conf.yaml"} {
		if !IndexableFile(name) {
			t.Fatalf("%q should be indexable", name)
		}
	}
	for _, name := range []string{
		".DS_Store", ".gitignore", ".env", "yarn.lock", "go.sum",
		"package-lock.json", "Thumbs.db", "app.min.js", "logo.png", "a.zip",
	} {
		if IndexableFile(name) {
			t.Fatalf("%q must be skipped", name)
		}
	}
	for _, name := range []string{".git", ".svn", ".hg", "node_modules", "vendor", "dist", "__pycache__", ".idea"} {
		if !SkipDirName(name) {
			t.Fatalf("directory %q must be skipped", name)
		}
	}
	if SkipDirName("docs") {
		t.Fatal("docs must be walked")
	}
}

func TestImportDirectorySkipsNoiseFiles(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	dir := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(body), 0o644)
	}
	write("keep.md", "# 保留\n\n正文")
	write("docs/also.md", "# 子目录\n\n正文")
	write(".DS_Store", "junk")
	write(".gitignore", "junk")
	write("yarn.lock", "junk")
	write("go.sum", "junk")
	write("bundle.min.js", "junk")
	write("node_modules/dep/index.js", "junk")
	write(".git/config", "junk")
	write("vendor/lib/a.go", "junk")

	res, err := db.ImportFiles(id, []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 {
		docs, _ := db.ListDocuments(id)
		titles := make([]string, 0, len(docs))
		for _, doc := range docs {
			titles = append(titles, doc.Path)
		}
		t.Fatalf("expected 2 documents, got %+v: %v", res, titles)
	}
}

func TestListChunksReturnsReadingOrder(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	docID, err := db.ImportText(id, "手册", "# 一\n\n第一节内容。\n\n# 二\n\n第二节内容。\n")
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := db.ListChunks(docID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	if chunks[0].Seq != 0 || chunks[0].Heading != "一" || chunks[1].Heading != "二" {
		t.Fatalf("unexpected chunk order: %+v", chunks)
	}
}

func TestSearchDocumentScopesToOneDocument(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.CreateKnowledgeBase("库", "")
	a, _ := db.ImportText(id, "甲", "甲文档提到检索实现细节。")
	db.ImportText(id, "乙", "乙文档也提到检索实现细节。")

	if hits, _ := db.Search(id, "检索实现", 10); len(hits) != 2 {
		t.Fatalf("knowledge-base search got %d, want 2", len(hits))
	}
	hits, err := db.SearchDocument(a, "检索实现", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Doc != "甲" {
		t.Fatalf("document search got %+v", hits)
	}
	if hits, _ := db.SearchDocument(a, "无关词xyz", 10); len(hits) != 0 {
		t.Fatalf("unrelated document query got %d hits", len(hits))
	}
}
