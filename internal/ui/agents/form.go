package agents

import (
	"strings"

	"chatgo/internal/config"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// LoadForm fills the form from the agent at index i.
func LoadForm(state *State, opts Options, i int) {
	a := opts.Config().Agents[i]
	state.Form = Form{
		Editing: i, Name: a.Name, Description: a.Description,
		SystemPrompt: a.SystemPrompt, Workspace: a.Workspace,
		Icon: a.Icon, AvatarSeed: a.AvatarSeed,
		Models:       append([]string(nil), a.Models...),
		BuiltinTools: append([]string(nil), a.BuiltinTools...),
		MCPServers:   append([]string(nil), a.MCPServers...),
		Skills:       append([]string(nil), a.Skills...),
	}
}

// ClearForm resets the form for a new agent, pre-selecting every globally
// enabled model, tool, MCP server and skill.
func ClearForm(state *State, opts Options) {
	state.Form = Form{Editing: -1}
	state.Form.Models = opts.AllModelRefs()
	state.Form.BuiltinTools = opts.EnabledBuiltinTools()
	state.Form.MCPServers = opts.EnabledMCPServers()
	for _, sk := range opts.EnabledSkills() {
		state.Form.Skills = append(state.Form.Skills, sk.Name)
	}
}

// Save validates the form and writes the agent preset into the config.
// It returns false when validation fails.
func Save(state *State, opts Options) bool {
	f := &state.Form
	if strings.TrimSpace(f.Name) == "" {
		opts.Toast("Agent 名称不能为空", mcore.SeverityError)
		return false
	}
	if f.Name == ChatName {
		opts.Toast("「"+ChatName+"」是内置 Agent，请换一个名称", mcore.SeverityError)
		return false
	}
	mode := "chat"
	if len(f.BuiltinTools) > 0 || len(f.MCPServers) > 0 {
		mode = "agent"
	}
	def := config.AgentDef{
		Name: f.Name, Description: f.Description, SystemPrompt: f.SystemPrompt,
		Workspace: f.Workspace, Icon: f.Icon, AvatarSeed: f.AvatarSeed, Models: append([]string(nil), f.Models...), Mode: mode,
		BuiltinTools: f.BuiltinTools, MCPServers: f.MCPServers, Skills: f.Skills,
	}
	cfg := opts.Config()
	if f.Editing >= 0 && f.Editing < len(cfg.Agents) {
		cfg.Agents[f.Editing] = def
	} else {
		cfg.Agents = append(cfg.Agents, def)
		f.Editing = len(cfg.Agents) - 1
	}
	_ = opts.SaveConfig()
	opts.Toast("已保存 Agent "+def.Name, mcore.SeveritySuccess)
	return true
}

// Delete removes the agent being edited after confirmation.
func Delete(state *State, opts Options) {
	f := &state.Form
	if f.Editing < 0 || f.Editing >= len(opts.Config().Agents) {
		return
	}
	DeleteAt(state, opts, f.Editing)
	state.EditorOpen = false
}

// DeleteAt removes the agent at index i after confirmation.
func DeleteAt(state *State, opts Options, i int) {
	cfg := opts.Config()
	if i < 0 || i >= len(cfg.Agents) {
		return
	}
	name := cfg.Agents[i].Name
	opts.AskConfirm("删除 Agent", "确定删除「"+name+"」？", func() {
		cfg.Agents = append(cfg.Agents[:i], cfg.Agents[i+1:]...)
		_ = opts.SaveConfig()
		ClearForm(state, opts)
		if opts.OnDeleted != nil {
			opts.OnDeleted(name)
		}
		opts.Toast("Agent 已删除", mcore.SeveritySuccess)
	})
}
