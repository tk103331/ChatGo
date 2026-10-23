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

// Theme is a named color theme the user can pick.
type Theme struct {
	Name   string
	Label  string
	Accent Accent
	// LightBG and DarkBG are the card backgrounds the theme preview shows
	// beside its accent, so each theme reads as a light and a dark pair.
	// LightSurface and DarkSurface are what Apply paints the window with,
	// empty to keep MyGo's own page and surface colors.
	LightBG, DarkBG           string
	LightSurface, DarkSurface string
}

// Themes lists every built-in theme. The first is the default: MyGo's own
// colors untouched, which an unknown or empty stored name resolves to.
//
// Accent steps are Tailwind's color scales: 600/700/800 in a light
// appearance, 500/400/600 in a dark one, as MyGo's own theme does.
var Themes = []Theme{
	{Name: DefaultName, Label: "默认"},
	{Name: "wine", Label: "象牙·酒红", Accent: Accent{
		Light: "#9f1239", LightHover: "#881337", LightPressed: "#4c0519",
		Dark: "#be123c", DarkHover: "#e11d48", DarkPressed: "#9f1239",
	}, LightBG: "#fdf6ec", DarkBG: "#1c1013", LightSurface: "#fdf6ec", DarkSurface: "#1c1013"},
	{Name: "blue", Label: "海洋", Accent: Accent{
		Light: "#0e7490", LightHover: "#155e75", LightPressed: "#164e63",
		Dark: "#06b6d4", DarkHover: "#22d3ee", DarkPressed: "#0891b2",
	}, LightBG: "#f0f9ff", DarkBG: "#0b1620", LightSurface: "#f2f8fd", DarkSurface: "#0b1620"},
	{Name: "emerald", Label: "森林", Accent: Accent{
		Light: "#047857", LightHover: "#065f46", LightPressed: "#064e3b",
		Dark: "#10b981", DarkHover: "#34d399", DarkPressed: "#059669",
	}, LightBG: "#f0fdf4", DarkBG: "#0c1a12", LightSurface: "#f2fcf5", DarkSurface: "#0c1a12"},
	{Name: "slate", Label: "石墨", Accent: Accent{
		Light: "#475569", LightHover: "#334155", LightPressed: "#1e293b",
		Dark: "#94a3b8", DarkHover: "#cbd5e1", DarkPressed: "#64748b",
	}, LightBG: "#f4f4f5", DarkBG: "#18181b", LightSurface: "#f7f7f8", DarkSurface: "#18181b"},
	{Name: "amber", Label: "落日", Accent: Accent{
		Light: "#c2410c", LightHover: "#9a3412", LightPressed: "#7c2d12",
		Dark: "#f97316", DarkHover: "#fb923c", DarkPressed: "#ea580c",
	}, LightBG: "#fff7ed", DarkBG: "#1c120a", LightSurface: "#fff8f1", DarkSurface: "#1c120a"},
	{Name: "rose", Label: "玫瑰", Accent: Accent{
		Light: "#be123c", LightHover: "#9f1239", LightPressed: "#881337",
		Dark: "#f43f5e", DarkHover: "#fb7185", DarkPressed: "#e11d48",
	}, LightBG: "#fff1f2", DarkBG: "#1c0f14", LightSurface: "#fff3f4", DarkSurface: "#1c0f14"},
	{Name: "violet", Label: "薰衣草", Accent: Accent{
		Light: "#7c3aed", LightHover: "#6d28d9", LightPressed: "#5b21b6",
		Dark: "#8b5cf6", DarkHover: "#a78bfa", DarkPressed: "#7c3aed",
	}, LightBG: "#f5f3ff", DarkBG: "#150f22", LightSurface: "#f7f5ff", DarkSurface: "#150f22"},
	{Name: "mint", Label: "薄荷", Accent: Accent{
		Light: "#0f766e", LightHover: "#115e59", LightPressed: "#134e4a",
		Dark: "#14b8a6", DarkHover: "#2dd4bf", DarkPressed: "#0d9488",
	}, LightBG: "#f0fdfa", DarkBG: "#0a1a18", LightSurface: "#f2fdfb", DarkSurface: "#0a1a18"},
	{Name: "ochre", Label: "琥珀", Accent: Accent{
		Light: "#b45309", LightHover: "#92400e", LightPressed: "#78350f",
		Dark: "#f59e0b", DarkHover: "#fbbf24", DarkPressed: "#d97706",
	}, LightBG: "#fffbeb", DarkBG: "#1a1207", LightSurface: "#fffcef", DarkSurface: "#1a1207"},
	{Name: "teal", Label: "青灰", Accent: Accent{
		Light: "#0f766e", LightHover: "#0e5f5a", LightPressed: "#134e4a",
		Dark: "#5eead4", DarkHover: "#99f6e4", DarkPressed: "#2dd4bf",
	}, LightBG: "#f8fafc", DarkBG: "#0d1b1e", LightSurface: "#f6fafb", DarkSurface: "#0d1b1e"},
	{Name: "mono", Label: "单色", Accent: Accent{
		Light: "#18181b", LightHover: "#27272a", LightPressed: "#000000",
		Dark: "#e4e4e7", DarkHover: "#f4f4f5", DarkPressed: "#a1a1aa",
	}, LightBG: "#ffffff", DarkBG: "#0a0a0a", LightSurface: "#ffffff", DarkSurface: "#000000"},
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

// Apply sets t's colors on the window theme: its accent family and, when the
// theme names its own page and surface colors, those too, so each theme reads
// as the light and dark pair its preview shows.
func (t Theme) Apply(w *mgo.Theme) {
	if t.Name == DefaultName {
		return
	}
	base, hover, pressed := t.Accent.Light, t.Accent.LightHover, t.Accent.LightPressed
	// A dark appearance lightens on hover, so it carries its own steps.
	selection, focus := float32(0.25), float32(0.55)
	if w.Dark {
		base, hover, pressed = t.Accent.Dark, t.Accent.DarkHover, t.Accent.DarkPressed
		selection, focus = 0.4, 0.6
	}
	t.applySurfaces(w)
	w.Accent = mgo.Hex(base)
	w.AccentHover = mgo.Hex(hover)
	w.AccentPressed = mgo.Hex(pressed)
	w.AccentText = onAccent(w.Accent)
	w.Selection = w.Accent.Alpha(selection)
	// The ring follows the hover step, which stands out on either appearance.
	w.Focus = mgo.Hex(hover).Alpha(focus)
}

// applySurfaces paints the theme's page and the surfaces on it, derived from
// its card color so the preview matches the window.
func (t Theme) applySurfaces(w *mgo.Theme) {
	cards := t.LightSurface
	if w.Dark {
		cards = t.DarkSurface
	}
	if cards == "" {
		return
	}
	bg := mgo.Hex(cards)
	w.Background = bg
	w.Surface = mix(bg, w.Text, 0.05)
	w.SurfaceHover = mix(bg, w.Text, 0.10)
	w.SurfacePressed = mix(bg, w.Text, 0.16)
	w.Border = mix(bg, w.Text, 0.16)
}

// mix blends b into a by amount, in sRGB space.
func mix(a, b mgo.Color, amount float32) mgo.Color {
	blend := func(x, y uint8) uint8 {
		return uint8(float32(x) + (float32(y)-float32(x))*amount)
	}
	return mgo.RGBA(blend(a.R, b.R), blend(a.G, b.G), blend(a.B, b.B), 1)
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

// DefaultName is the theme that keeps MyGo's own colors.
const DefaultName = "default"

// Cards returns the light and dark card colors the theme preview draws.
func (t Theme) Cards() (light, dark mgo.Color) {
	if t.Name == DefaultName {
		light, dark = mgo.LightTheme().Background, mgo.DarkTheme().Background
		return
	}
	light, dark = mgo.Hex(t.LightBG), mgo.Hex(t.DarkBG)
	return
}
