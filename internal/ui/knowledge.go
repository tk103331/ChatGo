package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/kb"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// knowledgePage shows the knowledge base cards, or the detail view of the one
// being managed.
func (cw *ChatWindow) knowledgePage(c *mgo.Context) {
	cw.ensureKnowledgeBases()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if cw.knowledge.loadErr != "" {
		mgo.Column(c).Fill().Padding(u * 4).Gap(u * 2).Children(func() {
			mgo.Text(c, "知识库").
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
			mgo.Text(c, cw.knowledge.loadErr).TextColor(k.Danger)
		})
		return
	}
	if b, ok := cw.currentKnowledgeBase(); ok && cw.knowledge.detail {
		cw.knowledgeDetailPage(c, b)
	} else {
		cw.knowledgeCardsPage(c)
	}
	cw.knowledgeEditorDialog(c)
	cw.knowledgePasteDialog(c)
	cw.knowledgeSearchDialog(c)
}

// knowledgeCardsPage is the knowledge base grid.
func (cw *ChatWindow) knowledgeCardsPage(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Text(c, "知识库").
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if cw.knowledge.basesBusy {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
			}
			if cw.knowledge.basesLoaded {
				mgo.Text(c, fmt.Sprintf("共 %d 个", len(cw.knowledge.bases))).TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
			if kit.Button(c, "新建知识库", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.openKnowledgeEditor(kb.KnowledgeBase{}, false)
			}
		})
		if !cw.knowledge.basesLoaded {
			cw.knowledgeLoadingState(c, "正在加载知识库…")
			return
		}
		if len(cw.knowledge.bases) == 0 {
			cw.knowledgeEmptyState(c)
			return
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			cardRows(c, 2, len(cw.knowledge.bases), func(i int) {
				cw.knowledgeCard(c, cw.knowledge.bases[i])
			})
		})
	})
}

// knowledgeCard is one knowledge base in the grid.
func (cw *ChatWindow) knowledgeCard(c *mgo.Context, b kb.KnowledgeBase) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cardShell(c, "kb:"+b.ID, func() {
		cardTitleRow(c, func() {
			mgo.Icon(c, icons.Must("file-text")).Size(18, 18).TextColor(k.Text).Shrink(0)
			cardTitle(c, b.Name)
		})
		desc := b.Description
		if desc == "" {
			desc = "暂无描述"
		}
		mgo.Text(c, desc).TextColor(k.TextMuted).FontSize(caption).MinWidth(0).MaxLines(2)
		cardMetaRow(c, func() {
			agentTag(c, fmt.Sprintf("%d 文档", b.Docs))
			agentTag(c, fmt.Sprintf("%d 分块", b.Chunks))
		})
		cardActionRow(c, func() {
			if kit.Button(c, "管理", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.selectKnowledgeBase(b.ID)
			}
			if kit.Button(c, "检索", kit.ButtonOptions{Variant: kit.Outline, Icon: icons.Must("search")}).Clicked() {
				cw.knowledge.sel = b.ID
				cw.openKnowledgeSearch("")
			}
			if kit.Button(c, "编辑", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.knowledge.sel = b.ID
				cw.openKnowledgeEditor(b, true)
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteKnowledgeBase(b)
			}
		})
	})
}

// knowledgeEmptyState shows the placeholder when no knowledge base exists.
func (cw *ChatWindow) knowledgeEmptyState(c *mgo.Context) {
	emptyState(c, emptyStateOptions{
		Icon:  "file-text",
		Title: "还没有知识库",
		Hint:  "新建知识库后导入文件或粘贴文本，即可本地全文检索",
		Actions: func() {
			if kit.Button(c, "新建知识库", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.openKnowledgeEditor(kb.KnowledgeBase{}, false)
			}
		},
	})
}

