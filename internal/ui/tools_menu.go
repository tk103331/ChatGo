package ui

import (
	"fmt"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

type toolPane struct {
	name    string
	items   []selectItem
	query   *string
	search  string
	enabled func(string) bool
	set     func(string, bool)
}

func (cw *ChatWindow) toolItemList(prefix string) []selectItem {
	var out []selectItem
	for _, mt := range cw.toolMetas() {
		if strings.HasPrefix(mt.ID, prefix) {
			out = append(out, selectItem{ID: mt.ID, Title: mt.Label, Desc: mt.Desc})
		}
	}
	return out
}

func (cw *ChatWindow) skillItemList() []selectItem {
	var out []selectItem
	for _, id := range cw.visibleSkillIDs() {
		s := cw.skillByID(id)
		if s == nil {
			continue
		}
		out = append(out, selectItem{ID: id, Title: s.Name, Desc: s.Description})
	}
	return out
}

func (cw *ChatWindow) toolsMenu(c *mgo.Context) {
	panes := []toolPane{
		{
			name: "工具", items: cw.toolItemList("builtin:"), query: &cw.toolQuery,
			search: "搜索工具", enabled: cw.toolEnabled,
			set: func(id string, on bool) { cw.toolOn[id] = on },
		},
		{
			name: "MCP", items: cw.toolItemList("mcp:"), query: &cw.mcpQuery,
			search: "搜索 MCP 服务器", enabled: cw.toolEnabled,
			set: func(id string, on bool) { cw.toolOn[id] = on },
		},
		{
			name: "Skills", items: cw.skillItemList(), query: &cw.skills.menuQuery,
			search: "搜索 Skill", enabled: cw.skillRuntime,
			set: func(id string, on bool) { cw.skillOn[id] = on },
		},
	}
	counts := make([]int, len(panes))
	for i, p := range panes {
		for _, it := range p.items {
			if p.enabled(it.ID) {
				counts[i]++
			}
		}
	}
	b := kit.IconButton(c, icons.Must("sliders-horizontal"), "工具与技能", kit.IconButtonOptions{Variant: kit.Secondary}).
		Tooltip("工具与技能")
	open := mgo.Local(b, "composer-tools", func() bool { return false })
	if b.Clicked() {
		*open = !*open
	}
	tab := mgo.Local(b, "composer-tools-tab", func() int {
		for i, p := range panes {
			if len(p.items) > 0 {
				return i
			}
		}
		return 0
	})
	segments := make([]kit.Segment, len(panes))
	for i, p := range panes {
		segments[i] = kit.Segment{Label: toolTabLabel(p.name, counts[i])}
	}
	var body func() bool
	changed := false
	kit.Popover(c, b, open, kit.PopoverOptions{
		Label: "工具与技能", Width: 360, MaxHeight: mcore.Density(c).Unit() * 120,
		Header: func() {
			kit.SegmentedControl(c, tab, segments, kit.SegmentedControlOptions{Equal: true, Label: "工具类别"})
			p := panes[*tab]
			if len(p.items) == 0 {
				t := *tab
				body = func() bool {
					cw.emptyToolPane(c, t, open)
					return false
				}
				return
			}
			pinned, paneBody := selectListSplit(c, p.query, p.search, p.items, p.enabled, p.set)
			pinned()
			body = paneBody
		},
	}, func() {
		if body != nil {
			changed = body()
		}
	})
	if changed {
		if cw.currentAgent == "" {
			cw.saveChatSelection()
		}
		cw.clientDirty.Store(true)
		c.Invalidate()
	}
}

func toolTabLabel(name string, on int) string {
	if on == 0 {
		return name
	}
	return fmt.Sprintf("%s (%d)", name, on)
}

func (cw *ChatWindow) emptyToolPane(c *mgo.Context, tab int, open *bool) {
	k := mcore.Tokens(c)
	names := []string{"工具", "MCP", "Skills"}
	name := names[tab]
	mgo.Text(c, "没有可用的"+name+"，请先在设置中启用。").
		FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
	if kit.Button(c, "去配置"+name, kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
		*open = false
		switch tab {
		case 0:
			cw.settings.tab = 1
			cw.goConfigure(sectionSettings)
		case 1:
			cw.goConfigure(sectionMCP)
		case 2:
			cw.goConfigure(sectionSkill)
		}
	}
}
