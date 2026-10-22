// Package kit builds ChatGo's widgets out of MyGo's native ui primitives and
// the design tokens in internal/ui/theme.
//
// Every widget here draws itself; none of them wrap an external component
// library. Options structs carry only what ChatGo actually uses.
package kit

import (
	"fmt"
	"unicode/utf8"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// InputResult is what a text control built and reported this frame.
type InputResult struct {
	Element mgo.Element
	// Input is the text field inside, for focus and keys.
	Input     mgo.Element
	changed   bool
	submitted bool
}

// Changed reports that the user changed the value this frame.
func (r InputResult) Changed() bool { return r.changed }

// Submitted reports Enter, or the submit shortcut of a TextArea.
func (r InputResult) Submitted() bool { return r.submitted }

// NewInputResult reports a text control built from parts in another widget.
func NewInputResult(e, input mgo.Element, changed, submitted bool) InputResult {
	return InputResult{e, input, changed, submitted}
}

// TextInputOptions configure a TextInput.
type TextInputOptions struct {
	Placeholder string
	// Label names the field for assistive technology.
	Label string
	// Leading and Trailing build content inside the field's edges.
	Leading  func()
	Trailing func()
	// Clearable shows a clear button while there is text; Escape clears too.
	Clearable bool
	Disabled  bool
	// ReadOnly keeps the text selectable and copyable but not editable.
	ReadOnly bool
	// Error marks the value invalid and shows the message below.
	Error string
}

// TextInput is a single-line field editing *value.
func TextInput(c *mgo.Context, value *string, opts TextInputOptions) InputResult {
	if value == nil {
		panic("kit: TextInput needs a value")
	}
	var r InputResult
	look := &fieldLook{disabled: opts.Disabled, readOnly: opts.ReadOnly, err: opts.Error}
	r.Element, r.Input = fieldFrame(c, look, func(frame mgo.Element) mgo.Element {
		if opts.Leading != nil {
			opts.Leading()
		}
		in := fieldEditor(c, frame, value, opts.ReadOnly || look.disabled, mgo.TextInputBase).
			Grow(1).MinWidth(uikit.Unit(c) * 10).
			Placeholder(opts.Placeholder).Label(opts.Label)
		r.changed = in.Changed() && !opts.ReadOnly && !look.disabled
		r.submitted = in.Submitted() && !look.disabled
		if opts.Clearable && *value != "" && !opts.ReadOnly && !look.disabled {
			if fieldClear(c, in) {
				*value = ""
				r.changed = true
			}
		}
		if opts.Trailing != nil {
			opts.Trailing()
		}
		return in
	}, nil)
	return r
}

// TextAreaOptions configure a TextArea.
type TextAreaOptions struct {
	Placeholder string
	Label       string
	// MinHeight and MaxHeight bound the editor; it scrolls past MaxHeight.
	MinHeight float32
	MaxHeight float32
	// MaxLength shows a character count against it; zero hides the count.
	MaxLength int
	// SubmitKey with SubmitMods submits; a zero key means Cmd+Enter.
	SubmitKey  mgo.Key
	SubmitMods mgo.Modifiers
	Disabled   bool
	ReadOnly   bool
	Error      string
	// Borderless drops the field's own border and background so the caller
	// can box the editor together with its own toolbar.
	Borderless bool
	// Keys sees keys before the editor, for a composer's menus.
	Keys func(mgo.InputEvent) bool
}

// TextArea is a multi-line field editing *value.
func TextArea(c *mgo.Context, value *string, opts TextAreaOptions) InputResult {
	if value == nil {
		panic("kit: TextArea needs a value")
	}
	if opts.MinHeight < 0 || opts.MaxHeight < 0 || opts.MaxHeight > 0 && opts.MaxHeight < opts.MinHeight {
		panic("kit: TextArea needs 0 <= MinHeight <= MaxHeight")
	}
	if opts.MaxLength < 0 {
		panic("kit: TextArea MaxLength is negative")
	}
	key, mods := opts.SubmitKey, opts.SubmitMods
	if key == 0 {
		key, mods = mgo.KeyEnter, mgo.Cmd
	}
	n := utf8.RuneCountInString(*value)
	errMsg := opts.Error
	if errMsg == "" && opts.MaxLength > 0 && n > opts.MaxLength {
		errMsg = fmt.Sprintf("超出 %d 个字符", n-opts.MaxLength)
	}
	var r InputResult
	look := &fieldLook{
		disabled: opts.Disabled, readOnly: opts.ReadOnly,
		err: errMsg, area: true, borderless: opts.Borderless,
	}
	u := uikit.Unit(c)
	r.Element, r.Input = fieldFrame(c, look, func(frame mgo.Element) mgo.Element {
		in := fieldEditor(c, frame, value, opts.ReadOnly || look.disabled, mgo.TextAreaBase).
			Grow(1).Padding(u*2, u*3).Placeholder(opts.Placeholder).Label(opts.Label)
		if opts.MinHeight > 0 {
			in.MinHeight(opts.MinHeight)
		}
		if opts.MaxHeight > 0 {
			in.MaxHeight(opts.MaxHeight)
		}
		r.changed = in.Changed() && !opts.ReadOnly && !look.disabled
		// Taken before the editor, which would make a plain Enter a newline.
		submit := mgo.Local(frame, "submit", func() bool { return false })
		in.HandleInput(func(ev mgo.InputEvent) bool {
			if opts.Keys != nil && opts.Keys(ev) {
				return true
			}
			if ev.Kind == mgo.InputKeyDown && ev.Key == key && ev.Mods == mods {
				*submit = true
				return true
			}
			return false
		})
		r.submitted = *submit && !look.disabled
		*submit = false
		return in
	}, textAreaCount(c, n, opts.MaxLength))
	return r
}

// textAreaCount builds the character count against max, or nil without one.
func textAreaCount(c *mgo.Context, n, max int) func() {
	if max == 0 {
		return nil
	}
	return func() {
		k := uikit.Tokens(c)
		color := k.TextMuted
		if n > max {
			color = k.Danger
		}
		mgo.Text(c, fmt.Sprintf("%d / %d", n, max)).TextColor(color).
			FontSize(uikit.FontSize(c, theme.CaptionSize)).FontFeatures("tnum").Shrink(0)
	}
}

// fieldLook is the state a field frame shows; disabled gains any disabled
// ancestor.
type fieldLook struct {
	disabled, readOnly, area bool
	borderless, invalid      bool
	err                      string
}

// fieldFrame builds the bordered field around what build returns as the input.
func fieldFrame(c *mgo.Context, look *fieldLook, build func(frame mgo.Element) mgo.Element, footer func()) (outer, in mgo.Element) {
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	outer = mgo.Column(c).Gap(u).Shrink(0).Disabled(look.disabled)
	// One disabled state, the field's own or an ancestor's, for build to obey.
	look.disabled = outer.IsDisabled()
	outer.Children(func() {
		frame := mgo.Row(c).Radius(theme.ControlRadius).Gap(u * 2).Cursor(mgo.CursorText)
		if look.area {
			frame.AlignItems(mgo.Stretch)
		} else {
			frame.MinHeight(d.ControlHeight()).Padding(0, u*3)
		}
		frame.Children(func() { in = build(frame) })
		in.FocusRing(false)
		if look.disabled && outer.FocusWithin() {
			// A field disabled while focused gives the focus up.
			outer.FocusRing(false).Focus()
		}
		if look.err != "" {
			in.Error(look.err)
		}
		if frame.Clicked() {
			in.Focus()
		}
		bg, border := k.Surface, k.ControlBorder
		switch {
		case look.readOnly:
			bg, border = k.Background, k.Border
		case frame.Hovered():
			border = k.ControlBorder.Mix(k.Text, 0.35)
		}
		switch {
		case look.err != "" || look.invalid:
			border = k.Danger
		case in.Focused() && !look.readOnly:
			border = k.Focus
		}
		if look.borderless {
			frame.BorderWidth(0, 0, 0, 0)
		} else {
			frame.Background(bg).Border(theme.BorderWidth, border).Transition(uikit.ColorFade)
		}
		// FocusVisible is read here, in the build pass: the paint callback
		// runs later, when this element handle is no longer valid.
		ringVisible := in.FocusVisible()
		frame.DrawOver(func(p *mgo.Painter, r mgo.Rect) {
			if ringVisible {
				uikit.FocusRing(p, r, theme.ControlRadius, k.Focus)
			}
		})
		if look.err == "" && footer == nil {
			return
		}
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Start).Children(func() {
			if look.err != "" {
				mgo.Text(c, look.err).TextColor(k.Danger).
					FontSize(uikit.FontSize(c, theme.CaptionSize)).Grow(1)
			} else {
				mgo.Spacer(c)
			}
			if footer != nil {
				footer()
			}
		})
	})
	return outer, in
}

// fieldEditor builds the MyGo editor; a locked one edits a copy, undone at once.
func fieldEditor(c *mgo.Context, frame mgo.Element, value *string, locked bool, base func(*mgo.Context, *string) mgo.Element) mgo.Element {
	if !locked {
		return base(c, value)
	}
	shown := mgo.Local(frame, "locked", func() string { return *value })
	*shown = *value
	in := base(c, shown)
	*shown = *value
	return in
}

// fieldClear builds the clear button and reports a clear by click or Escape.
func fieldClear(c *mgo.Context, in mgo.Element) bool {
	k := uikit.Tokens(c)
	b := mgo.Box(c).Size(theme.MinHitSize, theme.MinHitSize).Shrink(0).Center().
		Radius(theme.ControlRadius).Role(mgo.RoleButton).Label("清除").Cursor(mgo.CursorPointer)
	fg := k.TextMuted
	if b.Hovered() {
		b.Background(k.SurfaceHover)
		fg = k.Text
	}
	b.Children(func() { mgo.Icon(c, icons.Must("x")).Size(14, 14).TextColor(fg) })
	if b.Clicked() || in.Shortcut(0, mgo.KeyEscape) {
		in.Focus()
		return true
	}
	return false
}
