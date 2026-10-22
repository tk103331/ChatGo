package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// SelectOption is one choice of a Select.
type SelectOption[T comparable] struct {
	Value    T
	Label    string
	Disabled bool
}

// SelectOptions configure a Select.
type SelectOptions struct {
	// Placeholder shows while no option holds the value; empty uses "请选择…".
	Placeholder string
	Disabled    bool
	// Empty shows in the popup when there are no options; empty uses "无选项".
	Empty string
	// Label names the select for assistive technology.
	Label string
}

// SelectResult is what a Select built and reported this frame.
type SelectResult struct {
	Element mgo.Element
	changed bool
}

// Changed reports that the user chose another option this frame.
func (r SelectResult) Changed() bool { return r.changed }

// selectChevron draws the arrow of a select, as MyGo draws its own.
func selectChevron(c *mgo.Context, k *mgo.Theme) {
	u := uikit.Unit(c)
	color := k.TextMuted
	mgo.Box(c).Size(u*2.5, u*2.5).Shrink(0).Draw(func(p *mgo.Painter, r mgo.Rect) {
		var path mgo.Path
		path.MoveTo(r.X+r.W*0.1, r.Y+r.H*0.3).
			LineTo(r.X+r.W*0.5, r.Y+r.H*0.7).
			LineTo(r.X+r.W*0.9, r.Y+r.H*0.3)
		p.StrokePath(&path, 1.5, color)
	})
}

// Select chooses one of opts into *value, looking as MyGo's own Select does.
// MyGo's Select takes strings only, so this carries the value's type.
func Select[T comparable](c *mgo.Context, value *T, opts []SelectOption[T], o SelectOptions) SelectResult {
	if value == nil {
		panic("kit: Select needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)

	label := o.Placeholder
	if label == "" {
		label = "请选择…"
	}
	chosen := false
	for _, opt := range opts {
		if opt.Value == *value {
			label, chosen = opt.Label, true
			break
		}
	}

	sel := mgo.SelectBase(c, value)
	trigger := sel.Trigger.Justify(mgo.SpaceBetween).MinWidth(u*35).
		Padding(u*1.5, u*3.5).Gap(u * 1.5).Radius(k.Radius).
		Background(k.Surface).TextColor(k.Text).Disabled(o.Disabled)
	if o.Label != "" {
		trigger.Label(o.Label)
	}
	if trigger.Hovered() && !o.Disabled {
		trigger.Background(k.SurfaceHover)
	}
	trigger.Children(func() {
		text := mgo.Text(c, label).SingleLine()
		if !chosen {
			text.TextColor(k.TextMuted)
		}
		selectChevron(c, k)
	})

	sel.Popup(func(panel mgo.Element) {
		panel.Margin(u, 0, 0, 0).Padding(u).Radius(k.Radius+2).
			Background(k.Background).Border(theme.BorderWidth, k.Border).
			Shadow(0, 6, 20, 0, mgo.RGBA(0, 0, 0, 0.18))
		if len(opts) == 0 {
			empty := o.Empty
			if empty == "" {
				empty = "无选项"
			}
			mgo.Text(c, empty).Padding(u*1.5, u*2.5).
				FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
			return
		}
		for _, opt := range opts {
			item := sel.Item(opt.Value).Padding(u*1.5, u*2.5).Radius(k.Radius).
				Disabled(opt.Disabled)
			switch {
			case item.Highlighted():
				item.Background(k.Accent).TextColor(k.AccentText)
			case opt.Value == *value:
				item.Background(k.Surface)
			}
			item.Children(func() { mgo.Text(c, opt.Label).SingleLine() })
		}
	})
	return SelectResult{Element: trigger, changed: trigger.Changed()}
}
