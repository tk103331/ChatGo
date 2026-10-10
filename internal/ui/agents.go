package ui

import (
	"fmt"
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "github.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

const chatAgentName = "Chat"

const chatAgentIcon = "message-square"

type agentForm struct {
	editing      int
	name         string
	description  string
	icon         string
	systemPrompt string
	provider     string
	builtinTools []string
	mcpServers   []string
	skills       []string
}

var agentIcons = []string{
	"bot", "user", "brain", "star", "telescope", "terminal",
	"command", "git-branch", "shield-check", "key-round", "globe",
	"layout-grid", "sliders-horizontal", "folder", "file-code", "archive",
}

func agentIcon(name string) string {
	if name == "" {
		return "bot"
	}
	return name
}

// agentsPage shows the agent presets as a card grid.
func (cw *ChatWindow) agentsPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, "Agent 管理").Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "新建 Agent", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAgentForm()
				cw.agentEditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for i := range cw.cfg.Agents {
					cw.agentCard(c, i)
				}
			})
		})
	})
	cw.agentEditor(c)
}

func (cw *ChatWindow) agentCard(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	a := cw.cfg.Agents[i]
	mgo.Column(c).Key("agent:"+a.Name).Gap(u*2).Padding(u*3).FillWidth().
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
		Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				mgo.Icon(c, icons.Must(agentIcon(a.Icon))).Size(20, 20).TextColor(k.AccentText)
				mgo.Text(c, a.Name).Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H3Size)).FontWeight(600).Grow(1).SingleLine()
			})
			if a.Description != "" {
				mgo.Text(c, a.Description).TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MaxLines(2)
			}
			mgo.Row(c).Gap(u).Wrap().Children(func() {
				mode := "对话"
				if a.Mode == "agent" {
					mode = "Agents"
				}
				agentTag(c, mode)
				if a.Provider != "" {
					agentTag(c, a.Provider)
				}
				if n := len(a.BuiltinTools) + len(a.MCPServers); n > 0 {
					agentTag(c, fmt.Sprintf("%d 个工具", n))
				}
			})
			mgo.Row(c).Gap(u).Children(func() {
				if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					cw.loadAgentForm(i)
					cw.agentEditorOpen = true
				}
				if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
					cw.deleteAgentAt(i)
				}
			})
		})
}

func agentTag(c *mgo.Context, label string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.AccentText).
		Padding(0, u*1.5).Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, k.Border).SingleLine()
}

// agentEditor is the create/edit dialog with builtin and MCP tool switches.
func (cw *ChatWindow) agentEditor(c *mgo.Context) {
	f := &cw.agentForm
	moverlay.Dialog(c, &cw.agentEditorOpen, moverlay.DialogOptions{
		Title: "Agent 配置",
		Width: 620,
		Actions: func() {
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if cw.saveAgent() {
					cw.agentEditorOpen = false
				}
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteAgent()
			}
		},
	}, func() {
		k := mcore.Tokens(c)
		small := mcore.FontSize(c, mtheme.CaptionSize)
		mgo.Column(c).Gap(mcore.Density(c).Unit() * 2).Children(func() {
			minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "Agent 名称"})
			minput.TextInput(c, &f.description, minput.TextInputOptions{Label: "描述", Placeholder: "一句话说明"})
			mgo.Text(c, "头像").FontSize(small).TextColor(k.TextMuted)
			mgo.Row(c).Gap(mcore.Density(c).Unit()).Wrap().Children(func() {
				for _, ic := range agentIcons {
					ic := ic
					b := mgo.ButtonBase(c).Size(36, 36).Shrink(0).Radius(mtheme.ControlRadius).Label(ic)
					if agentIcon(f.icon) == ic {
						b.Background(k.Selection).Border(mtheme.BorderWidth, k.AccentText)
					}
					b.Children(func() { mgo.Icon(c, icons.Must(ic)).Size(18, 18) })
					if b.Clicked() {
						f.icon = ic
					}
				}
			})
			provOpts := []minput.SelectOption[string]{{Value: "", Label: "（默认当前模型）"}}
			for _, p := range cw.cfg.Providers {
				if p.Enabled && p.Name != "" {
					provOpts = append(provOpts, minput.SelectOption[string]{Value: p.Name, Label: p.Name})
				}
			}
			minput.Select(c, &f.provider, provOpts, minput.SelectOptions{Label: "Provider"})
			minput.TextArea(c, &f.systemPrompt, minput.TextAreaOptions{
				Label: "系统提示词", Placeholder: "设定该 Agent 的角色与行为", MinHeight: 120,
			})
			mgo.Text(c, "内置工具").FontWeight(600).FontSize(small).TextColor(k.TextMuted)
			for _, t := range config.GetAvailableBuiltinTools() {
				on := containsStr(f.builtinTools, t)
				minput.Checkbox(c, &on, t, minput.CheckboxOptions{})
				f.builtinTools = setTool(f.builtinTools, t, on)
				if desc := config.GetBuiltinToolDescription(t); desc != "" {
					mgo.Text(c, desc).FontSize(small).TextColor(k.TextMuted)
				}
			}
			mgo.Text(c, "MCP 服务器").FontWeight(600).FontSize(small).TextColor(k.TextMuted)
			if len(cw.cfg.MCPServers) == 0 {
				mgo.Text(c, "尚未配置 MCP 服务器。").FontSize(small).TextColor(k.TextMuted)
			}
			for _, s := range cw.cfg.MCPServers {
				on := containsStr(f.mcpServers, s.Name)
				minput.Checkbox(c, &on, s.Name, minput.CheckboxOptions{})
				f.mcpServers = setTool(f.mcpServers, s.Name, on)
			}
			if !cw.skillsLoaded {
				cw.loadSkills()
			}
			if len(cw.skills) > 0 {
				mgo.Text(c, "Skills").FontWeight(600).FontSize(small).TextColor(k.TextMuted)
				for _, sk := range cw.skills {
					on := containsStr(f.skills, sk.Name)
					minput.Checkbox(c, &on, sk.Name, minput.CheckboxOptions{})
					f.skills = setTool(f.skills, sk.Name, on)
				}
			}
		})
	})
}

