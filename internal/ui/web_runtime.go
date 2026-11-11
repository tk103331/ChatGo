package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	acp "github.com/coder/acp-go-sdk"

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
			out = append(out, webui.AgentInfo{
				Ref: "acp:" + a.Name, Name: a.Name, Kind: "acp",
				Description: a.Command, Icon: a.Icon, Workspace: a.Cwd,
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
				ID: id, Name: t.Name, Kind: "builtin", Enabled: on[id],
			})
		}
		for _, s := range cw.cfg.MCPServers {
			id := "mcp:" + s.Name
			out = append(out, webui.ToolInfo{
				ID: id, Name: s.Name, Kind: "mcp", Enabled: on[id],
			})
		}
		return out
	})
}

func (r webRuntime) Workspaces() []string {
	cw := r.cw
	return onMain(cw, func() []string { return cw.workspaces() })
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
