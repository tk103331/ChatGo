package ui

import (
	"fmt"

	mgo "github.com/egoist/mygo/ui"

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

// importDialog renders the scan results with per-entry checkboxes.
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
		Actions: func() {
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
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
					mcore.Spinner(c, mcore.SpinnerOptions{Size: 14})
					mgo.Text(c, "正在扫描其他 Agent 的配置…").FontSize(caption).TextColor(k.TextMuted)
				})
				return
			}
			if cw.imp.scanned && len(cw.imp.groups) == 0 {
				mgo.Text(c, "没有发现可导入的内容。请确认已安装 Claude Code、Codex、OpenCode 等并配置过。").
					FontSize(caption).TextColor(k.TextMuted)
				return
			}
			items, groupOf := cw.importFlatItems()
			mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
				if kind != importSkills {
					mgo.Spacer(c)
				} else {
					ov := cw.imp.overwrite
					if mgo.Checkbox(c, &ov, "覆盖同名").Changed() {
						cw.imp.overwrite = ov
					}
					mgo.Spacer(c)
				}
			})
			changed := selectListGrouped(c, &cw.imp.query, "搜索", items, groupOf,
				func(id string) bool { return cw.imp.chosen[id] },
				func(id string, on bool) { cw.imp.chosen[id] = on })
			if changed {
				c.Invalidate()
			}
		})
	})
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
