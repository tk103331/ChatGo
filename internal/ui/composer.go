package ui

import (
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// composerOptions configure the in-box prompt composer.
type composerOptions struct {
	Placeholder string
	// MaxHeight caps the editor height before it scrolls; zero grows freely.
	MaxHeight float32
	// Tools builds the controls on the left of the in-box footer.
	Tools func()
	// Actions builds the controls on the right of the in-box footer.
	Actions  func()
	Disabled bool
}

// composer edits a message with its model and send controls inside the box.
func composer(c *mgo.Context, value *string, opts composerOptions) kit.InputResult {
	if value == nil {
		panic("chatgo: composer needs a value")
	}
	d := mcore.Density(c)
	u := d.Unit()
	maxH := opts.MaxHeight
	if maxH == 0 {
		maxH = 200
	}
	r := kit.PromptComposer(c, value, kit.PromptComposerOptions{
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
