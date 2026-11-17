package webui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
)

// externalRequest is one turn started by a protocol endpoint instead of the
// browser. The endpoints share it so MCP, AG-UI and A2A all run through the
// same desktop runtime and conversation store.
type externalRequest struct {
	Text           string
	ConversationID string
	Title          string
	Agent          string
	Workspace      string
	Provider       string
	Model          string
	Tools          []string
	Skills         []string
	OnStart        func(*models.Conversation)
	OnUpdate       func(turnUpdate)
}

// turnUpdate is a snapshot of the assistant reply after one runtime update.
type turnUpdate struct {
	MessageID string
	Content   string
	Reasoning string
	ToolCalls []models.ToolCall
}

// runExternal appends one user turn to a conversation, runs it, and returns
// the conversation. It does not use runState: an external client has no
// browser to poll, and the protocol transport forwards updates itself.
func (s *Server) runExternal(ctx context.Context, req externalRequest) (*models.Conversation, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, errors.New("消息不能为空")
	}
	target, err := s.turnTarget(req.Agent, req.Provider, req.Model)
	if err != nil {
		return nil, err
	}

	var conv *models.Conversation
	if req.ConversationID != "" {
		if !validID(req.ConversationID) {
			return nil, errors.New("无效的会话 ID")
		}
		conv, err = s.cm.LoadConversation(req.ConversationID)
		if err != nil {
			return nil, err
		}
	} else {
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = titleFromText(text)
		}
		conv, err = s.cm.CreateConversation(truncate(title, 32), target.Name, target.Model)
		if err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	busy := s.runs[conv.ID] != nil
	s.mu.Unlock()
	if busy {
		return nil, errors.New("该会话已有回复正在运行")
	}

	if agent := strings.TrimSpace(req.Agent); agent != "" {
		conv.Agent = agent
	}
	if ws := strings.TrimSpace(req.Workspace); ws != "" {
		if err := validWorkspace(ws); err != nil {
			return nil, err
		}
		conv.Workspace = ws
	}
	if req.Tools != nil {
		conv.Tools = cleanTools(req.Tools)
		conv.ToolsSet = true
	}
	if target.Name != "" {
		conv.Provider = target.Name
	}
	if target.Model != "" {
		conv.Model = target.Model
	}

	now := time.Now()
	user := conv.NewHumanMessage(newID(), text, now)
	reply := conv.NewAgentMessage(newID(), conv.Agent, authorName(conv.Agent), now)
	conv.Messages = append(conv.Messages, user, reply)
	turn := models.Turn{
		ID:         newID(),
		UserMsgID:  user.ID,
		ReplyMsgID: reply.ID,
		User:       text,
		Provider:   conv.Provider,
		Model:      conv.Model,
		StartedAt:  now,
	}
	conv.Turns = append(conv.Turns, turn)
	if conv.Title == "" || conv.Title == defaultTitle {
		conv.Title = titleFromText(text)
	}
	conv.UpdatedAt = now
	replyIdx := len(conv.Messages) - 1
	turnIdx := len(conv.Turns) - 1

	runReq := RunRequest{
		Provider:  req.Provider,
		Model:     req.Model,
		Agent:     conv.Agent,
		Workspace: conv.Workspace,
		Tools:     req.Tools,
		Skills:    req.Skills,
	}
	runCtx, cancel := context.WithCancel(ctx)
	rs := &runState{
		conv: conv, target: target, req: runReq,
		cancel: cancel, done: make(chan struct{}),
	}
	s.mu.Lock()
	if s.runs[conv.ID] != nil {
		s.mu.Unlock()
		cancel()
		return nil, errors.New("该会话已有回复正在运行")
	}
	s.runs[conv.ID] = rs
	s.mu.Unlock()
	defer func() {
		close(rs.done)
		s.mu.Lock()
		if s.runs[conv.ID] == rs {
			delete(s.runs, conv.ID)
		}
		s.mu.Unlock()
		cancel()
	}()

	if err := s.cm.SaveConversation(conv); err != nil {
		return nil, err
	}
	if req.OnStart != nil {
		req.OnStart(conv)
	}

	live := &headlessLive{mu: &rs.mu, conv: conv, replyIdx: replyIdx, onUpdate: req.OnUpdate}
	live.emit()

	if s.runner != nil {
		err = s.runner.Run(runCtx, conv, replyIdx, runReq, live)
	} else {
		err = s.runPlainExternal(runCtx, conv, replyIdx, target, live)
	}

	live.mu.Lock()
	m := &conv.Messages[replyIdx]
	canceled := errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled)
	if canceled && strings.TrimSpace(m.Content) == "" {
		m.Content = "（已停止）"
		appendSegmentText(m, "text", m.Content)
	}
	if err != nil && !canceled {
		errText := "⚠️ " + err.Error()
		if strings.TrimSpace(m.Content) == "" {
			m.Content = errText
		} else {
			m.Content += "\n\n" + errText
		}
		appendSegmentText(m, "text", errText)
	}
	if turnIdx >= 0 && turnIdx < len(conv.Turns) {
		t := &conv.Turns[turnIdx]
		t.Assistant = m.Content
		t.EndedAt = time.Now()
	}
	conv.UpdatedAt = time.Now()
	saveErr := s.cm.SaveConversation(conv)
	live.mu.Unlock()
	if saveErr != nil {
		return conv, saveErr
	}
	if err != nil {
		return conv, err
	}
	return conv, nil
}

