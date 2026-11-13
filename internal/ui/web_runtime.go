package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// WebRuntime is the desktop agent runtime as the embedded web server uses it.
// The browser draws the conversation; the turn itself runs here, with the same
// tools, MCP servers, custom agents and ACP connections as the app.
func (cw *ChatWindow) WebRuntime() webui.Runner { return webRuntime{cw: cw} }

type webRuntime struct{ cw *ChatWindow }

func (r webRuntime) Capabilities() map[string]bool {
	return map[string]bool{
		webui.CapAgents:      true,
		webui.CapTools:       true,
		webui.CapACP:         true,
		webui.CapAsk:         true,
		webui.CapPermission:  true,
		webui.CapCheckpoints: true,
		webui.CapAttachments: true,
		webui.CapSkills:      true,
		webui.CapRooms:       true,
		webui.CapTasks:       true,
	}
}

func (r webRuntime) Agents() []webui.AgentInfo {
	cw := r.cw
	return onMain(cw, func() []webui.AgentInfo {
		out := make([]webui.AgentInfo, 0, len(cw.cfg.Agents)+len(cw.cfg.ACPAgents))
		for _, a := range cw.cfg.Agents {
			info := webui.AgentInfo{
				Ref: "agent:" + a.Name, Name: a.Name, Kind: "agent",
				Description: a.Description, Icon: a.Icon,
				Models: a.Models, Workspace: a.Workspace, Skills: a.Skills,
			}
			for _, t := range a.BuiltinTools {
				info.Tools = append(info.Tools, "builtin:"+t)
			}
			for _, s := range a.MCPServers {
				info.Tools = append(info.Tools, "mcp:"+s)
			}
			out = append(out, info)
		}
		for _, a := range cw.cfg.ACPAgents {
			if a.Disabled {
				continue
			}
			// The command is what tells two ACP connections apart, the way
			// the desktop's 新会话 page prints it under the picker.
			cmd := strings.TrimSpace(a.Command + " " + strings.Join(a.Args, " "))
			out = append(out, webui.AgentInfo{
				Ref: "acp:" + a.Name, Name: a.Name, Kind: "acp",
				Disabled:    cw.acpModelError(a.Name) != "",
				Description: cmd, Icon: a.Icon, Workspace: a.Cwd,
			})
		}
		return out
	})
}

func (r webRuntime) Tools(agent string) []webui.ToolInfo {
	cw := r.cw
	return onMain(cw, func() []webui.ToolInfo {
		_, _, _, _, defaults := cw.taskAgentSpec(agent)
		on := make(map[string]bool, len(defaults))
		for _, id := range defaults {
			on[id] = true
		}
		out := make([]webui.ToolInfo, 0, len(cw.cfg.BuiltinTools)+len(cw.cfg.MCPServers))
		for _, t := range cw.cfg.BuiltinTools {
			id := "builtin:" + t.Name
			out = append(out, webui.ToolInfo{
				ID: id, Name: t.Name,
				Description: config.GetBuiltinToolDescription(t.Type),
				Kind:        "builtin", Enabled: on[id],
			})
		}
		for _, s := range cw.cfg.MCPServers {
			id := "mcp:" + s.Name
			out = append(out, webui.ToolInfo{
				ID: id, Name: s.Name, Description: cw.mcpServerDesc(s.Name),
				Kind: "mcp", Enabled: on[id],
			})
		}
		return out
	})
}

func (r webRuntime) Workspaces() []string {
	cw := r.cw
	return onMain(cw, func() []string { return cw.workspaces() })
}

