package kit

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/uikit"
)

// SearchInputOptions configure a SearchInput.
type SearchInputOptions struct {
	Placeholder string
	// Label names the field for assistive technology.
	Label    string
	Disabled bool
}

// SearchInput is a single-line field with a magnifier and a clear button.
func SearchInput(c *mgo.Context, value *string, opts SearchInputOptions) InputResult {
	k := uikit.Tokens(c)
	return TextInput(c, value, TextInputOptions{
		Placeholder: opts.Placeholder,
		Label:       opts.Label,
		Disabled:    opts.Disabled,
		Clearable:   true,
		Leading: func() {
			mgo.Icon(c, icons.Must("search")).Size(16, 16).TextColor(k.TextMuted).Shrink(0)
		},
	})
}

// PasswordInputOptions configure a PasswordInput.
type PasswordInputOptions struct {
	Placeholder string
	// Label names the field for assistive technology.
	Label    string
	Disabled bool
	// Error marks the value invalid and shows the message below.
	Error string
}

// PasswordInput is a masked field with a show and hide toggle.
func PasswordInput(c *mgo.Context, value *string, opts PasswordInputOptions) InputResult {
	if value == nil {
		panic("kit: PasswordInput needs a value")
	}
	var r InputResult
	look := &fieldLook{disabled: opts.Disabled, err: opts.Error}
	r.Element, r.Input = fieldFrame(c, look, func(frame mgo.Element) mgo.Element {
		shown := mgo.Local(frame, "shown", func() bool { return false })
		var in mgo.Element
		// MyGo's mask cannot be lifted, so each mode keeps its own editor.
		mode := "masked"
		if *shown {
			mode = "shown"
		}
		mgo.Row(c.Key(mode)).Grow(1).Children(func() {
			in = fieldEditor(c, frame, value, look.disabled, mgo.TextInputBase).
				Grow(1).MinWidth(uikit.Unit(c) * 10).
				Placeholder(opts.Placeholder).Label(opts.Label)
			if !*shown {
				in.Password()
			}
		})
		r.changed = in.Changed() && !look.disabled
		r.submitted = in.Submitted() && !look.disabled
		if passwordToggle(c, *shown) {
			*shown = !*shown
		}
		return in
	}, nil)
	return r
}

func passwordToggle(c *mgo.Context, shown bool) bool {
	name, icon := "显示密码", "eye"
	if shown {
		name, icon = "隐藏密码", "eye-off"
	}
	return uikit.IconAction(c, icons.Must(icon), name).Tooltip(name).Clicked()
}

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
	// Invalid draws the border in the danger color.
	Invalid bool
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

// InputGroup is a field flanked by fixed text and buttons that share its box.
func InputGroup(c *mgo.Context, value *string, opts InputGroupOptions) InputGroupParts {
	if value == nil {
		panic("kit: InputGroup needs a value")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	parts := InputGroupParts{Clicked: -1}

	affix := func(text string) {
		if text == "" {
			return
		}
		mgo.Text(c, text).SingleLine().Shrink(0).TextColor(k.TextMuted)
	}
	button := func(i int, b InputGroupButton) {
		if b.IconOnly {
			if IconButton(c, b.Icon, b.Label, IconButtonOptions{
				Variant: Secondary, Disabled: b.Disabled || opts.Disabled,
			}).Clicked() {
				parts.Clicked = i
			}
			return
		}
		if Button(c, b.Label, ButtonOptions{
			Variant: Secondary, Icon: b.Icon, Disabled: b.Disabled || opts.Disabled,
		}).Clicked() {
			parts.Clicked = i
		}
	}

	look := &fieldLook{disabled: opts.Disabled, readOnly: opts.ReadOnly, invalid: opts.Invalid}
	group, in := fieldFrame(c, look, func(frame mgo.Element) mgo.Element {
		for i, b := range opts.Buttons {
			if b.Leading {
				button(i, b)
			}
		}
		affix(opts.Prefix)
		input := fieldEditor(c, frame, value, opts.ReadOnly || look.disabled, mgo.TextInputBase).
			Grow(1).MinWidth(u * 10).Placeholder(opts.Placeholder).Label(opts.Label)
		parts.changed = input.Changed() && !opts.ReadOnly && !look.disabled
		affix(opts.Suffix)
		for i, b := range opts.Buttons {
			if !b.Leading {
				button(i, b)
			}
		}
		return input
	}, nil)
	parts.Group, parts.Input = group, in
	return parts
}
