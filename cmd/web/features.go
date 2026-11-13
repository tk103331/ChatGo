package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// Capability names, mirroring internal/webui.
const (
	capAgents      = "agents"
	capTools       = "tools"
	capACP         = "acp"
	capAsk         = "ask"
	capPermission  = "permission"
	capCheckpoints = "checkpoints"
	capAttachments = "attachments"
	capSkills      = "skills"
	capRooms       = "rooms"
	capTasks       = "tasks"
)

func (a *app) cap(name string) bool { return a.caps[name] }

// loadFeatures fetches the assistants, projects and workspaces once, before
// a session asks for its tools and skills.
func (a *app) loadFeatures() {
	go func() {
		var agentsResp struct {
			Agents []agentInfo `json:"agents"`
		}
		if err := apiGet("/api/agents", &agentsResp); err == nil {
			a.win.Update(func() { a.agents = agentsResp.Agents })
		}
		var wsResp struct {
			Workspaces []string `json:"workspaces"`
		}
		if err := apiGet("/api/workspaces", &wsResp); err == nil {
			a.win.Update(func() { a.workspaces = wsResp.Workspaces })
		}
		var projResp struct {
			Projects []projectInfo `json:"projects"`
		}
		if err := apiGet("/api/projects", &projResp); err == nil {
			a.win.Update(func() { a.projects = projResp.Projects })
		}
	}()
}

// syncAgentFeatures loads the tool, skill and model lists for the current
// session's agent, once per agent change.
func (a *app) syncAgentFeatures() {
	if a.cap(capTools) && a.toolsFor != a.agentRef {
		a.loadTools(a.agentRef)
	}
	if a.cap(capSkills) && a.skillsFor != a.agentRef {
		a.loadSkills(a.agentRef)
	}
	if a.cap(capAgents) && a.modelsFor != a.agentRef {
		a.loadModels(a.agentRef)
	}
}

func (a *app) loadTools(agent string) {
	a.toolsFor = agent
	go func() {
		var resp struct {
			Tools []toolInfo `json:"tools"`
		}
		if err := apiGet("/api/tools?agent="+url.QueryEscape(agent), &resp); err != nil {
			a.win.Update(func() { a.status = "工具列表：" + err.Error() })
			return
		}
		a.win.Update(func() {
			if a.toolsFor == agent {
				a.tools = resp.Tools
			}
		})
	}()
}

func (a *app) loadSkills(agent string) {
	a.skillsFor = agent
	go func() {
		var resp struct {
			Skills []skillInfo `json:"skills"`
		}
		if err := apiGet("/api/skills?agent="+url.QueryEscape(agent), &resp); err != nil {
			a.win.Update(func() { a.status = "Skill 列表：" + err.Error() })
			return
		}
		a.win.Update(func() {
			if a.skillsFor == agent {
				a.skills = resp.Skills
			}
		})
	}()
}

// loadModels fetches the model list a new session may run with, per agent.
func (a *app) loadModels(agent string) {
	a.modelsFor = agent
	go func() {
		var resp struct {
			Models []modelInfo `json:"models"`
		}
		if err := apiGet("/api/models?agent="+url.QueryEscape(agent), &resp); err != nil {
			a.win.Update(func() { a.status = "模型列表：" + err.Error() })
			return
		}
		a.win.Update(func() {
			if a.modelsFor == agent {
				a.models = resp.Models
			}
		})
	}()
}

