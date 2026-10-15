package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/ui/theme"
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

func (cw *ChatWindow) shortcutTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "快捷键").Font(mtheme.SerifFont).FontWeight(600)
		if cw.status != "" {
			mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(small).
				Font(mtheme.MonoFont)
		}
		mgo.Text(c, "快捷对话").FontSize(small).TextColor(k.TextMuted)
		f := &cw.settings.shortcut
		minput.TextInput(c, &f.value, minput.TextInputOptions{
			Label: "快捷对话快捷键", Placeholder: defaultQuickShortcut,
		})
		mgo.Text(c, "示例：CmdOrCtrl+Shift+Space、Alt+Space、CmdOrCtrl+Shift+K").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				cw.cfg.QuickChatShortcut = strings.TrimSpace(f.value)
				cw.saveQuickShortcut()
				cw.toast("快捷键已更新", mcore.SeveritySuccess)
			}
			if minput.Button(c, "恢复默认", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				f.value = defaultQuickShortcut
				cw.cfg.QuickChatShortcut = defaultQuickShortcut
				cw.saveQuickShortcut()
				cw.toast("已恢复默认快捷键", mcore.SeveritySuccess)
			}
		})
	})
}

// appearanceTab changes the theme mode and color theme.

func (cw *ChatWindow) appearanceTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "外观").Font(mtheme.SerifFont).FontWeight(600)
		mgo.Text(c, "主题模式").FontSize(small).TextColor(k.TextMuted)
		mode := int(cw.themeMode)
		mnavigation.SegmentedControl(c, &mode, []mnavigation.Segment{
			{Label: "跟随系统"}, {Label: "浅色"}, {Label: "深色"},
		}, mnavigation.SegmentedControlOptions{Label: "主题模式", Equal: true})
		if mcore.Mode(mode) != cw.themeMode {
			cw.themeMode = mcore.Mode(mode)
			cw.saveAppearance()
		}
		mgo.Text(c, "颜色主题").FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u * 2).Wrap().Children(func() {
			for i := range theme.Themes {
				cw.colorThemeSwatch(c, theme.Themes[i])
			}
		})
		mgo.Text(c, "每种颜色主题都提供浅色与深色两套配色，随「主题模式」自动切换。").
			FontSize(small).TextColor(k.TextMuted)
	})
}

// colorThemeSwatch renders one palette as a selectable card showing its light
// and dark accent over their backgrounds.

func (cw *ChatWindow) colorThemeSwatch(c *mgo.Context, t theme.Theme) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	selected := cw.colorTheme.Name == t.Name
	b := mgo.ButtonBase(c).Width(160).Padding(u*2).Gap(u).Radius(mtheme.CardRadius).
		AlignItems(mgo.Center).Background(k.Surface).Border(mtheme.BorderWidth, k.Border)
	if selected {
		b.BorderColor(k.Accent)
	}
	if b.Hovered() && !selected {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Column(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u).Shrink(0).Children(func() {
				lb, la := t.SampleLight()
				db, da := t.SampleDark()
				colorThemeBlock(c, lb, la)
				colorThemeBlock(c, db, da)
			})
			mgo.Text(c, t.Label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
				TextColor(k.Text).SingleLine()
		})
	})
	if b.Clicked() {
		cw.colorTheme = t
		cw.saveAppearance()
	}
}

// colorThemeBlock draws one appearance sample: an accent bar over a background.

func colorThemeBlock(c *mgo.Context, bg, accent mgo.Color) {
	mgo.Box(c).Width(46).Height(34).Radius(mtheme.ControlRadius).Background(bg).
		Border(mtheme.BorderWidth, accent).Children(func() {
		mgo.Row(c).Fill().AlignItems(mgo.Center).Justify(mgo.Center).Children(func() {
			mgo.Box(c).Width(24).Height(9).Radius(mtheme.ControlRadius).Background(accent)
		})
	})
}

func (cw *ChatWindow) systemTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "系统").Font(mtheme.SerifFont).FontWeight(600)
		if minput.Switch(c, &cw.cfg.ShowTrayIcon, "显示托盘图标", minput.SwitchOptions{}).Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "在系统托盘 / 菜单栏显示 ChatGo 图标。点击图标打开程序，右键菜单可快速新建会话并进入最近会话。").
			FontSize(small).TextColor(k.TextMuted)
		if minput.Switch(c, &cw.cfg.RunInBackground, "后台运行（关闭窗口时最小化到托盘）", minput.SwitchOptions{}).Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "关闭窗口时不退出程序，而是隐藏到托盘继续后台运行，并隐藏任务栏 / Dock 图标（自动启用托盘图标）。").
			FontSize(small).TextColor(k.TextMuted)
	})
}

func (cw *ChatWindow) saveSystemSettings() {
	if cw.cfg.RunInBackground {
		cw.cfg.ShowTrayIcon = true
	}
	_ = config.SaveConfig(cw.cfg)
	cw.syncTray()
	cw.toast("系统设置已保存", mcore.SeveritySuccess)
}
