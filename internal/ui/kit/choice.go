package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
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

// Select chooses one of opts into *value, showing the chosen option's label.
func Select[T comparable](c *mgo.Context, value *T, opts []SelectOption[T], o SelectOptions) SelectResult {
	if value == nil {
		panic("kit: Select needs a value")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()

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
	trigger := sel.Trigger.MinHeight(d.ControlHeight()).Padding(0, u*3).Gap(u * 2).
		Radius(theme.ControlRadius).AlignItems(mgo.Center).Disabled(o.Disabled)
	if o.Label != "" {
		trigger.Label(o.Label)
	}
	uikit.Ring(trigger, k.Focus, theme.ControlRadius)
	border := k.ControlBorder
	switch {
	case trigger.Hovered() && !o.Disabled:
		border = k.ControlBorder.Mix(k.Text, 0.35)
	case sel.Open():
		border = k.Focus
	}
	trigger.Background(k.Surface).Border(theme.BorderWidth, border).Transition(uikit.ColorFade)
	trigger.Children(func() {
		text := mgo.Text(c, label).SingleLine().Grow(1)
		if !chosen {
			text.TextColor(k.TextMuted)
		}
		mgo.Icon(c, icons.Must("chevron-down")).Size(16, 16).TextColor(k.TextMuted).Shrink(0)
	})

	sel.Popup(func(panel mgo.Element) {
		panel.Padding(u).Gap(1).Radius(theme.PopupRadius).
			Background(k.Surface).Border(theme.BorderWidth, k.Border)
		if len(opts) == 0 {
			empty := o.Empty
			if empty == "" {
				empty = "无选项"
			}
			mgo.Text(c, empty).Padding(u*2, u*3).
				FontSize(uikit.FontSize(c, theme.CaptionSize)).TextColor(k.TextMuted)
			return
		}
		for _, opt := range opts {
			item := sel.Item(opt.Value).MinHeight(d.ControlHeight()).
				Padding(0, u*3).Gap(u * 2).Radius(theme.ControlRadius).
				AlignItems(mgo.Center).Disabled(opt.Disabled)
			switch {
			case opt.Disabled:
				item.TextColor(k.TextMuted)
			case item.Highlighted():
				item.Background(k.SurfaceHover).TextColor(k.Text)
			}
			current := opt.Value == *value
			item.Children(func() {
				mgo.Text(c, opt.Label).SingleLine().Grow(1)
				if current {
					mgo.Icon(c, icons.Must("check")).Size(14, 14).
						TextColor(k.AccentText).Shrink(0)
				}
			})
		}
	})
	return SelectResult{Element: trigger, changed: trigger.Changed()}
}

// SwitchOptions configure a Switch.
type SwitchOptions struct {
	Disabled bool
	// Loading shows a pending result: the switch keeps its place and takes
	// no input.
	Loading bool
	// Description is read out after the label.
	Description string
}

const (
	switchW float32 = 36
	switchH float32 = 20
	switchK float32 = 14
)

// Switch turns *on over by click or Space; Changed reports it, the caller
// applies it.
func Switch(c *mgo.Context, on *bool, label string, opts SwitchOptions) mgo.Element {
	if on == nil {
		panic("kit: Switch needs a value")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	blocked := opts.Disabled || opts.Loading
	var sw mgo.Element
	// Disabled on a wrapper blocks the base before it takes input.
	mgo.Row(c).Shrink(0).Disabled(blocked).Children(func() {
		sw = mgo.SwitchBase(c, on).Gap(d.Unit() * 2).AlignItems(mgo.Center).
			MinHeight(d.ControlHeight()).FocusRing(false)
	})
	if opts.Description != "" {
		sw.Description(opts.Description)
	}
	hover := sw.Hovered()
	// FocusVisible is read in the build pass; the paint callback runs later,
	// when this element handle is no longer valid.
	ringVisible := sw.FocusVisible()
	pos := sw.Animate("on", switchFloat(*on), uikit.Motion(c, theme.StateDuration))
	checked := *on
	sw.Children(func() {
		track := mgo.Box(c).Size(switchW, switchH).Shrink(0).Radius(theme.CardRadius)
		bg, border := k.Surface, k.ControlBorder
		thumb := k.ControlBorder
		if checked {
			bg, border, thumb = k.Accent, k.Accent, k.OnAccent
			if hover {
				bg, border = k.AccentHover, k.AccentHover
			}
		} else if hover {
			bg, border, thumb = k.SurfaceHover, k.Text, k.Text
		}
		track.Background(bg).Border(theme.BorderWidth, border)
		onAccent, focus := k.OnAccent, k.Focus
		track.Draw(func(p *mgo.Painter, r mgo.Rect) {
			if checked {
				// A short upright bar marks "on" without relying on color.
				p.Fill(mgo.Rect{X: r.X + 9, Y: r.Y + 6, W: 1.5, H: r.H - 12}, onAccent, 0)
			}
		})
		track.DrawOver(func(p *mgo.Painter, r mgo.Rect) {
			if ringVisible {
				uikit.FocusRing(p, r, theme.CardRadius, focus)
			}
		})
		inset := (switchH - switchK) / 2
		x := inset + pos*(switchW-switchK-2*inset)
		track.Children(func() {
			t := mgo.Box(c).Absolute().Left(x).Top(inset).Size(switchK, switchK).
				Radius(theme.ControlRadius).Background(thumb).Center()
			if opts.Loading {
				t.Children(func() { uikit.Spinner(c, uikit.SpinnerOptions{Size: 10, Color: bg}) })
			}
		})
		if label != "" {
			mgo.Text(c, label).SingleLine()
		}
	})
	return sw
}

func switchFloat(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// CheckboxOptions configure a Checkbox.
type CheckboxOptions struct {
	Disabled bool
	// Label names a box whose text lives elsewhere, for assistive technology.
	Label string
}

// Checkbox toggles *checked, with text beside the box when label is set.
func Checkbox(c *mgo.Context, checked *bool, label string, opts CheckboxOptions) mgo.Element {
	if checked == nil {
		panic("kit: Checkbox needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	side := u * 4

	b := mgo.CheckboxBase(c, checked).Gap(u * 2).AlignItems(mgo.Center).
		MinHeight(theme.MinHitSize).Disabled(opts.Disabled)
	if opts.Label != "" {
		b.Label(opts.Label)
	}
	uikit.Ring(b, k.Focus, theme.ControlRadius)
	on := *checked
	box, mark := k.Surface, k.OnAccent
	border := k.ControlBorder
	if on {
		box, border = k.Accent, k.Accent
		if b.Hovered() && !opts.Disabled {
			box, border = k.AccentHover, k.AccentHover
		}
	} else if b.Hovered() && !opts.Disabled {
		border = k.ControlBorder.Mix(k.Text, 0.35)
	}
	b.Children(func() {
		mgo.Box(c).Size(side, side).Shrink(0).Center().Radius(theme.ControlRadius).
			Background(box).Border(theme.BorderWidth, border).Transition(uikit.ColorFade).
			Children(func() {
				if on {
					mgo.Icon(c, icons.Must("check")).Size(side-u, side-u).TextColor(mark)
				}
			})
		if label != "" {
			mgo.Text(c, label).SingleLine()
		}
	})
	return b
}
