package ui

import (
	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

const navItemSize float32 = 44

type navSection int

const (
	sectionChat navSection = iota
	sectionApps
	sectionAgent
	sectionSkill
	sectionAuto
	sectionMCP
	sectionSettings
)

// navRail is the persistent leftmost icon menu bar.
func (cw *ChatWindow) navRail(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(60).Shrink(0).FillHeight().Background(k.Surface).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).
		Padding(u).Gap(u).AlignItems(mgo.Center).Children(func() {
		cw.navItem(c, "对话", "message-square", sectionChat)
		cw.navItem(c, "自动任务", "clock", sectionAuto)
		cw.navItem(c, "Agent", "bot", sectionAgent)
		cw.navItem(c, "Skill", "brain", sectionSkill)
		cw.navItemSVG(c, "MCP", mcpLogo(), sectionMCP)
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
	b := mgo.ButtonBase(c).Size(navItemSize, navItemSize).Shrink(0).Radius(mtheme.ControlRadius).Label(label).Tooltip(label)
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
		cw.goSection(section)
	}
}

// navItemSVG is navItem for a brand mark drawn from an SVG, tinted to match.
func (cw *ChatWindow) navItemSVG(c *mgo.Context, label string, svg *mgo.SVG, section navSection) {
	if svg == nil {
		cw.navItem(c, label, "layout-grid", section)
		return
	}
	k := mcore.Tokens(c)
	b := mgo.ButtonBase(c).Size(navItemSize, navItemSize).Shrink(0).Radius(mtheme.ControlRadius).Label(label).Tooltip(label)
	fg := k.TextMuted
	if cw.section == section {
		b.Background(k.Selection)
		fg = k.Text
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
		fg = k.Text
	}
	b.Children(func() { mgo.Icon(c, svg).Size(20, 20).TextColor(fg) })
	if b.Clicked() {
		cw.goSection(section)
	}
}

// goSection switches the navigation section, restoring the user's normal
// conversation when leaving the Apps section.
func (cw *ChatWindow) goSection(section navSection) {
	if cw.section == sectionApps && section != sectionApps {
		cw.appActive = ""
		if cw.preAppConvID != "" {
			cw.selectConversation(cw.preAppConvID)
			cw.preAppConvID = ""
		}
	}
	cw.section = section
}

// sectionView renders the page for the current section.
func (cw *ChatWindow) sectionView(c *mgo.Context) {
	switch cw.section {
	case sectionApps:
		if cw.appActive == "" {
			cw.appsPage(c)
		} else {
			cw.appWorkspace(c)
		}
	case sectionAgent:
		cw.agentsPage(c)
	case sectionSkill:
		cw.skillsPage(c)
	case sectionAuto:
		cw.autoTasksPage(c)
	case sectionMCP:
		cw.mcpPage(c)
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
