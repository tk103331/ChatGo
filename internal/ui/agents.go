package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdata "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/data"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

const chatAgentName = "Chat"

const chatAgentIcon = "message-square"

type agentForm struct {
	editing      int
	name         string
	description  string
	systemPrompt string
	workspace    string
	icon         string
	avatarSeed   string
	models       []string
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
			if len(cw.cfg.Agents) == 0 {
				cw.agentEmptyState(c)
				return
			}
			mgo.Grid(c).Columns(3).Gap(u * 3).Children(func() {
				for i := range cw.cfg.Agents {
					cw.agentCard(c, i)
				}
			})
		})
	})
}

// agentEmptyState shows a centered placeholder when no agents exist.
func (cw *ChatWindow) agentEmptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).
		Padding(u*8, 0).Children(func() {
		mgo.Icon(c, icons.Must("bot")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有 Agent").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "创建自定义 Agent，配置模型、工具和技能").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Button(c, "新建 Agent", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
			cw.clearAgentForm()
			cw.agentEditorOpen = true
		}
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
				cw.agentMark(c, a, 20)
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
	tools := cw.enabledBuiltinTools()
	servers := cw.enabledMCPServers()
	skillsEnabled := cw.enabledSkillList()
	names := make([]string, 0, len(skillsEnabled))
	for _, s := range skillsEnabled {
		names = append(names, s.Name)
	}
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
				mgo.Row(c).Gap(u * 3).AlignItems(mgo.Start).Children(func() {
					mgo.Column(c).Shrink(0).Gap(u).AlignItems(mgo.Center).Children(func() {
						cw.agentFormPreviewMark(c, f, 56)
						mgo.Row(c).Gap(u).AlignItems(mgo.Center).Children(func() {
							if mcore.IconAction(c, icons.Must("image"), "选择图片").Tooltip("选择图片").Clicked() {
								cw.pickIconFile(func(path string) {
									f.icon = path
									f.avatarSeed = ""
								})
							}
							if mcore.IconAction(c, icons.Must("refresh-cw"), "生成图标").Tooltip("生成图标").Clicked() {
								base := strings.TrimSpace(f.name)
								if base == "" {
									base = "Agent"
								}
								f.avatarSeed = fmt.Sprintf("%s#%d", base, time.Now().UnixNano())
								f.icon = ""
								cw.invalidate()
							}
							if f.icon != "" || f.avatarSeed != "" {
								if mcore.IconAction(c, icons.Must("x"), "清除图标").Tooltip("清除图标").Clicked() {
									f.icon = ""
									f.avatarSeed = ""
								}
							}
						})
					})
					mgo.Column(c).Grow(1).MinWidth(0).Gap(u * 2).Children(func() {
						minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "Agent 名称"})
						minput.TextInput(c, &f.description, minput.TextInputOptions{Label: "描述", Placeholder: "一句话说明"})
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
					agentAction(c, on, len(models), func(all bool) { f.models = toggleAll(models, all) },
						func() { cw.settings.tab = 0; cw.goConfigure(sectionSettings) })
					mgo.Row(c).Gap(u).Wrap().Children(func() {
						for _, ref := range models {
							ref := ref
							on := containsStr(f.models, ref)
							minput.Checkbox(c, &on, modelLabel(ref), minput.CheckboxOptions{})
							f.models = setTool(f.models, ref, on)
						}
					})
				})
				on = countSelected(f.builtinTools, tools)
				cw.agentGroup(c, &cw.agentSecTools, fmt.Sprintf("工具  %d/%d", on, len(tools)), func() {
					agentAction(c, on, len(tools), func(all bool) { f.builtinTools = toggleAll(tools, all) },
						func() { cw.settings.tab = 1; cw.goConfigure(sectionSettings) })
					agentSearch(c, &f.toolQuery, "搜索工具")
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
					agentAction(c, on, len(servers), func(all bool) { f.mcpServers = toggleAll(servers, all) },
						func() { cw.goConfigure(sectionMCP) })
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
					agentAction(c, on, len(names), func(all bool) { f.skills = toggleAll(names, all) },
						func() { cw.goConfigure(sectionSkill) })
					if len(names) == 0 {
						mgo.Text(c, "未发现 Skill。").FontSize(small).TextColor(k.TextMuted)
					}
					agentSearch(c, &f.skillQuery, "搜索 Skill")
					mgo.Column(c).Gap(u).Children(func() {
						for _, s := range filterSkills(skillsEnabled, f.skillQuery) {
							s := s
							on := containsStr(f.skills, s.Name)
							mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
								minput.Checkbox(c, &on, "", minput.CheckboxOptions{})
								body := mgo.ButtonBase(c).Grow(1).MinWidth(0).Justify(mgo.Start).Children(func() {
									mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
										mgo.Text(c, s.Name).SingleLine()
										if s.Description != "" {
											mgo.Text(c, s.Description).FontSize(small).TextColor(k.TextMuted).
												SingleLine().Ellipsis("…")
										}
									})
								})
								if body.Clicked() {
									on = !on
								}
							})
							f.skills = setTool(f.skills, s.Name, on)
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

// agentAction shows a group's count with a select-all/clear button and an
// optional shortcut to the global configuration page.
func agentAction(c *mgo.Context, on, total int, toggle func(all bool), configure func()) {
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
		if configure != nil {
			if minput.Button(c, "去配置", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("sliders-horizontal")}).Clicked() {
				configure()
			}
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

// agentMark draws an agent's mark: a configured icon file, else its generated
// avatar.
func (cw *ChatWindow) agentMark(c *mgo.Context, a config.AgentDef, size float32) {
	if a.Icon != "" && drawIconImage(c, cw.loadIconImage(a.Icon), size) {
		return
	}
	if bmp := cw.agentBitmap(agentSeed(a)); bmp != nil {
		mgo.Image(c, bmp).Size(size, size).Shrink(0)
	}
}

// agentMarkByName finds an agent by name and draws its mark.
func (cw *ChatWindow) agentMarkByName(c *mgo.Context, name string, size float32) {
	if a, ok := cw.agentByName(name); ok {
		cw.agentMark(c, a, size)
		return
	}
	if bmp := cw.agentBitmap(name); bmp != nil {
		mgo.Image(c, bmp).Size(size, size).Shrink(0)
	}
}

// agentFormPreviewMark draws the mark an agent form currently describes.
func (cw *ChatWindow) agentFormPreviewMark(c *mgo.Context, f *agentForm, size float32) {
	if f.icon != "" && drawIconImage(c, cw.loadIconImage(f.icon), size) {
		return
	}
	seed := strings.TrimSpace(f.avatarSeed)
	if seed == "" {
		seed = strings.TrimSpace(f.name)
		if seed == "" {
			seed = "Agent"
		}
	}
	if bmp := cw.agentBitmap(seed); bmp != nil {
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
func (cw *ChatWindow) enabledSkillList() []skill {
	var out []skill
	for _, s := range cw.skillList() {
		if cw.skillEnabled(s.ID) {
			out = append(out, s)
		}
	}
	return out
}

// skillList returns the loaded skills, scanning once when needed.
func (cw *ChatWindow) skillList() []skill {
	if !cw.skills.loaded {
		cw.loadSkills()
	}
	return cw.skills.list
}

// filterSkills returns the skills whose name or description contains query.
func filterSkills(list []skill, query string) []skill {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	var out []skill
	for _, s := range list {
		if strings.Contains(strings.ToLower(s.Name), q) || strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

func (cw *ChatWindow) loadAgentForm(i int) {
	a := cw.cfg.Agents[i]
	cw.agentForm = agentForm{
		editing: i, name: a.Name, description: a.Description,
		systemPrompt: a.SystemPrompt, workspace: a.Workspace,
		icon: a.Icon, avatarSeed: a.AvatarSeed,
		models:       append([]string(nil), a.Models...),
		builtinTools: append([]string(nil), a.BuiltinTools...),
		mcpServers:   append([]string(nil), a.MCPServers...),
		skills:       append([]string(nil), a.Skills...),
	}
}

func (cw *ChatWindow) clearAgentForm() {
	cw.agentForm = agentForm{editing: -1}
	cw.agentForm.models = cw.allModelRefs()
	cw.agentForm.builtinTools = cw.enabledBuiltinTools()
	cw.agentForm.mcpServers = cw.enabledMCPServers()
	for _, sk := range cw.enabledSkillList() {
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
		Workspace: f.workspace, Icon: f.icon, AvatarSeed: f.avatarSeed, Models: append([]string(nil), f.models...), Mode: mode,
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