// patchSession sends a partial session update and returns the refreshed
// conversation.
func (a *app) patchSession(id string, body map[string]any) {
	if id == "" {
		return
	}
	go func() {
		if err := apiPatch(sessionPath(id), body, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.loadSession(id, false)
		a.refreshSessions()
	}()
}

// setAgent switches the session's assistant and reloads its tools and skills.
func (a *app) setAgent(ref string) {
	ref = strings.TrimSpace(ref)
	if ref == a.agentRef {
		return
	}
	a.agentRef = ref
	a.tools = nil
	a.skills = nil
	a.toolsFor, a.skillsFor = "\x00", "\x00"
	a.syncAgentFeatures()
	if id := a.getSelected(); id != "" {
		a.patchSession(id, map[string]any{"agent": ref})
	}
	a.win.Update(func() {})
}

// agentSelector lets a session switch assistant: the searchable list of every
// assistant. The standalone server has no
// agents, so the control disappears there.
func (a *app) agentSelector(c *ui.Context, t *ui.Theme) {
	if !a.cap(capAgents) {
		return
	}
	a.agentPickerButton(c, t, &a.agentRef, func(ref string) { a.setAgent(ref) },
		agentPickerOptions{Up: true, Variant: kit.Ghost})
}

// toolEnabled reports whether a session runs with a tool: its own override
// when it has one, the agent or global default otherwise.
func (a *app) toolEnabled(id string) bool {
	if cur := a.current(); cur != nil && cur.ToolsSet {
		for _, t := range cur.Tools {
			if t == id {
				return true
			}
		}
		return false
	}
	for _, t := range a.tools {
		if t.ID == id {
			return t.Enabled
		}
	}
	return false
}

// toggleTool flips one tool for the session and persists the whole set.
func (a *app) toggleTool(id string, on bool) {
	cur := a.current()
	if cur == nil {
		return
	}
	next := make([]string, 0, len(a.tools))
	for _, tool := range a.tools {
		enabled := a.toolEnabled(tool.ID)
		if tool.ID == id {
			enabled = on
		}
		if enabled {
			next = append(next, tool.ID)
		}
	}
	cur.Tools = next
	cur.ToolsSet = true
	a.patchSession(cur.ID, map[string]any{"tools": next})
	a.win.Update(func() {})
}

// pickerItem is one row of the composer's tool picker.
type pickerItem struct {
	id    string
	title string
	desc  string
}

// webToolPane is one tab of the 工具与技能 popover: a searchable list with a
// switch per row.
type webToolPane struct {
	name   string
	items  []pickerItem
	query  *string
	search string
	// empty explains why the list is bare and where to configure the pane.
	empty string
	on    func(string) bool
	set   func(string, bool)
}

// toolPanes builds the picker's tabs for the current session: built-in tools
// and MCP servers when the runtime offers tools, skills when it offers
// skills. The desktop's menu shows all three, including empty ones.
func (a *app) toolPanes() []webToolPane {
	var panes []webToolPane
	if a.cap(capTools) {
		var builtin, mcp []pickerItem
		for _, tool := range a.tools {
			it := pickerItem{id: tool.ID, title: tool.Name, desc: tool.Description}
			if tool.Kind == "mcp" || strings.HasPrefix(tool.ID, "mcp:") {
				mcp = append(mcp, it)
				continue
			}
			builtin = append(builtin, it)
		}
		panes = append(panes,
			webToolPane{
				name: "工具", items: builtin, query: &a.toolQuery, search: "搜索工具",
				empty: "没有可用的工具，请在桌面版设置中启用。",
				on:    a.toolEnabled, set: a.toggleTool,
			},
			webToolPane{
				name: "MCP", items: mcp, query: &a.mcpQuery, search: "搜索 MCP 服务器",
				empty: "没有可用的 MCP 服务器，请在桌面版设置 → MCP 中添加。",
				on:    a.toolEnabled, set: a.toggleTool,
			})
	}
	if a.cap(capSkills) && !strings.HasPrefix(a.agentRef, "acp:") {
		items := make([]pickerItem, 0, len(a.skills))
		for _, s := range a.skills {
			items = append(items, pickerItem{id: s.ID, title: s.Name, desc: s.Description})
		}
		panes = append(panes, webToolPane{
			name: "Skills", items: items, query: &a.skillQuery, search: "搜索 Skill",
			empty: "没有可用的 Skill，请在桌面版设置 → Skill 中添加。",
			on:    a.skillOn, set: a.toggleSkill,
		})
	}
	return panes
}

// toolsButton opens the composer's 工具与技能 popover: tools, MCP servers and
// skills on tabs with per-row switches, matching the desktop's menu.
func (a *app) toolsButton(c *ui.Context, t *ui.Theme) {
	// An ACP session speaks its own commands; the picker would be noise.
	if strings.HasPrefix(a.agentRef, "acp:") || (!a.cap(capTools) && !a.cap(capSkills)) {
		return
	}
	panes := a.toolPanes()
	a.clampToolTab(len(panes))
	u := uikit.Unit(c)
	b := kit.IconButton(c, icons.Must("sliders-horizontal"), "工具与技能",
		kit.IconButtonOptions{Variant: kit.Ghost})
	b.Tooltip("工具与技能")
	if b.Clicked() {
		a.toolsOpen = !a.toolsOpen
	}
	kit.Popover(c, b, &a.toolsOpen, kit.PopoverOptions{
		Title: "工具与技能", Width: 360, Up: true, MaxHeight: 420,
		Header: func() {
			if len(panes) > 1 {
				segments := make([]kit.Segment, len(panes))
				for i, p := range panes {
					segments[i] = kit.Segment{Label: toolTabLabel(p.name, p.onCount())}
				}
				kit.SegmentedControl(c, &a.toolTab, segments,
					kit.SegmentedControlOptions{Equal: true, Label: "工具类别"})
			}
			p := panes[a.toolTab]
			in := kit.TextInput(c, p.query, kit.TextInputOptions{
				Placeholder: p.search, Label: p.search,
			})
			in.Element.FillWidth()
			uikit.AutoFocus(c, in.Input)
		},
	}, func() {
		p := panes[a.toolTab]
		q := strings.ToLower(strings.TrimSpace(*p.query))
		var items []pickerItem
		for _, it := range p.items {
			if q != "" && !strings.Contains(strings.ToLower(it.title), q) &&
				!strings.Contains(strings.ToLower(it.desc), q) {
				continue
			}
			items = append(items, it)
		}
		caption := uikit.FontSize(c, mtheme.CaptionSize)
		switch {
		case len(p.items) == 0:
			ui.Text(c, p.empty).FontSize(caption).TextColor(t.TextMuted)
		case len(items) == 0:
			ui.Text(c, "没有匹配项").FontSize(caption).TextColor(t.TextMuted).
				Padding(u * 2)
		default:
			for _, it := range items {
				it := it
				on := p.on(it.id)
				if r := kit.SwitchRow(c, &on, it.title, kit.SwitchRowOptions{Hint: it.desc}); r.Changed() {
					p.set(it.id, on)
				}
			}
		}
	})
}

// clampToolTab keeps the tab within the pane list, so an agent switch that
// adds or removes the skills tab cannot panic the segmented control.
func (a *app) clampToolTab(n int) {
	if a.toolTab < 0 || a.toolTab >= n {
		a.toolTab = max(min(a.toolTab, n-1), 0)
	}
}

// onCount counts the pane's rows that are on, for the tab label.
func (p webToolPane) onCount() int {
	n := 0
	for _, it := range p.items {
		if p.on(it.id) {
			n++
		}
	}
	return n
}

func toolTabLabel(name string, on int) string {
	if on == 0 {
		return name
	}
	return fmt.Sprintf("%s (%d)", name, on)
}

// Option values that are not real paths, so they can never collide with one.
const (
	newProjectCustom = "\x00custom" // type a directory outside the projects
	newTreeCreate    = "\x00new"    // create a worktree on a new branch
)

// openNewSession opens the new-session form with the current choices filled in.
func (a *app) openNewSession() {
	a.newAgent = a.agentRef
	a.newProject = ""
	a.newCustomDir = ""
	a.newTree = ""
	a.newBranch = ""
	a.newNewBranch = false
	a.newModel = ""
	a.newDraft = ""
	a.newCreating = false
	a.syncAgentFeatures()
	a.newSessionOpen = true
	a.newSessionClosing = false
	a.win.Update(func() {})
}

// acpAgent looks up the ACP connection the form's agent names.
func (a *app) acpAgent() (agentInfo, bool) {
	for _, ag := range a.agents {
		if ag.Ref == a.newAgent && ag.Kind == "acp" {
			return ag, true
		}
	}
	return agentInfo{}, false
}

// newSessionDialog collects the 新会话 choices the desktop's create page
// asks for: agent, project, worktree or branch, model and the first message.
func (a *app) newSessionDialog(c *ui.Context, t *ui.Theme) {
	if !a.newSessionOpen && !a.newSessionClosing {
		return
	}
	wasOpen := a.newSessionOpen
	kit.Dialog(c, &a.newSessionOpen, kit.DialogOptions{
		Title: "新建会话",
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				a.newSessionOpen = false
			}
			if kit.Button(c, "创建", kit.ButtonOptions{Variant: kit.Primary, Disabled: a.newCreating}).Clicked() {
				a.createFromForm()
			}
		},
	}, func() {
		// Agent：可搜索的 Agent 列表
		if a.cap(capAgents) && len(a.agents) > 0 {
			kit.FieldColumn(c, "Agent", kit.FieldOptions{}, func() {
				a.agentPickerButton(c, t, &a.newAgent, func(ref string) {
					a.newAgent = ref
					// 换 Agent 后，旧的选项不再属于它。
					a.newACPConfig = nil
				}, agentPickerOptions{Variant: kit.Secondary, Wide: true})
			})
		}
		// 项目 + 工作树 + 分支
		a.newSessionProject(c, t)
		// 模型：所选 Agent 自己的模型列表
		if _, acp := a.acpAgent(); !acp {
			st := a.ensureModelsFor(a.newAgent)
			// 默认模型 rides the list as a row, so the choice can go back.
			models := append([]modelInfo{{Label: "默认模型"}}, st.models...)
			// 换 Agent 后，旧模型可能不在它的列表里：回到默认。
			if a.newModel != "" && !modelInList(st.models, a.newModel) {
				a.newModel = ""
			}
			kit.FieldColumn(c, "模型", kit.FieldOptions{}, func() {
				a.modelPickerButton(c, t, currentModelLabel(models, a.newModel),
					models, &a.newModel, func(ref string) { a.newModel = ref },
					modelPickerOptions{Variant: kit.Secondary, Wide: true})
			})
		}
		// ACP：模型与选项，来自运行时的预热
		if _, acp := a.acpAgent(); acp {
			a.newSessionACPOptions(c, t)
		}
		// 首条消息，Cmd+Enter 创建并发送
		ta := kit.TextArea(c, &a.newDraft, kit.TextAreaOptions{
			Placeholder: "首条消息（可选；Cmd+Enter 创建并发送）", Label: "首条消息",
			MinHeight: 80, Disabled: a.newCreating,
		})
		ta.Element.FillWidth()
		if ta.Submitted() {
			a.createFromForm()
		}
	})
	switch {
	case a.newSessionOpen:
		a.newSessionClosing = false
	case wasOpen:
		// The close was requested this frame; render the closed state once
		// more so the dialog restores the focus it trapped.
		a.newSessionClosing = true
	default:
		a.newSessionClosing = false
	}
}

