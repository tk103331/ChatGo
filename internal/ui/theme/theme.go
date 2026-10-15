// Package theme defines ChatGo's color themes and appearance-preset parsing.
package theme

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Theme is a named palette that supplies a light and a dark token set.
type Theme struct {
	Name  string
	Label string
	Light *mtheme.Tokens
	Dark  *mtheme.Tokens
}

// ParseMode converts a stored mode string to a MujicaUI mode.
func ParseMode(s string) mcore.Mode {
	switch s {
	case "light":
		return mcore.Light
	case "dark":
		return mcore.Dark
	default:
		return mcore.System
	}
}

// ModeName converts a MujicaUI mode back to its storage string.
func ModeName(m mcore.Mode) string {
	switch m {
	case mcore.Light:
		return "light"
	case mcore.Dark:
		return "dark"
	default:
		return "system"
	}
}

// DensityName converts a MujicaUI density to its storage string.
func DensityName(d mtheme.Density) string {
	switch d {
	case mtheme.Compact:
		return "compact"
	case mtheme.Comfortable:
		return "comfortable"
	default:
		return "default"
	}
}

// ParseLocale converts a stored locale string to a MujicaUI locale.
func ParseLocale(s string) mcore.Locale {
	if s == "en-US" {
		return mcore.EnUS
	}
	return mcore.ZhCN
}

// LocaleName converts a MujicaUI locale back to its storage string.
func LocaleName(l mcore.Locale) string {
	if l == mcore.EnUS {
		return "en-US"
	}
	return "zh-CN"
}

// Palette holds the colors a preset overrides; every empty field keeps the
// library default. Accent drives the primary color, and AccentText is derived
// from it unless set.
type Palette struct {
	Bg, Surface, SurfaceHover, Text, TextMuted, Border, ControlBorder string
	Accent, AccentHover, AccentPressed, OnAccent, Selection, Focus    string
	AccentText, Ornament, Success, Warning, Danger, Info, DangerFill  string
	OnDanger                                                          string
}

// Apply copies the palette's non-empty colors onto t.
func (p Palette) Apply(t mtheme.Tokens) mtheme.Tokens {
	set := func(dst *mgo.Color, hex string) {
		if hex != "" {
			*dst = mgo.Hex(hex)
		}
	}
	set(&t.Background, p.Bg)
	set(&t.Surface, p.Surface)
	set(&t.SurfaceHover, p.SurfaceHover)
	set(&t.Text, p.Text)
	set(&t.TextMuted, p.TextMuted)
	set(&t.Border, p.Border)
	set(&t.ControlBorder, p.ControlBorder)
	set(&t.Accent, p.Accent)
	set(&t.AccentHover, p.AccentHover)
	set(&t.AccentPressed, p.AccentPressed)
	set(&t.OnAccent, p.OnAccent)
	set(&t.Selection, p.Selection)
	set(&t.Focus, p.Focus)
	set(&t.AccentText, p.AccentText)
	set(&t.Ornament, p.Ornament)
	set(&t.Success, p.Success)
	set(&t.Warning, p.Warning)
	set(&t.Danger, p.Danger)
	set(&t.Info, p.Info)
	set(&t.DangerFill, p.DangerFill)
	set(&t.OnDanger, p.OnDanger)
	return t
}

// Themes lists every built-in palette. The first entry keeps the
// library defaults (MujicaUI's ivory / charcoal + wine red).
var Themes = []Theme{
	{Name: "mujica", Label: "象牙·酒红"},
	{Name: "ocean", Label: "海洋", Light: mkLight(oceanLight), Dark: mkDark(oceanDark)},
	{Name: "forest", Label: "森林", Light: mkLight(forestLight), Dark: mkDark(forestDark)},
	{Name: "graphite", Label: "石墨", Light: mkLight(graphiteLight), Dark: mkDark(graphiteDark)},
	{Name: "sunset", Label: "落日", Light: mkLight(sunsetLight), Dark: mkDark(sunsetDark)},
	{Name: "rose", Label: "玫瑰", Light: mkLight(roseLight), Dark: mkDark(roseDark)},
	{Name: "lavender", Label: "薰衣草", Light: mkLight(lavenderLight), Dark: mkDark(lavenderDark)},
	{Name: "mint", Label: "薄荷", Light: mkLight(mintLight), Dark: mkDark(mintDark)},
	{Name: "amber", Label: "琥珀", Light: mkLight(amberLight), Dark: mkDark(amberDark)},
	{Name: "slate", Label: "青灰", Light: mkLight(slateLight), Dark: mkDark(slateDark)},
	{Name: "mono", Label: "单色", Light: mkLight(monoLight), Dark: mkDark(monoDark)},
}

// mkLight and mkDark adapt a palette to a base token set.
func mkLight(p Palette) *mtheme.Tokens {
	t := p.Apply(mtheme.Light())
	return &t
}

func mkDark(p Palette) *mtheme.Tokens {
	t := p.Apply(mtheme.Dark())
	return &t
}

// ByName resolves a stored name to a palette, falling back to the
// library default when it is unknown.
func ByName(name string) Theme {
	for _, t := range Themes {
		if t.Name == name {
			return t
		}
	}
	return Themes[0]
}

// Tokens returns the light and dark overrides to hand to mcore.Use.
func (t Theme) Tokens() (light, dark *mtheme.Tokens) {
	return t.Light, t.Dark
}

// SampleLight and SampleDark return the colors used to preview a palette as
// an accent block over a surface block.
func (t Theme) SampleLight() (bg, accent mgo.Color) {
	if t.Light != nil {
		return t.Light.Background, t.Light.Accent
	}
	d := mtheme.Light()
	return d.Background, d.Accent
}

func (t Theme) SampleDark() (bg, accent mgo.Color) {
	if t.Dark != nil {
		return t.Dark.Background, t.Dark.Accent
	}
	d := mtheme.Dark()
	return d.Background, d.Accent
}
