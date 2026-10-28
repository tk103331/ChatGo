package ui

import (
	"sort"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mcore "chatgo/internal/ui/uikit"
)

// acpConfigItem is one selectable control in the composer's ACP row: a
// mode picker or a configuration option.
type acpConfigItem struct {
	label   string
	rank    int
	mode    *acp.SessionMode
	acpsel  *acp.SessionConfigOptionSelect
	boolean *acp.SessionConfigOptionBoolean
}

// acpConfigItems builds the composer's ACP controls: the mode picker, when
// the adapter does not expose modes as config options, and the config
// options themselves. Ordering is model first, then thought level, then
// mode, then whatever else the adapter offers; the model stays the first
// thing a user reaches for.
func (cw *ChatWindow) acpConfigItems(rt *acpConv) []acpConfigItem {
	skipModes := acpConfigCoversModes(rt.configs)
	items := make([]acpConfigItem, 0, len(rt.configs)+1)
	for _, opt := range rt.configs {
		switch {
		case opt.Select != nil:
			items = append(items, acpConfigItem{
				label:  opt.Select.Name,
				rank:   acpConfigRank(opt.Select.Category),
				acpsel: opt.Select,
			})
		case opt.Boolean != nil:
			items = append(items, acpConfigItem{
				label:   opt.Boolean.Name,
				rank:    acpConfigRank(opt.Boolean.Category),
				boolean: opt.Boolean,
			})
		}
	}
	if !skipModes && len(rt.modes) > 1 {
		var cur *acp.SessionMode
		for i := range rt.modes {
			if rt.modes[i].Id == rt.modeID {
				cur = &rt.modes[i]
				break
			}
		}
		if cur != nil {
			items = append(items, acpConfigItem{label: "模式", rank: 2, mode: cur})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].rank < items[j].rank })
	return items
}

// acpConfigRank orders the categories the way a user reaches for them.
func acpConfigRank(cat *acp.SessionConfigOptionCategory) int {
	if cat == nil {
		return 3
	}
	switch *cat {
	case acp.SessionConfigOptionCategoryModel:
		return 0
	case acp.SessionConfigOptionCategoryThoughtLevel:
		return 1
	case acp.SessionConfigOptionCategoryMode:
		return 2
	default:
		return 3
	}
}

// acpConfigName titles a tab or a control for a category.
func acpConfigName(rank int) string {
	switch rank {
	case 0:
		return "模型"
	case 1:
		return "强度"
	case 2:
		return "模式"
	default:
		return "其他"
	}
}

// acpControls shows the ACP session's modes and configuration options: the
// first three as SearchSelect popovers in the composer row, the rest behind
// a settings button whose dialog groups them into category tabs. A settings
// dialog only appears when the extras exist.
func (cw *ChatWindow) acpControls(c *mgo.Context) {
	if cw.cur == nil {
		return
	}
	rt := cw.acpRuntimeFor(cw.cur.ID)
	if rt == nil {
		return
	}
	u := mcore.Density(c).Unit()
	items := cw.acpConfigItems(rt)
	const visible = 3
	mgo.Row(c).Gap(u * 2).Wrap().AlignItems(mgo.End).Children(func() {
		for _, it := range items {
			if it.rank >= visible {
				break
			}
			cw.acpConfigControl(c, rt, it)
		}
		if len(items) > visible {
			if kit.Button(c, "设置", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("settings")}).Clicked() {
				open := cw.acpConfigDialogOpen(c)
				*open = true
			}
		}
	})
	cw.acpConfigDialog(c, rt, items)
}

// acpConfigDialogOpen is the shared dialog state for one frame's controls.
func (cw *ChatWindow) acpConfigDialogOpen(c *mgo.Context) *bool {
	return mgo.Local(c.Root(), "acp:config:open", func() bool { return false })
}