// newSessionBody is the new-session form's payload: the agent, the
// workspace, the model and the ACP options picked for it.
func (a *app) newSessionBody() map[string]any {
	body := map[string]any{}
	if a.newAgent != "" {
		body["agent"] = a.newAgent
	}
	if ws := a.workspaceForCreate(); ws != "" {
		body["workspace"] = ws
	}
	if a.newModel != "" {
		provider, model := splitModelRef(a.newModel)
		body["provider"] = provider
		body["model"] = model
	}
	if len(a.newACPConfig) > 0 {
		// The ACP options picked in the dialog ride the new conversation, so
		// its session starts with them.
		body["acp_config"] = a.newACPConfig
	}
	return body
}

// newSessionACPOptions shows the ACP agent's model and the options its
// runtime offers, so the new session starts with the choices made here.
func (a *app) newSessionACPOptions(c *ui.Context, t *ui.Theme) {
	name := strings.TrimPrefix(a.newAgent, "acp:")
	st := a.ensureACPOptions(name)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	switch {
	case st.loading:
		ui.Text(c, "正在获取 "+name+" 的模型与选项…").FontSize(caption).TextColor(t.TextMuted)
		return
	case st.err != "":
		ui.Text(c, "预热失败："+st.err).FontSize(caption).TextColor(t.Danger)
		return
	}
	for _, opt := range st.options {
		switch opt.Kind {
		case "model":
			models := make([]modelInfo, 0, len(opt.Options))
			for _, v := range opt.Options {
				models = append(models, modelInfo{Ref: v.Value, Label: v.Name, Provider: name})
			}
			value := a.newACPConfig[opt.ID]
			if value == "" {
				value = opt.Current
			}
			cur := value
			kit.FieldColumn(c, "模型", kit.FieldOptions{}, func() {
				a.modelPickerButton(c, t, currentModelLabel(models, cur), models, &cur,
					func(ref string) { a.setNewACPConfig(opt.ID, ref) },
					modelPickerOptions{Variant: kit.Secondary, Wide: true})
			})
		case "boolean":
			opts := []kit.SelectOption[string]{
				{Value: "true", Label: "开"}, {Value: "false", Label: "关"},
			}
			a.newSessionACPChoice(c, t, opt, opts)
		default:
			opts := make([]kit.SelectOption[string], 0, len(opt.Options))
			for _, v := range opt.Options {
				opts = append(opts, kit.SelectOption[string]{Value: v.Value, Label: v.Name})
			}
			a.newSessionACPChoice(c, t, opt, opts)
		}
	}
}

