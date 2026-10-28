// Package kit builds ChatGo's widgets on MyGo's own controls.
//
// Each widget wraps a native ui control, adding only what ChatGo needs that
// the control does not carry: a leading icon and an emphasis on buttons,
// key-value options on selects, and the composites MyGo has no control for.
// The look is MyGo's own, read from the window theme.
package kit

import (
	"fmt"
	"unicode/utf8"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// InputResult is what a text control built and reported this frame.
type InputResult struct {
	Element mgo.Element
	// Input is the text field itself, for focus and keys: the control for
	// the single fields here, and the editor inside for the composites.
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
	Label    string
	Disabled bool
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
	in := mgo.TextInput(c, value).Placeholder(opts.Placeholder).
		Disabled(opts.Disabled).ReadOnly(opts.ReadOnly)
	if opts.Label != "" {
		in.Label(opts.Label)
	}
	if opts.Error != "" {
		in.Error(opts.Error)
	}
	return InputResult{
		Element: in, Input: in,
		changed:   in.Changed() && !opts.ReadOnly && !opts.Disabled,
		submitted: in.Submitted() && !opts.Disabled,
	}
}

// PasswordInputOptions configure a PasswordInput.
type PasswordInputOptions struct {
	Placeholder string
	// Label names the field for assistive technology.
	Label    string
	Disabled bool
	Error    string
}

// PasswordInput is a masked single-line field.
func PasswordInput(c *mgo.Context, value *string, opts PasswordInputOptions) InputResult {
	if value == nil {
		panic("kit: PasswordInput needs a value")
	}
	in := mgo.TextInput(c, value).Password().Placeholder(opts.Placeholder).
		Disabled(opts.Disabled)
	if opts.Label != "" {
		in.Label(opts.Label)
	}
	if opts.Error != "" {
		in.Error(opts.Error)
	}
	return InputResult{
		Element: in, Input: in,
		changed:   in.Changed() && !opts.Disabled,
		submitted: in.Submitted() && !opts.Disabled,
	}
}

// SearchInputOptions configure a SearchInput.
type SearchInputOptions struct {
	Placeholder string
	// Label names the field for assistive technology.
	Label    string
	Disabled bool
}

// SearchInput is MyGo's search field: a magnifier, and a clear button while
// there is text.
func SearchInput(c *mgo.Context, value *string, opts SearchInputOptions) InputResult {
	if value == nil {
		panic("kit: SearchInput needs a value")
	}
	// The bare search field is shorter than a text input; match the control
	// height so a search box lines up with the buttons beside it.
	in := mgo.SearchField(c, value).Placeholder(opts.Placeholder).Disabled(opts.Disabled).
		MinHeight(uikit.Density(c).ControlHeight())
	if opts.Label != "" {
		in.Label(opts.Label)
	}
	return InputResult{
		Element: in, Input: in,
		changed:   in.Changed() && !opts.Disabled,
		submitted: in.Submitted() && !opts.Disabled,
	}
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
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	n := utf8.RuneCountInString(*value)
	errMsg := opts.Error
	if errMsg == "" && opts.MaxLength > 0 && n > opts.MaxLength {
		errMsg = fmt.Sprintf("超出 %d 个字符", n-opts.MaxLength)
	}

	var r InputResult
	r.Element = mgo.Column(c).Gap(u).Shrink(0).Disabled(opts.Disabled)
	disabled := r.Element.IsDisabled()
	r.Element.Children(func() {
		in := mgo.TextArea(c, value).Placeholder(opts.Placeholder).
			Disabled(disabled).ReadOnly(opts.ReadOnly)
		r.Input = in
		if opts.Label != "" {
			in.Label(opts.Label)
		}
		if opts.MinHeight > 0 {
			in.MinHeight(opts.MinHeight)
		}
		if opts.MaxHeight > 0 {
			in.MaxHeight(opts.MaxHeight)
		}
		if opts.Borderless {
			in.BorderWidth(0, 0, 0, 0).Background(mgo.Transparent)
		}
		if errMsg != "" {
			in.Error(errMsg)
		}
		r.changed = in.Changed() && !opts.ReadOnly && !disabled
		// Taken before the editor, which would make a plain Enter a newline.
		submit := mgo.Local(r.Element, "submit", func() bool { return false })
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
		r.submitted = *submit && !disabled
		*submit = false
		if errMsg == "" && opts.MaxLength == 0 {
			return
		}
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Start).Children(func() {
			if errMsg != "" {
				mgo.Text(c, errMsg).TextColor(k.Danger).
					FontSize(uikit.FontSize(c, theme.CaptionSize)).Grow(1)
			} else {
				mgo.Spacer(c)
			}
			if opts.MaxLength > 0 {
				color := k.TextMuted
				if n > opts.MaxLength {
					color = k.Danger
				}
				mgo.Text(c, fmt.Sprintf("%d / %d", n, opts.MaxLength)).
					TextColor(color).FontSize(uikit.FontSize(c, theme.CaptionSize)).
					FontFeatures("tnum").Shrink(0)
			}
		})
	})
	return r
}
