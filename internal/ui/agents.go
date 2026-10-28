package ui

import (
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
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
	modelQuery   string
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
			mgo.Text(c, "Agent 管理").
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if kit.Button(c, "新建 Agent", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAgentForm()
				cw.agentEditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(cw.cfg.Agents) == 0 {
				cw.agentEmptyState(c)
				return
			}
			cardRows(c, 2, len(cw.cfg.Agents), func(i int) { cw.agentCard(c, i) })
		})
	})
}

// agentEmptyState shows a centered placeholder when no agents exist.
func (cw *ChatWindow) agentEmptyState(c *mgo.Context) {
	emptyState(c, emptyStateOptions{
		Icon:  "bot",
		Title: "还没有 Agent",
		Hint:  "创建自定义 Agent，配置模型、工具和技能",
		Actions: func() {
			if kit.Button(c, "新建 Agent", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAgentForm()
				cw.agentEditorOpen = true
			}
		},
	})
}

func (cw *ChatWindow) agentCard(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	a := cw.cfg.Agents[i]
	cardShell(c, "agent:"+a.Name, func() {
		cardTitleRow(c, func() {
			cw.agentMark(c, a, 20)
			cardTitle(c, a.Name)
		})
		if a.Description != "" {
			mgo.Text(c, a.Description).TextColor(k.TextMuted).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).MinWidth(0).MaxLines(2)
		}
		cardMetaRow(c, func() {
			mode := "对话"
			if a.Mode == "agent" {
				mode = "Agents"
			}
			agentTag(c, mode)
			if len(a.Models) > 0 {
				agentTag(c, cw.refLabel(a.Models[0]))
			}
			if n := len(a.BuiltinTools) + len(a.MCPServers); n > 0 {
				agentTag(c, fmt.Sprintf("%d 个工具", n))
			}
		})
		cardActionRow(c, func() {
			if kit.Button(c, "编辑", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.loadAgentForm(i)
				cw.agentEditorOpen = true
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteAgentAt(i)
			}
		})
	})
}

func agentTag(c *mgo.Context, label string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.Accent).
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
			if kit.Button(c, "返回", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.agentEditorOpen = false
			}
			mgo.Text(c, title).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				if cw.saveAgent() {
					cw.agentEditorOpen = false
				}
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger, Disabled: f.editing < 0}).Clicked() {
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
						kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "Agent 名称"})
						kit.TextInput(c, &f.description, kit.TextInputOptions{Label: "描述", Placeholder: "一句话说明"})
					})
				})
				kit.TextArea(c, &f.systemPrompt, kit.TextAreaOptions{
					Label: "系统提示词", Placeholder: "设定该 Agent 的角色与行为", MinHeight: 96,
				})
				mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
					in := kit.TextInput(c, &f.workspace, kit.TextInputOptions{Label: "工作目录", Placeholder: "留空使用当前项目"})
					in.Element.Grow(1)
					if kit.Button(c, "选择", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
						cw.pickDirectory("选择工作目录", func(path string) { f.workspace = path })
					}
				})
				on := countSelected(f.models, models)
				cw.agentGroup(c, &cw.agentSecModels, fmt.Sprintf("可用模型  %d/%d", on, len(models)), func() {
					agentConfigureLink(c, func() { cw.settings.tab = 0; cw.goConfigure(sectionSettings) })
					modelItems := make([]selectItem, 0, len(models))
					for _, ref := range models {
						desc, tags := cw.refDetail(ref)
						modelItems = append(modelItems, selectItem{
							ID: ref, Title: cw.refLabel(ref), Desc: desc, Tags: tags,
						})
					}
					selectList(c, &f.modelQuery, "搜索模型", modelItems,
						func(id string) bool { return containsStr(f.models, id) },
						func(id string, on bool) { f.models = setTool(f.models, id, on) })
				})
				on = countSelected(f.builtinTools, tools)
				cw.agentGroup(c, &cw.agentSecTools, fmt.Sprintf("工具  %d/%d", on, len(tools)), func() {
					toolItems := make([]selectItem, 0, len(tools))
					for _, t := range tools {
						toolItems = append(toolItems, selectItem{
							ID: t, Title: t, Desc: config.GetBuiltinToolDescription(t),
						})
					}
					agentConfigureLink(c, func() { cw.settings.tab = 1; cw.goConfigure(sectionSettings) })
					selectList(c, &f.toolQuery, "搜索工具", toolItems,
						func(id string) bool { return containsStr(f.builtinTools, id) },
						func(id string, on bool) { f.builtinTools = setTool(f.builtinTools, id, on) })
				})
				on = countSelected(f.mcpServers, servers)
				cw.agentGroup(c, &cw.agentSecMCP, fmt.Sprintf("MCP 服务器  %d/%d", on, len(servers)), func() {
					agentConfigureLink(c, func() { cw.goConfigure(sectionMCP) })
					if len(servers) == 0 {
						mgo.Text(c, "尚未配置 MCP 服务器。").FontSize(small).TextColor(k.TextMuted)
						return
					}
					mcpItems := make([]selectItem, 0, len(servers))
					for _, srv := range servers {
						mcpItems = append(mcpItems, selectItem{
							ID: srv, Title: srv, Desc: cw.mcpServerDesc(srv),
						})
					}
					selectList(c, &f.mcpQuery, "搜索 MCP 服务器", mcpItems,
						func(id string) bool { return containsStr(f.mcpServers, id) },
						func(id string, on bool) { f.mcpServers = setTool(f.mcpServers, id, on) })
				})
				on = countSelected(f.skills, names)
				cw.agentGroup(c, &cw.agentSecSkills, fmt.Sprintf("Skills  %d/%d", on, len(names)), func() {
					agentConfigureLink(c, func() { cw.goConfigure(sectionSkill) })
					if len(names) == 0 {
						mgo.Text(c, "未发现 Skill。").FontSize(small).TextColor(k.TextMuted)
						return
					}
					skillItems := make([]selectItem, 0, len(skillsEnabled))
					for _, sk := range skillsEnabled {
						skillItems = append(skillItems, selectItem{
							ID: sk.Name, Title: sk.Name, Desc: sk.Description,
						})
					}
					selectList(c, &f.skillQuery, "搜索 Skill", skillItems,
						func(id string) bool { return containsStr(f.skills, id) },
						func(id string, on bool) { f.skills = setTool(f.skills, id, on) })
				})
			})
		})
	})
}