// acpConfigDialog is the multi-tab dialog a "设置" button opens: one tab per
// present category, each holding that category's controls.
func (cw *ChatWindow) acpConfigDialog(c *mgo.Context, rt *acpConv, items []acpConfigItem) {
	open := cw.acpConfigDialogOpen(c)
	if !*open {
		return
	}
	// Tabs in natural order, then unspecified details.
	rankSet := make([]bool, 4)
	for _, it := range items {
		if it.rank < 4 {
			rankSet[it.rank] = true
		} else {
			rankSet[3] = true
		}
	}
	var tabs []kit.Tab
	sel := make(map[int]int)
	tabOf := map[int]int{}
	idx := 0
	for rank := 0; rank < 4; rank++ {
		if !rankSet[rank] {
			continue
		}
		tabs = append(tabs, kit.Tab{Label: acpConfigName(rank)})
		tabOf[rank] = idx
		sel[idx] = rank
		idx++
	}
	val := mgo.Local(c.Root(), "acp:config:tab", func() int { return 0 })
	if *val >= len(tabs) {
		*val = 0
	}
	kit.Dialog(c, open, kit.DialogOptions{Title: "ACP 选项", Width: 460,
		Actions: func() {
			if kit.Button(c, "完成", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				*open = false
			}
		},
	}, func() {
		if len(tabs) > 1 {
			kit.Tabs(c, val, tabs, kit.TabsOptions{})
		}
		rank := sel[*val]
		mgo.Column(c).FillWidth().Gap(mcore.Density(c).Unit() * 2).Children(func() {
			for _, it := range items {
				if it.rank != rank {
					continue
				}
				cw.acpConfigControl(c, rt, it)
			}
		})
	})
}

// acpConfigControl renders one control as a SearchSelect popover, both in
// the composer row and inside the dialog's tabs.
func (cw *ChatWindow) acpConfigControl(c *mgo.Context, rt *acpConv, it acpConfigItem) {
	switch {
	case it.mode != nil:
		opts := make([]kit.SearchSelectOption[string], 0, len(rt.modes))
		for _, m := range rt.modes {
			opts = append(opts, kit.SearchSelectOption[string]{Value: string(m.Id), Label: m.Name})
		}
		cur := string(rt.modeID)
		r := kit.SearchSelect(c, &cur, opts, kit.SearchSelectOptions[string]{
			Label: "模式", Placeholder: it.label, SearchPlaceholder: "搜索模式",
		})
		r.Element.Width(140)
		if r.Changed() {
			cw.acpSetMode(rt, acp.SessionModeId(cur))
		}
	case it.acpsel != nil:
		s := it.acpsel
		cur := string(s.CurrentValue)
		r := kit.SearchSelect(c, &cur, acpSearchSelectOptions(s.Options), kit.SearchSelectOptions[string]{
			Label: s.Name, Placeholder: s.Name, SearchPlaceholder: "搜索" + s.Name,
		})
		r.Element.Width(140)
		if r.Changed() {
			s.CurrentValue = acp.SessionConfigValueId(cur)
			cw.acpSetConfigValue(rt, s.Id, acp.SessionConfigValueId(cur))
		}
	case it.boolean != nil:
		b := it.boolean
		cur := "off"
		if b.CurrentValue {
			cur = "on"
		}
		r := kit.SearchSelect(c, &cur, []kit.SearchSelectOption[string]{
			{Value: "on", Label: "开"},
			{Value: "off", Label: "关"},
		}, kit.SearchSelectOptions[string]{Label: b.Name, Placeholder: b.Name, SearchPlaceholder: "搜索" + b.Name})
		r.Element.Width(110)
		if r.Changed() {
			b.CurrentValue = cur == "on"
			cw.acpSetConfigBool(rt, b.Id, b.CurrentValue)
		}
	}
}

// acpSearchSelectOptions converts an adapter's option list into SearchSelect
// choices, the same content acpSelectOptions feeds a plain Select.
func acpSearchSelectOptions(o acp.SessionConfigSelectOptions) []kit.SearchSelectOption[string] {
	var out []kit.SearchSelectOption[string]
	add := func(list []acp.SessionConfigSelectOption) {
		for _, it := range list {
			out = append(out, kit.SearchSelectOption[string]{Value: string(it.Value), Label: it.Name})
		}
	}
	if o.Ungrouped != nil {
		add(*o.Ungrouped)
	}
	if o.Grouped != nil {
		for _, g := range *o.Grouped {
			add(g.Options)
		}
	}
	return out
}
