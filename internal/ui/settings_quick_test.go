package ui

import (
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"
)

// The quick panel's settings name each toggle in the row: MyGo's Label is
// only an assistive-technology name, so a bare switch would draw nothing.
func TestQuickPanelSettingsShowSwitchLabels(t *testing.T) {
	cw := testWindow()
	on := true
	cw.cfg.QuickPanel = config.QuickPanelConfig{
		Enabled: &on, Background: &on, ClipboardFallback: &on,
	}
	cw.section = sectionSettings
	cw.settings.tab = settingsQuickTab
	tr := mgo.NewTester(cw.view, 1200, 1000)
	for range 8 {
		tr.Frame()
	}
	for _, want := range []string{
		"快捷任务面板",
		"启用浮窗（替代原来的紧凑窗口）",
		"后台执行，不切换主窗口",
		"没有辅助功能权限时读取剪贴板",
		"默认在新建 worktree 中执行",
	} {
		if !tr.HasText(want) {
			t.Fatalf("the 快捷任务 tab lacks %q; texts: %v", want, tr.Texts())
		}
	}
}

// The panel's settings are their own tab now, and no longer sit in 系统.
func TestQuickPanelSettingsLeftSystemTab(t *testing.T) {
	cw := testWindow()
	cw.section = sectionSettings
	cw.settings.tab = settingsSystemTab
	tr := mgo.NewTester(cw.view, 1200, 1000)
	for range 6 {
		tr.Frame()
	}
	if tr.HasText("快捷任务面板") {
		t.Fatal("the 系统 tab still configures the quick task panel")
	}
	if !tr.HasText("更新") {
		t.Fatalf("tab %d is not the 系统 tab; texts: %v", cw.settings.tab, tr.Texts())
	}
}
