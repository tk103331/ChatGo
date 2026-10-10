package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	mdata "github.com/ZacharyZhang-NY/MujicaUI/data"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "github.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "github.com/ZacharyZhang-NY/MujicaUI/theme"
)

const chatAgentName = "Chat"

const chatAgentIcon = "message-square"

type agentForm struct {
	editing      int
	name         string
	description  string
	systemPrompt string
	workspace    string
	avatarSeed   string
	models       []string
	newModel     string
	builtinTools []string
	toolQuery    string
	mcpServers   []string
	mcpQuery     string
	skills       []string
	skillQuery   string
}

// agentsPage shows the agent presets as a card grid, or the editor page.
func (cw *ChatWindow) agentsPage(c *mgo.Context) {
	if cw.agentEditorOpen {
		cw.agentEditor(c)
		return
	}
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
}

func (cw *ChatWindow) agentCard(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	a := cw.cfg.Agents[i]
	mgo.Column(c).Key("agent:"+a.Name).Gap(u*2).Padding(u*3).FillWidth().
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).
		Children(func() {
			mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
				cw.drawAgentAvatar(c, agentSeed(a), 20)
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
				if len(a.Models) > 0 {
					agentTag(c, modelLabel(a.Models[0]))
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

// agentEditor is the create/edit page, with collapsible capability sections.
func (cw *ChatWindow) agentEditor(c *mgo.Context) {
	f := &cw.agentForm
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	models := cw.modelOptions(f.models)
	tools := config.GetAvailableBuiltinTools()
	servers := cw.mcpServerNames()
	names := cw.skillNames()
	title := "新建 Agent"
	if f.editing >= 0 {
		title = "编辑 Agent"
	}
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if minput.Button(c, "返回", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.agentEditorOpen = false
			}
			mgo.Text(c, title).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if cw.saveAgent() {
					cw.agentEditorOpen = false
				}
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger, Disabled: f.editing < 0}).Clicked() {
				cw.deleteAgent()
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).Gap(u * 3).Children(func() {
				mgo.Row(c).Gap(u * 2).AlignItems(mgo.Start).Children(func() {
					mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
						minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "Agent 名称"})
						minput.TextInput(c, &f.description, minput.TextInputOptions{Label: "描述", Placeholder: "一句话说明"})
					})
					mgo.Column(c).Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
						seed := f.avatarSeed
						if seed == "" {
							seed = f.name
							if seed == "" {
								seed = "Agent"
							}
						}
						cw.drawAgentAvatar(c, seed, 48)
						if minput.Button(c, "生成头像", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
							base := f.name
							if base == "" {
								base = "Agent"
							}
							f.avatarSeed = fmt.Sprintf("%s#%d", base, time.Now().UnixNano())
						}
						mgo.Text(c, "点击生成头像").FontSize(small).TextColor(k.TextMuted)
					})
				})
				minput.TextArea(c, &f.systemPrompt, minput.TextAreaOptions{
					Label: "系统提示词", Placeholder: "设定该 Agent 的角色与行为", MinHeight: 96,
				})
				mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
					in := minput.TextInput(c, &f.workspace, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前项目"})
					in.Element.Grow(1)
					if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
						cw.pickDirectory("选择工作目录", func(path string) { f.workspace = path })
					}
				})
				on := countSelected(f.models, models)
				cw.agentGroup(c, &cw.agentSecModels, fmt.Sprintf("可用模型  %d/%d", on, len(models)), func() {
					agentAction(c, on, len(models), func(all bool) { f.models = toggleAll(models, all) })
					mgo.Row(c).Gap(u).Wrap().Children(func() {
						for _, ref := range models {
							ref := ref
							on := containsStr(f.models, ref)
							minput.Checkbox(c, &on, modelLabel(ref), minput.CheckboxOptions{})
							f.models = setTool(f.models, ref, on)
						}
					})
					mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
						in := minput.TextInput(c, &f.newModel, minput.TextInputOptions{Placeholder: "手动添加模型名"})
						in.Element.Grow(1)
						if minput.Button(c, "添加", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
							if name := strings.TrimSpace(f.newModel); name != "" {
								ref := modelRef("", name)
								if !containsStr(f.models, ref) {
									f.models = append(f.models, ref)
								}
								f.newModel = ""
							}
						}
					})
				})
				on = countSelected(f.builtinTools, tools)
				cw.agentGroup(c, &cw.agentSecTools, fmt.Sprintf("内置工具  %d/%d", on, len(tools)), func() {
					agentAction(c, on, len(tools), func(all bool) { f.builtinTools = toggleAll(tools, all) })
					agentSearch(c, &f.toolQuery, "搜索内置工具")
					mgo.Grid(c).Columns(2).Gap(u).Children(func() {
						for _, t := range filterStrings(tools, f.toolQuery) {
							t := t
							mgo.Column(c).Gap(0).MinWidth(0).Children(func() {
								on := containsStr(f.builtinTools, t)
								minput.Checkbox(c, &on, t, minput.CheckboxOptions{})
								f.builtinTools = setTool(f.builtinTools, t, on)
								mgo.Text(c, config.GetBuiltinToolDescription(t)).FontSize(small).TextColor(k.TextMuted).
									MaxLines(1).Ellipsis("…")
							})
						}
					})
				})
				on = countSelected(f.mcpServers, servers)
				cw.agentGroup(c, &cw.agentSecMCP, fmt.Sprintf("MCP 服务器  %d/%d", on, len(servers)), func() {
					agentAction(c, on, len(servers), func(all bool) { f.mcpServers = toggleAll(servers, all) })
					if len(servers) == 0 {
						mgo.Text(c, "尚未配置 MCP 服务器。").FontSize(small).TextColor(k.TextMuted)
					}
					agentSearch(c, &f.mcpQuery, "搜索 MCP 服务器")
					mgo.Grid(c).Columns(2).Gap(u).Children(func() {
						for _, s := range filterStrings(servers, f.mcpQuery) {
							on := containsStr(f.mcpServers, s)
							minput.Checkbox(c, &on, s, minput.CheckboxOptions{})
							f.mcpServers = setTool(f.mcpServers, s, on)
						}
					})
				})
				on = countSelected(f.skills, names)
				cw.agentGroup(c, &cw.agentSecSkills, fmt.Sprintf("Skills  %d/%d", on, len(names)), func() {
					agentAction(c, on, len(names), func(all bool) { f.skills = toggleAll(names, all) })
					if len(names) == 0 {
						mgo.Text(c, "未发现 Skill。").FontSize(small).TextColor(k.TextMuted)
					}
					agentSearch(c, &f.skillQuery, "搜索 Skill")
					mgo.Grid(c).Columns(2).Gap(u).Children(func() {
						for _, n := range filterStrings(names, f.skillQuery) {
							on := containsStr(f.skills, n)
							minput.Checkbox(c, &on, n, minput.CheckboxOptions{})
							f.skills = setTool(f.skills, n, on)
						}
					})
				})
			})
		})
	})
}