// Projects groups the known directories into projects and lists every
// worktree with its branch, the data the desktop's 新会话 pickers show.
// A directory outside any repository is its own project without trees.
func (r webRuntime) Projects() []webui.ProjectInfo {
	cw := r.cw
	return onMain(cw, func() []webui.ProjectInfo {
		seen := map[string]bool{}
		out := make([]webui.ProjectInfo, 0, len(cw.cfg.Workspaces)+1)
		for _, ws := range cw.workspaces() {
			project, _ := readProjectTrees(ws)
			main := ws
			if project != nil && project.Main != "" {
				main = project.Main
			}
			if seen[main] {
				continue
			}
			seen[main] = true
			info := webui.ProjectInfo{Main: main}
			if project != nil {
				for _, path := range project.Worktrees {
					info.Worktrees = append(info.Worktrees, webui.TreeInfo{
						Path: path, Branch: project.TreeBranches[path],
					})
				}
				info.Branches = project.Branches
			}
			out = append(out, info)
		}
		return out
	})
}

// Models lists the model references a session may run, the same list the
// desktop's composer model picker offers: the built-in Chat gets every
// configured model and provider group, a custom agent its whitelist. Provider
// groups are left out: the session target resolves one provider and model,
// which a group's routing cannot name.
func (r webRuntime) Models(agent string) []webui.ModelInfo {
	cw := r.cw
	return onMain(cw, func() []webui.ModelInfo {
		if strings.HasPrefix(agent, "acp:") {
			return nil
		}
		refs := cw.modelRefsFor(agent)
		out := make([]webui.ModelInfo, 0, len(refs))
		for _, ref := range refs {
			if cw.isGroupRef(ref) {
				continue
			}
			name, model := splitModelRef(ref)
			var typ string
			if name != "" {
				if p, ok := cw.findProvider(name); ok {
					typ = p.Type
				}
			}
			out = append(out, webui.ModelInfo{
				Ref: ref, Label: model, Provider: name, ProviderType: typ,
			})
		}
		return out
	})
}

// ACPOptions answers with the session config options an ACP connection's
// runtime offers: the model, the modes, the toggles, each with the values it
// takes. A cached answer returns at once; the first miss waits for the
// warm-up to land, so the browser's single fetch shows them. The wait happens
// on the HTTP handler's goroutine, never on the app's UI thread.
func (r webRuntime) ACPOptions(agent string) []webui.ACPConfigOptionInfo {
	name := strings.TrimPrefix(agent, "acp:")
	if name == agent || name == "" {
		return nil
	}
	cw := r.cw
	deadline := time.Now().Add(25 * time.Second)
	for {
		opts := cw.acpConfigs(name)
		if len(opts) > 0 {
			return acpConfigOptions(opts)
		}
		if cw.acpModelError(name) != "" || !cw.acpModelsLoading(name) || time.Now().After(deadline) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// acpConfigOptions turns the runtime's options into the shape the browser
// renders: every option with its values and its current choice.
func acpConfigOptions(opts []acp.SessionConfigOption) []webui.ACPConfigOptionInfo {
	var out []webui.ACPConfigOptionInfo
	for _, opt := range opts {
		switch {
		case opt.Select != nil:
			values := []webui.ACPConfigValue{}
			if ungrouped := opt.Select.Options.Ungrouped; ungrouped != nil {
				for _, o := range *ungrouped {
					values = append(values, webui.ACPConfigValue{
						Value: string(o.Value), Name: o.Name,
					})
				}
			}
			if grouped := opt.Select.Options.Grouped; grouped != nil {
				for _, g := range *grouped {
					for _, o := range g.Options {
						values = append(values, webui.ACPConfigValue{
							Value: string(o.Value), Name: o.Name,
						})
					}
				}
			}
			current := string(opt.Select.CurrentValue)
			kind := "select"
			if opt.Select.Category != nil &&
				*opt.Select.Category == acp.SessionConfigOptionCategoryModel {
				kind = "model"
			}
			out = append(out, webui.ACPConfigOptionInfo{
				ID: string(opt.Select.Id), Name: opt.Select.Name,
				Current: current, Kind: kind, Options: values,
			})
		case opt.Boolean != nil:
			out = append(out, webui.ACPConfigOptionInfo{
				ID: string(opt.Boolean.Id), Name: opt.Boolean.Name,
				Current: strconv.FormatBool(opt.Boolean.CurrentValue), Kind: "boolean",
			})
		}
	}
	return out
}

// CreateWorktree runs the same checks and git invocation as the desktop's
// worktree creation, reporting problems as errors instead of toasts.
func (r webRuntime) CreateWorktree(project, branch string, newBranch bool) (string, error) {
	cw := r.cw
	// onMain infers a single result type, so the path and error travel
	// together.
	res := onMain(cw, func() struct {
		path string
		err  error
	} {
		path, err := createWorktreeIn(cw, project, branch, newBranch)
		return struct {
			path string
			err  error
		}{path, err}
	})
	return res.path, res.err
}

// createWorktreeIn is CreateWorktree's body, run on the app's main thread.
func createWorktreeIn(cw *ChatWindow, project, branch string, newBranch bool) (string, error) {
	project = strings.TrimSpace(project)
	branch = strings.TrimSpace(branch)
	if project == "" {
		return "", fmt.Errorf("请选择项目")
	}
	if branch == "" {
		return "", fmt.Errorf("请填写分支名")
	}
	path := defaultWorktreePath(project, branch)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("目录已存在：%s", path)
	}
	branches := gitBranchNames(project)
	if !newBranch && !containsStr(branches, branch) {
		return "", fmt.Errorf("分支不存在：%s", branch)
	}
	if newBranch && containsStr(branches, branch) {
		return "", fmt.Errorf("分支已存在：%s", branch)
	}
	if gitBranchCheckedOut(project, branch) {
		return "", fmt.Errorf("分支已被其他工作树签出：%s", branch)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败：%w", err)
	}
	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "-b", branch, path)
	} else {
		args = append(args, path, branch)
	}
	out, err := gitRun(project, args...)
	if err != nil {
		return "", errors.New(worktreeError(out, err))
	}
	// The desktop's own worktree tab and session list must see the tree.
	cw.loadWorktrees()
	cw.bumpConvGit(project)
	return path, nil
}

