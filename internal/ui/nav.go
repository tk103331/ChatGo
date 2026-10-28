package ui

import (
	"chatgo/internal/ui/assets"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

const navItemSize float32 = 44

type navSection int

const (
	sectionChat navSection = iota
	sectionApps
	sectionRoom
	sectionAgent
	sectionSkill
	sectionAuto
	sectionKnowledge
	sectionMCP
	sectionStats
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
		cw.navItem(c, "群聊", "users", sectionRoom)
		cw.navItem(c, "自动任务", "clock", sectionAuto)
		cw.navItem(c, "Agent", "bot", sectionAgent)
		cw.navItem(c, "Skill", "brain", sectionSkill)
		cw.navItem(c, "知识库", "file-text", sectionKnowledge)
		cw.navItemSVG(c, "MCP", assets.MCPLogo(), sectionMCP)
		cw.navItem(c, "统计", "chart-column", sectionStats)
		mgo.Spacer(c)
		label := "外观：" + themeModeLabel(cw.themeMode)
		mode := mcore.IconAction(c, icons.Must(themeModeIcon(cw.themeMode)), label)
		mcore.TooltipEnd(c, mode, label)
		if mode.Clicked() {
			cw.cycleTheme()
		}
		cw.navItem(c, "设置", "settings", sectionSettings)
	})
}

func (cw *ChatWindow) navItem(c *mgo.Context, label, icon string, section navSection) {
	k := mcore.Tokens(c)
	b := mgo.ButtonBase(c).Size(navItemSize, navItemSize).Shrink(0).Radius(mtheme.ControlRadius).Label(label)
	if cw.section == section {
		b.Background(k.Selection).TextColor(k.Text)
	} else {
		b.TextColor(k.TextMuted)
		if b.Hovered() {
			b.Background(k.SurfaceHover).TextColor(k.Text)
		}
	}
	b.Children(func() { mgo.Icon(c, icons.Must(icon)).Size(20, 20) })
	mcore.TooltipEnd(c, b, label)
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
	b := mgo.ButtonBase(c).Size(navItemSize, navItemSize).Shrink(0).Radius(mtheme.ControlRadius).Label(label)
	fg := k.TextMuted
	if cw.section == section {
		b.Background(k.Selection)
		fg = k.Text
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
		fg = k.Text
	}
	b.Children(func() { mgo.Icon(c, svg).Size(20, 20).TextColor(fg) })
	mcore.TooltipEnd(c, b, label)
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

// goConfigure leaves the quick interface and opens a navigation section.
func (cw *ChatWindow) goConfigure(section navSection) {
	if cw.quickMode {
		cw.toggleQuickMode()
	}
	cw.section = section
	cw.invalidate()
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
	case sectionRoom:
		cw.roomsPage(c)
	case sectionAgent:
		cw.agentsPage(c)
	case sectionSkill:
		cw.skillsPage(c)
	case sectionAuto:
		cw.autoTasksPage(c)
	case sectionKnowledge:
		cw.knowledgePage(c)
	case sectionMCP:
		cw.mcpPage(c)
	case sectionStats:
		cw.statsPage(c)
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
