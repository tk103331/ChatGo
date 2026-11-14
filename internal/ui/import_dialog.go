package ui

import (
	"fmt"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// importKind is what an import dialog brings in.
type importKind int

const (
	importSkills importKind = iota
	importMCP
	importProviders
)

// importState is the shared state of the three import dialogs: which one is
// open, what the scan found and which entries are ticked.
type importState struct {
	open bool
	kind importKind

	scanning bool
	scanned  bool
	// groups are the scan results, one per source agent.
	groups []importGroup
	// chosen holds the ticked entry IDs, which are unique across groups.
	chosen    map[string]bool
	query     string
	overwrite bool
	status    string
	busy      bool
}

// importGroup is one source agent with what it offers.
type importGroup struct {
	From  string
	Items []importCandidate
}

// importCandidate is one importable entry.
type importCandidate struct {
	// ID is unique across groups, so the same name from two agents stays
	// separately selectable.
	ID    string
	Title string
	Desc  string
	Tags  []string
}

// openImportDialog starts a scan for the given kind.
func (cw *ChatWindow) openImportDialog(kind importKind) {
	cw.imp = importState{open: true, kind: kind, chosen: map[string]bool{}}
	cw.startImportScan()
}

// importDialogTitle names the dialog of a kind.
func importDialogTitle(kind importKind) string {
	switch kind {
	case importMCP:
		return "从其他 Agent 导入 MCP 服务"
	case importProviders:
		return "从其他 Agent 导入模型"
	default:
		return "从其他 Agent 导入 Skill"
	}
}

// importDialog renders the scan results with per-entry checkboxes. The
// search field and the bulk actions stay pinned above the scrolling
// entries, so a long list never buries them.
func (cw *ChatWindow) importDialog(c *mgo.Context) {
	if !cw.imp.open {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	kind := cw.imp.kind
	kit.Dialog(c, &cw.imp.open, kit.DialogOptions{
		Title:       importDialogTitle(kind),
		Description: importDialogHint(kind),
		Width:       680,
		// A long catalog of skills needs the entries to scroll inside the
		// dialog, not the dialog to grow past the window.
		MaxHeight: 560,
		Actions: func() {
			// The market offers what the scan could not: an importer plugin
			// for an agent the built-ins do not read, or a vendor preset.
			if kit.Button(c, "从插件添加", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("layout-grid")}).Clicked() {
				cw.imp.open = false
				cw.goPluginMarket(importMarketKind(kind))
			}
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.imp.open = false
			}
			n := cw.importChosenCount()
			label := "导入"
			if n > 0 {
				label = fmt.Sprintf("导入 %d 项", n)
			}
			if kit.Button(c, label, kit.ButtonOptions{
				Variant: kit.Primary, Disabled: n == 0 || cw.imp.busy, Loading: cw.imp.busy,
			}).Clicked() {
				cw.runImport()
			}
		},
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if cw.imp.status != "" {
				mgo.Text(c, cw.imp.status).FontSize(caption).TextColor(k.TextMuted).MaxLines(2)
			}
			if cw.imp.scanning {
				mgo.Column(c).FillWidth().AlignItems(mgo.Center).Gap(u).Padding(u * 4).Children(func() {
					mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
						mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
						mgo.Text(c, "正在扫描其他 Agent 的配置…").FontSize(caption).TextColor(k.TextMuted)
					})
					if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
						cw.imp.open = false
					}
				})
				return
			}
			if cw.imp.scanned && len(cw.imp.groups) == 0 {
				mgo.Column(c).FillWidth().AlignItems(mgo.Center).Gap(u).Padding(u * 3).Children(func() {
					mgo.Icon(c, icons.Must("search")).Size(28, 28).TextColor(k.TextMuted)
					mgo.Text(c, "没有发现可导入的内容").FontSize(caption).TextColor(k.Text)
					mgo.Text(c, "请确认已安装并配置过 Claude Code、Codex、OpenCode 等 Agent；").
						FontSize(caption).TextColor(k.TextMuted)
					mgo.Text(c, "或到插件市场安装对应 Agent 的导入器。").
						FontSize(caption).TextColor(k.TextMuted)
					if kit.Button(c, "去插件市场", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("layout-grid")}).Clicked() {
						cw.imp.open = false
						cw.goPluginMarket("importers")
					}
				})
				return
			}
			// The pinned part carries the search, the bulk actions and the
			// per-kind options; the entries scroll under them.
			var body func() bool
			items, groupOf := cw.importFlatItems()
			pinned := func() {
				mgo.Column(c).FillWidth().MinWidth(0).Gap(u).Children(func() {
					cw.importBulkHeader(c, kind, items)
				})
			}
			_, body = selectListGroupedSplit(c, &cw.imp.query, "搜索 "+importKindLabel(kind), items, groupOf,
				func(id string) bool { return cw.imp.chosen[id] },
				func(id string, on bool) { cw.imp.chosen[id] = on })
			pinned()
			mgo.Scroll(c).FillWidth().MinHeight(0).Children(func() {
				mgo.Column(c).FillWidth().MinWidth(0).Children(func() {
					if body() {
						c.Invalidate()
					}
				})
			})
		})
	})
}

