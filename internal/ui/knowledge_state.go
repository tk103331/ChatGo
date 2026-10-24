package ui

import (
	"strings"
	"sync"

	"chatgo/internal/kb"

	mgo "github.com/egoist/mygo/ui"
)

// kbState is the knowledge base page's view state. Every store read runs on a
// worker goroutine and is handed back through postMain, so neither opening the
// database nor a slow query can block the frame being drawn.
type kbState struct {
	db *kb.DB
	mu sync.Mutex

	loadErr string

	bases       []kb.KnowledgeBase
	basesLoaded bool
	basesBusy   bool
	sel         string

	editorOpen bool
	editing    bool
	draft      kbForm

	docs      []kb.Document
	docsFor   string
	docsBusy  bool
	docsErr   string
	docQuery  string
	docSel    string
	docList   mgo.ListState
	docsToken int

	chunks      []kb.Chunk
	chunksFor   string
	chunksBusy  bool
	chunksErr   string
	chunkList   mgo.ListState
	chunksToken int

	pasteOpen  bool
	pasteTitle string
	pasteText  string

	searchOpen bool
	// searchDoc scopes the retrieval test to one document; empty tests the
	// whole knowledge base.
	searchDoc   string
	query       string
	results     []kb.Result
	searched    bool
	searchBusy  bool
	searchErr   string
	searchToken int

	detail bool

	busy   bool
	status string
}

// kbForm is the create/edit form of a knowledge base.
type kbForm struct {
	name        string
	description string
}

// withKnowledgeDB runs fn against the store on a worker goroutine. Opening the
// store runs the schema migration, which is why it may not happen on the UI
// thread even once.
func (cw *ChatWindow) withKnowledgeDB(fn func(db *kb.DB)) {
	go func() {
		db, err := cw.knowledgeDB()
		if err != nil {
			cw.postMain(func() {
				cw.knowledge.loadErr = err.Error()
				cw.knowledge.basesLoaded = true
				cw.knowledge.basesBusy = false
				cw.knowledge.docsBusy = false
				cw.knowledge.chunksBusy = false
				cw.knowledge.searchBusy = false
				cw.invalidate()
			})
			return
		}
		fn(db)
	}()
}

// ensureKnowledgeBases starts the first load, once.
func (cw *ChatWindow) ensureKnowledgeBases() {
	if cw.knowledge.basesLoaded || cw.knowledge.basesBusy {
		return
	}
	cw.reloadKnowledgeBases()
}

// reloadKnowledgeBases refreshes the knowledge base list in the background.
func (cw *ChatWindow) reloadKnowledgeBases() {
	if cw.knowledge.basesBusy {
		return
	}
	cw.knowledge.basesBusy = true
	cw.withKnowledgeDB(func(db *kb.DB) {
		bases, err := db.ListKnowledgeBases()
		cw.postMain(func() {
			cw.knowledge.basesBusy = false
			cw.knowledge.basesLoaded = true
			if err != nil {
				cw.knowledge.loadErr = err.Error()
				cw.invalidate()
				return
			}
			cw.knowledge.loadErr = ""
			cw.knowledge.bases = bases
			if cw.knowledge.sel != "" {
				if _, ok := cw.currentKnowledgeBase(); !ok {
					cw.knowledge.sel = ""
					cw.knowledge.detail = false
				}
			}
			cw.invalidate()
		})
	})
}

// ensureKnowledgeDocs loads the selected knowledge base's documents, once.
func (cw *ChatWindow) ensureKnowledgeDocs() {
	if cw.knowledge.sel == "" || cw.knowledge.docsBusy || cw.knowledge.docsFor == cw.knowledge.sel {
		return
	}
	cw.reloadKnowledgeDocs()
}

// reloadKnowledgeDocs refreshes the documents of the selected knowledge base.
// The token drops an answer that arrives after the user moved on.
func (cw *ChatWindow) reloadKnowledgeDocs() {
	kbID := cw.knowledge.sel
	cw.knowledge.docsToken++
	if kbID == "" {
		cw.knowledge.docs = nil
		cw.knowledge.docsFor = ""
		cw.knowledge.docsBusy = false
		return
	}
	token := cw.knowledge.docsToken
	cw.knowledge.docsBusy = true
	cw.withKnowledgeDB(func(db *kb.DB) {
		docs, err := db.ListDocuments(kbID)
		cw.postMain(func() {
			if token != cw.knowledge.docsToken {
				return
			}
			cw.knowledge.docsBusy = false
			if err != nil {
				cw.knowledge.docsErr = err.Error()
				cw.invalidate()
				return
			}
			cw.knowledge.docsErr = ""
			cw.knowledge.docs = docs
			cw.knowledge.docsFor = kbID
			if cw.knowledge.docSel != "" {
				if _, ok := cw.currentKnowledgeDoc(); !ok {
					cw.selectKnowledgeDoc("")
				}
			}
			cw.invalidate()
		})
	})
}

// ensureKnowledgeChunks loads the selected document's chunks, once.
func (cw *ChatWindow) ensureKnowledgeChunks() {
	if cw.knowledge.docSel == "" || cw.knowledge.chunksBusy || cw.knowledge.chunksFor == cw.knowledge.docSel {
		return
	}
	cw.reloadKnowledgeChunks()
}

