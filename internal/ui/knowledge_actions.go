package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"chatgo/internal/kb"

	mcore "chatgo/internal/ui/uikit"
	"github.com/egoist/mygo"
)

// saveKnowledgeBase creates or updates a knowledge base from the form.
func (cw *ChatWindow) saveKnowledgeBase() {
	name := strings.TrimSpace(cw.knowledge.draft.name)
	if name == "" {
		cw.fail("知识库名称不能为空")
		return
	}
	description := strings.TrimSpace(cw.knowledge.draft.description)
	editing := cw.knowledge.editing
	var editID string
	if editing {
		b, ok := cw.currentKnowledgeBase()
		if !ok {
			return
		}
		editID = b.ID
	}
	cw.knowledge.editorOpen = false
	cw.knowledge.editing = false
	cw.withKnowledgeDB(func(db *kb.DB) {
		var newID string
		var err error
		if editing {
			err = db.UpdateKnowledgeBase(editID, name, description)
		} else {
			newID, err = db.CreateKnowledgeBase(name, description)
		}
		cw.postMain(func() {
			if err != nil {
				cw.fail("保存失败: " + err.Error())
				return
			}
			if editing {
				cw.toast("已保存知识库 "+name, mcore.SeveritySuccess)
			} else {
				cw.knowledge.sel = newID
				cw.knowledge.docsFor = ""
				cw.toast("已创建知识库 "+name, mcore.SeveritySuccess)
			}
			cw.reloadKnowledgeBases()
		})
	})
}

// deleteKnowledgeBase removes a knowledge base after confirmation.
func (cw *ChatWindow) deleteKnowledgeBase(b kb.KnowledgeBase) {
	cw.askConfirm("删除知识库", "确定删除「"+b.Name+"」及其全部文档索引？此操作不可撤销。", func() {
		cw.knowledge.busy = true
		cw.knowledge.status = "正在删除…"
		cw.withKnowledgeDB(func(db *kb.DB) {
			err := db.DeleteKnowledgeBase(b.ID)
			cw.postMain(func() {
				cw.knowledge.busy = false
				cw.knowledge.status = ""
				if err != nil {
					cw.fail("删除失败: " + err.Error())
					return
				}
				if cw.knowledge.sel == b.ID {
					cw.knowledge.sel = ""
					cw.knowledge.detail = false
					cw.knowledge.docs = nil
					cw.knowledge.docsFor = ""
					cw.knowledge.docQuery = ""
					cw.knowledge.searchOpen = false
					cw.selectKnowledgeDoc("")
					cw.resetKnowledgeSearch()
				}
				cw.reloadKnowledgeBases()
				cw.toast("知识库已删除", mcore.SeveritySuccess)
			})
		})
	})
}

// deleteKnowledgeDoc removes one indexed document.
func (cw *ChatWindow) deleteKnowledgeDoc(b kb.KnowledgeBase, doc kb.Document) {
	cw.askConfirm("删除文档", "确定删除「"+doc.Title+"」的索引？", func() {
		cw.knowledge.busy = true
		cw.knowledge.status = "正在删除文档…"
		cw.withKnowledgeDB(func(db *kb.DB) {
			err := db.DeleteDocument(doc.ID)
			cw.postMain(func() {
				cw.knowledge.busy = false
				cw.knowledge.status = ""
				if err != nil {
					cw.fail("删除失败: " + err.Error())
					return
				}
				if cw.knowledge.docSel == doc.ID {
					cw.selectKnowledgeDoc("")
				}
				if cw.knowledge.searchDoc == doc.ID {
					cw.knowledge.searchDoc = ""
					cw.resetKnowledgeSearch()
				}
				cw.refreshKnowledgeAfterIngest()
			})
		})
	})
}