// newSessionACPChoice draws one ACP option as a picker, keeping the choice
// until the session starts.
func (a *app) newSessionACPChoice(c *ui.Context, t *ui.Theme, opt acpConfigOptionInfo, opts []kit.SelectOption[string]) {
	value := a.newACPConfig[opt.ID]
	if value == "" {
		value = opt.Current
	}
	cur := value
	kit.FieldColumn(c, opt.Name, kit.FieldOptions{}, func() {
		sel := kit.Select(c, &cur, opts, kit.SelectOptions{
			Label: opt.Name, Placeholder: opt.Name,
		})
		sel.Element.FillWidth()
		if sel.Changed() {
			a.setNewACPConfig(opt.ID, cur)
		}
	})
}

// setNewACPConfig records an ACP option's choice for the new session.
func (a *app) setNewACPConfig(id, value string) {
	if a.newACPConfig == nil {
		a.newACPConfig = map[string]string{}
	}
	a.newACPConfig[id] = value
}

// newSessionProject draws the project, worktree and branch pickers, or the
// plain workspace list when the runtime offers no project grouping.
func (a *app) newSessionProject(c *ui.Context, t *ui.Theme) {
	if len(a.projects) == 0 {
		wsOpts := []kit.SelectOption[string]{{Value: "", Label: "默认工作区"}}
		for _, ws := range sortedWorkspaces(a.workspaces) {
			wsOpts = append(wsOpts, kit.SelectOption[string]{Value: ws, Label: ws})
		}
		kit.FieldColumn(c, "工作区", kit.FieldOptions{}, func() {
			sel := kit.Select(c, &a.newProject, wsOpts, kit.SelectOptions{
				Label: "工作区", Placeholder: "选择工作区",
			})
			sel.Element.FillWidth()
		})
		return
	}
	opts := []kit.SelectOption[string]{{Value: "", Label: "默认工作区"}}
	var chosen *projectInfo
	for i := range a.projects {
		p := a.projects[i]
		opts = append(opts, kit.SelectOption[string]{
			Value: p.Main, Label: pathBase(p.Main),
		})
		if p.Main == a.newProject {
			chosen = &a.projects[i]
		}
	}
	opts = append(opts, kit.SelectOption[string]{Value: newProjectCustom, Label: "其他目录…"})
	kit.FieldColumn(c, "项目", kit.FieldOptions{}, func() {
		sel := kit.Select(c, &a.newProject, opts, kit.SelectOptions{
			Label: "项目", Placeholder: "选择项目",
		})
		sel.Element.FillWidth()
		if sel.Changed() {
			// Picking another project drops the worktree, exactly like the
			// desktop: the tree below never belongs to a different project.
			a.newTree = ""
			a.newBranch = ""
			a.newNewBranch = false
		}
	})
	if a.newProject == newProjectCustom {
		kit.FieldColumn(c, "目录路径", kit.FieldOptions{}, func() {
			in := kit.TextInput(c, &a.newCustomDir, kit.TextInputOptions{
				Placeholder: "/path/to/project", Label: "目录路径",
			})
			in.Element.FillWidth()
			// A phone cannot type a host path from memory: walk the connected
			// host's file system instead.
			a.fsBrowserButton(c, t)
		})
		return
	}
	if chosen == nil || len(chosen.Worktrees) == 0 {
		return
	}
	// 工作树
	treeOpts := []kit.SelectOption[string]{{Value: newTreeCreate, Label: "新建 worktree…"}}
	for _, wt := range chosen.Worktrees {
		label := wt.Branch
		if wt.Path == chosen.Main {
			label = "[主] " + label
		}
		treeOpts = append(treeOpts, kit.SelectOption[string]{
			Value: wt.Path, Label: label,
		})
	}
	if a.newTree == "" {
		a.newTree = chosen.Main
	}
	if _, ok := treeIndex(treeOpts, a.newTree); !ok {
		// The tree list may have changed; fall back to the main tree rather
		// than holding a stale path.
		a.newTree = chosen.Main
	}
	cur := a.newTree
	kit.FieldColumn(c, "工作树", kit.FieldOptions{}, func() {
		tsel := kit.Select(c, &cur, treeOpts, kit.SelectOptions{
			Label: "工作树", Placeholder: "选择工作树",
		})
		tsel.Element.FillWidth()
	})
	a.newTree = cur
	// 分支 of a new worktree, searchable and able to create one, like the
	// desktop's picker.
	if a.newTree != newTreeCreate {
		return
	}
	branches := make([]kit.Branch, 0, len(chosen.Branches))
	for _, n := range chosen.Branches {
		branches = append(branches, kit.Branch{Name: n})
	}
	kit.FieldColumn(c, "分支", kit.FieldOptions{}, func() {
		bsel := kit.BranchSelector(c, &a.newBranch, branches, kit.BranchSelectorOptions{
			AllowCreate: true, Key: "web-create",
		})
		bsel.Element.FillWidth()
		if bsel.Changed() {
			a.newNewBranch = false
		}
		if name, ok := bsel.Created(); ok {
			a.newBranch = name
			a.newNewBranch = true
		}
	})
}