// agentGroup renders a collapsible capability section.
func (cw *ChatWindow) agentGroup(c *mgo.Context, open *bool, title string, body func()) {
	kit.Collapsible(c, open, kit.CollapsibleOptions{Title: title}, body)
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
func agentSearch(c *mgo.Context, query *string, placeholder string) mgo.Element {
	in := kit.SearchInput(c, query, kit.SearchInputOptions{Placeholder: placeholder, Label: placeholder})
	return in.Element.FillWidth()
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
// applyAgent applies an agent's provider, mode, prompt and tools as an
// explicit choice, moving the window to the agent's own workspace.
func (cw *ChatWindow) applyAgent(name string) {
	cw.applyAgentState(name, true)
}

// restoreAgent applies the agent a conversation was saved with, leaving the
// workspace where the conversation puts it: an agent's saved workspace must
// not drag an opened session into another project's directory, where its tab
// would sit beside that project's own.
func (cw *ChatWindow) restoreAgent(name string) {
	cw.applyAgentState(name, false)
}

// applyAgentState applies an agent, moving the workspace only when move is
// set. The built-in Chat restores its saved selection; a custom agent starts
// with every whitelisted model, tool, MCP server and skill enabled.
func (cw *ChatWindow) applyAgentState(name string, move bool) {
	cw.currentAgent = name
	cw.systemPrompt = ""
	if name == "" {
		cw.agentMode = false
		cw.cfg.UseReactAgent = false
		cw.loadChatSelection()
		cw.provider = cw.cfg.Chat.Provider
		cw.model = cw.cfg.Chat.Model
		if move && cw.cfg.Chat.Workspace != "" {
			cw.pointAtWorkspace(cw.cfg.Chat.Workspace)
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
		if move && a.Workspace != "" {
			cw.pointAtWorkspace(a.Workspace)
		}
		if refs := cw.agentModelRefs(); len(refs) > 0 {
			cw.setModelRef(refs[0])
		}
		cw.setupProvider(cw.provider)
		return
	}
	cw.currentAgent = ""
	cw.applyAgentState("", move)
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

// agentConfigureLink offers the shortcut to the page that defines what a
// capability group can contain.
func agentConfigureLink(c *mgo.Context, configure func()) {
	if kit.Button(c, "去配置", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("sliders-horizontal")}).Clicked() {
		configure()
	}
}
