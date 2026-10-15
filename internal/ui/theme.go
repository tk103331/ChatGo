package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/ui/theme"
)

// saveAppearance persists the window's theme choices.
func (cw *ChatWindow) saveAppearance() {
	cw.cfg.ThemeMode = theme.ModeName(cw.themeMode)
	cw.cfg.Density = theme.DensityName(cw.density)
	cw.cfg.Locale = theme.LocaleName(cw.locale)
	cw.cfg.ColorTheme = cw.colorTheme.Name
	_ = config.SaveConfig(cw.cfg)
}
