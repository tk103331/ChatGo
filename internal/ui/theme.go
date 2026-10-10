package ui

import (
	"chatgo/internal/config"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

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

// saveAppearance persists the window's theme choices.
func (cw *ChatWindow) saveAppearance() {
	cw.cfg.ThemeMode = themeModeName(cw.themeMode)
	cw.cfg.Density = densityName(cw.density)
	cw.cfg.Locale = localeName(cw.locale)
	_ = config.SaveConfig(cw.cfg)
}
