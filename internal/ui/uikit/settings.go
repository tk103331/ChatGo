package uikit

import "chatgo/internal/ui/theme"

// ParseMode converts a stored mode string to a Mode.
func ParseMode(s string) Mode {
	switch s {
	case "light":
		return Light
	case "dark":
		return Dark
	default:
		return System
	}
}

// ModeName converts a Mode back to its storage string.
func ModeName(m Mode) string {
	switch m {
	case Light:
		return "light"
	case Dark:
		return "dark"
	default:
		return "system"
	}
}

// ParseDensity converts a stored density string to a Density.
func ParseDensity(s string) theme.Density {
	switch s {
	case "compact":
		return theme.Compact
	case "comfortable":
		return theme.Comfortable
	default:
		return theme.DefaultDensity
	}
}

// DensityName converts a Density to its storage string.
func DensityName(d theme.Density) string {
	switch d {
	case theme.Compact:
		return "compact"
	case theme.Comfortable:
		return "comfortable"
	default:
		return "default"
	}
}

// ParseLocale converts a stored locale string to a Locale.
func ParseLocale(s string) Locale {
	if s == "en-US" {
		return EnUS
	}
	return ZhCN
}

// LocaleName converts a Locale back to its storage string.
func LocaleName(l Locale) string {
	if l == EnUS {
		return "en-US"
	}
	return "zh-CN"
}
