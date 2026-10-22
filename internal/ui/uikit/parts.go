package uikit

import (
	"fmt"
	"math"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
)

// Severity is the meaning of a message. The zero value is SeverityInfo.
type Severity int

const (
	SeverityInfo Severity = iota
	SeveritySuccess
	SeverityWarning
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeveritySuccess:
		return "success"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	}
	panic(fmt.Sprintf("uikit: unknown Severity %d", int(s)))
}

// SeverityLook returns the color and icon that, with text, mark s.
func SeverityLook(k *mgo.Theme, s Severity) (mgo.Color, *mgo.SVG) {
	switch s {
	case SeveritySuccess:
		return k.Success, icons.Must("circle-check")
	case SeverityWarning:
		return k.Warning, icons.Must("triangle-alert")
	case SeverityError:
		return k.Danger, icons.Must("circle-x")
	case SeverityInfo:
		return k.Accent, icons.Must("info")
	}
	panic(fmt.Sprintf("uikit: unknown Severity %d", int(s)))
}

// FocusRing strokes the keyboard ring outside r, leaving the focus gap.
func FocusRing(p *mgo.Painter, r mgo.Rect, radius float32, c mgo.Color) {
	o := theme.FocusGap + theme.FocusWidth
	p.Stroke(mgo.Rect{X: r.X - o, Y: r.Y - o, W: r.W + 2*o, H: r.H + 2*o}, c, radius+o, theme.FocusWidth)
}

// Ring replaces MyGo's focus ring on e with ChatGo's.
//
// FocusVisible is read in the build pass: a ui.Element is only valid inside
// the pass that made it, and the paint callback runs later.
func Ring(e mgo.Element, c mgo.Color, radius float32) mgo.Element {
	visible := e.FocusVisible()
	return e.FocusRing(false).DrawOver(func(p *mgo.Painter, r mgo.Rect) {
		if visible {
			FocusRing(p, r, radius, c)
		}
	})
}

// Diamond returns a rhombus of size s centered at (cx, cy).
func Diamond(cx, cy, s float32) *mgo.Path {
	var p mgo.Path
	h := s / 2
	p.MoveTo(cx, cy-h)
	p.LineTo(cx+h, cy)
	p.LineTo(cx, cy+h)
	p.LineTo(cx-h, cy)
	p.Close()
	return &p
}

// Snap rounds v to MyGo's quarter-device-pixel path cache grid.
func Snap(p *mgo.Painter, v float32) float32 {
	g := 4 * p.Scale()
	return float32(math.Round(float64(v*g))) / g
}

// IconAction is a square, borderless button showing icon, named label.
func IconAction(c *mgo.Context, icon *mgo.SVG, label string) mgo.Element {
	k := Tokens(c)
	b := mgo.ButtonBase(c).Size(theme.MinHitSize, theme.MinHitSize).
		Radius(theme.ControlRadius).Label(label).TextColor(k.TextMuted)
	Ring(b, k.Focus, theme.ControlRadius)
	switch {
	case b.Pressed():
		b.Background(k.Selection).TextColor(k.Text)
	case b.Hovered():
		b.Background(k.SurfaceHover).TextColor(k.Text)
	}
	b.Children(func() { mgo.Icon(c, icon).Size(16, 16) })
	return b
}

// SpinnerOptions configure a Spinner. Size is 16 DIPs when zero.
type SpinnerOptions struct {
	Size float32
	// Label is read out and shown beside the mark; empty uses "加载中",
	// unread on screen unless ShowLabel.
	Label     string
	ShowLabel bool
	// Color paints the mark; zero uses AccentText.
	Color mgo.Color
}

// Spinner turns four diamonds only while painted; hidden or under reduced
// motion it stops.
func Spinner(c *mgo.Context, opts SpinnerOptions) mgo.Element {
	if !(opts.Size >= 0) || math.IsInf(float64(opts.Size), 1) {
		panic(fmt.Sprintf("uikit: Spinner Size %v invalid", opts.Size))
	}
	k := Tokens(c)
	size := opts.Size
	if size == 0 {
		size = 16
	}
	label := opts.Label
	if label == "" {
		label = "加载中"
		if CurrentLocale(c) == EnUS {
			label = "Loading"
		}
	}
	color := opts.Color
	if color.A == 0 {
		color = k.Accent
	}
	still := c.Preferences().ReduceMotion
	row := mgo.Row(c).Gap(Unit(c) * 2).AlignItems(mgo.Center)
	row.Children(func() {
		// Keyed by motion so a flip repaints; MyGo's spinner carries the
		// progress role.
		mgo.Box(c.Key(still)).Shrink(0).Children(func() { spinnerMark(c, label, size, color, still) })
		if opts.ShowLabel {
			mgo.Text(c, label).TextColor(k.TextMuted).SingleLine()
		}
	})
	return row
}

func spinnerMark(c *mgo.Context, label string, size float32, color mgo.Color, still bool) {
	mgo.Spinner(c).Label(label).Size(size, size).Draw(func(p *mgo.Painter, r mgo.Rect) {
		turn := 0.0
		if !still {
			turn = float64(p.Now().UnixMilli()%1200) / 1200
			p.AnimationFrame()
		}
		cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, r.W*0.32
		for i := range 4 {
			a := 2 * math.Pi * (turn + float64(i)/4)
			x := cx + rad*float32(math.Cos(a))
			y := cy + rad*float32(math.Sin(a))
			alpha := 1 - float32(i)*0.22
			p.FillPath(Diamond(Snap(p, x), Snap(p, y), Snap(p, r.W*0.3)), color.Alpha(alpha))
		}
	})
}
