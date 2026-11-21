package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
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
	Icon *mgo.SVG
	// Leading draws a custom mark before the label, for a logo or avatar an
	// Icon cannot carry; it replaces Icon when both are set.
	Leading  func()
	Loading  bool
	Disabled bool
	// Label names an icon-only button for assistive technology; required
	// without a label.
	Label string
	// Foreground overrides the variant's text color, for a ghost control that
	// should read as body text rather than a link. The zero color keeps the
	// variant's own.
	Foreground mgo.Color
	// Chevron adds a trailing down arrow, marking the button as a dropdown
	// trigger.
	Chevron bool
}

// buttonIconSize is the leading icon slot, which the spinner takes while
// loading.
const buttonIconSize float32 = 16

// Button is MyGo's button, with a leading icon and the emphasis ChatGo asks
// for. Loading keeps its size and blocks clicks.
func Button(c *mgo.Context, label string, opts ButtonOptions) mgo.Element {
	if label == "" && opts.Label == "" {
		panic("kit: Button without a label needs Label to name it")
	}
	k := uikit.Tokens(c)
	// The label is built here, so the icon and the spinner share its row.
	var b mgo.Element
	if opts.Variant == Primary {
		b = mgo.PrimaryButton(c, "")
	} else {
		b = mgo.Button(c, "")
	}
	b.Disabled(opts.Disabled || opts.Loading)
	if opts.Label != "" {
		b.Label(opts.Label)
	}
	fg := variantLook(b, k, opts.Variant, opts.Disabled)
	if opts.Foreground.A != 0 {
		fg = opts.Foreground
		b.TextColor(fg)
	}
	b.Children(func() {
		switch {
		case opts.Loading:
			uikit.Spinner(c, uikit.SpinnerOptions{Size: buttonIconSize, Color: fg})
		case opts.Leading != nil:
			opts.Leading()
		case opts.Icon != nil:
			icon := mgo.Icon(c, opts.Icon).Size(buttonIconSize, buttonIconSize)
			if opts.Foreground.A != 0 {
				icon.TextColor(opts.Foreground)
			}
		}
		if label != "" {
			t := mgo.Text(c, label).SingleLine()
			if opts.Foreground.A != 0 {
				t.TextColor(opts.Foreground)
			}
		}
		if opts.Chevron {
			mgo.Icon(c, icons.Must("chevron-down")).Size(12, 12).Shrink(0).
				TextColor(k.TextMuted)
		}
	})
	return b
}

// variantLook recolors a native button for the variants MyGo has no look
// for, and returns the text color so the spinner matches.
//
// Primary and Secondary keep MyGo's own look, whose hover and pressed states
// the button tracks itself; the others set every state here.
func variantLook(b mgo.Element, k *mgo.Theme, v ButtonVariant, disabled bool) mgo.Color {
	hover := b.Hovered() && !disabled
	switch v {
	case Primary:
		return k.AccentText
	case Secondary:
		return k.Text
	case Outline:
		fg := k.Accent
		b.Background(mgo.Transparent).Border(theme.BorderWidth, k.Accent).TextColor(fg)
		if b.Pressed() {
			b.Background(k.Selection)
		} else if hover {
			b.Background(k.SurfaceHover)
		}
		return fg
	case Ghost:
		fg := k.Accent
		b.Background(mgo.Transparent).BorderWidth(0, 0, 0, 0).TextColor(fg)
		if b.Pressed() {
			b.Background(k.Selection)
		} else if hover {
			b.Background(k.SurfaceHover)
		}
		return fg
	case Danger:
		fg, bg := k.AccentText, k.Danger
		switch {
		case b.Pressed():
			bg = k.Danger.Mix(mgo.RGB(0, 0, 0), 0.25)
		case hover:
			bg = k.Danger.Mix(mgo.RGB(0, 0, 0), 0.12)
		}
		b.Background(bg).BorderWidth(0, 0, 0, 0).TextColor(fg)
		return fg
	}
	return k.Text
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
	size := opts.Size
	if size == 0 {
		size = uikit.Density(c).ControlHeight()
	}
	b := Button(c, "", ButtonOptions{
		Variant: opts.Variant, Icon: icon, Disabled: opts.Disabled, Label: label,
	})
	return b.Size(size, size).Padding(0)
}
