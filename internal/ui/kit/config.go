package kit

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/uikit"
)

// ConfigControlOptions are shared by live sessions and session creation.
type ConfigControlOptions struct {
	Key, Label, Description string
	AllowButtons, Disabled  bool
}

// ConfigControl keeps short and searchable lists visually identical. Button
// groups are reserved for the settings popover, never the composer toolbar.
func ConfigControl(c *mgo.Context, value *string, choices []SearchSelectOption[string], o ConfigControlOptions) bool {
	changed := false
	if o.AllowButtons && len(choices) > 0 && len(choices) <= 3 {
		mgo.Row(c).Gap(uikit.Unit(c)).Wrap().Children(func() {
			for _, choice := range choices {
				variant := Secondary
				if choice.Value == *value {
					variant = Primary
				}
				if Button(c, choice.Label, ButtonOptions{Variant: variant, Disabled: o.Disabled}).Clicked() && choice.Value != *value {
					*value, changed = choice.Value, true
				}
			}
		})
		return changed
	}
	r := SearchSelect(c, value, choices, SearchSelectOptions[string]{
		Key: o.Key, Label: o.Label, Placeholder: o.Label, SearchPlaceholder: "搜索" + o.Label,
		NoSearch: len(choices) <= 10, Up: true, Disabled: o.Disabled || len(choices) == 0,
	})
	tooltip := o.Label
	for _, choice := range choices {
		if choice.Value == *value {
			tooltip += "：" + choice.Label
			break
		}
	}
	if desc := strings.TrimSpace(o.Description); desc != "" {
		tooltip += "\n" + desc
	}
	r.Element.MaxWidth(190).MinWidth(0).Tooltip(tooltip)
	return r.Changed()
}

// ConfigItem describes the placement and visible label of a configuration
// control. Ranks 0–2 are model, thought level and mode; other settings go in
// the popover. Narrow composers keep just the model in the toolbar.
type ConfigItem struct {
	Name, Kind string
	Rank       int
}

func ConfigRow(c *mgo.Context, items []ConfigItem, scope string, control func(int, bool)) {
	if len(items) == 0 {
		return
	}
	u := uikit.Unit(c)
	row := mgo.Row(c).Grow(1).MinWidth(0).Gap(u).Wrap().AlignItems(mgo.Center)
	visible := 3
	// A narrow composer keeps only the model inline; the settings popover
	// holds the rest so the send button never leaves the screen.
	if w := row.Bounds().W; w > 0 && w < 420 {
		visible = 1
	}
	open := mgo.Local(row, "config.open:"+scope, func() bool { return false })
	var extras []int
	var settings mgo.Element
	row.Children(func() {
		for i, item := range items {
			if item.Rank >= visible {
				extras = append(extras, i)
			} else {
				control(i, false)
			}
		}
		if len(extras) > 0 {
			settings = IconButton(c, icons.Must("sliders-horizontal"), "ACP 设置", IconButtonOptions{Variant: Secondary}).Tooltip("ACP 设置")
			if settings.Clicked() {
				*open = !*open
			}
		}
	})
	if len(extras) == 0 {
		*open = false
		return
	}
	Popover(c, settings, open, PopoverOptions{Label: "ACP 设置", Width: 320, Up: true}, func() {
		mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
			for _, i := range extras {
				item := items[i]
				label := item.Name
				if item.Kind != "" {
					label += "（" + item.Kind + "）"
				}
				FieldColumn(c, label, FieldOptions{}, func() { control(i, true) })
			}
		})
	})
}
