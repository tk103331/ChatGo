package ui

import (
	"chatgo/internal/config"
	agentspkg "chatgo/internal/ui/agents"
	skillspkg "chatgo/internal/ui/skills"

	mgo "github.com/egoist/mygo/ui"

	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
)

const chatAgentName = agentspkg.ChatName

const chatAgentIcon = "message-square"

// drawAgentMark draws an agent's mark from its icon path, avatar seed or name.
func (cw *ChatWindow) drawAgentMark(c *mgo.Context, icon, avatarSeed, name string, size float32) {
	if icon != "" && drawIconImage(c, cw.loadIconImage(icon), size) {
		return
	}
	seed := avatarSeed
	if seed == "" {
		seed = name
	}
	if bmp := cw.agentBitmap(seed); bmp != nil {
		mgo.Image(c, bmp).Size(size, size).Shrink(0)
	}
}

// agentMarkByName finds an agent by name and draws its mark.
func (cw *ChatWindow) agentMarkByName(c *mgo.Context, name string, size float32) {
	if a, ok := cw.agentByName(name); ok {
		cw.drawAgentMark(c, a.Icon, a.AvatarSeed, a.Name, size)
		return
	}
	if bmp := cw.agentBitmap(name); bmp != nil {
		mgo.Image(c, bmp).Size(size, size).Shrink(0)
	}
}

// agentByName finds an agent preset by its display name.
func (cw *ChatWindow) agentByName(name string) (config.AgentDef, bool) {
	for _, a := range cw.cfg.Agents {
		if a.Name == name {
			return a, true
		}
	}
	return config.AgentDef{}, false
}

// enabledBuiltinTools lists the globally enabled built-in tool types.
func (cw *ChatWindow) enabledBuiltinTools() []string {
	var out []string
	for _, t := range cw.cfg.BuiltinTools {
		if t.Enabled {
			out = append(out, t.Type)
		}
	}
	return out
}

// enabledMCPServers lists the globally enabled MCP server names.
func (cw *ChatWindow) enabledMCPServers() []string {
	var out []string
	for _, s := range cw.cfg.MCPServers {
		if s.Enabled {
			out = append(out, s.Name)
		}
	}
	return out
}

// enabledSkillList lists the globally enabled skills.
func (cw *ChatWindow) enabledSkillList() []skillspkg.Skill {
	var out []skillspkg.Skill
	for _, s := range cw.skillList() {
		if cw.skillEnabled(s.ID) {
			out = append(out, s)
		}
	}
	return out
}

// skillList returns the loaded skills, scanning once when needed.
func (cw *ChatWindow) skillList() []skillspkg.Skill {
	if !cw.skills.Loaded {
		cw.loadSkills()
	}
	return cw.skills.List
}

// agentOptions builds the agents page Options from host state.
func (cw *ChatWindow) agentOptions() agentspkg.Options {
	return agentspkg.Options{
		Config:              func() *config.Config { return cw.cfg },
		ModelOptions:        cw.modelOptions,
		ModelLabel:          modelLabel,
		AllModelRefs:        cw.allModelRefs,
		EnabledBuiltinTools: cw.enabledBuiltinTools,
		EnabledMCPServers:   cw.enabledMCPServers,
		EnabledSkills:       cw.enabledSkillList,
		DrawMark:            cw.drawAgentMark,
		PickIconFile:        cw.pickIconFile,
		PickDirectory:       cw.pickDirectory,
		Invalidate:          cw.invalidate,
		Toast:               cw.toast,
		AskConfirm:          cw.askConfirm,
		SaveConfig:          func() error { return config.SaveConfig(cw.cfg) },
		GoConfigure: func(target int) {
			switch target {
			case agentspkg.NavModels:
				cw.settings.Tab = 0
				cw.goConfigure(sectionSettings)
			case agentspkg.NavTools:
				cw.settings.Tab = 1
				cw.goConfigure(sectionSettings)
			case agentspkg.NavMCP:
				cw.goConfigure(sectionMCP)
			case agentspkg.NavSkills:
				cw.goConfigure(sectionSkill)
			}
		},
		OnDeleted: func(name string) {
			if cw.currentAgent == name {
				cw.currentAgent = ""
				cw.applyAgent("")
			}
		},
	}
}

// agentSelector chooses an agent preset for the current conversation.
func (cw *ChatWindow) agentSelector(c *mgo.Context) {
	if len(cw.cfg.Agents) == 0 {
		return
	}
	opts := make([]minput.SelectOption[string], 0, len(cw.cfg.Agents)+1)
	opts = append(opts, minput.SelectOption[string]{Value: "", Label: chatAgentName})
	for _, a := range cw.cfg.Agents {
		opts = append(opts, minput.SelectOption[string]{Value: a.Name, Label: a.Name})
	}
	r := minput.Select(c, &cw.currentAgent, opts, minput.SelectOptions{Label: "Agent"})
	if r.Changed() {
		cw.applyAgent(cw.currentAgent)
		if cw.cur != nil {
			if cw.currentAgent == "" {
				cw.cur.Agent = ""
			} else {
				cw.cur.Agent = "agent:" + cw.currentAgent
			}
			_ = cw.convManager.SaveConversation(cw.cur)
		}
	}
}

// applyAgent applies an agent's provider, mode, prompt and tools. The built-in
// Chat restores its saved selection; a custom agent starts with every
// whitelisted model, tool, MCP server and skill enabled.
func (cw *ChatWindow) applyAgent(name string) {
	cw.currentAgent = name
	cw.systemPrompt = ""
	if name == "" {
		cw.agentMode = false
		cw.cfg.UseReactAgent = false
		cw.loadChatSelection()
		cw.provider = cw.cfg.Chat.Provider
		cw.model = cw.cfg.Chat.Model
		if cw.cfg.Chat.Workspace != "" {
			cw.workspace = cw.cfg.Chat.Workspace
		}
		cw.setupProvider(cw.provider)
		return
	}
	for _, a := range cw.cfg.Agents {
		if a.Name != name {
			continue
		}
		cw.systemPrompt = a.SystemPrompt
		cw.agentMode = a.Mode == "agent"
		cw.cfg.UseReactAgent = cw.agentMode
		cw.toolOn = map[string]bool{}
		cw.skillOn = map[string]bool{}
		if a.Workspace != "" {
			cw.workspace = a.Workspace
		}
		if refs := cw.agentModelRefs(); len(refs) > 0 {
			cw.setModelRef(refs[0])
		}
		cw.setupProvider(cw.provider)
		return
	}
	cw.currentAgent = ""
	cw.applyAgent("")
}