// treeIndex finds an option by value.
func treeIndex(opts []kit.SelectOption[string], value string) (int, bool) {
	for i, o := range opts {
		if o.Value == value {
			return i, true
		}
	}
	return -1, false
}

// pathBase names a project by its last path element.
func pathBase(path string) string {
	name := filepath.Base(path)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}

// createFromForm runs the form's chain: a new worktree is created first, then
// the session, then its first message.
func (a *app) createFromForm() {
	if !a.newSessionOpen || a.newCreating {
		return
	}
	if a.newTree == newTreeCreate {
		a.newCreating = true
		a.status = "创建工作树…"
		a.win.Update(func() {})
		go func() {
			var resp struct {
				Path string `json:"path"`
			}
			err := apiPost("/api/worktrees", map[string]any{
				"project":    a.newProject,
				"branch":     strings.TrimSpace(a.newBranch),
				"new_branch": a.newNewBranch,
			}, &resp)
			a.win.Update(func() {
				if err != nil {
					a.newCreating = false
					a.status = err.Error()
					return
				}
				a.newTree = resp.Path
				a.startSession()
			})
		}()
		return
	}
	a.startSession()
}

// startSession posts the new-session form and, when the form holds a first
// message, sends it right away like the desktop's composer does.
func (a *app) startSession() {
	a.newCreating = true
	a.status = "创建中…"
	a.win.Update(func() {})
	body := a.newSessionBody()
	first := strings.TrimSpace(a.newDraft)
	go func() {
		var resp sessionResponse
		err := apiPost("/api/sessions", body, &resp)
		a.win.Update(func() {
			if err != nil {
				a.newCreating = false
				a.status = err.Error()
				return
			}
			a.setSelected(resp.Session.ID)
			a.cur = &resp.Session
			a.agentRef = resp.Session.Agent
			a.modelRef = modelRefOf(resp.Session.Provider, resp.Session.Model)
			a.setRunning(false)
			a.status = ""
			a.menuFor = ""
			a.pending = nil
			a.attachments = nil
			a.skillsOn = nil
			a.sidebarOpen = false
			a.contextOpen = false
			a.msgList.ScrollToEnd()
			a.syncAgentFeatures()
			a.newSessionOpen = false
			a.newDraft = ""
			a.newTree = ""
			a.newBranch = ""
			a.newNewBranch = false
			a.newModel = ""
			a.newCreating = false
			if first != "" {
				a.draft = first
				a.send()
			}
		})
		a.refreshSessions()
	}()
}

