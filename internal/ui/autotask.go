package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	autotaskpkg "chatgo/internal/ui/autotask"
	"chatgo/pkg/models"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// agentLabel renders an agent reference for display.
func agentLabel(agent string) string {
	return autotaskpkg.AgentLabel(agent, chatAgentName)
}

// autoTaskOptions builds the autotask page Options from host state.
func (cw *ChatWindow) autoTaskOptions() autotaskpkg.Options {
	return autotaskpkg.Options{
		Config:           func() *config.Config { return cw.cfg },
		ChatAgentName:    chatAgentName,
		ConvLast:         convLast,
		SnapshotConvs: func(taskID string) []models.Conversation {
			var convs []models.Conversation
			for _, conv := range cw.taskConvs {
				if conv.TaskID == taskID {
					convs = append(convs, conv)
				}
			}
			return convs
		},
		SaveConfig:       func() error { return config.SaveConfig(cw.cfg) },
		Fail:             cw.fail,
		Toast:            cw.toast,
		AskConfirm:       cw.askConfirm,
		PickDirectory:    cw.pickDirectory,
		NewID:            cw.newID,
		OpenConversation: cw.openConversation,
		StartTask:        cw.startAutoTask,
		Invalidate:       cw.invalidate,
	}
}

// openConversation opens any conversation, including task ones, in chat.
func (cw *ChatWindow) openConversation(id string) {
	cw.selectConversation(id)
	cw.section = sectionChat
}

// autoTaskLoop checks due tasks every 30 seconds.
func (cw *ChatWindow) autoTaskLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		cw.autoTaskTick()
	}
}

func (cw *ChatWindow) autoTaskTick() {
	now := time.Now()
	for i := range cw.cfg.AutoTasks {
		t := cw.cfg.AutoTasks[i]
		if !t.Enabled || cw.autotasks.Running[t.ID] {
			continue
		}
		from := t.LastRun
		if from.IsZero() {
			from = now
		}
		if !autotaskpkg.NextRun(t.Schedule, from).After(now) {
			cw.startAutoTask(i)
		}
	}
}

// startAutoTask builds the client and runs one task off the main thread.
func (cw *ChatWindow) startAutoTask(i int) {
	if i < 0 || i >= len(cw.cfg.AutoTasks) {
		return
	}
	t := cw.cfg.AutoTasks[i]
	if cw.autotasks.Running[t.ID] {
		return
	}
	client, react, err := cw.taskClient(t.Agent, t.Workspace)
	if err != nil {
		cw.cfg.AutoTasks[i].LastRun = time.Now()
		cw.cfg.AutoTasks[i].LastNote = "失败: " + err.Error()
		_ = config.SaveConfig(cw.cfg)
		cw.toast("任务失败: "+err.Error(), mcore.SeverityError)
		return
	}
	conv, err := cw.ensureTaskConv(&cw.cfg.AutoTasks[i])
	if err != nil {
		cw.cfg.AutoTasks[i].LastRun = time.Now()
		cw.cfg.AutoTasks[i].LastNote = "失败: " + err.Error()
		_ = config.SaveConfig(cw.cfg)
		return
	}
	conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "user", Content: t.Prompt, Timestamp: time.Now()})
	msgs := buildMessages(conv, len(conv.Messages))
	if sys := cw.taskSystemPrompt(t.Agent); sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: sys}}, msgs...)
	}
	cw.autotasks.Running[t.ID] = true
	go func() {
		reply := ""
		reasoning := ""
		var runErr error
		ctx := context.Background()
		if react != nil {
			resp, err := react.Chat(ctx, msgs, nil, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		} else {
			resp, err := client.Chat(ctx, msgs, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		}
		cw.postMain(func() {
			if runErr != nil {
				reply = "⚠️ " + runErr.Error()
				cw.cfg.AutoTasks[i].LastNote = "失败: " + runErr.Error()
			} else {
				cw.cfg.AutoTasks[i].LastNote = "成功"
			}
			conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Content: reply, Reasoning: reasoning, Timestamp: time.Now()})
			_ = cw.convManager.SaveConversation(conv)
			cw.cfg.AutoTasks[i].LastRun = time.Now()
			cw.autotasks.Running[t.ID] = false
			_ = config.SaveConfig(cw.cfg)
			cw.reloadConvs()
			cw.invalidate()
			cw.notifySessionComplete(conv, reply)
		})
	}()
}

