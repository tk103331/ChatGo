// Package uikit holds ChatGo's window theme runtime and the types and
// helpers the hand-drawn UI shares.
//
// Use installs the window's appearance once per frame; Tokens, Density and
// FontSize read it back. Everything here is drawn with MyGo's native ui
// package — there is no external component library.
package uikit

import (
	"fmt"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/theme"
)

// Mode chooses the appearance of a window.
type Mode int

const (
	System Mode = iota
	Light
	Dark
)

func (m Mode) String() string {
	switch m {
	case Light:
		return "Light"
	case Dark:
		return "Dark"
	}
	return "System"
}

// Locale names the language the UI speaks.
type Locale string

const (
	EnUS Locale = "en-US"
	ZhCN Locale = "zh-CN"
)

// Settings are a window's theme. Nil token sets use ChatGo's defaults.
type Settings struct {
	Mode    Mode
	Density theme.Density
	Light   *theme.Tokens
	Dark    *theme.Tokens
	Locale  Locale
}

type windowTheme struct {
	tokens  theme.Tokens
	density theme.Density
	locale  Locale
	set     bool
}

type themeKey struct{}

// Use applies s to the window. Call it first in every frame of the view.
func Use(c *mgo.Context, s Settings) {
	if s.Mode < System || s.Mode > Dark {
		panic(fmt.Sprintf("uikit: invalid Mode %d", s.Mode))
	}
	if s.Density < theme.DefaultDensity || s.Density > theme.Comfortable {
		panic(fmt.Sprintf("uikit: invalid Density %d", s.Density))
	}
	if s.Locale != "" && s.Locale != EnUS && s.Locale != ZhCN {
		panic("uikit: unsupported Locale " + string(s.Locale))
	}
	dark := c.Theme().Dark
	switch s.Mode {
	case Light:
		dark = false
	case Dark:
		dark = true
	}
	k := theme.Light()
	if s.Light != nil {
		k = *s.Light
	}
	if dark {
		k = theme.Dark()
		if s.Dark != nil {
			k = *s.Dark
		}
	}
	if c.Preferences().HighContrast {
		k = k.HighContrast()
	}
	c.SetTheme(&mgo.Theme{
		Dark:           k.Dark,
		Background:     k.Background,
		Surface:        k.Surface,
		SurfaceHover:   k.SurfaceHover,
		SurfacePressed: k.Selection,
		Border:         k.ControlBorder,
		Text:           k.Text,
		TextMuted:      k.TextMuted,
		Accent:         k.Accent,
		AccentHover:    k.AccentHover,
		AccentPressed:  k.AccentPressed,
		AccentText:     k.OnAccent,
		Danger:         k.Danger,
		Warning:        k.Warning,
		Success:        k.Success,
		Selection:      k.Selection,
		Focus:          k.Focus,
		Scrollbar:      k.TextMuted.Alpha(0.5),
		Radius:         theme.ControlRadius,
		Spacing:        s.Density.Unit(),
		FontSize:       FontSize(c, theme.BodySize),
	})
	c.Root().LineHeight(theme.LineHeight)
	*mgo.Local(c.Root(), themeKey{}, func() windowTheme { return windowTheme{} }) = windowTheme{k, s.Density, s.Locale, true}
}

func current(c *mgo.Context) windowTheme {
	w := *mgo.Local(c.Root(), themeKey{}, func() windowTheme { return windowTheme{} })
	if !w.set {
		panic("uikit: call uikit.Use at the start of the view")
	}
	return w
}

// Tokens returns the window's colors.
func Tokens(c *mgo.Context) theme.Tokens { return current(c).tokens }

// Density returns the window's density.
func Density(c *mgo.Context) theme.Density { return current(c).density }

// CurrentLocale returns the window's language.
func CurrentLocale(c *mgo.Context) Locale {
	if l := current(c).locale; l != "" {
		return l
	}
	return ZhCN
}

// Unit is the window's base spacing unit in DIPs.
func Unit(c *mgo.Context) float32 { return Density(c).Unit() }

// ColorFade eases background and border colors between states; MyGo skips it
// under reduced motion.
var ColorFade = mgo.ElementTransition{Colors: true, Duration: theme.StateDuration}

// Motion returns d, or zero when the desktop asks for reduced motion.
func Motion(c *mgo.Context, d time.Duration) time.Duration {
	if c.Preferences().ReduceMotion {
		return 0
	}
	return d
}

// FontSize scales a design size by the desktop's text scale.
func FontSize(c *mgo.Context, size float32) float32 {
	if s := c.Preferences().TextScale; s > 0 {
		return size * s
	}
	return size
}
