package ui

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

type navSection int

const (
	sectionChat navSection = iota
	sectionAgent
	sectionACP
	sectionSettings
)

// navRail is the persistent leftmost icon menu bar.
func (cw *ChatWindow) navRail(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(60).Shrink(0).FillHeight().Background(k.Surface).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).
		Padding(u).Gap(u).AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "CG").Font(mtheme.SerifFont).FontSize(mcore.FontSize(c, mtheme.BodySize)).
			FontWeight(600).Margin(u, 0, u*2, 0)
		cw.navItem(c, "对话", "message-square", sectionChat)
		cw.navItem(c, "Agent", "bot", sectionAgent)
		cw.navItem(c, "ACP", "terminal", sectionACP)
		mgo.Spacer(c)
		label := "外观：" + themeModeLabel(cw.themeMode)
		if mcore.IconAction(c, icons.Must(themeModeIcon(cw.themeMode)), label).Tooltip(label).Clicked() {
			cw.cycleTheme()
		}
		cw.navItem(c, "设置", "settings", sectionSettings)
	})
}

func (cw *ChatWindow) navItem(c *mgo.Context, label, icon string, section navSection) {
	k := mcore.Tokens(c)
	b := mgo.ButtonBase(c).Size(44, 44).Shrink(0).Radius(mtheme.ControlRadius).Label(label).Tooltip(label)
	if cw.section == section {
		b.Background(k.Selection).TextColor(k.Text)
	} else {
		b.TextColor(k.TextMuted)
		if b.Hovered() {
			b.Background(k.SurfaceHover).TextColor(k.Text)
		}
	}
	b.Children(func() { mgo.Icon(c, icons.Must(icon)).Size(20, 20) })
	if b.Clicked() {
		cw.section = section
	}
}

// sectionView renders the page for the current section.
func (cw *ChatWindow) sectionView(c *mgo.Context) {
	switch cw.section {
	case sectionAgent:
		cw.agentsPage(c)
	case sectionACP:
		cw.acpPage(c)
	case sectionSettings:
		cw.settingsPage(c)
	default:
		if cw.mode == modeHome {
			cw.homeView(c)
		} else {
			cw.chatView(c)
		}
	}
}

func themeModeLabel(m mcore.Mode) string {
	switch m {
	case mcore.Light:
		return "浅色"
	case mcore.Dark:
		return "深色"
	default:
		return "跟随系统"
	}
}

func themeModeIcon(m mcore.Mode) string {
	switch m {
	case mcore.Light:
		return "sun"
	case mcore.Dark:
		return "moon"
	default:
		return "palette"
	}
}