// reloadKnowledgeChunks loads the chunks of the selected document.
func (cw *ChatWindow) reloadKnowledgeChunks() {
	docID := cw.knowledge.docSel
	cw.knowledge.chunksToken++
	if docID == "" {
		cw.knowledge.chunks = nil
		cw.knowledge.chunksFor = ""
		cw.knowledge.chunksBusy = false
		return
	}
	token := cw.knowledge.chunksToken
	cw.knowledge.chunksBusy = true
	cw.withKnowledgeDB(func(db *kb.DB) {
		chunks, err := db.ListChunks(docID)
		cw.postMain(func() {
			if token != cw.knowledge.chunksToken {
				return
			}
			cw.knowledge.chunksBusy = false
			if err != nil {
				cw.knowledge.chunksErr = err.Error()
				cw.invalidate()
				return
			}
			cw.knowledge.chunksErr = ""
			cw.knowledge.chunks = chunks
			cw.knowledge.chunksFor = docID
			cw.invalidate()
		})
	})
}

// selectKnowledgeBase opens a knowledge base's detail view, dropping the
// filters and results that belonged to the previous one.
func (cw *ChatWindow) selectKnowledgeBase(id string) {
	if cw.knowledge.sel != id {
		cw.knowledge.sel = id
		cw.knowledge.docs = nil
		cw.knowledge.docsFor = ""
		cw.knowledge.docQuery = ""
		cw.knowledge.docsErr = ""
		cw.knowledge.docList = mgo.ListState{}
		cw.selectKnowledgeDoc("")
		cw.resetKnowledgeSearch()
		cw.knowledge.status = ""
		cw.ensureKnowledgeDocs()
	}
	cw.knowledge.detail = true
}

// closeKnowledgeDetail returns to the knowledge base cards.
func (cw *ChatWindow) closeKnowledgeDetail() {
	cw.knowledge.detail = false
	cw.knowledge.searchOpen = false
}

// selectKnowledgeDoc shows a document's chunks.
func (cw *ChatWindow) selectKnowledgeDoc(id string) {
	if cw.knowledge.docSel == id {
		return
	}
	cw.knowledge.docSel = id
	cw.knowledge.chunks = nil
	cw.knowledge.chunksFor = ""
	cw.knowledge.chunksErr = ""
	cw.knowledge.chunkList = mgo.ListState{}
	cw.reloadKnowledgeChunks()
}

// resetKnowledgeSearch clears the retrieval test results.
func (cw *ChatWindow) resetKnowledgeSearch() {
	cw.knowledge.results = nil
	cw.knowledge.searched = false
	cw.knowledge.searchErr = ""
	cw.knowledge.searchBusy = false
	cw.knowledge.searchToken++
}

// currentKnowledgeDoc returns the selected document.
func (cw *ChatWindow) currentKnowledgeDoc() (kb.Document, bool) {
	for _, doc := range cw.knowledge.docs {
		if doc.ID == cw.knowledge.docSel {
			return doc, true
		}
	}
	return kb.Document{}, false
}

// filteredKnowledgeDocs returns the documents matching the document filter.
func (cw *ChatWindow) filteredKnowledgeDocs() []kb.Document {
	q := strings.ToLower(strings.TrimSpace(cw.knowledge.docQuery))
	if q == "" {
		return cw.knowledge.docs
	}
	out := make([]kb.Document, 0, len(cw.knowledge.docs))
	for _, doc := range cw.knowledge.docs {
		if strings.Contains(strings.ToLower(doc.Title), q) || strings.Contains(strings.ToLower(doc.Path), q) {
			out = append(out, doc)
		}
	}
	return out
}

// openKnowledgeEditor opens the create or edit dialog.
func (cw *ChatWindow) openKnowledgeEditor(b kb.KnowledgeBase, editing bool) {
	cw.knowledge.editorOpen = true
	cw.knowledge.editing = editing
	cw.knowledge.draft = kbForm{name: b.Name, description: b.Description}
}

// openKnowledgePaste opens the paste-text dialog with empty fields.
func (cw *ChatWindow) openKnowledgePaste() {
	cw.knowledge.pasteOpen = true
	cw.knowledge.pasteTitle = ""
	cw.knowledge.pasteText = ""
}

// openKnowledgeSearch opens the retrieval test over the whole knowledge base,
// or over one document when docID is set.
func (cw *ChatWindow) openKnowledgeSearch(docID string) {
	cw.knowledge.searchOpen = true
	if cw.knowledge.searchDoc != docID {
		cw.knowledge.searchDoc = docID
		cw.resetKnowledgeSearch()
		return
	}
	cw.knowledge.searchErr = ""
}

// currentKnowledgeBase returns the selected knowledge base.
func (cw *ChatWindow) currentKnowledgeBase() (kb.KnowledgeBase, bool) {
	for _, b := range cw.knowledge.bases {
		if b.ID == cw.knowledge.sel {
			return b, true
		}
	}
	return kb.KnowledgeBase{}, false
}

// knowledgeLoading reports whether any knowledge store read is in flight.
func (cw *ChatWindow) knowledgeLoading() bool {
	return cw.knowledge.basesBusy || cw.knowledge.docsBusy ||
		cw.knowledge.chunksBusy || cw.knowledge.searchBusy || cw.knowledge.busy
}
