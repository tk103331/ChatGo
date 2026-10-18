package ui

import (
	"fmt"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// appContextChips draws the tab-context selector above the composer: one
// chip per open tab plus an "全部" chip, multi-selectable.
func (cw *ChatWindow) appContextChips(c *mgo.Context) {
	if cw.currentApp == "" {
		return
	}
	a, ok := appByID(cw.currentApp)
	if !ok || a.Static {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	tabs := cw.appTabList(cw.currentApp)
	sel := cw.appCtxTabs[cw.currentApp]
	all := cw.appCtxAll[cw.currentApp]
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "上下文").FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		allVariant := minput.Ghost
		if all {
			allVariant = minput.Secondary
		}
		if minput.Button(c, "全部", minput.ButtonOptions{Variant: allVariant, Label: "全部"}).Clicked() {
			cw.setAppContextAll(cw.currentApp, !all)
		}
		for _, tab := range tabs {
			tab := tab
			selected := false
			if !all {
				for _, s := range sel {
					if s == tab {
						selected = true
						break
					}
				}
			}
			variant := minput.Ghost
			if selected {
				variant = minput.Secondary
			}
			label := cw.appTabLabel(cw.currentApp, tab)
			if minput.Button(c, label, minput.ButtonOptions{Variant: variant, Label: label}).Clicked() {
				cw.toggleAppContextTab(cw.currentApp, tab)
			}
		}
		mgo.Box(c).Grow(1)
	})
}

// toggleAppContextTab toggles one tab in the context set, keeping at least
// the active tab selected.
func (cw *ChatWindow) toggleAppContextTab(id, tab string) {
	cw.ensureAppState()
	if cw.appCtxAll[id] {
		cw.appCtxAll[id] = false
	}
	cur := cw.appCtxTabs[id]
	found := false
	for i, s := range cur {
		if s == tab {
			cur = append(cur[:i], cur[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		cur = append(cur, tab)
	}
	if len(cur) == 0 {
		cur = []string{cw.appTab[id]}
	}
	cw.appCtxTabs[id] = cur
	cw.refreshAppSystemPrompt(id)
	cw.syncAppContext(id)
	cw.invalidate()
}

// setAppContextAll toggles the "全部 tab" mode, keeping the picked set as
// the fallback when it turns off.
func (cw *ChatWindow) setAppContextAll(id string, all bool) {
	cw.ensureAppState()
	cw.appCtxAll[id] = all
	if all {
		cw.appCtxTabs[id] = append([]string(nil), cw.appTabList(id)...)
	} else if len(cw.appCtxTabs[id]) == 0 {
		cw.appCtxTabs[id] = []string{cw.appTab[id]}
	}
	cw.refreshAppSystemPrompt(id)
	cw.syncAppContext(id)
	cw.invalidate()
}

// appContextPrompt describes the selected context tabs for the system
// prompt, or "" when none are picked.
func (cw *ChatWindow) appContextPrompt(id string) string {
	var names []string
	for _, t := range cw.appCtxTabs[id] {
		names = append(names, cw.appTabLabel(id, t))
	}
	if cw.appCtxAll[id] {
		return fmt.Sprintf("\n当前上下文：全部标签（%d 个：%s）。\n", len(names), strings.Join(names, "、"))
	}
	if len(names) == 0 {
		return ""
	}
	return "\n当前上下文标签：" + strings.Join(names, "、") + "。\n"
}

// refreshAppSystemPrompt re-injects the context suffix into the live prompt.
func (cw *ChatWindow) refreshAppSystemPrompt(id string) {
	if cw.currentApp != id {
		return
	}
	cw.systemPrompt = cw.appSystemPrompt(id) + cw.appContextPrompt(id)
}

// syncAppContext pushes the selected context tabs onto the app client, so
// its tab-scoped tools (and list_tabs) reflect the selection.
func (cw *ChatWindow) syncAppContext(id string) {
	all := cw.appCtxAll[id]
	ids := cw.appCtxTabs[id]
	switch id {
	case "terminal":
		cw.terminalClient().SetContext(all, ids)
	case "markdown":
		cw.markdownClient().SetContext(all, ids)
	case "code":
		cw.codeClient().SetContext(all, ids)
	}
}