// knowledgeDetailPage manages one knowledge base: files on the left, the
// selected file's chunks on the right.
func (cw *ChatWindow) knowledgeDetailPage(c *mgo.Context, b kb.KnowledgeBase) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	cw.ensureKnowledgeDocs()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if kit.Button(c, "返回", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.closeKnowledgeDetail()
			}
			mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 3).Children(func() {
				mgo.Text(c, b.Name).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).
					SingleLine().Ellipsis("…")
				desc := b.Description
				if desc == "" {
					desc = fmt.Sprintf("%d 个文档 · %d 个分块", b.Docs, b.Chunks)
				}
				mgo.Text(c, desc).TextColor(k.TextMuted).FontSize(caption).SingleLine().Ellipsis("…")
			})
			if kit.Button(c, "检索测试", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("search")}).Clicked() {
				cw.openKnowledgeSearch("")
			}
			if mcore.IconAction(c, icons.Must("pencil"), "编辑知识库").Tooltip("编辑").Clicked() {
				cw.openKnowledgeEditor(b, true)
			}
		})
		mgo.Row(c).Gap(u).Wrap().AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "添加文件", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("plus")}).Clicked() {
				cw.pickKnowledgeFiles()
			}
			if kit.Button(c, "添加目录", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("folder")}).Clicked() {
				cw.pickKnowledgeDir()
			}
			if kit.Button(c, "粘贴文本", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("file-text")}).Clicked() {
				cw.openKnowledgePaste()
			}
			if kit.Button(c, "重建索引", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("refresh-cw"), Loading: cw.knowledge.busy}).Clicked() {
				cw.reindexKnowledge()
			}
		})
		if cw.knowledge.status != "" {
			color := k.TextMuted
			if strings.Contains(cw.knowledge.status, "失败") {
				color = k.Danger
			}
			mgo.Text(c, cw.knowledge.status).FontSize(caption).TextColor(color).MaxLines(2)
		}
		if cw.knowledge.docsErr != "" {
			mgo.Text(c, cw.knowledge.docsErr).TextColor(k.Danger).FontSize(caption)
		}
		if cw.knowledge.docsFor != b.ID {
			cw.knowledgeLoadingState(c, "正在加载文件…")
			return
		}
		if len(cw.knowledge.docs) == 0 {
			cw.knowledgeNoDocsState(c)
			return
		}
		mgo.Row(c).Grow(1).MinWidth(0).MinHeight(0).Gap(u * 3).AlignItems(mgo.Stretch).Children(func() {
			cw.knowledgeDocList(c, b)
			mgo.Divider(c)
			cw.knowledgeChunkList(c)
		})
	})
}

// knowledgeNoDocsState prompts for the first import.
func (cw *ChatWindow) knowledgeNoDocsState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u).Children(func() {
		mgo.Icon(c, icons.Must("folder")).Size(36, 36).TextColor(k.TextMuted)
		mgo.Text(c, "还没有导入文档").TextColor(k.TextMuted)
		mgo.Text(c, "用上面的按钮添加文件、目录或粘贴文本").
			TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}

// knowledgeDocList is the left column: the indexed files of the knowledge base.
func (cw *ChatWindow) knowledgeDocList(c *mgo.Context, b kb.KnowledgeBase) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	docs := cw.filteredKnowledgeDocs()
	mgo.Column(c).Width(320).Shrink(0).Gap(u * 2).Children(func() {
		label := fmt.Sprintf("文件 %d", len(cw.knowledge.docs))
		if len(docs) != len(cw.knowledge.docs) {
			label = fmt.Sprintf("文件 %d / %d", len(docs), len(cw.knowledge.docs))
		}
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, label).FontWeight(600).Grow(1)
			if cw.knowledge.docsBusy {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
			}
		})
		in := kit.SearchInput(c, &cw.knowledge.docQuery, kit.SearchInputOptions{Placeholder: "按标题或路径过滤"})
		in.Element.FillWidth()
		if len(docs) == 0 {
			mgo.Text(c, "没有匹配的文件").TextColor(k.TextMuted).FontSize(caption)
			return
		}
		mgo.List(c, &cw.knowledge.docList, len(docs), func(i int) {
			doc := docs[i]
			source := knowledgeDocSource(doc)
			selected := cw.knowledge.docSel == doc.ID
			btn := mgo.ButtonBase(c).Key(doc.ID).FillWidth().Padding(u, u*2).Gap(u).
				AlignItems(mgo.Center).Justify(mgo.Start).Radius(mtheme.ControlRadius)
			if selected {
				btn.Background(k.Selection)
			} else if btn.Hovered() {
				btn.Background(k.SurfaceHover)
			}
			if selected {
				btn.DrawOver(func(p *mgo.Painter, r mgo.Rect) {
					p.Fill(mgo.Rect{X: r.X, Y: r.Y + u, W: u * 0.75, H: r.H - u*2}, k.Accent, u*0.375)
				})
			}
			btn.Children(func() {
				mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 3).Children(func() {
					mgo.Text(c, doc.Title).SingleLine().Ellipsis("…")
					mgo.Text(c, source).FontSize(caption).TextColor(k.TextMuted).
						SingleLine().Ellipsis("…").Tooltip(source)
				})
				mgo.Text(c, fmt.Sprintf("%d", doc.Chunks)).
					FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			})
			if btn.Clicked() {
				cw.selectKnowledgeDoc(doc.ID)
			}
		}).Grow(1).MinHeight(0)
	})
}

