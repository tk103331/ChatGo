package kb

import (
	"path/filepath"
	"testing"
)

// TestSearchSurvivesHostileQueries checks a user query cannot break the FTS5
// MATCH expression or the store: the segmenter quotes every term, so operator
// characters and quotes are searched for literally.
func TestSearchSurvivesHostileQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, _ := db.CreateKnowledgeBase("k", "")
	db.ImportText(id, "doc", "正常内容 hello world")

	for _, q := range []string{
		`"`, `""`, `*`, `AND`, `OR`, `NOT`, `(`, `)`, `a OR`, `NEAR(a b)`,
		`" OR 1=1 --`, `'; DROP TABLE chunk; --`, `^`, `-`, `a:b`, `{`, `\`,
		"a\x00b", "😀", "　", `col:*`,
	} {
		if _, err := db.Search(id, q, 5); err != nil {
			t.Errorf("query %q errored: %v", q, err)
		}
	}
	// The store must still work afterwards.
	if hits, err := db.Search(id, "正常内容", 5); err != nil || len(hits) != 1 {
		t.Fatalf("store broken after hostile queries: %d %v", len(hits), err)
	}
}
