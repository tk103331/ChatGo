package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	mnavigation "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/navigation"
)

// commandPalette renders the Cmd+K quick-navigation palette over the window.
func (cw *ChatWindow) commandPalette(c *mgo.Context) {
	var commands []mnavigation.Command
	if cw.paletteOpen {
		commands = cw.paletteCommands()
	}
	res := mnavigation.CommandPalette(c, &cw.paletteOpen, commands, mnavigation.CommandPaletteOptions{})
	if id, ok := res.Chosen(); ok {
		cw.runPaletteCommand(id)
	}
}

// paletteCommands builds the palette entries: navigation, actions, theme,
// settings tabs, recent sessions, agents, MCP servers and skills.
func (cw *ChatWindow) paletteCommands() []mnavigation.Command {
	var cmds []mnavigation.Command
	cmds = append(cmds,
		mnavigation.Command{ID: "nav:chat", Label: "前往：对话", Group: "导航", Icon: icons.Must("message-square"), Keywords: []string{"chat", "会话", "conversation"}},
		mnavigation.Command{ID: "nav:auto", Label: "前往：自动任务", Group: "导航", Icon: icons.Must("clock"), Keywords: []string{"auto", "task", "定时"}},
		mnavigation.Command{ID: "nav:agent", Label: "前往：Agent", Group: "导航", Icon: icons.Must("bot"), Keywords: []string{"代理", "助手"}},
		mnavigation.Command{ID: "nav:skill", Label: "前往：Skill", Group: "导航", Icon: icons.Must("brain"), Keywords: []string{"技能"}},
		mnavigation.Command{ID: "nav:mcp", Label: "前往：MCP", Group: "导航", Icon: icons.Must("layout-grid"), Keywords: []string{"服务器"}},
		mnavigation.Command{ID: "nav:stats", Label: "前往：统计", Group: "导航", Icon: icons.Must("chart-column"), Keywords: []string{"stats", "用量"}},
		mnavigation.Command{ID: "nav:settings", Label: "前往：设置", Group: "导航", Icon: icons.Must("settings"), Keywords: []string{"配置", "preferences"}},
	)
	cmds = append(cmds,
		mnavigation.Command{ID: "action:new", Label: "新建会话", Group: "操作", Icon: icons.Must("plus"), Keywords: []string{"new", "创建", "create"}},
		mnavigation.Command{ID: "action:terminal", Label: "切换终端", Group: "操作", Icon: icons.Must("terminal"), Keywords: []string{"toggle", "terminal"}},
		mnavigation.Command{ID: "action:left", Label: "切换会话列表", Group: "操作", Icon: icons.Must("panel-left"), Keywords: []string{"sidebar", "侧栏"}},
		mnavigation.Command{ID: "action:quick", Label: "聚焦对话", Group: "操作", Icon: icons.Must("maximize"), Keywords: []string{"quick", "快捷"}},
	)
	cmds = append(cmds,
		mnavigation.Command{ID: "theme:light", Label: "主题：浅色", Group: "外观", Icon: icons.Must("sun"), Keywords: []string{"theme", "light"}},
		mnavigation.Command{ID: "theme:dark", Label: "主题：深色", Group: "外观", Icon: icons.Must("moon"), Keywords: []string{"theme", "dark"}},
		mnavigation.Command{ID: "theme:system", Label: "主题：跟随系统", Group: "外观", Icon: icons.Must("palette"), Keywords: []string{"theme", "system"}},
	)
	for _, t := range settingsPaletteTabs {
		cmds = append(cmds, mnavigation.Command{ID: t.id, Label: t.label, Group: "设置", Icon: icons.Must("settings"), Keywords: t.keywords})
	}
	for i := 0; i < len(cw.convs) && i < 20; i++ {
		conv := cw.convs[i]
		title := conv.Title
		if title == "" {
			title = defaultTitle
		}
		cmds = append(cmds, mnavigation.Command{ID: "conv:" + conv.ID, Label: title, Group: "会话", Keywords: []string{conv.Workspace}})
	}
	for _, a := range cw.cfg.Agents {
		cmds = append(cmds, mnavigation.Command{ID: "agent:" + a.Name, Label: "Agent：" + a.Name, Group: "Agent", Icon: icons.Must("bot"), Keywords: []string{a.Description}})
	}
	for _, s := range cw.cfg.MCPServers {
		cmds = append(cmds, mnavigation.Command{ID: "mcp:" + s.Name, Label: "MCP：" + s.Name, Group: "MCP 服务器", Icon: icons.Must("layout-grid")})
	}
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	for _, s := range cw.skills.list {
		cmds = append(cmds, mnavigation.Command{ID: "skill:" + s.ID, Label: "Skill：" + s.Name, Group: "Skill", Icon: icons.Must("brain"), Keywords: []string{s.Description}})
	}
	return cmds
}