// importBulkHeader is the pinned header above the scrolling entries: the
// search field, the select/clear actions and the per-kind option row
// (覆盖同名 for skills, where a re-import can replace a local edit).
func (cw *ChatWindow) importBulkHeader(c *mgo.Context, kind importKind, items []selectItem) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().MinWidth(0).Gap(u).Children(func() {
		in := kit.SearchInput(c, &cw.imp.query, kit.SearchInputOptions{
			Placeholder: "搜索 " + importKindLabel(kind), Label: "搜索 " + importKindLabel(kind),
		})
		in.Element.FillWidth()
		shown := filterSelectItems(items, cw.imp.query)
		on := 0
		for _, it := range shown {
			if cw.imp.chosen[it.ID] {
				on++
			}
		}
		all, none := selectListHeader(c, on, len(shown))
		if all || none {
			for _, it := range shown {
				cw.imp.chosen[it.ID] = all
			}
			c.Invalidate()
		}
		if kind == importSkills {
			ov := cw.imp.overwrite
			if mgo.Checkbox(c, &ov, "覆盖同名 Skill（默认保留本地修改）").Changed() {
				cw.imp.overwrite = ov
			}
		}
	})
}

// importKindLabel is a kind's short name for the search placeholder.
func importKindLabel(kind importKind) string {
	switch kind {
	case importMCP:
		return "MCP 服务"
	case importProviders:
		return "模型"
	default:
		return "Skill"
	}
}

// importMarketKind maps an import dialog's kind to the market tab filter
// its "从插件添加" button lands on: skills and MCP both come from
// importers, models from the provider catalog.
func importMarketKind(kind importKind) string {
	switch kind {
	case importProviders:
		return "catalog-providers"
	default:
		return "importers"
	}
}

// importDialogHint explains where a kind is read from.
func importDialogHint(kind importKind) string {
	switch kind {
	case importMCP:
		return "扫描 Claude Code、Codex、OpenCode、Cursor 与 Gemini CLI 的配置，导入为 ChatGo 的 MCP 服务。"
	case importProviders:
		return "扫描其他 Agent 中 OpenAI 兼容的模型配置，导入为 ChatGo 的 custom 模型。"
	default:
		return "把其他 Agent 的 Skill 复制到 ~/.chatgo/skills，之后由 ChatGo 自己管理。"
	}
}

// importChosenCount counts the ticked entries that are still on offer.
func (cw *ChatWindow) importChosenCount() int {
	n := 0
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			if cw.imp.chosen[it.ID] {
				n++
			}
		}
	}
	return n
}

// importFlatItems flattens the groups for the selection list, remembering
// which source each entry came from so the list can show headers.
func (cw *ChatWindow) importFlatItems() ([]selectItem, map[string]string) {
	var items []selectItem
	groupOf := map[string]string{}
	for _, g := range cw.imp.groups {
		for _, it := range g.Items {
			items = append(items, selectItem{ID: it.ID, Title: it.Title, Desc: it.Desc, Tags: it.Tags})
			groupOf[it.ID] = g.From
		}
	}
	return items, groupOf
}
