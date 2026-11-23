package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
)

// sheet shows a bottom panel over the page while *open. It is the phone's
// own sheet rather than mobile.Sheet's: a modal sheet keeps its dialog in
// the last frame it built, and the phone then stops routing the system's
// back to the page after the sheet closed.
func sheet(c *ui.Context, open *bool, o mobile.SheetOptions, content func()) {
	if !*open {
		return
	}
	t := c.Theme()
	_, winH := c.Size()
	detents := o.Detents
	if len(detents) == 0 {
		detents = []float32{0.5}
	}
	tall := float32(0)
	for _, d := range detents {
		tall = max(tall, d)
	}
	height := winH * tall
	_, _, bottom, _ := c.SafeArea()
	bg := t.Background
	if o.Background != nil {
		bg = *o.Background
	}
	ui.Overlay(c, func() {
		back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		if !o.NoDismiss && (back.Clicked() || back.OverlayShortcut(0, ui.KeyEscape)) {
			*open = false
			return
		}
		back.Background(ui.RGBA(0, 0, 0, 0.4))
		back.Children(func() {
			panel := ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Height(height).
				Background(bg).Radius(28, 28, 0, 0).
				Shadow(0, -2, 16, 0, ui.RGBA(0, 0, 0, 0.18)).
				Role(ui.RoleDialog).Label(o.Label)
			panel.Clicked()
			panel.Children(func() {
				if !o.NoGrabber {
					ui.Box(c).Width(36).Height(5).Radius(2.5).Background(t.TextMuted.Alpha(0.35)).
						AlignSelf(ui.Center).Margin(8, 0, 4, 0)
				}
				ui.Column(c).Grow(1).Basis(0).Padding(0, 0, bottom, 0).Children(content)
			})
		})
	})
}
