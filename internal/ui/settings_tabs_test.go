package ui

import "testing"

func TestSettingsTabsAreNamed(t *testing.T) {
	for id, index := range map[string]int{
		"models": settingsModelsTab, "tools": settingsToolsTab,
		"acp": settingsACPTab, "a2a": settingsA2ATab,
		"channels": settingsChannelsTab, "shortcuts": settingsShortcutsTab,
		"quick": settingsQuickTab, "appearance": settingsAppearanceTab,
		"experimental": settingsExperimentalTab, "system": settingsSystemTab,
		"web": settingsWebTab, "mobile": settingsMobileTab, "plugins": settingsPluginsTab,
	} {
		if got := settingsTabIndex(id); got != index {
			t.Errorf("settingsTabIndex(%q) = %d, want %d", id, got, index)
		}
	}
}
