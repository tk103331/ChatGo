package ui

import (
	"sort"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
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

// acpChoiceSink takes a config choice together with the value it replaces:
// the live session sends it to the CLI and restores the old value when the
// CLI refuses; the 新会话 page keeps it for the session it is about to start.
type acpChoiceSink struct {
	selectValue func(id acp.SessionConfigId, value, old acp.SessionConfigValueId)
	boolValue   func(id acp.SessionConfigId, value, old bool)
}

// acpConfigItemsFromOptions builds the controls for a list of config options.
// Ordering is model first, then thought level, then mode, then whatever else
// the adapter offers; the model stays the first thing a user reaches for.
func acpConfigItemsFromOptions(opts []acp.SessionConfigOption) []acpConfigItem {
	items := make([]acpConfigItem, 0, len(opts))
	for _, opt := range opts {
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
	sort.SliceStable(items, func(i, j int) bool { return items[i].rank < items[j].rank })
	return items
}

// acpConfigItems builds the composer's ACP controls: the mode picker, when
// the adapter does not expose modes as config options, and the config
// options themselves.
func (cw *ChatWindow) acpConfigItems(rt *acpConv) []acpConfigItem {
	skipModes := acpConfigCoversModes(rt.configs)
	items := acpConfigItemsFromOptions(rt.configs)
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

// acpConfigName keeps the category labels available to accessibility and
// diagnostics even though the settings UI no longer uses category tabs.
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

// acpControls shows the ACP session's modes and configuration options in the
// composer's row through the shared config row: the first three inline, the
// rest behind the settings popover.
func (cw *ChatWindow) acpControls(c *mgo.Context) {
	if cw.cur == nil {
		return
	}
	rt := cw.acpRuntimeFor(cw.cur.ID)
	if rt == nil {
		return
	}
	items := cw.acpConfigItems(rt)
	scope := "conv:" + cw.cur.ID
	kitItems := make([]kit.ConfigItem, len(items))
	for i, it := range items {
		kitItems[i] = kit.ConfigItem{Name: it.label, Kind: acpOptionKind(it), Rank: it.rank}
	}
	kit.ConfigRow(c, kitItems, scope, func(i int, allowButtons bool) {
		cw.acpConfigControl(c, rt, items[i], scope, allowButtons)
	})
}

// acpConfigControl renders one control of a running session. The choice is
// optimistic on screen but saved on the conversation only once the CLI
// accepts it.
func (cw *ChatWindow) acpConfigControl(c *mgo.Context, rt *acpConv, it acpConfigItem, scope string, allowButtons bool) {
	o := kit.ConfigControlOptions{AllowButtons: allowButtons}
	switch {
	case it.mode != nil:
		opts := make([]kit.SearchSelectOption[string], 0, len(rt.modes))
		for _, m := range rt.modes {
			opts = append(opts, kit.SearchSelectOption[string]{Value: string(m.Id), Label: m.Name})
		}
		old := rt.modeID
		cur := string(rt.modeID)
		o.Key, o.Label = scope+":mode", "模式"
		if kit.ConfigControl(c, &cur, opts, o) {
			cw.acpSetMode(rt, acp.SessionModeId(cur), old)
		}
	case it.acpsel != nil:
		s := it.acpsel
		old := s.CurrentValue
		cur := string(s.CurrentValue)
		o.Key, o.Label = scope+":"+string(s.Id), s.Name
		if s.Description != nil {
			o.Description = *s.Description
		}
		if kit.ConfigControl(c, &cur, acpSearchSelectOptions(s.Options), o) {
			s.CurrentValue = acp.SessionConfigValueId(cur)
			cw.acpSetConfigValue(rt, s.Id, acp.SessionConfigValueId(cur), old)
		}
	case it.boolean != nil:
		b := it.boolean
		old := b.CurrentValue
		cur := "off"
		if old {
			cur = "on"
		}
		o.Key, o.Label = scope+":"+string(b.Id), b.Name
		if b.Description != nil {
			o.Description = *b.Description
		}
		if kit.ConfigControl(c, &cur, []kit.SearchSelectOption[string]{
			{Value: "on", Label: "开"},
			{Value: "off", Label: "关"},
		}, o) {
			b.CurrentValue = cur == "on"
			cw.acpSetConfigBool(rt, b.Id, cur == "on", old)
		}
	}
}

// acpOptionKind names a setting's kind for the settings popover's caption:
// the trigger shows only the current value, so the popover's field label is
// where the option's name and type are spelled out.
func acpOptionKind(it acpConfigItem) string {
	switch {
	case it.mode != nil:
		return "模式"
	case it.boolean != nil:
		return "开关"
	case it.acpsel != nil:
		return "选择"
	default:
		return ""
	}
}

// acpOptionControl renders one config option for a session that has not
// started yet, handing the choice and the value it replaces to the sink.
func (cw *ChatWindow) acpOptionControl(c *mgo.Context, it acpConfigItem, scope string, allowButtons bool, sink acpChoiceSink) {
	o := kit.ConfigControlOptions{AllowButtons: allowButtons}
	switch {
	case it.acpsel != nil:
		s := it.acpsel
		old := s.CurrentValue
		cur := string(s.CurrentValue)
		o.Key, o.Label = scope+":"+string(s.Id), s.Name
		if s.Description != nil {
			o.Description = *s.Description
		}
		if kit.ConfigControl(c, &cur, acpSearchSelectOptions(s.Options), o) {
			s.CurrentValue = acp.SessionConfigValueId(cur)
			sink.selectValue(s.Id, acp.SessionConfigValueId(cur), old)
		}
	case it.boolean != nil:
		b := it.boolean
		old := b.CurrentValue
		cur := "off"
		if old {
			cur = "on"
		}
		o.Key, o.Label = scope+":"+string(b.Id), b.Name
		if b.Description != nil {
			o.Description = *b.Description
		}
		if kit.ConfigControl(c, &cur, []kit.SearchSelectOption[string]{
			{Value: "on", Label: "开"},
			{Value: "off", Label: "关"},
		}, o) {
			b.CurrentValue = cur == "on"
			sink.boolValue(b.Id, cur == "on", old)
		}
	}
}

// acpSearchSelectOptions converts an adapter's option list into SearchSelect
// choices, flattening grouped lists the way a coding CLI reports them.
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
