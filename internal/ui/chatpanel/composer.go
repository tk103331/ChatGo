package chatpanel

import (
	mgo "github.com/egoist/mygo/ui"

	mchat "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/chat"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// ComposerOptions configure the in-box prompt composer.
type ComposerOptions struct {
	Placeholder string
	// MaxHeight caps the editor height before it scrolls; zero grows freely.
	MaxHeight float32
	// Tools builds the controls on the left of the in-box footer.
	Tools func()
	// Actions builds the controls on the right of the in-box footer.
	Actions  func()
	Disabled bool
}

// Composer edits a message with its model and send controls inside the box.
func Composer(c *mgo.Context, value *string, opts ComposerOptions) minput.InputResult {
	if value == nil {
		panic("chatgo: composer needs a value")
	}
	d := mcore.Density(c)
	u := d.Unit()
	maxH := opts.MaxHeight
	if maxH == 0 {
		maxH = 200
	}
	r := mchat.PromptComposer(c, value, mchat.PromptComposerOptions{
		Placeholder: opts.Placeholder,
		MaxHeight:   maxH,
		Tools:       opts.Tools,
		Actions:     opts.Actions,
		Disabled:    opts.Disabled,
		Box:         true,
	})
	frame := r.Element
	frame.Radius(mtheme.ControlRadius).Padding(u*2, u*3).Cursor(mgo.CursorText)
	if frame.Clicked() {
		r.Input.Focus()
	}
	k := mcore.Tokens(c)
	bg, border := k.Surface, k.ControlBorder
	switch {
	case frame.Hovered():
		border = k.ControlBorder.Mix(k.Text, 0.35)
	case r.Input.Focused():
		border = k.Focus
	}
	frame.Background(bg).Border(mtheme.BorderWidth, border).Transition(mcore.ColorFade)
	frame.DrawOver(func(p *mgo.Painter, rect mgo.Rect) {
		if r.Input.FocusVisible() {
			p.FocusRing(rect, [4]float32{mtheme.ControlRadius, mtheme.ControlRadius, mtheme.ControlRadius, mtheme.ControlRadius})
		}
	})
	return r
}
