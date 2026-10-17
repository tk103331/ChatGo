package ui

import (
	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
	"context"
	"fmt"
	"strings"
	"time"
)

func (cw *ChatWindow) workspaces() []string {
	seen := map[string]bool{}
	var out []string
	add := func(w string) {
		if w != "" && !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	add(cw.workspace)
	for _, w := range cw.cfg.Workspaces {
		add(w)
	}
	for _, conv := range cw.convs {
		add(conv.Workspace)
	}
	return out
}

func (cw *ChatWindow) addWorkspace(dir string) {
	if dir == "" || containsStr(cw.cfg.Workspaces, dir) {
		cw.workspace = dir
		return
	}
	cw.cfg.Workspaces = append(cw.cfg.Workspaces, dir)
	_ = config.SaveConfig(cw.cfg)
	cw.workspace = dir
}

func (cw *ChatWindow) removeWorkspace(dir string) {
	for i, w := range cw.cfg.Workspaces {
		if w == dir {
			cw.cfg.Workspaces = append(cw.cfg.Workspaces[:i], cw.cfg.Workspaces[i+1:]...)
			_ = config.SaveConfig(cw.cfg)
			return
		}
	}
}

// newConversation creates a conversation in the current workspace.

func (cw *ChatWindow) newConversation() {
	cw.newConversationIn(cw.workspace, "")
}

// newConversationIn creates a conversation in ws, run by the chosen agent.

func (cw *ChatWindow) newConversationIn(ws, agent string) {
	if aw := cw.agentWorkspace(agent); aw != "" {
		ws = aw
	}
	name := cw.provider
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		}
	}
	model := cw.model
	if p, ok := cw.findProvider(name); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(defaultTitle, name, model)
	if err != nil {
		cw.status = "创建会话失败: " + err.Error()
		return
	}
	conv.Workspace = ws
	conv.Agent = agent
	_ = cw.convManager.SaveConversation(conv)
	cw.cur = conv
	cw.provider = name
	if ws != "" {
		cw.workspace = ws
	}
	cw.mode = modeChat
	cw.msgList.ScrollToEnd()
	cw.applyConversationAgent(conv)
	cw.reloadConvs()
}

// selectConversation loads a conversation and shows it in chat mode.

func (cw *ChatWindow) selectConversation(id string) {
	conv, err := cw.convManager.LoadConversation(id)
	if err != nil {
		return
	}
	cw.cur = conv
	cw.mode = modeChat
	if conv.Workspace != "" {
		cw.workspace = conv.Workspace
	}
	cw.applyConversationAgent(conv)
	cw.applyConversationModel(conv)
	cw.msgList.ScrollToEnd()
}

// applyConversationModel restores the model the conversation last used, so
// continuing it keeps that model instead of the global default.
func (cw *ChatWindow) applyConversationModel(conv *models.Conversation) {
	if strings.HasPrefix(conv.Agent, "acp:") {
		return
	}
	for i := len(conv.Turns) - 1; i >= 0; i-- {
		if t := conv.Turns[i]; t.Model != "" || t.Provider != "" {
			cw.applyModel(t.Provider, t.Model)
			return
		}
	}
}

// sendDraft appends the draft as a user turn and streams the assistant reply.