func (r webRuntime) Skills(agent string) []webui.SkillInfo {
	cw := r.cw
	return onMain(cw, func() []webui.SkillInfo {
		if !cw.skills.loaded {
			cw.loadSkills()
		}
		var allow map[string]bool
		if name, ok := strings.CutPrefix(agent, "agent:"); ok {
			def, found := cw.agentByName(name)
			if !found {
				return nil
			}
			allow = make(map[string]bool, len(def.Skills))
			for _, n := range def.Skills {
				allow[n] = true
			}
		}
		cw.skills.mu.RLock()
		defer cw.skills.mu.RUnlock()
		out := make([]webui.SkillInfo, 0, len(cw.skills.list))
		for _, s := range cw.skills.list {
			if allow != nil && !allow[s.Name] {
				continue
			}
			token := skillToken(s.Name)
			if token == "" {
				continue
			}
			out = append(out, webui.SkillInfo{
				ID: s.ID, Name: s.Name, Token: token, Description: s.Description,
			})
		}
		return out
	})
}

func (r webRuntime) Revert(ctx context.Context, conv *models.Conversation, messageID string) error {
	if conv == nil {
		return fmt.Errorf("会话不存在")
	}
	dir := conv.Workspace
	if dir == "" {
		return fmt.Errorf("该会话没有工作区，无法回退")
	}
	sha := findCheckpointCommit(dir, messageID)
	if sha == "" {
		return fmt.Errorf("该回复没有 checkpoint")
	}
	if err := resetToCheckpoint(dir, sha); err != nil {
		return fmt.Errorf("回退失败：%w", err)
	}
	return nil
}