// agentGroup renders a collapsible capability section.
func (cw *ChatWindow) agentGroup(c *mgo.Context, open *bool, title string, body func()) {
	mdata.Collapsible(c, open, mdata.CollapsibleOptions{Title: title}, body)
}

// agentAction shows a group's count and a select-all/clear button.
func agentAction(c *mgo.Context, on, total int, toggle func(all bool)) {
	k := mcore.Tokens(c)
	small := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
		mgo.Text(c, "已选 "+strconv.Itoa(on)).FontSize(small).TextColor(k.TextMuted).Grow(1)
		label := "全选"
		if total > 0 && on == total {
			label = "清空"
		}
		if minput.Button(c, label, minput.ButtonOptions{Variant: minput.Ghost}).Clicked() {
			toggle(on != total)
		}
	})
}

// agentSeed is the seed an agent's generated avatar uses.
func agentSeed(a config.AgentDef) string {
	if a.AvatarSeed != "" {
		return a.AvatarSeed
	}
	return a.Name
}

// agentSeedByName resolves an agent's avatar seed by name.
func (cw *ChatWindow) agentSeedByName(name string) string {
	for _, a := range cw.cfg.Agents {
		if a.Name == name {
			return agentSeed(a)
		}
	}
	return name
}

// countSelected counts how many of want are in have.
func countSelected(have, want []string) int {
	n := 0
	for _, w := range want {
		if containsStr(have, w) {
			n++
		}
	}
	return n
}

// agentSearch renders a compact search field filtering a list.
func agentSearch(c *mgo.Context, query *string, placeholder string) *mgo.Element {
	in := minput.SearchInput(c, query, minput.SearchInputOptions{Placeholder: placeholder, Label: placeholder})
	return in.Element.FillWidth()
}

// filterStrings returns the items containing query, case-insensitively.
func filterStrings(list []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	var out []string
	for _, s := range list {
		if strings.Contains(strings.ToLower(s), q) {
			out = append(out, s)
		}
	}
	return out
}

// toggleAll returns every item in want, or none.
func toggleAll(want []string, all bool) []string {
	if all {
		return append([]string(nil), want...)
	}
	return nil
}

// mcpServerNames lists the configured MCP server names.
func (cw *ChatWindow) mcpServerNames() []string {
	out := make([]string, 0, len(cw.cfg.MCPServers))
	for _, s := range cw.cfg.MCPServers {
		out = append(out, s.Name)
	}
	return out
}

// skillNames lists the loaded skill names.
func (cw *ChatWindow) skillNames() []string {
	if !cw.skillsLoaded {
		cw.loadSkills()
	}
	out := make([]string, 0, len(cw.skills))
	for _, s := range cw.skills {
		out = append(out, s.Name)
	}
	return out
}

func (cw *ChatWindow) loadAgentForm(i int) {
	a := cw.cfg.Agents[i]
	cw.agentForm = agentForm{
		editing: i, name: a.Name, description: a.Description,
		systemPrompt: a.SystemPrompt, workspace: a.Workspace, avatarSeed: agentSeed(a),
		models:       append([]string(nil), a.Models...),
		builtinTools: append([]string(nil), a.BuiltinTools...),
		mcpServers:   append([]string(nil), a.MCPServers...),
		skills:       append([]string(nil), a.Skills...),
	}
}

func (cw *ChatWindow) clearAgentForm() {
	cw.agentForm = agentForm{editing: -1}
	cw.agentForm.models = cw.allModelRefs()
	for _, t := range config.GetAvailableBuiltinTools() {
		cw.agentForm.builtinTools = append(cw.agentForm.builtinTools, t)
	}
	for _, s := range cw.cfg.MCPServers {
		cw.agentForm.mcpServers = append(cw.agentForm.mcpServers, s.Name)
	}
	if !cw.skillsLoaded {
		cw.loadSkills()
	}
	for _, sk := range cw.skills {
		cw.agentForm.skills = append(cw.agentForm.skills, sk.Name)
	}
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
		Name: f.name, Description: f.description, SystemPrompt: f.systemPrompt,
		Workspace: f.workspace, AvatarSeed: f.avatarSeed, Models: append([]string(nil), f.models...), Mode: mode,
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