// runPlainExternal is the standalone webd turn: one plain chat client with no
// desktop tools. It mirrors runPlain but writes through the headless live so
// protocol clients receive the same streaming updates.
func (s *Server) runPlainExternal(ctx context.Context, conv *models.Conversation, replyIdx int, target config.Provider, live *headlessLive) error {
	msgs := make([]llm.ChatMessage, 0, replyIdx)
	for i := 0; i < replyIdx && i < len(conv.Messages); i++ {
		m := conv.Messages[i]
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		msgs = append(msgs, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	client, err := llm.NewClient(target)
	if err != nil {
		return err
	}
	resp, err := client.Chat(ctx, msgs, func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
		})
	}, func(chunk string) {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			m.Reasoning += chunk
			appendSegmentText(m, "reasoning", chunk)
		})
	})
	if resp != nil {
		live.Update(func(c *models.Conversation) {
			m := &c.Messages[replyIdx]
			if strings.TrimSpace(m.Content) == "" {
				m.Content = resp.Content
			}
			if strings.TrimSpace(m.Reasoning) == "" {
				m.Reasoning = resp.Reasoning
			}
			if t := turnByReply(c, m.ID); t != nil {
				t.Input = resp.PromptTokens
				t.Output = resp.CompletionTokens
				t.Total = resp.TotalTokens
			}
		})
	}
	return err
}

// headlessLive applies runtime updates without a browser interaction channel.
// An external client cannot answer questions or permissions, so those calls
// fail safely instead of leaving the turn waiting forever.
type headlessLive struct {
	mu       *sync.Mutex
	sendMu   sync.Mutex
	conv     *models.Conversation
	replyIdx int
	onUpdate func(turnUpdate)
}

func (l *headlessLive) Update(fn func(*models.Conversation)) {
	l.sendMu.Lock()
	defer l.sendMu.Unlock()
	l.mu.Lock()
	fn(l.conv)
	update, ok := l.snapshotLocked()
	l.mu.Unlock()
	if ok && l.onUpdate != nil {
		l.onUpdate(update)
	}
}

func (l *headlessLive) Ask(ctx context.Context, items []AskItem) (string, error) {
	return "", errors.New("外部 Agent 端点不支持交互式提问")
}

func (l *headlessLive) Permission(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
	return PermissionDecision{Canceled: true}, nil
}

// emit sends the current reply snapshot outside the conversation lock so a
// slow SSE writer never blocks runtime updates; sendMu keeps frames ordered.
func (l *headlessLive) emit() {
	if l.onUpdate == nil {
		return
	}
	l.sendMu.Lock()
	defer l.sendMu.Unlock()
	l.mu.Lock()
	update, ok := l.snapshotLocked()
	l.mu.Unlock()
	if ok {
		l.onUpdate(update)
	}
}

func (l *headlessLive) snapshotLocked() (turnUpdate, bool) {
	if l.replyIdx < 0 || l.replyIdx >= len(l.conv.Messages) {
		return turnUpdate{}, false
	}
	m := l.conv.Messages[l.replyIdx]
	return turnUpdate{
		MessageID: m.ID,
		Content:   m.Content,
		Reasoning: m.Reasoning,
		ToolCalls: append([]models.ToolCall(nil), m.ToolCalls...),
	}, true
}

func (s *Server) threadConversation(key string) string {
	if key == "" {
		return ""
	}
	s.externalMu.Lock()
	defer s.externalMu.Unlock()
	return s.externalThreads[key]
}

func (s *Server) bindThread(key, convID string) {
	if key == "" || convID == "" {
		return
	}
	s.externalMu.Lock()
	s.externalThreads[key] = convID
	s.externalMu.Unlock()
}

// externalOptions is the shared metadata block the AG-UI and A2A endpoints
// accept for choosing an agent, workspace or model.
type externalOptions struct {
	Agent     string
	Workspace string
	Provider  string
	Model     string
	Tools     []string
	Skills    []string
}

func optionsFromMap(values map[string]any) externalOptions {
	if len(values) == 0 {
		return externalOptions{}
	}
	return externalOptions{
		Agent:     mapString(values, "agent"),
		Workspace: mapString(values, "workspace"),
		Provider:  mapString(values, "provider"),
		Model:     mapString(values, "model"),
		Tools:     mapStrings(values, "tools"),
		Skills:    mapStrings(values, "skills"),
	}
}

func mapString(values map[string]any, key string) string {
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func mapStrings(values map[string]any, key string) []string {
	raw, ok := values[key]
	if !ok {
		return nil
	}
	switch list := raw.(type) {
	case []string:
		return append([]string(nil), list...)
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
