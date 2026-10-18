package ui

import (
	"strings"

	"chatgo/internal/config"
	"chatgo/internal/ui/common"
)

// agentWorkspace is the directory an agent runs in, or empty for the caller's.
func (cw *ChatWindow) agentWorkspace(agent string) string {
	switch {
	case strings.HasPrefix(agent, "agent:"):
		name := strings.TrimPrefix(agent, "agent:")
		for _, a := range cw.cfg.Agents {
			if a.Name == name {
				return a.Workspace
			}
		}
	case strings.HasPrefix(agent, "acp:"):
		name := strings.TrimPrefix(agent, "acp:")
		for _, a := range cw.cfg.ACPAgents {
			if a.Name == name {
				return a.Cwd
			}
		}
	}
	return ""
}

// currentAgentDef returns the custom agent driving the conversation, if any.
func (cw *ChatWindow) currentAgentDef() (config.AgentDef, bool) {
	if cw.currentAgent == "" {
		return config.AgentDef{}, false
	}
	for _, a := range cw.cfg.Agents {
		if a.Name == cw.currentAgent {
			return a, true
		}
	}
	return config.AgentDef{}, false
}

// availableProviderNames lists the enabled providers, in config order.
func (cw *ChatWindow) availableProviderNames() []string {
	var out []string
	for _, p := range cw.cfg.Providers {
		if p.Enabled && p.Name != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

// visibleBuiltinToolNames lists the builtin tools the active agent may use.
func (cw *ChatWindow) visibleBuiltinToolNames() []string {
	a, ok := cw.currentAgentDef()
	var out []string
	for _, t := range cw.cfg.BuiltinTools {
		if !t.Enabled {
			continue
		}
		if ok && !common.ContainsStr(a.BuiltinTools, t.Name) {
			continue
		}
		out = append(out, t.Name)
	}
	return out
}

// visibleMCPServerNames lists the MCP servers the active agent may use.
func (cw *ChatWindow) visibleMCPServerNames() []string {
	a, ok := cw.currentAgentDef()
	var out []string
	for _, s := range cw.cfg.MCPServers {
		if !s.Enabled {
			continue
		}
		if ok && !common.ContainsStr(a.MCPServers, s.Name) {
			continue
		}
		out = append(out, s.Name)
	}
	return out
}

// availableSkillIDs lists the skills enabled globally on the Skill page.
func (cw *ChatWindow) availableSkillIDs() []string {
	if !cw.skills.Loaded {
		cw.loadSkills()
	}
	var out []string
	for _, s := range cw.skills.List {
		if cw.skillEnabled(s.ID) {
			out = append(out, s.ID)
		}
	}
	return out
}

// visibleSkillIDs lists the skills the active agent may use.
func (cw *ChatWindow) visibleSkillIDs() []string {
	out := cw.availableSkillIDs()
	a, ok := cw.currentAgentDef()
	if !ok {
		return out
	}
	allow := cw.skillIDsByName(a.Skills)
	var filtered []string
	for _, id := range out {
		if common.ContainsStr(allow, id) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// loadChatSelection fills the runtime toggles from the saved Chat profile, or
// leaves them all on when the profile was never customized.
func (cw *ChatWindow) loadChatSelection() {
	cw.toolOn = map[string]bool{}
	cw.skillOn = map[string]bool{}
	if !cw.cfg.ChatInitialized {
		return
	}
	for _, name := range cw.visibleBuiltinToolNames() {
		cw.toolOn["builtin:"+name] = common.ContainsStr(cw.cfg.Chat.BuiltinTools, name)
	}
	for _, name := range cw.visibleMCPServerNames() {
		cw.toolOn["mcp:"+name] = common.ContainsStr(cw.cfg.Chat.MCPServers, name)
	}
	for _, id := range cw.visibleSkillIDs() {
		cw.skillOn[id] = common.ContainsStr(cw.cfg.Chat.Skills, id)
	}
}

// saveChatSelection persists the built-in Chat's model, tools and skills.
func (cw *ChatWindow) saveChatSelection() {
	cw.cfg.Chat.Provider = cw.provider
	cw.cfg.Chat.Model = cw.model
	if cw.workspace != "" {
		cw.cfg.Chat.Workspace = cw.workspace
	}
	cw.cfg.Chat.BuiltinTools = nil
	for _, name := range cw.visibleBuiltinToolNames() {
		if cw.toolEnabled("builtin:" + name) {
			cw.cfg.Chat.BuiltinTools = append(cw.cfg.Chat.BuiltinTools, name)
		}
	}
	cw.cfg.Chat.MCPServers = nil
	for _, name := range cw.visibleMCPServerNames() {
		if cw.toolEnabled("mcp:" + name) {
			cw.cfg.Chat.MCPServers = append(cw.cfg.Chat.MCPServers, name)
		}
	}
	cw.cfg.Chat.Skills = nil
	for _, id := range cw.visibleSkillIDs() {
		if cw.skillRuntime(id) {
			cw.cfg.Chat.Skills = append(cw.cfg.Chat.Skills, id)
		}
	}
	cw.cfg.ChatInitialized = true
	_ = config.SaveConfig(cw.cfg)
}
