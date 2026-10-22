package settings

import (
	"strings"

	"chatgo/internal/ui/assets"
	"chatgo/internal/ui/theme"

	"github.com/egoist/mygo/plugins/updater"
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// shortcutTab edits the global shortcut that opens the focused interface.
func shortcutTab(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "快捷键").Font(mtheme.SerifFont).FontWeight(600)
		if s := opts.Status(); s != "" {
			mgo.Text(c, s).TextColor(k.Danger).FontSize(small).
				Font(mtheme.MonoFont)
		}
		mgo.Text(c, "快捷对话").FontSize(small).TextColor(k.TextMuted)
		f := &state.Shortcut
		minput.TextInput(c, &f.Value, minput.TextInputOptions{
			Label: "快捷对话快捷键", Placeholder: opts.DefaultShortcut,
		})
		mgo.Text(c, "示例：CmdOrCtrl+Shift+Space、Alt+Space、CmdOrCtrl+Shift+K").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				opts.Config().QuickChatShortcut = strings.TrimSpace(f.Value)
				opts.SaveQuickShortcut()
				opts.Toast("快捷键已更新", mcore.SeveritySuccess)
			}
			if minput.Button(c, "恢复默认", minput.ButtonOptions{Variant: minput.Outline}).Clicked() {
				f.Value = opts.DefaultShortcut
				opts.Config().QuickChatShortcut = opts.DefaultShortcut
				opts.SaveQuickShortcut()
				opts.Toast("已恢复默认快捷键", mcore.SeveritySuccess)
			}
		})
	})
}

// appearanceTab changes the theme mode and color theme.
func appearanceTab(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "外观").Font(mtheme.SerifFont).FontWeight(600)
		mgo.Text(c, "主题模式").FontSize(small).TextColor(k.TextMuted)
		mode := int(opts.ThemeMode())
		mnavigation.SegmentedControl(c, &mode, []mnavigation.Segment{
			{Label: "跟随系统"}, {Label: "浅色"}, {Label: "深色"},
		}, mnavigation.SegmentedControlOptions{Label: "主题模式", Equal: true})
		if mcore.Mode(mode) != opts.ThemeMode() {
			opts.SetThemeMode(mcore.Mode(mode))
		}
		mgo.Text(c, "颜色主题").FontSize(small).TextColor(k.TextMuted)
		mgo.Row(c).Gap(u * 2).Wrap().Children(func() {
			for i := range theme.Themes {
				colorThemeSwatch(c, state, opts, theme.Themes[i])
			}
		})
		mgo.Text(c, "每种颜色主题都提供浅色与深色两套配色，随「主题模式」自动切换。").
			FontSize(small).TextColor(k.TextMuted)
	})
}

// colorThemeSwatch renders one palette as a selectable card showing its light
// and dark accent over their backgrounds.
func colorThemeSwatch(c *mgo.Context, state *State, opts Options, t theme.Theme) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	selected := opts.ColorThemeName() == t.Name
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
		opts.SetColorTheme(t.Name)
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

// systemTab shows tray, background-run, update and about options.
func systemTab(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Gap(u * 2).MaxWidth(560).Children(func() {
		mgo.Text(c, "系统").Font(mtheme.SerifFont).FontWeight(600)
		cfg := opts.Config()
		if cfg.ShowTrayIcon == nil {
			on := true
			cfg.ShowTrayIcon = &on
		}
		if minput.Switch(c, cfg.ShowTrayIcon, "显示托盘图标", minput.SwitchOptions{}).Changed() {
			opts.SaveSystemSettings()
		}
		mgo.Text(c, "在系统托盘 / 菜单栏显示 ChatGo 图标。点击图标打开程序，右键菜单可快速新建会话并进入最近会话。").
			FontSize(small).TextColor(k.TextMuted)
		if minput.Switch(c, &opts.Config().RunInBackground, "允许后台运行（关闭窗口时最小化到托盘）", minput.SwitchOptions{}).Changed() {
			opts.SaveSystemSettings()
		}
		mgo.Text(c, "关闭窗口时不退出程序，而是隐藏到托盘继续后台运行（自动启用托盘图标）。").
			FontSize(small).TextColor(k.TextMuted)
		mgo.Text(c, "更新").Font(mtheme.SerifFont).FontWeight(600)
		mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
			if minput.Button(c, "检查更新", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				updater.CheckForUpdates()
			}
			mgo.Text(c, "通过 MyGo Updater 从官方发布自动更新，仅在打包安装后的应用中可用。").
				FontSize(small).TextColor(k.TextMuted)
		})
		mgo.Text(c, "关于").Font(mtheme.SerifFont).FontWeight(600)
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			mgo.Image(c, assets.BrandBitmap()).Size(48, 48)
			mgo.Column(c).Gap(u / 2).Children(func() {
				mgo.Text(c, "ChatGo").FontWeight(600)
				mgo.Text(c, "版本 "+opts.AppVersion()).FontSize(small).TextColor(k.TextMuted)
				mgo.Text(c, "MyGo 原生 UI · MIT 许可").FontSize(small).TextColor(k.TextMuted)
			})
		})
	})
}
