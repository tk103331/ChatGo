package kit

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/uikit"
)

// PromptComposerOptions configure a PromptComposer.
type PromptComposerOptions struct {
	Placeholder string
	// Label names the field; empty uses "消息".
	Label string
	// CmdEnterSends sends with Cmd+Enter (Ctrl+Enter) and makes Enter a
	// newline.
	CmdEnterSends bool
	// Composing blocks submission while the caller's input method composes.
	Composing bool
	// MaxHeight caps input height; zero means 200 DIPs.
	MaxHeight float32
	// Attachments builds the row above the field.
	Attachments func()
	// Tools and Actions build opposite ends of the toolbar.
	Tools, Actions func()
	Disabled       bool
	// Box drops the field's own border and background so the caller can wrap
	// the composer and its toolbar in a single frame.
	Box bool
}

// promptHooks hold menu handlers for the current frame.
type promptHooks struct{ list []func(mgo.InputEvent) bool }

type promptHooksKey struct{}

// PromptComposer edits prompts with configurable send shortcuts. Submitted
// never fires for blank text.
func PromptComposer(c *mgo.Context, value *string, opts PromptComposerOptions) InputResult {
	if value == nil {
		panic("kit: PromptComposer needs a value")
	}
	if opts.MaxHeight < 0 {
		panic("kit: PromptComposer MaxHeight must not be negative")
	}
	u := uikit.Unit(c)
	maxH := opts.MaxHeight
	if maxH == 0 {
		maxH = 200
	}
	label := opts.Label
	if label == "" {
		label = "消息"
	}
	var r InputResult
	r.Element = mgo.Column(c).Gap(u * 2).MinWidth(0).Disabled(opts.Disabled)
	disabled := r.Element.IsDisabled()
	hooks := mgo.Local(r.Element, promptHooksKey{}, func() *promptHooks { return &promptHooks{} })
	h := *hooks
	h.list = h.list[:0]
	r.Element.Children(func() {
		if opts.Attachments != nil {
			mgo.Row(c).Gap(u * 2).Wrap().Children(opts.Attachments)
		}
		ta := TextAreaOptions{
			Label: label, Placeholder: opts.Placeholder,
			MaxHeight: maxH, Borderless: opts.Box,
			Keys: func(ev mgo.InputEvent) bool {
				if disabled {
					return true
				}
				if opts.Composing && ev.Kind == mgo.InputKeyDown {
					return ev.Key == mgo.KeyEnter
				}
				for _, f := range h.list {
					if f(ev) {
						return true
					}
				}
				return false
			},
		}
		if !opts.CmdEnterSends {
			ta.SubmitKey = mgo.KeyEnter
		}
		area := TextArea(c, value, ta)
		r = NewInputResult(r.Element, area.Input, area.Changed(),
			area.Submitted() && !opts.Composing && strings.TrimSpace(*value) != "")
		if opts.Tools != nil || opts.Actions != nil {
			mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				mgo.Row(c).Gap(u).AlignItems(mgo.Center).Grow(1).Children(func() {
					if opts.Tools != nil {
						opts.Tools()
					}
				})
				if opts.Actions != nil {
					opts.Actions()
				}
			})
		}
	})
	return r
}

// promptComposerHook registers a handler before editor input.
func promptComposerHook(composer InputResult, f func(mgo.InputEvent) bool) {
	if !composer.Element.Valid() {
		panic("kit: menus need a built PromptComposer")
	}
	h := *mgo.Local(composer.Element, promptHooksKey{}, func() *promptHooks { return &promptHooks{} })
	h.list = append(h.list, f)
}
