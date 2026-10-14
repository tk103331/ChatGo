package ui

import (
	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// colorTheme is a named palette that supplies a light and a dark token set.
type colorTheme struct {
	Name  string
	Label string
	Light *mtheme.Tokens
	Dark  *mtheme.Tokens
}

func parseThemeMode(s string) mcore.Mode {
	switch s {
	case "light":
		return mcore.Light
	case "dark":
		return mcore.Dark
	default:
		return mcore.System
	}
}

func themeModeName(m mcore.Mode) string {
	switch m {
	case mcore.Light:
		return "light"
	case mcore.Dark:
		return "dark"
	default:
		return "system"
	}
}

func densityName(d mtheme.Density) string {
	switch d {
	case mtheme.Compact:
		return "compact"
	case mtheme.Comfortable:
		return "comfortable"
	default:
		return "default"
	}
}

func parseLocale(s string) mcore.Locale {
	if s == "en-US" {
		return mcore.EnUS
	}
	return mcore.ZhCN
}

func localeName(l mcore.Locale) string {
	if l == mcore.EnUS {
		return "en-US"
	}
	return "zh-CN"
}

// palette holds the colors a preset overrides; every empty field keeps the
// library default. Accent drives the primary color, and AccentText is derived
// from it unless set.
type palette struct {
	bg, surface, surfaceHover, text, textMuted, border, controlBorder string
	accent, accentHover, accentPressed, onAccent, selection, focus    string
	accentText, ornament, success, warning, danger, info, dangerFill  string
	onDanger                                                          string
}

// apply copies the palette's non-empty colors onto t.
func (p palette) apply(t mtheme.Tokens) mtheme.Tokens {
	set := func(dst *mgo.Color, hex string) {
		if hex != "" {
			*dst = mgo.Hex(hex)
		}
	}
	set(&t.Background, p.bg)
	set(&t.Surface, p.surface)
	set(&t.SurfaceHover, p.surfaceHover)
	set(&t.Text, p.text)
	set(&t.TextMuted, p.textMuted)
	set(&t.Border, p.border)
	set(&t.ControlBorder, p.controlBorder)
	set(&t.Accent, p.accent)
	set(&t.AccentHover, p.accentHover)
	set(&t.AccentPressed, p.accentPressed)
	set(&t.OnAccent, p.onAccent)
	set(&t.Selection, p.selection)
	set(&t.Focus, p.focus)
	set(&t.AccentText, p.accentText)
	set(&t.Ornament, p.ornament)
	set(&t.Success, p.success)
	set(&t.Warning, p.warning)
	set(&t.Danger, p.danger)
	set(&t.Info, p.info)
	set(&t.DangerFill, p.dangerFill)
	set(&t.OnDanger, p.onDanger)
	return t
}

// colorThemes lists every built-in palette. The first entry keeps the
// library defaults (MujicaUI's ivory / charcoal + wine red).
var colorThemes = []colorTheme{
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
func mkLight(p palette) *mtheme.Tokens {
	t := p.apply(mtheme.Light())
	return &t
}

func mkDark(p palette) *mtheme.Tokens {
	t := p.apply(mtheme.Dark())
	return &t
}

// colorThemeByName resolves a stored name to a palette, falling back to the
// library default when it is unknown.
func colorThemeByName(name string) colorTheme {
	for _, t := range colorThemes {
		if t.Name == name {
			return t
		}
	}
	return colorThemes[0]
}

// tokens returns the light and dark overrides to hand to mcore.Use.
func (t colorTheme) tokens() (light, dark *mtheme.Tokens) {
	return t.Light, t.Dark
}

// sampleLight and sampleDark return the colors used to preview a palette as
// an accent block over a surface block.
func (t colorTheme) sampleLight() (bg, accent mgo.Color) {
	if t.Light != nil {
		return t.Light.Background, t.Light.Accent
	}
	d := mtheme.Light()
	return d.Background, d.Accent
}

func (t colorTheme) sampleDark() (bg, accent mgo.Color) {
	if t.Dark != nil {
		return t.Dark.Background, t.Dark.Accent
	}
	d := mtheme.Dark()
	return d.Background, d.Accent
}

var (
	oceanLight = palette{
		bg: "#EAF1F5", surface: "#F6FAFC", surfaceHover: "#DCE8EF",
		text: "#16232B", textMuted: "#526772", border: "#B9CDD8", controlBorder: "#6E8592",
		accent: "#0F5C7A", accentHover: "#0B4A61", accentPressed: "#083A4C",
		onAccent: "#F2FAFF", selection: "#CFE3EC", focus: "#0F5C7A",
		accentText: "#0F5C7A", ornament: "#3C8FB0",
		success: "#1F6B4A", warning: "#8A5A12", danger: "#A12635", info: "#24608A", dangerFill: "#A12635", onDanger: "#F2FAFF",
	}
	oceanDark = palette{
		bg: "#0F1A20", surface: "#16252D", surfaceHover: "#1F333D",
		text: "#E8F2F7", textMuted: "#A6BCC6", border: "#2F4A57", controlBorder: "#7C97A3",
		accent: "#2E93BC", accentHover: "#3BA3CD", accentPressed: "#2279A0",
		onAccent: "#F2FAFF", selection: "#1E3B49", focus: "#7FC4E0",
		accentText: "#7FC4E0", ornament: "#5AA9C9",
		success: "#8FCBAE", warning: "#DCC07E", danger: "#EF9AA5", info: "#9CC2E0", dangerFill: "#A12635", onDanger: "#F2FAFF",
	}

	forestLight = palette{
		bg: "#EDF1E7", surface: "#F8FAF3", surfaceHover: "#DDE6D2",
		text: "#1C2418", textMuted: "#5A6852", border: "#C2CFB4", controlBorder: "#788A6A",
		accent: "#2F6B3A", accentHover: "#25562F", accentPressed: "#1D4425",
		onAccent: "#F7FBF2", selection: "#D6E3C9", focus: "#2F6B3A",
		accentText: "#2F6B3A", ornament: "#7E9B5B",
		success: "#2F6B3A", warning: "#8A5A12", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#F7FBF2",
	}
	forestDark = palette{
		bg: "#141A12", surface: "#1E271A", surfaceHover: "#2B3725",
		text: "#EAF0E2", textMuted: "#AEBEA2", border: "#3E4E34", controlBorder: "#87997A",
		accent: "#5EA96C", accentHover: "#6FBA7D", accentPressed: "#4B8F58",
		onAccent: "#F7FBF2", selection: "#2B3D28", focus: "#9ED0A6",
		accentText: "#9ED0A6", ornament: "#8FAE6A",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#F7FBF2",
	}

	graphiteLight = palette{
		bg: "#F0F0F2", surface: "#FAFAFB", surfaceHover: "#E2E2E6",
		text: "#1A1A1D", textMuted: "#5C5C63", border: "#C8C8CF", controlBorder: "#7A7A83",
		accent: "#4A4A55", accentHover: "#3A3A44", accentPressed: "#2C2C34",
		onAccent: "#F7F7F9", selection: "#DCDCE2", focus: "#4A4A55",
		accentText: "#3A3A44", ornament: "#8A8A96",
		success: "#326247", warning: "#805714", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#F7F7F9",
	}
	graphiteDark = palette{
		bg: "#171719", surface: "#212124", surfaceHover: "#2E2E33",
		text: "#EDEDF0", textMuted: "#B0B0B8", border: "#45454C", controlBorder: "#8B8B94",
		accent: "#9A9AA6", accentHover: "#AAAAB6", accentPressed: "#84848F",
		onAccent: "#171719", selection: "#33333A", focus: "#C9C9D2",
		accentText: "#C9C9D2", ornament: "#A5A5B0",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#F7F7F9",
	}

	sunsetLight = palette{
		bg: "#F7EEE6", surface: "#FDF7F1", surfaceHover: "#F0DFD0",
		text: "#2A1D17", textMuted: "#6E574A", border: "#DCC7B3", controlBorder: "#93796A",
		accent: "#B5541F", accentHover: "#9A4516", accentPressed: "#7E3711",
		onAccent: "#FFF7F0", selection: "#F0D9C8", focus: "#B5541F",
		accentText: "#B5541F", ornament: "#C98A3F",
		success: "#326247", warning: "#95610E", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#FFF7F0",
	}
	sunsetDark = palette{
		bg: "#1E1512", surface: "#2A1E19", surfaceHover: "#3B2A22",
		text: "#F5E9E0", textMuted: "#C4AB9C", border: "#543C30", controlBorder: "#A08574",
		accent: "#E1793B", accentHover: "#EC8A4E", accentPressed: "#C4632C",
		onAccent: "#2A1D17", selection: "#4A2E20", focus: "#F0A96E",
		accentText: "#F0A96E", ornament: "#D9A25B",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#FFF7F0",
	}

	roseLight = palette{
		bg: "#F9ECEF", surface: "#FDF6F8", surfaceHover: "#F2DCE2",
		text: "#2C1A1E", textMuted: "#705761", border: "#DFC2CB", controlBorder: "#96737F",
		accent: "#B0284E", accentHover: "#962041", accentPressed: "#7C1834",
		onAccent: "#FFF4F7", selection: "#F1D3DC", focus: "#B0284E",
		accentText: "#B0284E", ornament: "#C86B84",
		success: "#326247", warning: "#95610E", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#FFF4F7",
	}
	roseDark = palette{
		bg: "#1F1317", surface: "#2C1B21", surfaceHover: "#3E272F",
		text: "#F6E7EC", textMuted: "#C9A9B4", border: "#553540", controlBorder: "#A5808C",
		accent: "#E0608A", accentHover: "#EA7298", accentPressed: "#C64D75",
		onAccent: "#2C1A1E", selection: "#4B2933", focus: "#F0A0BB",
		accentText: "#F0A0BB", ornament: "#D98BA3",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#FFF4F7",
	}

	lavenderLight = palette{
		bg: "#F0EEF7", surface: "#F8F7FC", surfaceHover: "#E4E0F1",
		text: "#201B2C", textMuted: "#605A72", border: "#CCC6DE", controlBorder: "#8079A0",
		accent: "#5B45A8", accentHover: "#4C3990", accentPressed: "#3F2F79",
		onAccent: "#F7F5FF", selection: "#DDD6EE", focus: "#5B45A8",
		accentText: "#5B45A8", ornament: "#8C7BC0",
		success: "#326247", warning: "#95610E", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#F7F5FF",
	}
	lavenderDark = palette{
		bg: "#161320", surface: "#211C2E", surfaceHover: "#302941",
		text: "#EBE7F5", textMuted: "#B2AAC8", border: "#443A5B", controlBorder: "#8E84AC",
		accent: "#A98FE8", accentHover: "#B7A1EE", accentPressed: "#9176D8",
		onAccent: "#201B2C", selection: "#372E4C", focus: "#C6B4F4",
		accentText: "#C6B4F4", ornament: "#A392D6",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#F7F5FF",
	}

	mintLight = palette{
		bg: "#E9F4EE", surface: "#F5FBF7", surfaceHover: "#D8EBE0",
		text: "#15251D", textMuted: "#4E6A5B", border: "#B8D3C4", controlBorder: "#6E8D7C",
		accent: "#177A52", accentHover: "#126545", accentPressed: "#0E5238",
		onAccent: "#F2FCF7", selection: "#CCE7D9", focus: "#177A52",
		accentText: "#177A52", ornament: "#4CA47F",
		success: "#177A52", warning: "#8A5A12", danger: "#A12635", info: "#24608A", dangerFill: "#A12635", onDanger: "#F2FCF7",
	}
	mintDark = palette{
		bg: "#0F1A15", surface: "#16261E", surfaceHover: "#1F362B",
		text: "#E6F3EC", textMuted: "#A0C0B0", border: "#2F4E3E", controlBorder: "#7C9C8B",
		accent: "#35B37C", accentHover: "#45C08A", accentPressed: "#2A9668",
		onAccent: "#0F1A15", selection: "#1E3D2F", focus: "#7EDCB0",
		accentText: "#7EDCB0", ornament: "#57B78C",
		success: "#8FCBAE", warning: "#DFC080", danger: "#EF9AA5", info: "#9CC2E0", dangerFill: "#A12635", onDanger: "#F2FCF7",
	}

	amberLight = palette{
		bg: "#FAF1E0", surface: "#FDF9F0", surfaceHover: "#F3E2C4",
		text: "#2B2110", textMuted: "#6E5C3C", border: "#DBC6A0", controlBorder: "#947C56",
		accent: "#A96A0B", accentHover: "#8F5908", accentPressed: "#754906",
		onAccent: "#FFF9EE", selection: "#F0DCB8", focus: "#A96A0B",
		accentText: "#A96A0B", ornament: "#C6923B",
		success: "#326247", warning: "#A96A0B", danger: "#A12635", info: "#355E80", dangerFill: "#A12635", onDanger: "#FFF9EE",
	}
	amberDark = palette{
		bg: "#1D160B", surface: "#292012", surfaceHover: "#3A2E1B",
		text: "#F5ECDC", textMuted: "#C8B18C", border: "#513F26", controlBorder: "#9C8360",
		accent: "#DFA644", accentHover: "#E8B257", accentPressed: "#C78F33",
		onAccent: "#1D160B", selection: "#453419", focus: "#F0C877",
		accentText: "#F0C877", ornament: "#D4A554",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9DBDDA", dangerFill: "#A12635", onDanger: "#FFF9EE",
	}

	slateLight = palette{
		bg: "#ECF0F2", surface: "#F7F9FA", surfaceHover: "#DDE4E8",
		text: "#1B2326", textMuted: "#566469", border: "#C2CCD1", controlBorder: "#77868C",
		accent: "#2C5C6E", accentHover: "#234D5C", accentPressed: "#1B3F4C",
		onAccent: "#F2F9FB", selection: "#D2E0E5", focus: "#2C5C6E",
		accentText: "#2C5C6E", ornament: "#5E8A99",
		success: "#2F6B4A", warning: "#8A5A12", danger: "#A12635", info: "#2C5C6E", dangerFill: "#A12635", onDanger: "#F2F9FB",
	}
	slateDark = palette{
		bg: "#121819", surface: "#1B2426", surfaceHover: "#263235",
		text: "#E6EEF0", textMuted: "#A2B4BA", border: "#324347", controlBorder: "#7E9298",
		accent: "#4E93A8", accentHover: "#5DA3B8", accentPressed: "#3F7E92",
		onAccent: "#121819", selection: "#22363C", focus: "#8FC6D6",
		accentText: "#8FC6D6", ornament: "#6CA4B4",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#8FC6D6", dangerFill: "#A12635", onDanger: "#F2F9FB",
	}

	monoLight = palette{
		bg: "#FFFFFF", surface: "#F5F5F5", surfaceHover: "#E8E8E8",
		text: "#101010", textMuted: "#5A5A5A", border: "#CACACA", controlBorder: "#707070",
		accent: "#1A1A1A", accentHover: "#000000", accentPressed: "#000000",
		onAccent: "#FFFFFF", selection: "#E0E0E0", focus: "#1A1A1A",
		accentText: "#1A1A1A", ornament: "#7A7A7A",
		success: "#256B45", warning: "#7A5210", danger: "#A12635", info: "#2C5C6E", dangerFill: "#A12635", onDanger: "#FFFFFF",
	}
	monoDark = palette{
		bg: "#0E0E0E", surface: "#1A1A1A", surfaceHover: "#282828",
		text: "#F2F2F2", textMuted: "#ADADAD", border: "#3C3C3C", controlBorder: "#8A8A8A",
		accent: "#E8E8E8", accentHover: "#FFFFFF", accentPressed: "#CACACA",
		onAccent: "#0E0E0E", selection: "#333333", focus: "#FFFFFF",
		accentText: "#E8E8E8", ornament: "#9A9A9A",
		success: "#94C6A4", warning: "#DFC080", danger: "#F0A1AA", info: "#9CC2E0", dangerFill: "#A12635", onDanger: "#0E0E0E",
	}
)

// saveAppearance persists the window's theme choices.
func (cw *ChatWindow) saveAppearance() {
	cw.cfg.ThemeMode = themeModeName(cw.themeMode)
	cw.cfg.Density = densityName(cw.density)
	cw.cfg.Locale = localeName(cw.locale)
	cw.cfg.ColorTheme = cw.colorTheme.Name
	_ = config.SaveConfig(cw.cfg)
}
