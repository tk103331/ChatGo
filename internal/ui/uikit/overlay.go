package uikit

import (
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
)

// OverlayEnter is the fade of a popup appearing, 0 to 1 over PopupDuration.
func OverlayEnter(c *mgo.Context, e mgo.Element) float32 {
	born := mgo.Local(e, "chatgo.overlay.born", func() time.Time { return c.Now() })
	d := Motion(c, theme.PopupDuration)
	if d == 0 {
		return 1
	}
	t := float32(c.Now().Sub(*born)) / float32(d)
	if t >= 1 {
		return 1
	}
	c.AnimationFrame()
	return mgo.EaseOut(t)
}

// OverlayStyle gives a popup panel the surface, edge and shadow of the theme.
func OverlayStyle(c *mgo.Context, panel mgo.Element, p float32) {
	k := Tokens(c)
	panel.Background(k.Surface).Border(theme.BorderWidth, k.Border).Radius(theme.PopupRadius).
		TextColor(k.Text).Opacity(p).Shadow(0, 6, 18, 0, mgo.RGBA(0, 0, 0, 0.16*p))
}

// PlaceOverlay puts a w×h box below anchor a, above when there is no room,
// inside the window.
func PlaceOverlay(c *mgo.Context, a mgo.Rect, w, h, gap float32) (x, y float32) {
	ww, wh := c.Size()
	const edge = 4
	x, y = a.X, a.Y+a.H+gap
	if y+h > wh-edge && a.Y-gap-h >= edge {
		y = a.Y - gap - h
	}
	x = max(edge, min(x, ww-edge-w))
	y = max(edge, min(y, wh-edge-h))
	return x, y
}

// OverlayAnchored is OverlayEnter for a panel anchored to b, which waits a
// frame for a bounds the layout has not measured yet.
func OverlayAnchored(c *mgo.Context, panel mgo.Element, b mgo.Rect) float32 {
	if b.W == 0 && b.H == 0 {
		c.AnimationFrame()
		return 0
	}
	return OverlayEnter(c, panel)
}
