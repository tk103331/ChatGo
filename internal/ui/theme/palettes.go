package theme

import (
	"math"

	mgo "github.com/egoist/mygo/ui"
)

// Accent is a theme's primary color in both appearances. Only the accent
// family is set: the neutral colors stay MyGo's own, so contrast holds.
type Accent struct {
	// Base is the fill of primary controls, Hover and Pressed its states.
	// Light and dark each carry their own, since a dark appearance lightens
	// on hover where a light one darkens.
	Light, LightHover, LightPressed string
	Dark, DarkHover, DarkPressed    string
}

// Theme is a named accent the user can pick.
type Theme struct {
	Name   string
	Label  string
	Accent Accent
}

// Themes lists every built-in accent. The first is MyGo's own blue, which
// an empty stored name resolves to.
//
// Each step is one of Tailwind's color scales: 600/700/800 in a light
// appearance, 500/400/600 in a dark one, as MyGo's own theme does.
var Themes = []Theme{
	{Name: "blue", Label: "蓝色", Accent: Accent{
		Light: "#2563eb", LightHover: "#1d4ed8", LightPressed: "#1e40af",
		Dark: "#3b82f6", DarkHover: "#60a5fa", DarkPressed: "#2563eb",
	}},
	{Name: "cyan", Label: "青色", Accent: Accent{
		Light: "#0e7490", LightHover: "#155e75", LightPressed: "#164e63",
		Dark: "#06b6d4", DarkHover: "#22d3ee", DarkPressed: "#0891b2",
	}},
	{Name: "emerald", Label: "翠绿", Accent: Accent{
		Light: "#047857", LightHover: "#065f46", LightPressed: "#064e3b",
		Dark: "#10b981", DarkHover: "#34d399", DarkPressed: "#059669",
	}},
	{Name: "violet", Label: "紫色", Accent: Accent{
		Light: "#7c3aed", LightHover: "#6d28d9", LightPressed: "#5b21b6",
		Dark: "#8b5cf6", DarkHover: "#a78bfa", DarkPressed: "#7c3aed",
	}},
	{Name: "rose", Label: "玫红", Accent: Accent{
		Light: "#be123c", LightHover: "#9f1239", LightPressed: "#881337",
		Dark: "#f43f5e", DarkHover: "#fb7185", DarkPressed: "#e11d48",
	}},
	{Name: "amber", Label: "琥珀", Accent: Accent{
		Light: "#b45309", LightHover: "#92400e", LightPressed: "#78350f",
		Dark: "#f59e0b", DarkHover: "#fbbf24", DarkPressed: "#d97706",
	}},
	{Name: "slate", Label: "石墨", Accent: Accent{
		Light: "#475569", LightHover: "#334155", LightPressed: "#1e293b",
		Dark: "#94a3b8", DarkHover: "#cbd5e1", DarkPressed: "#64748b",
	}},
}

// ByName resolves a stored name to a theme, falling back to the first when
// it is unknown.
func ByName(name string) Theme {
	for _, t := range Themes {
		if t.Name == name {
			return t
		}
	}
	return Themes[0]
}

// Apply sets t's accent family on the window theme, leaving every other
// color as it is.
func (t Theme) Apply(w *mgo.Theme) {
	base, hover, pressed := t.Accent.Light, t.Accent.LightHover, t.Accent.LightPressed
	// A dark appearance lightens on hover, so it carries its own steps.
	selection, focus := float32(0.25), float32(0.55)
	if w.Dark {
		base, hover, pressed = t.Accent.Dark, t.Accent.DarkHover, t.Accent.DarkPressed
		selection, focus = 0.4, 0.6
	}
	w.Accent = mgo.Hex(base)
	w.AccentHover = mgo.Hex(hover)
	w.AccentPressed = mgo.Hex(pressed)
	w.AccentText = onAccent(w.Accent)
	w.Selection = w.Accent.Alpha(selection)
	// The ring follows the hover step, which stands out on either appearance.
	w.Focus = mgo.Hex(hover).Alpha(focus)
}

// onAccent returns the text color a fill carries. White is the default, and
// the fallback is near-black, for a bright accent that white cannot sit on:
// amber reaches only 2.2:1 against white, well under the 3:1 that text on a
// fill needs.
func onAccent(fill mgo.Color) mgo.Color {
	const minRatio = 3
	if white := mgo.Hex("#ffffff"); Contrast(white, fill) >= minRatio {
		return white
	}
	return mgo.Hex("#18181b")
}

// Contrast returns the WCAG contrast ratio of two opaque colors.
func Contrast(a, b mgo.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c mgo.Color) float64 {
	ch := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// Sample returns the colors that preview the theme: its accent in a light
// and in a dark appearance.
func (t Theme) Sample() (light, dark mgo.Color) {
	return mgo.Hex(t.Accent.Light), mgo.Hex(t.Accent.Dark)
}