// pickKnowledgeFiles asks for files and indexes them.
func (cw *ChatWindow) pickKnowledgeFiles() {
	cw.winMu.Lock()
	w := cw.window
	cw.winMu.Unlock()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: w, Title: "选择要索引的文本文件",
			Filters: []mygo.FileFilter{
				{Name: "文本与代码", Extensions: []string{
					"md", "markdown", "txt", "text", "rst", "adoc", "org",
					"json", "yaml", "yml", "toml", "ini", "conf", "csv", "log",
					"go", "py", "js", "ts", "tsx", "jsx", "java", "kt", "rs",
					"c", "h", "cc", "cpp", "hpp", "cs", "rb", "php", "sh", "sql",
					"html", "css",
				}},
			},
			Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		cw.postMain(func() { cw.importKnowledgeFiles(paths) })
	}()
}

// pickKnowledgeDir asks for a directory and indexes every text file in it.
func (cw *ChatWindow) pickKnowledgeDir() {
	cw.pickDirectory("选择要索引的目录", func(path string) {
		cw.importKnowledgeFiles([]string{path})
	})
}

// importKnowledgeFiles indexes files in the background and reports the result.
func (cw *ChatWindow) importKnowledgeFiles(paths []string) {
	kbID := cw.knowledge.sel
	if kbID == "" {
		return
	}
	cw.knowledge.busy = true
	cw.knowledge.status = "正在建立索引…"
	cw.withKnowledgeDB(func(db *kb.DB) {
		result, err := db.ImportFiles(kbID, paths)
		cw.postMain(func() {
			cw.knowledge.busy = false
			cw.knowledge.status = describeIngest("索引完成", result, err)
			cw.refreshKnowledgeAfterIngest()
			cw.invalidate()
		})
	})
}

// importKnowledgeText indexes pasted text.
func (cw *ChatWindow) importKnowledgeText() {
	kbID := cw.knowledge.sel
	if kbID == "" {
		return
	}
	title := strings.TrimSpace(cw.knowledge.pasteTitle)
	text := cw.knowledge.pasteText
	cw.knowledge.pasteOpen = false
	cw.knowledge.pasteTitle = ""
	cw.knowledge.pasteText = ""
	cw.knowledge.busy = true
	cw.knowledge.status = "正在建立索引…"
	cw.withKnowledgeDB(func(db *kb.DB) {
		_, err := db.ImportText(kbID, title, text)
		cw.postMain(func() {
			cw.knowledge.busy = false
			if err != nil {
				cw.knowledge.status = "导入失败: " + err.Error()
				cw.invalidate()
				return
			}
			cw.knowledge.status = "已导入粘贴文本"
			cw.refreshKnowledgeAfterIngest()
			cw.invalidate()
		})
	})
}

// reindexKnowledge rebuilds the selected knowledge base from its files.
func (cw *ChatWindow) reindexKnowledge() {
	kbID := cw.knowledge.sel
	if kbID == "" || cw.knowledge.busy {
		return
	}
	cw.knowledge.busy = true
	cw.knowledge.status = "正在重建索引…"
	cw.withKnowledgeDB(func(db *kb.DB) {
		result, err := db.Reindex(kbID)
		cw.postMain(func() {
			cw.knowledge.busy = false
			cw.knowledge.status = describeIngest("重建完成", result, err)
			cw.refreshKnowledgeAfterIngest()
			cw.invalidate()
		})
	})
}

// runKnowledgeSearch searches the selected knowledge base, or the selected
// document when the retrieval test is scoped to one.
func (cw *ChatWindow) runKnowledgeSearch(b kb.KnowledgeBase) {
	query := strings.TrimSpace(cw.knowledge.query)
	if query == "" {
		return
	}
	kbID, docID := b.ID, cw.knowledge.searchDoc
	cw.knowledge.searchToken++
	token := cw.knowledge.searchToken
	cw.knowledge.searchBusy = true
	cw.knowledge.searchErr = ""
	cw.withKnowledgeDB(func(db *kb.DB) {
		var results []kb.Result
		var err error
		if docID != "" {
			results, err = db.SearchDocument(docID, query, 10)
		} else {
			results, err = db.Search(kbID, query, 10)
		}
		cw.postMain(func() {
			if token != cw.knowledge.searchToken {
				return
			}
			cw.knowledge.searchBusy = false
			cw.knowledge.searched = true
			if err != nil {
				cw.knowledge.searchErr = err.Error()
				cw.knowledge.results = nil
				cw.invalidate()
				return
			}
			cw.knowledge.searchErr = ""
			cw.knowledge.results = results
			cw.invalidate()
		})
	})
}