func (cw *ChatWindow) loadAgentForm(i int) {
	a := cw.cfg.Agents[i]
	cw.agentForm = agentForm{
		editing: i, name: a.Name, description: a.Description, icon: a.Icon,
		systemPrompt: a.SystemPrompt, provider: a.Provider,
		builtinTools: append([]string(nil), a.BuiltinTools...),
		mcpServers:   append([]string(nil), a.MCPServers...),
		skills:       append([]string(nil), a.Skills...),
	}
}

func (cw *ChatWindow) clearAgentForm() {
	cw.agentForm = agentForm{editing: -1}
}

func (cw *ChatWindow) saveAgent() bool {
	f := &cw.agentForm
	if strings.TrimSpace(f.name) == "" {
		cw.toast("Agent 名称不能为空", mcore.SeverityError)
		return false
	}
	if f.name == chatAgentName {
		cw.toast("「"+chatAgentName+"」是内置 Agent，请换一个名称", mcore.SeverityError)
		return false
	}
	mode := "chat"
	if len(f.builtinTools) > 0 || len(f.mcpServers) > 0 {
		mode = "agent"
	}
	def := config.AgentDef{
		Name: f.name, Description: f.description, Icon: f.icon, SystemPrompt: f.systemPrompt,
		Provider: f.provider, Mode: mode,
		BuiltinTools: f.builtinTools, MCPServers: f.mcpServers, Skills: f.skills,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.Agents) {
		cw.cfg.Agents[f.editing] = def
	} else {
		cw.cfg.Agents = append(cw.cfg.Agents, def)
		f.editing = len(cw.cfg.Agents) - 1
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存 Agent "+def.Name, mcore.SeveritySuccess)
	return true
}

func (cw *ChatWindow) deleteAgent() {
	f := &cw.agentForm
	if f.editing < 0 || f.editing >= len(cw.cfg.Agents) {
		return
	}
	cw.deleteAgentAt(f.editing)
	cw.agentEditorOpen = false
}

func (cw *ChatWindow) deleteAgentAt(i int) {
	if i < 0 || i >= len(cw.cfg.Agents) {
		return
	}
	name := cw.cfg.Agents[i].Name
	cw.askConfirm("删除 Agent", "确定删除「"+name+"」？", func() {
		cw.cfg.Agents = append(cw.cfg.Agents[:i], cw.cfg.Agents[i+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.clearAgentForm()
		if cw.currentAgent == name {
			cw.currentAgent = ""
			cw.applyAgent("")
		}
		cw.toast("Agent 已删除", mcore.SeveritySuccess)
	})
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
	}
}

// applyAgent applies an agent's provider, mode, prompt and tools.
func (cw *ChatWindow) applyAgent(name string) {
	cw.systemPrompt = ""
	if name == "" {
		cw.agentMode = false
		cw.cfg.UseReactAgent = false
		_ = config.SaveConfig(cw.cfg)
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
		if a.Provider != "" {
			if _, ok := cw.findProvider(a.Provider); ok {
				cw.provider = a.Provider
			}
		}
		if len(a.BuiltinTools) > 0 || len(a.MCPServers) > 0 {
			for _, m := range cw.toolMetas() {
				cw.toolOn[m.ID] = agentAllowsTool(a, m.ID)
			}
		}
		if len(a.Skills) > 0 && cw.cur != nil {
			ids := cw.skillIDsByName(a.Skills)
			if !sameStrings(ids, cw.cur.Skills) {
				cw.cur.Skills = ids
				_ = cw.convManager.SaveConversation(cw.cur)
			}
		}
		_ = config.SaveConfig(cw.cfg)
		cw.setupProvider(cw.provider)
		return
	}
}

func agentAllowsTool(a config.AgentDef, id string) bool {
	if strings.HasPrefix(id, "builtin:") {
		return containsStr(a.BuiltinTools, strings.TrimPrefix(id, "builtin:"))
	}
	if strings.HasPrefix(id, "mcp:") {
		parts := strings.SplitN(id, ":", 3)
		if len(parts) >= 2 {
			return containsStr(a.MCPServers, parts[1])
		}
	}
	return false
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func setTool(s []string, id string, on bool) []string {
	if on {
		if containsStr(s, id) {
			return s
		}
		return append(s, id)
	}
	out := s[:0]
	for _, x := range s {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}
