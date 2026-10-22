package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// ButtonVariant is a button's emphasis. The zero value is Primary.
type ButtonVariant int

const (
	Primary ButtonVariant = iota
	Secondary
	Outline
	Ghost
	Danger
)

// ButtonOptions configure a Button.
type ButtonOptions struct {
	Variant ButtonVariant
	// Icon goes before the label.
	Icon     *mgo.SVG
	Loading  bool
	Disabled bool
	// Label names an icon-only button for assistive technology; required
	// without a label.
	Label string
}

// buttonIconSize is the leading icon slot, which the spinner takes while
// loading.
const buttonIconSize float32 = 16

// Button is a push button. Loading keeps its size and blocks clicks.
func Button(c *mgo.Context, label string, opts ButtonOptions) mgo.Element {
	if label == "" && opts.Label == "" {
		panic("kit: Button without a label needs Label to name it")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	pad, gap := d.Unit()*4, d.Unit()*2
	if opts.Loading && opts.Icon == nil && label != "" {
		// The spinner slot comes out of the padding, so the width holds.
		pad -= (buttonIconSize + gap) / 2
	}
	b := mgo.ButtonBase(c).MinHeight(d.ControlHeight()).MinWidth(theme.MinHitSize).
		Padding(0, pad).Gap(gap).Radius(theme.ControlRadius).
		Disabled(opts.Disabled || opts.Loading)
	uikit.Ring(b, k.Focus, theme.ControlRadius)
	if opts.Label != "" {
		b.Label(opts.Label)
	}
	bg, fg, border := ButtonColors(k, opts.Variant, b.Hovered() && !opts.Disabled, b.Pressed())
	b.Background(bg).TextColor(fg)
	if border.A > 0 {
		b.Border(theme.BorderWidth, border)
	}
	b.Children(func() {
		if opts.Loading {
			uikit.Spinner(c, uikit.SpinnerOptions{Size: buttonIconSize, Color: fg})
		} else if opts.Icon != nil {
			mgo.Icon(c, opts.Icon).Size(buttonIconSize, buttonIconSize)
		}
		if label != "" {
			mgo.Text(c, label).SingleLine()
		}
	})
	return b
}

// ButtonColors returns the fill, text and border color of variant v in state
// hover / pressed.
func ButtonColors(k theme.Tokens, v ButtonVariant, hover, pressed bool) (bg, fg, border mgo.Color) {
	switch v {
	case Secondary:
		bg, fg, border = k.Surface, k.Text, k.ControlBorder
	case Outline:
		bg, fg, border = mgo.Transparent, k.AccentText, k.AccentText
	case Ghost:
		bg, fg = mgo.Transparent, k.AccentText
	case Danger:
		bg, fg = k.DangerFill, k.OnDanger
		if pressed {
			bg = k.DangerFill.Mix(mgo.RGB(0, 0, 0), 0.25)
		} else if hover {
			bg = k.DangerFill.Mix(mgo.RGB(0, 0, 0), 0.12)
		}
		return
	default:
		bg, fg = k.Accent, k.OnAccent
		if pressed {
			bg = k.AccentPressed
		} else if hover {
			bg = k.AccentHover
		}
		return
	}
	if pressed {
		bg = k.Selection
	} else if hover {
		bg = k.SurfaceHover
	}
	return
}

// IconButtonOptions configure an IconButton.
type IconButtonOptions struct {
	Variant  ButtonVariant
	Disabled bool
	// Size is the square side in DIPs; zero uses the density's control height.
	Size float32
}

// IconButton is a square button showing only icon, named label.
func IconButton(c *mgo.Context, icon *mgo.SVG, label string, opts IconButtonOptions) mgo.Element {
	if label == "" {
		panic("kit: IconButton needs a label to name it")
	}
	k := uikit.Tokens(c)
	size := opts.Size
	if size == 0 {
		size = uikit.Density(c).ControlHeight()
	}
	b := mgo.ButtonBase(c).Size(size, size).Radius(theme.ControlRadius).
		Label(label).Disabled(opts.Disabled)
	uikit.Ring(b, k.Focus, theme.ControlRadius)
	bg, fg, border := ButtonColors(k, opts.Variant, b.Hovered() && !opts.Disabled, b.Pressed())
	b.Background(bg).TextColor(fg)
	if border.A > 0 {
		b.Border(theme.BorderWidth, border)
	}
	b.Children(func() { mgo.Icon(c, icon).Size(buttonIconSize, buttonIconSize) })
	return b
}