// knowledgeDocSource names where a document came from.
func knowledgeDocSource(doc kb.Document) string {
	if doc.Source == "file" {
		return doc.Path
	}
	return "粘贴文本"
}

// knowledgeChunkList is the right column: the chunks of the selected file.
func (cw *ChatWindow) knowledgeChunkList(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	doc, ok := cw.currentKnowledgeDoc()
	if !ok {
		mgo.Column(c).Grow(1).MinWidth(0).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u).Children(func() {
			mgo.Icon(c, icons.Must("file-text")).Size(36, 36).TextColor(k.TextMuted)
			mgo.Text(c, "从左侧选择一个文件查看分块").TextColor(k.TextMuted)
		})
		return
	}
	cw.ensureKnowledgeChunks()
	mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Column(c).Grow(1).MinWidth(0).Gap(u / 3).Children(func() {
				mgo.Text(c, doc.Title).FontWeight(600).SingleLine().Ellipsis("…")
				source := knowledgeDocSource(doc)
				mgo.Text(c, fmt.Sprintf("%s · %d 分块", source, doc.Chunks)).
					FontSize(caption).TextColor(k.TextMuted).SingleLine().Ellipsis("…").Tooltip(source)
			})
			if cw.knowledge.chunksBusy {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
			}
			if kit.Button(c, "在此文件检索", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("search")}).Clicked() {
				cw.openKnowledgeSearch(doc.ID)
			}
			if mcore.IconAction(c, icons.Must("x"), "删除文档").Tooltip("删除文档").Clicked() {
				if b, ok := cw.currentKnowledgeBase(); ok {
					cw.deleteKnowledgeDoc(b, doc)
				}
			}
		})
		if cw.knowledge.chunksErr != "" {
			mgo.Text(c, cw.knowledge.chunksErr).TextColor(k.Danger).FontSize(caption)
			return
		}
		if cw.knowledge.chunksFor != doc.ID {
			cw.knowledgeLoadingState(c, "正在加载分块…")
			return
		}
		if len(cw.knowledge.chunks) == 0 {
			mgo.Text(c, "该文件没有分块").TextColor(k.TextMuted).FontSize(caption)
			return
		}
		mgo.List(c, &cw.knowledge.chunkList, len(cw.knowledge.chunks), func(i int) {
			chunk := cw.knowledge.chunks[i]
			mgo.Column(c).Key(fmt.Sprintf("chunk:%s:%d", doc.ID, chunk.Seq)).FillWidth().
				Gap(u/2).Padding(u*1.5).Margin(0, 0, u, 0).
				Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mgo.Text(c, fmt.Sprintf("#%d", chunk.Seq+1)).
						FontSize(caption).TextColor(k.Accent).Shrink(0)
					if chunk.Heading != "" {
						mgo.Text(c, chunk.Heading).FontSize(caption).TextColor(k.TextMuted).
							Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
					} else {
						mgo.Spacer(c)
					}
					mgo.Text(c, fmt.Sprintf("%d 字", runeCount(chunk.Text))).
						FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				})
				mgo.Text(c, chunk.Text).MaxLines(8)
			})
		}).Grow(1).MinHeight(0)
	})
}

// runeCount counts characters so CJK text is not reported in bytes.
func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// knowledgeEditorDialog creates or renames a knowledge base.
func (cw *ChatWindow) knowledgeEditorDialog(c *mgo.Context) {
	if !cw.knowledge.editorOpen {
		return
	}
	f := &cw.knowledge.draft
	title := "新建知识库"
	if cw.knowledge.editing {
		title = "编辑知识库"
	}
	kit.Dialog(c, &cw.knowledge.editorOpen, kit.DialogOptions{
		Title: title,
		Width: 460,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.knowledge.editorOpen = false
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveKnowledgeBase()
			}
		},
	}, func() {
		u := mcore.Density(c).Unit()
		mgo.Column(c).Gap(u * 2).Children(func() {
			if in := kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "知识库名称"}); in.Submitted() {
				cw.saveKnowledgeBase()
			}
			kit.TextArea(c, &f.description, kit.TextAreaOptions{Label: "描述", Placeholder: "可选", MinHeight: 72})
		})
	})
}

