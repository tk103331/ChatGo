package main

import (
	"fmt"
	"net/url"
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

// loadFeatures fetches the assistants and workspaces once, before a session
// asks for its tools and skills.
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
	}()
}

// syncAgentFeatures loads the tool and skill lists for the current session's
// agent, once per agent change.
func (a *app) syncAgentFeatures() {
	if a.cap(capTools) && a.toolsFor != a.agentRef {
		a.loadTools(a.agentRef)
	}
	if a.cap(capSkills) && a.skillsFor != a.agentRef {
		a.loadSkills(a.agentRef)
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

// agentSelector lets a session switch assistant. The standalone server has no
// agents, so the control disappears there.
func (a *app) agentSelector(c *ui.Context, _ *ui.Theme) {
	if !a.cap(capAgents) {
		return
	}
	opts := []kit.SelectOption[string]{{Value: "", Label: "Chat"}}
	for _, ag := range a.agents {
		label := ag.Name
		switch ag.Kind {
		case "acp":
			label += " · ACP"
		case "agent":
			label += " · Agent"
		}
		opts = append(opts, kit.SelectOption[string]{Value: ag.Ref, Label: label})
	}
	cur := a.agentRef
	r := kit.Select(c, &cur, opts, kit.SelectOptions{
		Label: "Agent", Placeholder: "选择 Agent", MinWidth: 180,
	})
	r.Element.MaxWidth(240).MinWidth(0)
	if r.Changed() {
		a.setAgent(cur)
	}
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

// toolsButton opens the per-session tool switches.
func (a *app) toolsButton(c *ui.Context, t *ui.Theme) {
	if !a.cap(capTools) || len(a.tools) == 0 || strings.HasPrefix(a.agentRef, "acp:") {
		return
	}
	n := 0
	for _, tool := range a.tools {
		if a.toolEnabled(tool.ID) {
			n++
		}
	}
	b := kit.IconButton(c, icons.Must("wrench"), fmt.Sprintf("工具（%d）", n),
		kit.IconButtonOptions{Variant: kit.Ghost})
	b.Tooltip(fmt.Sprintf("工具（%d）", n))
	if b.Clicked() {
		a.toolsOpen = !a.toolsOpen
	}
	kit.Popover(c, b, &a.toolsOpen, kit.PopoverOptions{
		Title: "工具", Width: 320, Up: true,
	}, func() {
		if len(a.tools) == 0 {
			ui.Text(c, "没有可配置的工具").FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
				TextColor(t.TextMuted).Padding(uikit.Unit(c) * 2)
			return
		}
		for _, tool := range a.tools {
			on := a.toolEnabled(tool.ID)
			hint := "内置工具"
			if tool.Kind == "mcp" {
				hint = "MCP 服务"
			}
			r := kit.SwitchRow(c, &on, tool.Name, kit.SwitchRowOptions{Hint: hint})
			if r.Changed() {
				a.toggleTool(tool.ID, on)
			}
		}
	})
}

// openNewSession opens the new-session form with the current choices filled in.
func (a *app) openNewSession() {
	a.newTitle = ""
	a.newAgent = a.agentRef
	a.newWorkspace = ""
	if cur := a.current(); cur != nil {
		a.newWorkspace = cur.Workspace
	}
	if a.newWorkspace == "" && len(a.workspaces) > 0 {
		a.newWorkspace = a.workspaces[0]
	}
	a.newSessionOpen = true
	a.newSessionClosing = false
	a.win.Update(func() {})
}

// newSessionDialog collects the title, agent and workspace of a new session.
func (a *app) newSessionDialog(c *ui.Context) {
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
			if kit.Button(c, "创建", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				a.createFromForm()
			}
		},
	}, func() {
		in := kit.TextInput(c, &a.newTitle, kit.TextInputOptions{
			Placeholder: "会话标题（可留空）", Label: "会话标题",
		})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)
		if in.Submitted() {
			a.createFromForm()
		}
		if a.cap(capAgents) && len(a.agents) > 0 {
			opts := []kit.SelectOption[string]{{Value: "", Label: "Chat"}}
			for _, ag := range a.agents {
				label := ag.Name
				if ag.Kind == "acp" {
					label += " · ACP"
				}
				opts = append(opts, kit.SelectOption[string]{Value: ag.Ref, Label: label})
			}
			sel := kit.Select(c, &a.newAgent, opts, kit.SelectOptions{
				Label: "Agent", Placeholder: "选择 Agent",
			})
			sel.Element.FillWidth()
		}
		wsOpts := []kit.SelectOption[string]{{Value: "", Label: "默认工作区"}}
		for _, ws := range sortedWorkspaces(a.workspaces) {
			wsOpts = append(wsOpts, kit.SelectOption[string]{Value: ws, Label: ws})
		}
		sel := kit.Select(c, &a.newWorkspace, wsOpts, kit.SelectOptions{
			Label: "工作区", Placeholder: "选择工作区",
		})
		sel.Element.FillWidth()
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

// createFromForm posts the new-session form.
func (a *app) createFromForm() {
	if !a.newSessionOpen {
		return
	}
	a.newSessionOpen = false
	provider, model := a.selectedModel()
	body := map[string]any{}
	if title := strings.TrimSpace(a.newTitle); title != "" {
		body["title"] = title
	}
	if a.newAgent != "" {
		body["agent"] = a.newAgent
	}
	if a.newWorkspace != "" {
		body["workspace"] = a.newWorkspace
	}
	if provider != "" {
		body["provider"] = provider
		body["model"] = model
	}
	a.status = "创建中…"
	go func() {
		var resp sessionResponse
		if err := apiPost("/api/sessions", body, &resp); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.setSelected(resp.Session.ID)
		a.win.Update(func() {
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
		})
		a.refreshSessions()
	}()
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