var settingsPaletteTabs = []struct {
	id       string
	label    string
	keywords []string
}{
	{"settings:models", "设置：模型", []string{"模型", "provider"}},
	{"settings:tools", "设置：工具", []string{"工具", "builtin"}},
	{"settings:acp", "设置：ACP", []string{"acp"}},
	{"settings:channels", "设置：渠道", []string{"渠道", "channel", "feishu", "wechat"}},
	{"settings:shortcuts", "设置：快捷键", []string{"快捷键", "shortcut"}},
	{"settings:appearance", "设置：外观", []string{"外观", "appearance"}},
	{"settings:system", "设置：系统", []string{"系统", "system"}},
}

var settingsPaletteTabIdx = map[string]int{
	"models": 0, "tools": 1, "acp": 2, "channels": 3,
	"shortcuts": 4, "appearance": 5, "system": 6,
}

// runPaletteCommand executes the chosen palette command.
func (cw *ChatWindow) runPaletteCommand(id string) {
	switch {
	case id == "nav:chat":
		cw.goConfigure(sectionChat)
	case id == "nav:auto":
		cw.goConfigure(sectionAuto)
	case id == "nav:agent":
		cw.goConfigure(sectionAgent)
	case id == "nav:skill":
		cw.goConfigure(sectionSkill)
	case id == "nav:mcp":
		cw.goConfigure(sectionMCP)
	case id == "nav:stats":
		cw.goConfigure(sectionStats)
	case id == "nav:settings":
		cw.settings.tab = 0
		cw.goConfigure(sectionSettings)
	case id == "action:new":
		cw.goConfigure(sectionChat)
		cw.newConversation()
	case id == "action:terminal":
		cw.goConfigure(sectionChat)
		cw.toggleTerminal()
	case id == "action:left":
		cw.goConfigure(sectionChat)
		cw.leftHidden = !cw.leftHidden
	case id == "action:quick":
		cw.openQuick()
	case id == "theme:light":
		cw.themeMode = mcore.Light
		cw.saveAppearance()
	case id == "theme:dark":
		cw.themeMode = mcore.Dark
		cw.saveAppearance()
	case id == "theme:system":
		cw.themeMode = mcore.System
		cw.saveAppearance()
	case strings.HasPrefix(id, "settings:"):
		if n, ok := settingsPaletteTabIdx[strings.TrimPrefix(id, "settings:")]; ok {
			cw.settings.tab = n
			cw.goConfigure(sectionSettings)
		}
	case strings.HasPrefix(id, "conv:"):
		cw.goConfigure(sectionChat)
		cw.selectConversation(strings.TrimPrefix(id, "conv:"))
	case strings.HasPrefix(id, "agent:"):
		cw.goConfigure(sectionAgent)
	case strings.HasPrefix(id, "mcp:"):
		cw.goConfigure(sectionMCP)
	case strings.HasPrefix(id, "skill:"):
		cw.goConfigure(sectionSkill)
	}
	cw.invalidate()
}
