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

// acpChoiceSink takes a config choice: the live session sends it to the CLI,
// the 新会话 page keeps it for the session it is about to start.
type acpChoiceSink struct {
	selectValue func(id acp.SessionConfigId, value acp.SessionConfigValueId)
	boolValue   func(id acp.SessionConfigId, value bool)
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
// composer's row, the way a conversation's composer shows them.
func (cw *ChatWindow) acpControls(c *mgo.Context) {
	if cw.cur == nil {
		return
	}
	rt := cw.acpRuntimeFor(cw.cur.ID)
	if rt == nil {
		return
	}
	cw.acpControlRow(c, cw.acpConfigItems(rt), "session", func(it acpConfigItem, _ bool) {
		cw.acpConfigControl(c, rt, it, false)
	})
}

// acpControlRow shows the first three controls in the composer's row and puts
// the remaining controls in a small settings popover. The popover contains
// only controls that are not already visible in the composer.
func (cw *ChatWindow) acpControlRow(c *mgo.Context, items []acpConfigItem, scope string, control func(acpConfigItem, bool)) {
	u := mcore.Density(c).Unit()
	const visible = 3
	var extras []acpConfigItem
	for _, it := range items {
		if it.rank >= visible {
			extras = append(extras, it)
		}
	}
	if len(extras) == 0 {
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.End).Children(func() {
			for _, it := range items {
				control(it, false)
			}
		})
		return
	}
	open := cw.acpConfigPopoverOpen(c, scope)
	var settings mgo.Element
	mgo.Row(c).Gap(u * 2).AlignItems(mgo.End).Children(func() {
		for _, it := range items {
			if it.rank >= visible {
				continue
			}
			control(it, false)
		}
		settings = kit.IconButton(c, icons.Must("sliders-horizontal"), "ACP 设置", kit.IconButtonOptions{Variant: kit.Secondary}).
			Tooltip("ACP 设置")
		if settings.Clicked() {
			*open = !*open
		}
	})
	kit.Popover(c, settings, open, kit.PopoverOptions{
		Label: "ACP 设置", Width: 320, Up: true,
	}, func() {
		mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
			for _, it := range extras {
				kit.FieldColumn(c, acpOptionTypeLabel(it), kit.FieldOptions{}, func() {
					control(it, true)
				})
			}
		})
	})
}

// acpOptionTypeLabel gives each setting a visible type/name in the popover;
// the trigger itself only needs to show its current value.
func acpOptionTypeLabel(it acpConfigItem) string {
	switch {
	case it.mode != nil:
		return "模式"
	case it.boolean != nil:
		return it.label + "（开关）"
	case it.acpsel != nil:
		return it.label + "（选择）"
	default:
		return it.label
	}
}

// acpConfigPopoverOpen is the shared popover state for one frame's controls.
func (cw *ChatWindow) acpConfigPopoverOpen(c *mgo.Context, scope string) *bool {
	return mgo.Local(c.Root(), "acp:config:open:"+scope, func() bool { return false })
}

// acpConfigControl renders one control of a running session: the mode picker
// and the config options, which the session sends to the CLI.
func (cw *ChatWindow) acpConfigControl(c *mgo.Context, rt *acpConv, it acpConfigItem, allowButtons bool) {
	switch {
	case it.mode != nil:
		opts := make([]kit.SearchSelectOption[string], 0, len(rt.modes))
		for _, m := range rt.modes {
			opts = append(opts, kit.SearchSelectOption[string]{Value: string(m.Id), Label: m.Name})
		}
		cur := string(rt.modeID)
		acpChoiceControl(c, &cur, opts, "模式", allowButtons, func(value string) {
			cw.acpSetMode(rt, acp.SessionModeId(value))
		})
	default:
		cw.acpOptionControl(c, it, acpChoiceSink{
			selectValue: func(id acp.SessionConfigId, value acp.SessionConfigValueId) {
				cw.acpSetConfigValue(rt, id, value)
			},
			boolValue: func(id acp.SessionConfigId, value bool) {
				cw.acpSetConfigBool(rt, id, value)
			},
		}, allowButtons)
	}
}

// acpOptionControl renders one config option as a SearchSelect popover in the
// composer row or the ACP settings popover, and hands the choice to the sink.
func (cw *ChatWindow) acpOptionControl(c *mgo.Context, it acpConfigItem, sink acpChoiceSink, allowButtons bool) {
	switch {
	case it.acpsel != nil:
		s := it.acpsel
		cur := string(s.CurrentValue)
		acpChoiceControl(c, &cur, acpSearchSelectOptions(s.Options), s.Name, allowButtons, func(value string) {
			s.CurrentValue = acp.SessionConfigValueId(value)
			sink.selectValue(s.Id, acp.SessionConfigValueId(value))
		})
	case it.boolean != nil:
		b := it.boolean
		cur := "off"
		if b.CurrentValue {
			cur = "on"
		}
		acpChoiceControl(c, &cur, []kit.SearchSelectOption[string]{
			{Value: "on", Label: "开"},
			{Value: "off", Label: "关"},
		}, b.Name, allowButtons, func(value string) {
			b.CurrentValue = cur == "on"
			sink.boolValue(b.Id, b.CurrentValue)
		})
	}
}

// acpChoiceControl keeps the composer on a select control, while allowing
// the compact ACP settings popover to turn very small option sets into
// buttons. Up to ten values use a plain select; larger lists are searchable.
func acpChoiceControl(c *mgo.Context, value *string, opts []kit.SearchSelectOption[string], label string, allowButtons bool, changed func(string)) {
	if allowButtons && len(opts) <= 3 {
		mgo.Row(c).Gap(mcore.Density(c).Unit()).Wrap().Children(func() {
			for _, opt := range opts {
				opt := opt
				variant := kit.Secondary
				if opt.Value == *value {
					variant = kit.Primary
				}
				if kit.Button(c, opt.Label, kit.ButtonOptions{Variant: variant}).Clicked() {
					*value = opt.Value
					changed(opt.Value)
				}
			}
		})
		return
	}
	if len(opts) <= 10 {
		plain := make([]kit.SelectOption[string], 0, len(opts))
		for _, opt := range opts {
			plain = append(plain, kit.SelectOption[string]{Value: opt.Value, Label: opt.Label})
		}
		r := kit.Select(c, value, plain, kit.SelectOptions{Label: label, Placeholder: label, MinWidth: 130})
		if r.Changed() {
			changed(*value)
		}
		return
	}
	r := kit.SearchSelect(c, value, opts, kit.SearchSelectOptions[string]{
		Label: label, Placeholder: label, SearchPlaceholder: "搜索" + label, Up: true,
	})
	r.Element.MaxWidth(130).MinWidth(0)
	if r.Changed() {
		changed(*value)
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