func (cw *ChatWindow) sendDraft() {
	text := strings.TrimSpace(cw.draft)
	if text == "" {
		return
	}
	if cw.cur == nil {
		cw.newConversation()
	}
	if cw.cur == nil {
		return
	}
	conv := cw.cur
	now := time.Now()
	user := models.Message{ID: cw.newID(), Role: "user", Content: text, Timestamp: now}
	conv.Messages = append(conv.Messages, user)
	conv.UpdatedAt = now
	idx := len(conv.Messages)
	reply := models.Message{ID: cw.newID(), Role: "assistant", Timestamp: now}
	conv.Messages = append(conv.Messages, reply)
	turn := models.Turn{
		ID:         cw.newID(),
		UserMsgID:  user.ID,
		ReplyMsgID: reply.ID,
		User:       text,
		StartedAt:  now,
	}
	if strings.HasPrefix(conv.Agent, "acp:") {
		turn.Provider = strings.TrimPrefix(conv.Agent, "acp:")
	} else {
		turn.Provider = cw.provider
		turn.Model = cw.model
	}
	conv.Turns = append(conv.Turns, turn)
	cw.draft = ""
	if strings.HasPrefix(conv.Agent, "acp:") && conv.Title == defaultTitle {
		conv.Title = titleFromText(text)
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.msgList.ScrollToEnd()
	cw.streaming = true
	if strings.HasPrefix(conv.Agent, "acp:") {
		cw.reloadConvs()
		cw.acpChatStart(conv, strings.TrimPrefix(conv.Agent, "acp:"), idx, text)
		return
	}
	msgs := cw.chatMessages(conv, idx)
	ctx, cancel := context.WithCancel(context.Background())
	cw.streamCancel = cancel
	go cw.runStream(ctx, conv, idx, msgs)
}

// applyConversationAgent restores the client state for a conversation's agent.

func (cw *ChatWindow) applyConversationAgent(conv *models.Conversation) {
	cw.currentAgent = ""
	cw.currentApp = ""
	cw.systemPrompt = ""
	switch {
	case strings.HasPrefix(conv.Agent, "agent:"):
		cw.currentAgent = strings.TrimPrefix(conv.Agent, "agent:")
		cw.applyAgent(cw.currentAgent)
	case strings.HasPrefix(conv.Agent, "app:"):
		cw.applyApp(strings.TrimPrefix(conv.Agent, "app:"))
	case strings.HasPrefix(conv.Agent, "acp:"):
		cw.llmClient, cw.reactClient = nil, nil
		cw.status = ""
		cw.acpWarmup(conv)
	default:
		cw.applyAgent("")
	}
}

// startAppSession loads or creates the app's own conversation and makes it
// current, so the right pane chats against the app.

func (cw *ChatWindow) startAppSession(id string) {
	cw.ensureAppState()
	if convID := cw.appConv[id]; convID != "" {
		if conv, err := cw.convManager.LoadConversation(convID); err == nil {
			cw.cur = conv
			cw.mode = modeChat
			cw.applyApp(id)
			cw.msgList.ScrollToEnd()
			return
		}
	}
	cw.newAppConversation(id)
}

// newAppConversation creates a fresh conversation bound to an app.

func (cw *ChatWindow) newAppConversation(id string) {
	a, ok := appByID(id)
	if !ok {
		return
	}
	name := cw.provider
	if name == "" {
		if p, ok := cw.firstEnabled(); ok {
			name = p.Name
		}
	}
	model := cw.model
	if p, ok := cw.findProvider(name); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(a.Title, name, model)
	if err != nil {
		cw.status = "创建应用会话失败: " + err.Error()
		return
	}
	conv.Agent = "app:" + id
	if cw.workspace != "" {
		conv.Workspace = cw.workspace
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.appConv[id] = conv.ID
	cw.cur = conv
	cw.mode = modeChat
	cw.applyApp(id)
	cw.msgList.ScrollToEnd()
}

// applyApp switches the client to an app's session: its system prompt and its
// own tool set, with no builtin or MCP tools.

func (cw *ChatWindow) applyApp(id string) {
	if _, ok := appByID(id); !ok {
		cw.applyAgent("")
		return
	}
	cw.currentAgent = ""
	cw.currentApp = id
	cw.agentMode = true
	cw.systemPrompt = cw.appSystemPrompt(id)
	cw.setupProvider(cw.provider)
}

// runStream drives one assistant reply, updating state on the main thread.

func (cw *ChatWindow) runStream(ctx context.Context, conv *models.Conversation, idx int, msgs []llm.ChatMessage) {
	var (
		resp *llm.ChatResponse
		err  error
	)
	onChunk := func(chunk string) {
		cw.postMain(func() {
			m := &conv.Messages[idx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
		})
	}
	onReasoning := func(chunk string) {
		cw.postMain(func() {
			m := &conv.Messages[idx]
			m.Reasoning += chunk
			appendSegmentText(m, "reasoning", chunk)
		})
	}
	onTool := func(info llm.ToolCallInfo) {
		cw.postMain(func() {
			m := &conv.Messages[idx]
			applyToolCall(m, info)
			appendSegmentTool(m, info)
		})
	}
	switch {
	case cw.reactClient != nil:
		resp, err = cw.reactClient.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	case cw.llmClient != nil:
		resp, err = cw.llmClient.Chat(ctx, msgs, onChunk, onReasoning)
	default:
		err = fmt.Errorf("尚未配置模型")
	}
	cw.postMain(func() {
		switch {
		case err != nil && conv.Messages[idx].Content == "":
			conv.Messages[idx].Content = "⚠️ " + err.Error()
		case resp != nil && conv.Messages[idx].Content == "":
			conv.Messages[idx].Content = resp.Content
		}
		if resp != nil && conv.Messages[idx].Reasoning == "" {
			conv.Messages[idx].Reasoning = resp.Reasoning
		}
		if resp != nil && resp.TotalTokens > 0 {
			if t := turnOf(conv, conv.Messages[idx].ID); t != nil {
				t.Input = resp.PromptTokens
				t.Output = resp.CompletionTokens
				t.Total = resp.TotalTokens
			}
		}
		if t := turnOf(conv, conv.Messages[idx].ID); t != nil {
			t.Assistant = conv.Messages[idx].Content
			t.EndedAt = time.Now()
		}
		cw.streaming = false
		cw.streamCancel = nil
		conv.UpdatedAt = time.Now()
		_ = cw.convManager.SaveConversation(conv)
		cw.reloadConvs()
		cw.generateTitle(conv)
		cw.notifySessionComplete(conv, conv.Messages[idx].Content)
	})
}

// turnProvider returns the provider of the conversation's last turn, falling
// back to the legacy conversation-level provider.
func turnProvider(conv *models.Conversation) string {
	if n := len(conv.Turns); n > 0 {
		if conv.Turns[n-1].Provider != "" {
			return conv.Turns[n-1].Provider
		}
	}
	return conv.Provider
}

// turnModel returns the model of the conversation's last turn, falling back to
// the legacy conversation-level model.
func turnModel(conv *models.Conversation) string {
	if n := len(conv.Turns); n > 0 {
		if conv.Turns[n-1].Model != "" {
			return conv.Turns[n-1].Model
		}
	}
	return conv.Model
}

// turnOf finds the turn whose reply is the message with replyID.
func turnOf(conv *models.Conversation, replyID string) *models.Turn {
	for i := len(conv.Turns) - 1; i >= 0; i-- {
		if conv.Turns[i].ReplyMsgID == replyID {
			return &conv.Turns[i]
		}
	}
	return nil
}

func (cw *ChatWindow) generateTitle(conv *models.Conversation) {
	if conv.Title != defaultTitle || len(conv.Messages) < 2 {
		return
	}
	user, assistant := firstExchange(conv)
	if user == "" || assistant == "" || strings.HasPrefix(assistant, "⚠️") {
		return
	}
	provider, ok := cw.findProvider(turnProvider(conv))
	if !ok {
		provider, ok = cw.firstEnabled()
	}
	if !ok {
		return
	}
	if m := turnModel(conv); m != "" {
		provider.Model = m
	}
	client, err := llm.NewClient(provider)
	if err != nil {
		return
	}
	go func() {
		resp, err := client.Chat(context.Background(), []llm.ChatMessage{
			{Role: "system", Content: titleSystemPrompt},
			{Role: "user", Content: "用户：" + user + "\n助手：" + assistant},
		}, nil)
		if err != nil || resp == nil {
			return
		}
		title := cleanTitle(resp.Content)
		if title == "" {
			return
		}
		cw.postMain(func() {
			if conv.Title != defaultTitle {
				return
			}
			conv.Title = title
			conv.UpdatedAt = time.Now()
			_ = cw.convManager.SaveConversation(conv)
			cw.reloadConvs()
		})
	}()
}

func firstExchange(conv *models.Conversation) (user, assistant string) {
	for _, m := range conv.Messages {
		switch m.Role {
		case "user":
			if user == "" {
				user = m.Content
			}
		case "assistant":
			if assistant == "" {
				assistant = m.Content
			}
		}
		if user != "" && assistant != "" {
			break
		}
	}
	return user, assistant
}

// titleFromText makes a short title from the user's first message.

func titleFromText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 24 {
		return string(r[:24]) + "…"
	}
	return s
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "「」\"'“”‘’《》#*`")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 20 {
		r = r[:20]
	}
	return strings.TrimSpace(string(r))
}

// stopStreaming cancels the reply being generated.

func (cw *ChatWindow) stopStreaming() {
	if cw.streamCancel != nil {
		cw.streamCancel()
		cw.streamCancel = nil
	}
}

// appendSegmentText appends streamed text to the last matching segment, or
// starts a new one so blocks stay in the order the model produced them.

func appendSegmentText(m *models.Message, kind, text string) {
	if n := len(m.Segments); n > 0 && m.Segments[n-1].Kind == kind {
		m.Segments[n-1].Text += text
		return
	}
	m.Segments = append(m.Segments, models.Segment{Kind: kind, Text: text})
}

// appendSegmentTool records a tool call as its own segment, updating the
// existing one on the completion callback.

func appendSegmentTool(m *models.Message, info llm.ToolCallInfo) {
	for i := range m.Segments {
		s := &m.Segments[i]
		if s.Kind == "tool" && s.ToolCall != nil && s.ToolCall.ID == info.ID {
			if info.Arguments != "" {
				s.ToolCall.Arguments = info.Arguments
			}
			if info.Error != nil {
				s.ToolCall.Error = info.Error.Error()
			}
			if info.Result != "" {
				s.ToolCall.Result = info.Result
			}
			if info.Duration > 0 {
				s.ToolCall.DurationMs = info.Duration.Milliseconds()
			}
			return
		}
	}
	tc := models.ToolCall{ID: info.ID, Name: info.Name, Arguments: info.Arguments}
	if info.Error != nil {
		tc.Error = info.Error.Error()
	}
	if info.Result != "" {
		tc.Result = info.Result
	}
	if info.Duration > 0 {
		tc.DurationMs = info.Duration.Milliseconds()
	}
	m.Segments = append(m.Segments, models.Segment{Kind: "tool", ToolCall: &tc})
}

// applyToolCall records a tool call start or result on an assistant message.

func applyToolCall(m *models.Message, info llm.ToolCallInfo) {
	for i := range m.ToolCalls {
		if m.ToolCalls[i].ID == info.ID {
			if info.Arguments != "" {
				m.ToolCalls[i].Arguments = info.Arguments
			}
			if info.Error != nil {
				m.ToolCalls[i].Error = info.Error.Error()
			}
			if info.Result != "" {
				m.ToolCalls[i].Result = info.Result
			}
			if info.Duration > 0 {
				m.ToolCalls[i].DurationMs = info.Duration.Milliseconds()
			}
			return
		}
	}
	tc := models.ToolCall{ID: info.ID, Name: info.Name, Arguments: info.Arguments}
	if info.Error != nil {
		tc.Error = info.Error.Error()
	}
	if info.Result != "" {
		tc.Result = info.Result
	}
	if info.Duration > 0 {
		tc.DurationMs = info.Duration.Milliseconds()
	}
	m.ToolCalls = append(m.ToolCalls, tc)
}

// chatMessages are the conversation's turns with the active system prompt.

func (cw *ChatWindow) chatMessages(conv *models.Conversation, upto int) []llm.ChatMessage {
	out := buildMessages(conv, upto)
	sys := cw.systemPrompt
	if skills := cw.skillPrompt(); skills != "" {
		if sys != "" {
			sys += "\n\n"
		}
		sys += skills
	}
	if sys != "" {
		out = append([]llm.ChatMessage{{Role: "system", Content: sys}}, out...)
	}
	return out
}

// buildMessages converts stored messages up to upto into LLM chat messages.

func buildMessages(conv *models.Conversation, upto int) []llm.ChatMessage {
	out := make([]llm.ChatMessage, 0, upto)
	for i := 0; i < upto && i < len(conv.Messages); i++ {
		m := conv.Messages[i]
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// switchProvider changes the active model and reconfigures the client.
