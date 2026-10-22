package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/ui/uikit"
)

// saveAppearance persists the window's theme choices.
func (cw *ChatWindow) saveAppearance() {
	cw.cfg.ThemeMode = uikit.ModeName(cw.themeMode)
	cw.cfg.Density = uikit.DensityName(cw.density)
	cw.cfg.Locale = uikit.LocaleName(cw.locale)
	cw.cfg.ColorTheme = cw.colorTheme.Name
	_ = config.SaveConfig(cw.cfg)
}
