package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/uikit"
)

// InputGroupButton is one button attached to an InputGroup's field.
type InputGroupButton struct {
	// Label is shown, or names an icon-only button for assistive technology.
	Label string
	Icon  *mgo.SVG
	// IconOnly hides Label on screen; Icon is then required.
	IconOnly bool
	// Leading puts the button before the input.
	Leading  bool
	Disabled bool
}

// InputGroupOptions configure an InputGroup.
type InputGroupOptions struct {
	// Prefix and Suffix are fixed text beside the input, such as "https://".
	Prefix, Suffix string
	Buttons        []InputGroupButton
	Placeholder    string
	// Label names the input for assistive technology.
	Label    string
	Disabled bool
	ReadOnly bool
	// Error marks the value invalid and shows the message below.
	Error string
}

// InputGroupParts are the parts of an InputGroup.
type InputGroupParts struct {
	Group mgo.Element
	// Input takes the focus.
	Input mgo.Element
	// Clicked is the index in Buttons of the button clicked, or -1.
	Clicked int
	changed bool
}

// Changed reports that the user changed the value this frame.
func (p InputGroupParts) Changed() bool { return p.changed }

// InputGroup is a field with fixed text and buttons beside it, laid out as
// one row.
func InputGroup(c *mgo.Context, value *string, opts InputGroupOptions) InputGroupParts {
	if value == nil {
		panic("kit: InputGroup needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	parts := InputGroupParts{Clicked: -1}

	parts.Group = mgo.Row(c).Gap(u).AlignItems(mgo.Center).Disabled(opts.Disabled)
	disabled := parts.Group.IsDisabled()
	affix := func(text string) {
		if text == "" {
			return
		}
		mgo.Text(c, text).SingleLine().Shrink(0).TextColor(k.TextMuted)
	}
	button := func(i int, b InputGroupButton) {
		opt := ButtonOptions{Variant: Secondary, Icon: b.Icon, Disabled: b.Disabled || disabled}
		label := b.Label
		if b.IconOnly {
			label, opt.Label = "", b.Label
		}
		if Button(c, label, opt).Clicked() {
			parts.Clicked = i
		}
	}
	parts.Group.Children(func() {
		for i, b := range opts.Buttons {
			if b.Leading {
				button(i, b)
			}
		}
		affix(opts.Prefix)
		in := TextInput(c, value, TextInputOptions{
			Placeholder: opts.Placeholder, Label: opts.Label,
			Disabled: disabled, ReadOnly: opts.ReadOnly, Error: opts.Error,
		})
		in.Element.Grow(1).MinWidth(0)
		parts.Input, parts.changed = in.Input, in.Changed()
		affix(opts.Suffix)
		for i, b := range opts.Buttons {
			if !b.Leading {
				button(i, b)
			}
		}
	})
	return parts
}