func (r webRuntime) Run(ctx context.Context, conv *models.Conversation, replyIdx int, req webui.RunRequest, live webui.Live) error {
	if strings.HasPrefix(req.Agent, "acp:") {
		return r.cw.webRunACP(ctx, conv, replyIdx, req, live)
	}
	return r.cw.webRunLLM(ctx, conv, replyIdx, req, live)
}

// webRunLLM runs one turn through the agent runtime: the custom agent or the
// plain chat model, with the session's tools and MCP servers.
func (cw *ChatWindow) webRunLLM(ctx context.Context, conv *models.Conversation, replyIdx int, req webui.RunRequest, live webui.Live) error {
	env := &builtinToolEnv{
		tools: req.Tools,
		ask: func(actx context.Context, items []*askQuestionItem) (string, error) {
			qs := make([]webui.AskItem, 0, len(items))
			for _, it := range items {
				qs = append(qs, webui.AskItem{Question: it.question, Options: it.options})
			}
			return live.Ask(actx, qs)
		},
	}
	live.Update(func(c *models.Conversation) {
		if req.Agent != "" {
			c.Agent = req.Agent
		}
		if req.Workspace != "" {
			c.Workspace = req.Workspace
		}
	})
	agent := req.Agent
	workspace := req.Workspace
	modelRef := ""
	if req.Provider != "" {
		modelRef = req.Provider + "::" + req.Model
	} else if req.Model != "" {
		modelRef = "::" + req.Model
	}

	type setup struct {
		client     *llm.Client
		react      *llm.ReactClient
		group      *llm.GroupClient
		groupReact *llm.GroupReactClient
		sys        string
		provider   string
		modelName  string
		err        error
	}
	st := onMain(cw, func() setup {
		var s setup
		s.sys = cw.taskSystemPrompt(agent)
		for _, id := range req.Skills {
			if sk := cw.skillByID(id); sk != nil && sk.Body != "" {
				skill := "## Skill: " + sk.Name + "\n\n" + sk.Body
				if s.sys != "" {
					s.sys += "\n\n"
				}
				s.sys += skill
			}
		}
		s.provider, s.modelName, _, _, _ = cw.taskAgentSpec(agent)
		if req.Provider != "" {
			s.provider, s.modelName = req.Provider, req.Model
		} else if req.Model != "" {
			s.modelName = req.Model
		}
		s.client, s.react, s.group, s.groupReact, s.err =
			cw.taskClientWithEnvModel(agent, workspace, modelRef, env, conv)
		return s
	})
	if st.err != nil {
		return st.err
	}

	msgs := buildMessages(conv, replyIdx)
	if st.sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: st.sys}}, msgs...)
	}

	onChunk := func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
		})
	}
	onReasoning := func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Reasoning += chunk
			appendSegmentText(m, "reasoning", chunk)
		})
	}
	onTool := func(info llm.ToolCallInfo) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			applyToolCall(m, info)
			appendSegmentTool(m, info)
		})
	}

	var (
		resp *llm.ChatResponse
		err  error
	)
	switch {
	case st.groupReact != nil:
		resp, err = st.groupReact.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	case st.react != nil:
		resp, err = st.react.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	case st.group != nil:
		resp, err = st.group.Chat(ctx, msgs, onChunk, onReasoning)
	default:
		resp, err = st.client.Chat(ctx, msgs, onChunk, onReasoning)
	}

	live.Update(func(c *models.Conversation) {
		m := &c.Messages[replyIdx]
		if resp != nil {
			if strings.TrimSpace(m.Content) == "" && strings.TrimSpace(resp.Content) != "" {
				m.Content = resp.Content
				appendSegmentText(m, "text", resp.Content)
			}
			if strings.TrimSpace(m.Reasoning) == "" {
				m.Reasoning = resp.Reasoning
			}
		}
		if st.provider != "" {
			c.Provider = st.provider
		}
		if st.modelName != "" {
			c.Model = st.modelName
		}
		if t := turnOf(c, m.ID); t != nil {
			t.Provider, t.Model = c.Provider, c.Model
			if resp != nil {
				t.Input = resp.PromptTokens
				t.Output = resp.CompletionTokens
				t.Total = resp.TotalTokens
			}
		}
	})

	if err == nil {
		// Commit the turn's changes as one checkpoint, like the desktop does.
		var toolNames []string
		live.Update(func(c *models.Conversation) {
			toolNames = toolNamesOf(c.Messages[replyIdx])
		})
		cw.autoCommitCheckpoint(conv, conv.Messages[replyIdx].ID, toolNames)
	}
	return err
}