// workspaceForCreate resolves the directory the form creates the session in:
// a picked worktree, the project's main tree, a typed directory, or empty for
// the default workspace.
func (a *app) workspaceForCreate() string {
	if a.newTree != "" && a.newTree != newTreeCreate {
		return a.newTree
	}
	if a.newProject == newProjectCustom {
		return strings.TrimSpace(a.newCustomDir)
	}
	return a.newProject
}

// skillByToken finds a skill the slash menu offered.
func (a *app) skillByToken(token string) (skillInfo, bool) {
	for _, s := range a.skills {
		if s.Token == token || s.Name == token {
			return s, true
		}
	}
	return skillInfo{}, false
}

// addSkill adds one skill chip to the next message.
func (a *app) addSkill(token string) {
	s, ok := a.skillByToken(token)
	if !ok {
		return
	}
	for _, id := range a.skillsOn {
		if id == s.ID {
			return
		}
	}
	a.skillsOn = append(a.skillsOn, s.ID)
}

// skillName labels a chip.
func (a *app) skillName(id string) string {
	for _, s := range a.skills {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

// skillOn reports whether the next message carries a skill.
func (a *app) skillOn(id string) bool {
	for _, v := range a.skillsOn {
		if v == id {
			return true
		}
	}
	return false
}

// toggleSkill adds or removes one skill for the next message, the same state
// the chips and the slash menu manage.
func (a *app) toggleSkill(id string, on bool) {
	if on {
		for _, v := range a.skillsOn {
			if v == id {
				return
			}
		}
		a.skillsOn = append(a.skillsOn, id)
	} else {
		for i, v := range a.skillsOn {
			if v != id {
				continue
			}
			a.skillsOn = append(a.skillsOn[:i], a.skillsOn[i+1:]...)
			break
		}
	}
	a.win.Update(func() {})
}

// sortedWorkspaces returns the workspace list without duplicates.
func sortedWorkspaces(list []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list))
	for _, w := range list {
		if w == "" || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}