// ensureTaskConv returns the task's conversation, creating one when needed.
func (cw *ChatWindow) ensureTaskConv(task *config.AutoTask) (*models.Conversation, error) {
	if task.Reuse && task.ConvID != "" {
		if conv, err := cw.convManager.LoadConversation(task.ConvID); err == nil {
			conv.TaskID = task.ID
			return conv, nil
		}
	}
	provName, model, _, ws, _ := cw.taskAgentSpec(task.Agent)
	if task.Workspace != "" {
		ws = task.Workspace
	}
	if p, ok := cw.findProvider(provName); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(task.Name, provName, model)
	if err != nil {
		return nil, err
	}
	conv.TaskID = task.ID
	conv.Agent = task.Agent
	conv.Workspace = ws
	if task.Reuse {
		task.ConvID = conv.ID
	}
	_ = cw.convManager.SaveConversation(conv)
	return conv, nil
}

// taskClient builds a client for an agent without touching the active state.
func (cw *ChatWindow) taskClient(agent, workspace string) (*llm.Client, *llm.ReactClient, error) {
	provName, model, sys, ws, toolIDs := cw.taskAgentSpec(agent)
	root := workspace
	if root == "" {
		root = ws
	}
	if root == "" {
		root = cw.fileRoot()
	}
	p, ok := cw.findProvider(provName)
	if !ok {
		p, ok = cw.firstEnabled()
	}
	if !ok {
		return nil, nil, fmt.Errorf("尚未配置模型")
	}
	if model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	p.Model = model
	if len(toolIDs) > 0 {
		if rc, err := llm.NewReactClientWithEinoTools(p, cw.buildEinoTools(toolIDs, root), &llm.ReactAgentConfig{MaxStep: cw.cfg.ReactAgentMaxStep, SystemPrompt: sys}); err == nil {
			return nil, rc, nil
		}
	}
	c, err := llm.NewClient(p)
	return c, nil, err
}

// taskSystemPrompt returns the system prompt an agent runs with.
func (cw *ChatWindow) taskSystemPrompt(agent string) string {
	_, _, sys, _, _ := cw.taskAgentSpec(agent)
	return sys
}

// taskAgentSpec resolves an agent's provider, model, prompt, workspace and tools.
func (cw *ChatWindow) taskAgentSpec(agent string) (provider, model, systemPrompt, workspace string, toolIDs []string) {
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		for _, a := range cw.cfg.Agents {
			if a.Name != name {
				continue
			}
			if len(a.Models) > 0 {
				provider, model = splitModelRef(a.Models[0])
			}
			systemPrompt = a.SystemPrompt
			workspace = a.Workspace
			for _, t := range a.BuiltinTools {
				if builtinEnabled(cw.cfg, t) {
					toolIDs = append(toolIDs, "builtin:"+t)
				}
			}
			for _, s := range a.MCPServers {
				if mcpEnabled(cw.cfg, s) {
					toolIDs = append(toolIDs, "mcp:"+s)
				}
			}
			return
		}
	}
	provider = cw.cfg.Chat.Provider
	model = cw.cfg.Chat.Model
	for _, t := range cw.cfg.BuiltinTools {
		if t.Enabled {
			toolIDs = append(toolIDs, "builtin:"+t.Name)
		}
	}
	for _, s := range cw.cfg.MCPServers {
		if s.Enabled {
			toolIDs = append(toolIDs, "mcp:"+s.Name)
		}
	}
	return
}

func builtinEnabled(cfg *config.Config, name string) bool {
	for _, t := range cfg.BuiltinTools {
		if t.Name == name {
			return t.Enabled
		}
	}
	return false
}

func mcpEnabled(cfg *config.Config, name string) bool {
	for _, s := range cfg.MCPServers {
		if s.Name == name {
			return s.Enabled
		}
	}
	return false
}