// toolNamesOf lists the tool calls a reply made, for the checkpoint summary.
func toolNamesOf(m models.Message) []string {
	var out []string
	for _, tc := range m.ToolCalls {
		if tc.Name != "" {
			out = append(out, tc.Name)
		}
	}
	return out
}

// webRunACP runs one turn through an ACP connection, streaming its updates
// into the browser's conversation and asking the browser for permissions.
func (cw *ChatWindow) webRunACP(ctx context.Context, conv *models.Conversation, replyIdx int, req webui.RunRequest, live webui.Live) error {
	name := strings.TrimPrefix(req.Agent, "acp:")
	conn, ok := cw.findACPConn(name)
	if !ok {
		return fmt.Errorf("未找到 ACP 连接：%s", name)
	}
	replyID := conv.Messages[replyIdx].ID
	workspace := req.Workspace
	if conn.Cwd != "" {
		workspace = conn.Cwd
	}
	live.Update(func(c *models.Conversation) {
		c.Agent = req.Agent
		if workspace != "" {
			c.Workspace = workspace
		}
	})
	text := ""
	for i := replyIdx - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			text = conv.Messages[i].Content
			break
		}
	}

	cw.acp.mu.Lock()
	cw.acp.convObj[conv.ID] = conv
	cw.acp.webSink[conv.ID] = func(fn func(*models.Conversation)) { live.Update(fn) }
	cw.acp.mu.Unlock()
	defer func() {
		cw.acp.mu.Lock()
		delete(cw.acp.webSink, conv.ID)
		delete(cw.acp.webPerm, conv.ID)
		cw.acp.mu.Unlock()
	}()

	rt, err := cw.acpEnsureSession(conv, conn)
	if err != nil {
		return err
	}

	cw.acp.mu.Lock()
	cw.acp.webPerm[conv.ID] = func(pctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		opts := make([]webui.PermissionOption, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, webui.PermissionOption{
				ID: string(o.OptionId), Name: o.Name, Kind: string(o.Kind),
			})
		}
		detail := acpContentText(p.ToolCall.Content)
		if detail == "" && p.ToolCall.RawInput != nil {
			if b, err := json.Marshal(p.ToolCall.RawInput); err == nil {
				detail = string(b)
			}
		}
		decision, derr := live.Permission(pctx, webui.PermissionRequest{
			Title: acpPermissionTitle(&p), Detail: detail, Options: opts,
		})
		if derr != nil || decision.Canceled || decision.OptionID == "" {
			return acpCancelled(), nil
		}
		return acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{
				OptionId: acp.PermissionOptionId(decision.OptionID),
			}},
		}, nil
	}
	cw.acp.mu.Unlock()

	resp, perr := rt.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: rt.sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	})
	cw.acp.mu.Lock()
	delete(cw.acp.greeting, rt.sessionID)
	cw.acp.mu.Unlock()

	// ACP updates reach the conversation through postMain; wait for the
	// callbacks already queued so the turn is finalized after them.
	onMain(cw, func() struct{} { return struct{}{} })
	live.Update(func(c *models.Conversation) {
		cw.acpApplyUsage(c, replyID, resp)
	})
	onMain(cw, func() struct{} {
		cw.acpFlushTurn(conv, replyID)
		return struct{}{}
	})
	return perr
}
