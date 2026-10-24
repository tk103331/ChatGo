package ui

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"chatgo/internal/kb"

	mgo "github.com/egoist/mygo/ui"
)

// knowledgeTestWindow is a window whose knowledge store lives in a temp dir,
// so rendering never touches the user's real ~/.chatgo. Knowledge loads run on
// worker goroutines, so the window collects their main-thread callbacks and
// drainKnowledge runs them.
func knowledgeTestWindow(t *testing.T) (*ChatWindow, func()) {
	t.Helper()
	cw := testWindow()
	db, err := kb.Open(filepath.Join(t.TempDir(), "kb.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cw.knowledge.db = db
	cw.section = sectionKnowledge

	var mu sync.Mutex
	var pending []func()
	cw.dispatch = func(fn func()) {
		mu.Lock()
		pending = append(pending, fn)
		mu.Unlock()
	}
	drain := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			mu.Lock()
			queued := pending
			pending = nil
			mu.Unlock()
			for _, fn := range queued {
				fn()
			}
			if len(queued) > 0 {
				continue
			}
			if !cw.knowledgeLoading() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("knowledge load did not settle")
			}
			time.Sleep(time.Millisecond)
		}
	}
	return cw, drain
}

func TestKnowledgeEmptyPageRenders(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	cw.ensureKnowledgeBases()
	drain()
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("knowledge render failed: %v", img)
	}
}

func TestKnowledgeCardsPageRenders(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	id, err := cw.knowledge.db.CreateKnowledgeBase("手册", "产品说明")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cw.knowledge.db.ImportText(id, "安装", "# 安装\n\n执行 go build 即可。"); err != nil {
		t.Fatal(err)
	}
	cw.reloadKnowledgeBases()
	drain()
	if len(cw.knowledge.bases) != 1 {
		t.Fatalf("expected the knowledge base to load, got %+v", cw.knowledge.bases)
	}
	if cw.knowledge.detail {
		t.Fatal("the cards page must be the entry view")
	}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("cards render failed: %v", img)
	}
}

func TestKnowledgeDetailRendersFilesAndChunks(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	id, _ := cw.knowledge.db.CreateKnowledgeBase("手册", "")
	docID, err := cw.knowledge.db.ImportText(id, "安装", "# 安装\n\n执行 go build。\n\n# 部署\n\n设置环境变量。")
	if err != nil {
		t.Fatal(err)
	}
	cw.reloadKnowledgeBases()
	drain()
	cw.selectKnowledgeBase(id)
	if !cw.knowledge.detail {
		t.Fatal("selecting a knowledge base must open its detail view")
	}
	drain()
	cw.selectKnowledgeDoc(docID)
	drain()
	if len(cw.knowledge.chunks) != 2 {
		t.Fatalf("expected the document chunks, got %+v", cw.knowledge.chunks)
	}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil || img.Bounds().Dx() != 1280 {
		t.Fatalf("detail render failed: %v", img)
	}
	cw.closeKnowledgeDetail()
	if cw.knowledge.detail {
		t.Fatal("closing the detail view must go back to the cards")
	}
}

func TestKnowledgeSearchScopesToSelectedDocument(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	id, _ := cw.knowledge.db.CreateKnowledgeBase("手册", "")
	a, _ := cw.knowledge.db.ImportText(id, "甲", "甲文件提到部署步骤。")
	cw.knowledge.db.ImportText(id, "乙", "乙文件也提到部署步骤。")
	cw.reloadKnowledgeBases()
	drain()
	cw.selectKnowledgeBase(id)
	drain()
	b, _ := cw.currentKnowledgeBase()

	cw.knowledge.query = "部署步骤"
	cw.openKnowledgeSearch("")
	cw.runKnowledgeSearch(b)
	drain()
	if len(cw.knowledge.results) != 2 {
		t.Fatalf("knowledge-base scope got %d results, want 2", len(cw.knowledge.results))
	}

	cw.selectKnowledgeDoc(a)
	drain()
	cw.openKnowledgeSearch(a)
	if cw.knowledge.searched {
		t.Fatal("changing the scope must clear the previous results")
	}
	cw.knowledge.query = "部署步骤"
	cw.runKnowledgeSearch(b)
	drain()
	if len(cw.knowledge.results) != 1 || cw.knowledge.results[0].Doc != "甲" {
		t.Fatalf("document scope got %+v", cw.knowledge.results)
	}
	img := mgo.Render(cw.view, 1280, 820, 1)
	if img == nil {
		t.Fatal("search dialog render failed")
	}
}

func TestKnowledgeSearchToolResolvesBaseByName(t *testing.T) {
	cw, _ := knowledgeTestWindow(t)
	id, _ := cw.knowledge.db.CreateKnowledgeBase("手册", "")
	cw.knowledge.db.ImportText(id, "部署", "部署时需要设置环境变量 CHATGO_HOME。")

	run := cw.knowledgeSearchTool(map[string]string{"top_k": "3"})
	out, err := run(t.Context(), `{"query":"环境变量","knowledge_base":"手册"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" || out == "(没有找到相关内容)" {
		t.Fatalf("tool returned %q", out)
	}
	if _, err := run(t.Context(), `{"query":"x","knowledge_base":"不存在"}`); err == nil {
		t.Fatal("an unknown knowledge base must be reported")
	}
	if _, err := run(t.Context(), `{"knowledge_base":"手册"}`); err == nil {
		t.Fatal("a missing query must be rejected")
	}
}

// TestKnowledgeRenderDoesNotTouchStore guards the freeze this page had when
// every frame queried SQLite: rendering must only draw what is loaded, and the
// store reads must come from workers.
func TestKnowledgeRenderDoesNotTouchStore(t *testing.T) {
	cw, drain := knowledgeTestWindow(t)
	id, _ := cw.knowledge.db.CreateKnowledgeBase("手册", "")
	docID, _ := cw.knowledge.db.ImportText(id, "安装", "# 安装\n\n执行 go build。")
	cw.reloadKnowledgeBases()
	drain()
	cw.selectKnowledgeBase(id)
	drain()
	cw.selectKnowledgeDoc(docID)
	drain()

	// Closing the store makes any query fail, so a render that still queried
	// would surface an error instead of the cached data.
	if err := cw.knowledge.db.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if img := mgo.Render(cw.view, 1280, 820, 1); img == nil {
			t.Fatalf("render %d failed", i)
		}
	}
	if cw.knowledge.loadErr != "" || cw.knowledge.docsErr != "" || cw.knowledge.chunksErr != "" {
		t.Fatalf("rendering queried the store: load=%q docs=%q chunks=%q",
			cw.knowledge.loadErr, cw.knowledge.docsErr, cw.knowledge.chunksErr)
	}
	if len(cw.knowledge.chunks) != 1 {
		t.Fatalf("cached chunks lost: %+v", cw.knowledge.chunks)
	}
}
