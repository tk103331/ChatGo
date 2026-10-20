package appkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/ui/chatpanel"
	"chatgo/pkg/models"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	mgo "github.com/egoist/mygo/ui"
)

// Tool is one operation a standalone app exposes to its chat agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// Param is one parameter of a standalone app tool.
type Param struct {
	Desc     string
	Required bool
}

// appkitTool adapts a Tool to Eino's tool interface with call observation.
type appkitTool struct {
	info *schema.ToolInfo
	run  func(args map[string]any) (string, error)
}

func newAppkitTool(t Tool) tool.BaseTool {
	params := make(map[string]*schema.ParameterInfo, len(t.Params))
	for name, p := range t.Params {
		params[name] = &schema.ParameterInfo{Type: schema.String, Desc: p.Desc, Required: p.Required}
	}
	return &appkitTool{
		info: &schema.ToolInfo{
			Name:        t.Name,
			Desc:        t.Desc,
			ParamsOneOf: schema.NewParamsOneOfByParams(params),
		},
		run: t.Run,
	}
}

func (t *appkitTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *appkitTool) InvokableRun(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
	args := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "", fmt.Errorf("参数解析失败: %w", err)
		}
	}
	return t.run(args)
}

func (t *appkitTool) StreamableRun(context.Context, string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported")
}

// einoTools wraps the app's tools with call observation so the chat shows
// each tool call the agent makes.
func (s *Standalone) einoTools() []tool.BaseTool {
	out := make([]tool.BaseTool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, llm.ObserveTool(newAppkitTool(t)))
	}
	return out
}

// ensureAgent lazily builds the React agent from the saved config.
func (s *Standalone) ensureAgent() {
	if s.react != nil || s.agentTried {
		return
	}
	s.agentTried = true
	cfg, err := config.LoadConfig()
	if err != nil {
		s.status = "加载配置失败: " + err.Error()
		return
	}
	s.cfg = cfg
	var p config.Provider
	if name := cfg.CurrentProvider; name != "" {
		for _, c := range cfg.Providers {
			if c.Name == name && c.Enabled {
				p = c
				break
			}
		}
	}
	if p.Name == "" {
		for _, c := range cfg.Providers {
			if c.Enabled {
				p = c
				break
			}
		}
	}
	if p.Name == "" {
		s.status = "尚未配置模型，请在主窗口设置中启用 Provider"
		return
	}
	if p.Model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			p.Model = ms[0]
		}
	}
	if p.Model == "" {
		s.status = "Provider「" + p.Name + "」尚未配置模型"
		return
	}
	maxStep := cfg.ReactAgentMaxStep
	if maxStep <= 0 {
		maxStep = 40
	}
	agent, err := llm.NewReactClientWithEinoTools(p, s.einoTools(), &llm.ReactAgentConfig{
		MaxStep:      maxStep,
		SystemPrompt: s.system,
	})
	if err != nil {
		s.status = "模型初始化失败: " + err.Error()
		return
	}
	s.react = agent
	s.status = ""
}

// chatMessages converts the local history to LLM messages, prepending the
// system prompt and dropping empty assistant placeholders.
func chatMessages(system string, msgs []models.Message) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, llm.ChatMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// Send delivers the user's text and starts the agent's reply.
func (s *Standalone) Send(text string) {
	text = strings.TrimSpace(text)
	if text == "" || s.streaming {
		return
	}
	s.ensureAgent()
	if s.react == nil {
		s.Invalidate()
		return
	}
	now := time.Now()
	s.messages = append(s.messages,
		models.Message{ID: s.NewID(), Role: "user", Content: text, Timestamp: now},
		models.Message{ID: s.NewID(), Role: "assistant", Timestamp: now},
	)
	idx := len(s.messages) - 1
	s.streaming = true
	s.status = ""
	ctx, cancel := context.WithCancel(context.Background())
	s.streamCancel = cancel
	msgs := chatMessages(s.system, s.messages[:idx])
	go s.runStream(ctx, idx, msgs)
	s.Invalidate()
}

// Stop cancels the streaming reply.
func (s *Standalone) Stop() {
	if s.streamCancel != nil {
		s.streamCancel()
	}
}

// runStream drives one assistant reply, updating state on the UI thread.
func (s *Standalone) runStream(ctx context.Context, idx int, msgs []llm.ChatMessage) {
	onChunk := func(chunk string) {
		s.Post(func() {
			s.messages[idx].Content += chunk
			s.Invalidate()
		})
	}
	onReasoning := func(chunk string) {
		s.Post(func() {
			s.messages[idx].Reasoning += chunk
			s.Invalidate()
		})
	}
	onTool := func(info llm.ToolCallInfo) {
		s.Post(func() {
			applyChatToolCall(&s.messages[idx], info)
			s.Invalidate()
		})
	}
	resp, err := s.react.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	s.Post(func() {
		switch {
		case err != nil && s.messages[idx].Content == "":
			s.messages[idx].Content = "⚠️ " + err.Error()
		case resp != nil && s.messages[idx].Content == "":
			s.messages[idx].Content = resp.Content
		}
		if resp != nil && s.messages[idx].Reasoning == "" {
			s.messages[idx].Reasoning = resp.Reasoning
		}
		s.streaming = false
		s.streamCancel = nil
		s.Invalidate()
	})
}

// applyChatToolCall records one tool call on the message.
func applyChatToolCall(m *models.Message, info llm.ToolCallInfo) {
	for i := range m.ToolCalls {
		if m.ToolCalls[i].ID == info.ID {
			if info.Arguments != "" {
				m.ToolCalls[i].Arguments = info.Arguments
			}
			if info.Error != nil {
				m.ToolCalls[i].Error = info.Error.Error()
				m.ToolCalls[i].Status = "failed"
			}
			if info.Result != "" {
				m.ToolCalls[i].Result = info.Result
				m.ToolCalls[i].Status = "completed"
			}
			if info.Duration > 0 {
				m.ToolCalls[i].DurationMs = info.Duration.Milliseconds()
			}
			return
		}
	}
	tc := models.ToolCall{ID: info.ID, Name: info.Name, Arguments: info.Arguments, Status: "in_progress"}
	if info.Duration > 0 {
		tc.DurationMs = info.Duration.Milliseconds()
	}
	if info.Error != nil {
		tc.Error = info.Error.Error()
		tc.Status = "failed"
	}
	if info.Result != "" {
		tc.Result = info.Result
		tc.Status = "completed"
	}
	m.ToolCalls = append(m.ToolCalls, tc)
}

// ChatView renders the conversation panel for the standalone window.
func (s *Standalone) ChatView(c *mgo.Context) {
	if s.panel == nil {
		s.panel = chatpanel.New()
	}
	s.panel.View(c, chatpanel.Options{
		Placeholder: "给 Agent 发消息…",
		Messages:    func() []models.Message { return s.messages },
		Streaming:   func() bool { return s.streaming },
		Status:      func() string { return s.status },
		Send:        func(text string) { s.Send(text); s.panel.Draft = "" },
		Stop:        func() { s.Stop() },
		Invalidate:  func() { s.Invalidate() },
	})
	if len(s.messages) == 0 && !s.streaming {
		s.panel.List.ScrollToEnd()
	}
}

// ToggleChat shows or hides the chat pane.
func (s *Standalone) ToggleChat() {
	s.chatHidden = !s.chatHidden
	s.Invalidate()
}

// HasChat reports whether the standalone window offers a chat agent.
func (s *Standalone) HasChat() bool { return len(s.tools) > 0 }