// knowledgePasteDialog imports pasted text as a document.
func (cw *ChatWindow) knowledgePasteDialog(c *mgo.Context) {
	if !cw.knowledge.pasteOpen {
		return
	}
	kit.Dialog(c, &cw.knowledge.pasteOpen, kit.DialogOptions{
		Title:       "粘贴文本",
		Description: "文本会按标题与段落切分后建立索引。",
		Width:       620,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.knowledge.pasteOpen = false
			}
			if kit.Button(c, "导入", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.importKnowledgeText()
			}
		},
	}, func() {
		u := mcore.Density(c).Unit()
		mgo.Column(c).Gap(u * 2).Children(func() {
			kit.TextInput(c, &cw.knowledge.pasteTitle, kit.TextInputOptions{Label: "标题", Placeholder: "留空则记为「未命名文本」"})
			kit.TextArea(c, &cw.knowledge.pasteText, kit.TextAreaOptions{Label: "内容", Placeholder: "粘贴要索引的文本", MinHeight: 220})
		})
	})
}

// knowledgeSearchDialog is the retrieval test, scoped to the knowledge base or
// to a single file.
func (cw *ChatWindow) knowledgeSearchDialog(c *mgo.Context) {
	if !cw.knowledge.searchOpen {
		return
	}
	b, ok := cw.currentKnowledgeBase()
	if !ok {
		cw.knowledge.searchOpen = false
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	scope := b.Name
	if doc, ok := cw.currentKnowledgeDoc(); ok && cw.knowledge.searchDoc == doc.ID {
		scope = doc.Title
	}
	kit.Dialog(c, &cw.knowledge.searchOpen, kit.DialogOptions{
		Title:       "检索测试 · " + scope,
		Description: "查看一个问题会命中哪些片段，以及它们的 BM25 得分。",
		Width:       680,
		Actions: func() {
			if kit.Button(c, "关闭", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.knowledge.searchOpen = false
			}
		},
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if cw.knowledge.searchDoc != "" {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					agentTag(c, "仅此文件")
					if kit.Button(c, "改为检索整个知识库", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
						cw.openKnowledgeSearch("")
					}
				})
			}
			mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
				in := kit.SearchInput(c, &cw.knowledge.query, kit.SearchInputOptions{
					Label: "检索词", Placeholder: "输入关键词或问题，回车检索",
				})
				in.Element.Grow(1).MinWidth(0)
				if in.Submitted() {
					cw.runKnowledgeSearch(b)
				}
				if kit.Button(c, "检索", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("search")}).Clicked() {
					cw.runKnowledgeSearch(b)
				}
			})
			switch {
			case cw.knowledge.searchBusy:
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
					mgo.Text(c, "正在检索…").TextColor(k.TextMuted).FontSize(caption)
				})
			case cw.knowledge.searchErr != "":
				mgo.Text(c, cw.knowledge.searchErr).TextColor(k.Danger).FontSize(caption)
			case !cw.knowledge.searched:
				mgo.Text(c, "尚未检索").TextColor(k.TextMuted).FontSize(caption)
			case len(cw.knowledge.results) == 0:
				mgo.Text(c, "没有匹配的内容").TextColor(k.TextMuted).FontSize(caption)
			default:
				mgo.Text(c, fmt.Sprintf("命中 %d 个片段", len(cw.knowledge.results))).
					FontSize(caption).TextColor(k.TextMuted)
				mgo.Scroll(c).MaxHeight(380).Children(func() {
					mgo.Column(c).FillWidth().Gap(u).Children(func() {
						for i, r := range cw.knowledge.results {
							knowledgeResultCard(c, i, r)
						}
					})
				})
			}
		})
	})
}

// knowledgeResultCard renders one retrieved passage with its source.
func knowledgeResultCard(c *mgo.Context, i int, r kb.Result) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	head := r.Doc
	if r.Heading != "" && r.Heading != r.Doc {
		head += " › " + r.Heading
	}
	mgo.Column(c).Key(r.ChunkID).FillWidth().Gap(u / 2).Padding(u * 1.5).
		Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, fmt.Sprintf("#%d", i+1)).FontSize(caption).TextColor(k.Accent).Shrink(0)
			mgo.Text(c, head).FontSize(caption).TextColor(k.TextMuted).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(r.Path)
			mgo.Text(c, fmt.Sprintf("%.2f", r.Score)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})
		mgo.Text(c, r.Text).MaxLines(6)
	})
}

// knowledgeLoadingState fills the page body while a store read is running.
func (cw *ChatWindow) knowledgeLoadingState(c *mgo.Context, label string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).MinHeight(0).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mcore.Spinner(c, mcore.SpinnerOptions{Size: 24})
		mgo.Text(c, label).TextColor(k.TextMuted).FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}
