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

// OverlayScrim is the wash over the window behind a modal layer.
func OverlayScrim(k *mgo.Theme) mgo.Color {
	if k.Dark {
		return mgo.RGBA(0, 0, 0, 0.6)
	}
	return k.Text.Alpha(0.42)
}

// overlayLayers is the window's open layers, oldest first.
type overlayLayers struct {
	root mgo.Element // compared only: a new root means a new build pass
	pass int
	keys []any
	seen []int
}

type overlayLayersKey struct{}

// OverlayLayer marks the layer key open in this frame and reports whether it
// is on top.
func OverlayLayer(c *mgo.Context, key any) bool {
	s := mgo.Local(c.Root(), overlayLayersKey{}, func() overlayLayers { return overlayLayers{} })
	if root := c.Root(); root != s.root {
		s.root = root
		s.pass++
		n := 0
		for i, k := range s.keys {
			if s.seen[i] >= s.pass-1 { // drop layers the last pass did not build
				s.keys[n], s.seen[n] = k, s.seen[i]
				n++
			}
		}
		clear(s.keys[n:])
		s.keys, s.seen = s.keys[:n], s.seen[:n]
	}
	at := -1
	for i, k := range s.keys {
		if k == key {
			at = i
		}
	}
	if at < 0 {
		s.keys, s.seen = append(s.keys, key), append(s.seen, s.pass)
		at = len(s.keys) - 1
	}
	s.seen[at] = s.pass
	return at == len(s.keys)-1
}

// OverlayEscape reports Escape for a window-level layer, delivered only while
// it is on top.
func OverlayEscape(c *mgo.Context, key any) bool {
	if !OverlayLayer(c, key) {
		return false
	}
	// The root is around every focus, so it takes Escape before MyGo's overlays.
	byFocus := c.Root().Shortcut(0, mgo.KeyEscape)
	byWindow := c.Shortcut(0, mgo.KeyEscape)
	if byFocus || byWindow {
		s := mgo.Local(c.Root(), overlayLayersKey{}, func() overlayLayers { return overlayLayers{} })
		for i, k := range s.keys {
			if k == key {
				s.keys = append(s.keys[:i], s.keys[i+1:]...)
				s.seen = append(s.seen[:i], s.seen[i+1:]...)
				break
			}
		}
		c.Invalidate()
		return true
	}
	return false
}

// OverlaySiteState follows the focus of a layer after it closes.
type OverlaySiteState struct {
	focusIn bool // the focus was inside the layer when it last showed
	wait    int  // frames left to check the focus after a close
	holding bool // the site holds the fallback focus
}

// OverlaySite builds the invisible focus fallback at a layer's call site.
func OverlaySite(c *mgo.Context) (mgo.Element, *OverlaySiteState) {
	site := mgo.Box(c).Size(0, 0).FocusRing(false)
	st := mgo.Local(site, "chatgo.overlay.site", func() OverlaySiteState { return OverlaySiteState{} })
	st.holding = st.holding && site.FocusWithin()
	if st.holding {
		site.Focusable() // in flow, so Tab goes on from the layer's place
	} else {
		site.Absolute()
	}
	return site, st
}

// Shown records whether the focus is in the open layer's panel.
func (st *OverlaySiteState) Shown(panel mgo.Element) {
	st.focusIn, st.wait = panel.Valid() && panel.FocusWithin(), 0
}

// Closed moves a lost focus to the site once MyGo has had its chance to
// restore it.
func (st *OverlaySiteState) Closed(c *mgo.Context, site mgo.Element) {
	if st.focusIn {
		st.focusIn, st.wait = false, 2 // MyGo prunes the panel at the end of the next frame
	}
	if st.wait == 0 {
		return
	}
	st.wait--
	if st.wait > 0 {
		c.AnimationFrame()
		return
	}
	// The site itself counts as lost: it may inherit a vanished opener's
	// positional ID.
	if !c.Root().FocusWithin() || site.FocusWithin() {
		site.Focus()
		st.holding = true
	}
}

// AutoFocus gives e the initial focus of an overlay once, on the frame after
// it builds: MyGo records an overlay's opener after its content builds, so a
// same-frame autofocus loses focus restore on close.
func AutoFocus(c *mgo.Context, e mgo.Element) {
	done := mgo.Local(e, "chatgo.overlay.autofocus", func() bool { return false })
	if *done {
		return
	}
	*done = true
	e.Focus()
	c.Invalidate()
}
