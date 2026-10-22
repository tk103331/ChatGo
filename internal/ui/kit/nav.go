package kit

import (
	"math"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// ChoiceResult is a choice control's element and whether its value changed
// this frame.
type ChoiceResult struct {
	Element mgo.Element
	changed bool
}

// Changed reports whether the user changed the value in this frame.
func (r ChoiceResult) Changed() bool { return r.changed }

// ease transitions an element's text color, which MyGo does not tween on its
// own, by animating each channel.
func ease(e mgo.Element, d time.Duration, fg mgo.Color) mgo.Element {
	if d > 0 {
		e.Transition(mgo.ElementTransition{Colors: true, Duration: d})
	}
	ch := func(key string, v uint8) uint8 {
		return uint8(math.Round(float64(e.Animate(key, float32(v), d))))
	}
	return e.TextColor(mgo.Color{
		R: ch("chatgo.fg.r", fg.R), G: ch("chatgo.fg.g", fg.G),
		B: ch("chatgo.fg.b", fg.B), A: ch("chatgo.fg.a", fg.A),
	})
}

// Segment is one choice of a SegmentedControl.
type Segment struct {
	// Label is required: it is shown, or with HideLabel names the icon.
	Label     string
	Icon      *mgo.SVG
	HideLabel bool
}

// SegmentedControlOptions configure a SegmentedControl.
type SegmentedControlOptions struct {
	// Equal gives every segment the width of the widest.
	Equal bool
	// Label names the group for assistive technology.
	Label    string
	Disabled bool
}

// SegmentedControl chooses one of segments into *selected; Changed reports a
// new choice.
func SegmentedControl(c *mgo.Context, selected *int, segments []Segment, opts SegmentedControlOptions) mgo.Element {
	if len(segments) == 0 {
		panic("kit: SegmentedControl needs at least one segment")
	}
	if *selected < 0 || *selected >= len(segments) {
		panic("kit: SegmentedControl selected index out of range")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	const inset float32 = 2
	parts := mgo.SegmentedBase(c, selected, len(segments))
	track := parts.Track.Padding(inset).Gap(inset).Radius(theme.ControlRadius+inset).
		Background(k.Background).Border(theme.BorderWidth, k.ControlBorder).
		AlignItems(mgo.Stretch).MinHeight(d.ControlHeight()).Disabled(opts.Disabled)
	if opts.Equal {
		track.Grid().Columns(len(segments))
	}
	if opts.Label != "" {
		track.Label(opts.Label)
	}
	track.Children(func() {
		for i, s := range segments {
			if s.Label == "" {
				panic("kit: Segment needs a Label")
			}
			if s.HideLabel && s.Icon == nil {
				panic("kit: Segment with HideLabel needs an Icon")
			}
			seg := parts.Segment(i).Padding(0, u*3).Gap(u * 1.5).
				Radius(theme.ControlRadius).MinWidth(theme.MinHitSize).Label(s.Label)
			if !opts.Disabled {
				seg.Cursor(mgo.CursorPointer)
			}
			fg := k.TextMuted
			switch {
			case i == *selected:
				seg.Background(k.Accent).FontWeight(600)
				fg = k.OnAccent
			case seg.Pressed():
				seg.Background(k.Selection)
				fg = k.Text
			case seg.Hovered():
				seg.Background(k.SurfaceHover)
				fg = k.Text
			}
			ease(seg, uikit.Motion(c, theme.StateDuration), fg)
			uikit.Ring(seg, k.Focus, theme.ControlRadius)
			if s.HideLabel {
				seg.Tooltip(s.Label)
			}
			seg.Children(func() {
				if s.Icon != nil {
					mgo.Icon(c, s.Icon).Size(16, 16)
				}
				if !s.HideLabel {
					mgo.Text(c, s.Label).SingleLine()
				}
			})
		}
	})
	return track
}

// Tab is one tab of Tabs.
type Tab struct {
	// Label is required; it names the tab and its panel.
	Label    string
	Icon     *mgo.SVG
	Disabled bool
	// Panel builds the tab's content while it is chosen; nil shows none.
	Panel func()
}

// TabsOptions configure Tabs.
type TabsOptions struct {
	// Vertical stacks the tabs at the start with the panel beside them.
	Vertical bool
	// Label names the tab list for assistive technology.
	Label string
}

// Tabs shows tabs choosing *selected and the chosen tab's panel. Disabled
// tabs are skipped by the arrows.
func Tabs(c *mgo.Context, selected *int, tabs []Tab, opts TabsOptions) ChoiceResult {
	if len(tabs) == 0 {
		panic("kit: Tabs needs at least one tab")
	}
	if *selected < 0 || *selected >= len(tabs) {
		panic("kit: Tabs selected index out of range")
	}
	if tabs[*selected].Disabled {
		panic("kit: Tabs selected tab is disabled")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	pad := theme.FocusGap + theme.FocusWidth
	// TabsBase only sees enabled tabs, so the arrows skip disabled ones.
	var enabled []int
	at := 0
	for i, t := range tabs {
		if !t.Disabled {
			if i == *selected {
				at = len(enabled)
			}
			enabled = append(enabled, i)
		}
	}
	root := mgo.Column(c).AlignItems(mgo.Stretch)
	if root.IsDisabled() {
		for _, key := range []mgo.Key{mgo.KeyRight, mgo.KeyDown, mgo.KeyLeft, mgo.KeyUp, mgo.KeyHome, mgo.KeyEnd} {
			root.Shortcut(0, key)
		}
	}
	if opts.Vertical {
		root.Row()
	}
	res := ChoiceResult{Element: root}
	root.Children(func() {
		var strip mgo.Element
		if opts.Vertical {
			strip = mgo.Scroll(c).Shrink(0).Padding(pad)
		} else {
			strip = mgo.ScrollHorizontal(c).Shrink(0).MinWidth(0).Padding(pad, pad, 0, pad)
		}
		vertical, border := opts.Vertical, k.Border
		strip.Draw(func(p *mgo.Painter, r mgo.Rect) {
			if vertical {
				p.Fill(mgo.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, border, 0)
			} else {
				p.Fill(mgo.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, border, 0)
			}
		})
		strip.Children(func() {
			base := mgo.TabsBase(c, &at, len(enabled))
			list := base.List.Gap(u)
			if opts.Vertical {
				list.Column().AlignItems(mgo.Stretch)
			}
			if opts.Label != "" {
				list.Label(opts.Label)
			}
			built := make([]mgo.Element, len(tabs))
			list.Children(func() {
				n := 0
				for i, t := range tabs {
					var tab mgo.Element
					if t.Disabled {
						tab = mgo.Row(c).Shrink(0).Role(mgo.RoleTab).Disabled(true)
					} else {
						tab = base.Tab(n)
						n++
					}
					tabStyle(c, tab, t, i == enabled[at], opts.Vertical)
					built[i] = tab
				}
			})
			if !list.IsDisabled() && list.Changed() {
				*selected = enabled[at]
				res = ChoiceResult{Element: res.Element, changed: true}
				built[*selected].ScrollIntoView()
			}
		})
		panel := mgo.Column(c).MinWidth(0).MinHeight(0).Padding(u * 4).
			Role(mgo.RoleGroup).Label(tabs[*selected].Label)
		// Horizontal tabs grow too: a panel holding a Scroll collapses to
		// zero height without it.
		panel.Grow(1)
		if p := tabs[*selected].Panel; p != nil {
			panel.Children(p)
		}
	})
	return res
}

func tabStyle(c *mgo.Context, tab mgo.Element, t Tab, on, vertical bool) {
	if t.Label == "" {
		panic("kit: Tab needs a Label")
	}
	k := uikit.Tokens(c)
	d := uikit.Density(c)
	u := d.Unit()
	tab.MinHeight(d.ControlHeight()).Padding(0, u*4).Gap(u * 2).
		AlignItems(mgo.Center).Cursor(mgo.CursorPointer)
	if vertical {
		tab.Radius(theme.ControlRadius, 0, 0, theme.ControlRadius)
	} else {
		tab.Radius(theme.ControlRadius, theme.ControlRadius, 0, 0)
	}
	if t.Disabled {
		tab.Cursor(mgo.CursorNotAllowed)
	}
	fg := k.TextMuted
	switch {
	case on:
		tab.Background(k.Selection).FontWeight(600)
		fg = k.Text
	case !t.Disabled && tab.Pressed():
		tab.Background(k.Selection)
		fg = k.Text
	case !t.Disabled && tab.Hovered():
		tab.Background(k.SurfaceHover)
		fg = k.Text
	}
	ease(tab, uikit.Motion(c, theme.StateDuration), fg)
	// FocusVisible is read in the build pass; the paint callback runs later,
	// when this element handle is no longer valid.
	ringVisible := tab.FocusVisible()
	accent, focus := k.AccentText, k.Focus
	tab.FocusRing(false).DrawOver(func(p *mgo.Painter, r mgo.Rect) {
		if on {
			if vertical {
				p.Fill(mgo.Rect{X: r.X + r.W - 2, Y: r.Y, W: 2, H: r.H}, accent, 0)
			} else {
				p.Fill(mgo.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2}, accent, 0)
			}
		}
		if ringVisible {
			uikit.FocusRing(p, r, theme.ControlRadius, focus)
		}
	})
	tab.Children(func() {
		if t.Icon != nil {
			mgo.Icon(c, t.Icon).Size(16, 16)
		}
		mgo.Text(c, t.Label).SingleLine()
	})
}