// describeIngest summarizes an import or reindex run.
func describeIngest(prefix string, result kb.IngestResult, err error) string {
	if err != nil {
		return prefix + "失败: " + err.Error()
	}
	msg := fmt.Sprintf("%s：新增 %d，更新 %d，跳过 %d", prefix, result.Added, result.Updated, result.Skipped)
	if result.Failed > 0 {
		msg += fmt.Sprintf("，失败 %d", result.Failed)
		if len(result.Errors) > 0 {
			msg += "（" + result.Errors[0] + "）"
		}
	}
	return msg
}

// knowledgeSearchTool is the builtin tool that lets an agent query the local
// knowledge bases. The tool config supplies the defaults a model may override.
func (cw *ChatWindow) knowledgeSearchTool(cfg map[string]string) func(context.Context, string) (string, error) {
	defaultBase := strings.TrimSpace(cfg["knowledge_base"])
	defaultTopK := 5
	if n, err := strconv.Atoi(strings.TrimSpace(cfg["top_k"])); err == nil && n > 0 {
		defaultTopK = n
	}
	return func(_ context.Context, args string) (string, error) {
		var in struct {
			Query         string `json:"query"`
			KnowledgeBase string `json:"knowledge_base"`
			TopK          int    `json:"top_k"`
		}
		if err := json.Unmarshal([]byte(args), &in); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
		if strings.TrimSpace(in.Query) == "" {
			return "", fmt.Errorf("缺少必填参数 query")
		}
		name := strings.TrimSpace(in.KnowledgeBase)
		if name == "" {
			name = defaultBase
		}
		topK := in.TopK
		if topK <= 0 {
			topK = defaultTopK
		}
		if topK > 20 {
			topK = 20
		}
		db, err := cw.knowledgeDB()
		if err != nil {
			return "", err
		}
		kbID, err := resolveKnowledgeBase(db, name)
		if err != nil {
			return "", err
		}
		results, err := db.Search(kbID, in.Query, topK)
		if err != nil {
			return "", err
		}
		return kb.Format(results, 6000), nil
	}
}

// knowledgeDB opens the store off the UI thread, without touching view state.
func (cw *ChatWindow) knowledgeDB() (*kb.DB, error) {
	cw.knowledge.mu.Lock()
	defer cw.knowledge.mu.Unlock()
	if cw.knowledge.db != nil {
		return cw.knowledge.db, nil
	}
	db, err := kb.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("打开知识库失败: %w", err)
	}
	cw.knowledge.db = db
	return db, nil
}

// resolveKnowledgeBase maps a knowledge base name to its id; an empty name
// searches every knowledge base.
func resolveKnowledgeBase(db *kb.DB, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	bases, err := db.ListKnowledgeBases()
	if err != nil {
		return "", err
	}
	for _, b := range bases {
		if strings.EqualFold(b.Name, name) || b.ID == name {
			return b.ID, nil
		}
	}
	names := make([]string, 0, len(bases))
	for _, b := range bases {
		names = append(names, b.Name)
	}
	return "", fmt.Errorf("没有名为 %q 的知识库（现有：%s）", name, strings.Join(names, ", "))
}

// refreshKnowledgeAfterIngest reloads the lists an import or deletion changed,
// including the chunks of the open document.
func (cw *ChatWindow) refreshKnowledgeAfterIngest() {
	cw.reloadKnowledgeDocs()
	cw.reloadKnowledgeBases()
	cw.knowledge.chunksFor = ""
	if cw.knowledge.docSel != "" {
		if _, ok := cw.currentKnowledgeDoc(); !ok {
			cw.selectKnowledgeDoc("")
		}
	}
}
