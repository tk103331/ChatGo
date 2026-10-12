package ui

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

// composerOptions configure the in-box prompt composer.
type composerOptions struct {
	Placeholder string
	// MinHeight is the least editor height before it scrolls; zero fits a line.
	MinHeight float32
	// MaxHeight caps the editor height before it scrolls; zero grows freely.
	MaxHeight float32
	// Tools builds the controls on the left of the in-box footer.
	Tools func()
	// Actions builds the controls on the right of the in-box footer.
	Actions func()
	// Keys sees keys before the editor, for the menus.
	Keys func(mgo.InputEvent) bool
}

// composer edits a message with its model and send controls inside the box.
func composer(c *mgo.Context, value *string, opts composerOptions) minput.InputResult {
	if value == nil {
		panic("chatgo: composer needs a value")
	}
	d := mcore.Density(c)
	u := d.Unit()
	minH := opts.MinHeight
	if minH == 0 {
		minH = 72
	}
	maxH := opts.MaxHeight
	if maxH == 0 {
		maxH = 200
	}
	var r minput.InputResult
	r.Element = mgo.Column(c).Gap(u * 2).MinWidth(0)
	submit := mgo.Local(r.Element, "submit", func() bool { return false })
	r.Element.Children(func() {
		frame := mgo.Column(c).Radius(mtheme.ControlRadius).Gap(u*2).
			Padding(u*2, u*3).Cursor(mgo.CursorText)
		frame.Children(func() {
			in := mgo.TextAreaBase(c, value).Grow(1).MinWidth(0).
				Placeholder(opts.Placeholder).Label(opts.Placeholder).
				MinHeight(minH).MaxHeight(maxH)
			r.Input = in
			in.FocusRing(false)
			changed := in.Changed()
			in.HandleInput(func(ev mgo.InputEvent) bool {
				if opts.Keys != nil && opts.Keys(ev) {
					return true
				}
				if ev.Kind == mgo.InputKeyDown && ev.Key == mgo.KeyEnter && ev.Mods == 0 {
					*submit = true
					return true
				}
				return false
			})
			r = minput.NewInputResult(r.Element, in, changed, *submit)
			*submit = false
			if opts.Tools != nil || opts.Actions != nil {
				mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Wrap().Children(func() {
					mgo.Row(c).Gap(u).AlignItems(mgo.Center).Wrap().Grow(1).Children(func() {
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
	})
	return r
}
