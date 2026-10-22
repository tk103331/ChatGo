package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/ui/assets"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

func (cw *ChatWindow) shortcutTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "快捷键").FontWeight(600)
		if cw.status != "" {
			mgo.Text(c, cw.status).TextColor(k.Danger).FontSize(small).
				Font(mtheme.MonoFont)
		}
		mgo.Text(c, "快捷对话").FontSize(small).TextColor(k.TextMuted)
		f := &cw.settings.shortcut
		kit.TextInput(c, &f.value, kit.TextInputOptions{
			Label: "快捷对话快捷键", Placeholder: defaultQuickShortcut,
		})
		mgo.Text(c, "示例：CmdOrCtrl+Shift+Space、Alt+Space、CmdOrCtrl+Shift+K").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.cfg.QuickChatShortcut = strings.TrimSpace(f.value)
				cw.saveQuickShortcut()
				cw.toast("快捷键已更新", mcore.SeveritySuccess)
			}
			if kit.Button(c, "恢复默认", kit.ButtonOptions{Variant: kit.Outline}).Clicked() {
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
		mgo.Text(c, "外观").FontWeight(600)
		mgo.Text(c, "主题模式").FontSize(small).TextColor(k.TextMuted)
		mode := int(cw.themeMode)
		kit.SegmentedControl(c, &mode, []kit.Segment{
			{Label: "跟随系统"}, {Label: "浅色"}, {Label: "深色"},
		}, kit.SegmentedControlOptions{Label: "主题模式", Equal: true})
		if mcore.Mode(mode) != cw.themeMode {
			cw.themeMode = mcore.Mode(mode)
			cw.saveAppearance()
		}
		mgo.Text(c, "主题色").FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u * 2).Wrap().Children(func() {
			for i := range mtheme.Themes {
				cw.colorThemeSwatch(c, mtheme.Themes[i])
			}
		})
		mgo.Text(c, "主题色只改强调色，底色与文字沿用 MyGo 原生的浅色/深色主题。").
			FontSize(small).TextColor(k.TextMuted)
	})
}

// colorThemeSwatch is one selectable accent, previewed in both appearances.
func (cw *ChatWindow) colorThemeSwatch(c *mgo.Context, t mtheme.Theme) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	selected := cw.colorTheme.Name == t.Name
	b := mgo.ButtonBase(c.Key("accent:"+t.Name)).Width(104).Padding(u*2).Gap(u).
		AlignItems(mgo.Center).Radius(k.Radius).Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border).Label(t.Label).Selected(selected)
	switch {
	case selected:
		b.BorderColor(k.Accent)
	case b.Hovered():
		b.Background(k.SurfaceHover)
	}
	light, dark := t.Sample()
	b.Children(func() {
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			accentDot(c, light)
			accentDot(c, dark)
		})
		label := mgo.Text(c, t.Label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).SingleLine()
		if selected {
			label.FontWeight(600).TextColor(k.Accent)
		}
	})
	if b.Clicked() && !selected {
		cw.colorTheme = t
		cw.saveAppearance()
	}
}

// accentDot previews one appearance's accent.
func accentDot(c *mgo.Context, fill mgo.Color) {
	mgo.Box(c).Size(18, 18).Radius(9).Background(fill).
		Border(mtheme.BorderWidth, mcore.Tokens(c).Border)
}

func (cw *ChatWindow) systemTab(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "系统").FontWeight(600)
		if mgo.Switch(c, &cw.cfg.ShowTrayIcon).Label("显示托盘图标").Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "在系统托盘 / 菜单栏显示 ChatGo 图标。点击图标打开程序，右键菜单可快速新建会话并进入最近会话。").
			FontSize(small).TextColor(k.TextMuted)
		if mgo.Switch(c, &cw.cfg.RunInBackground).Label("允许后台运行（关闭窗口时最小化到托盘）").Changed() {
			cw.saveSystemSettings()
		}
		mgo.Text(c, "关闭窗口时不退出程序，而是隐藏到托盘继续后台运行（自动启用托盘图标）。").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Text(c, "更新").FontWeight(600)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "检查更新", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				updater.CheckForUpdates()
			}
			mgo.Text(c, "通过 MyGo Updater 从官方发布自动更新，仅在打包安装后的应用中可用。").
				FontSize(small).TextColor(k.TextMuted)
		})
		mgo.Text(c, "关于").FontWeight(600)
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			mgo.Icon(c, assets.BrandMark()).Size(48, 48).TextColor(k.Accent)
			mgo.Column(c).Gap(u / 2).Children(func() {
				mgo.Text(c, "ChatGo").FontWeight(600)
				mgo.Text(c, "版本 "+cw.appVersion()).FontSize(small).TextColor(k.TextMuted)
				mgo.Text(c, "MyGo 原生 UI · MIT 许可").FontSize(small).TextColor(k.TextMuted)
			})
		})
	})
}

// appVersion returns the packaged version, or a dev marker when unbundled.
func (cw *ChatWindow) appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "(开发版)"
}

func (cw *ChatWindow) saveSystemSettings() {
	if cw.cfg.RunInBackground {
		cw.cfg.ShowTrayIcon = true
	}
	_ = config.SaveConfig(cw.cfg)
	cw.syncTray()
	cw.toast("系统设置已保存", mcore.SeveritySuccess)
}
