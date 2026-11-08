package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/llm"

	"github.com/Calcium-Ion/moejs"
)

// Hook timeouts, matching the extension-point design.
const (
	eventTimeout   = 3 * time.Second
	toolTimeout    = 60 * time.Second
	commandTimeout = 5 * time.Second
	contextTimeout = 3 * time.Second
	resolveTimeout = 5 * time.Second
	triggerTimeout = 1 * time.Second
	channelTimeout = 10 * time.Second
)

// hook resolves one exported member of the entry module.
func (in *InstanceState) hook(export string, members ...string) (moejs.Hook, error) {
	if in.mod == nil {
		return moejs.Hook{}, fmt.Errorf("插件未加载")
	}
	return in.mod.Hook(export, members...)
}

// InvokeTool runs one plugin tool and returns its result as text.
func (m *Manager) InvokeTool(ctx context.Context, pluginID, toolName, args string) (string, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return "", fmt.Errorf("插件未加载: %s", pluginID)
	}
	spec := findTool(inst.Manifest, toolName)
	if spec == nil {
		return "", fmt.Errorf("工具不存在: %s", toolName)
	}
	hook, err := inst.hook("tools", spec.Handler)
	if err != nil {
		return "", fmt.Errorf("工具 %s 未导出 handler %s: %w", toolName, spec.Handler, err)
	}
	arg := any(map[string]any{})
	if strings.TrimSpace(args) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(args), &parsed); err == nil {
			arg = parsed
		} else {
			arg = args
		}
	}
	out, err := inst.pool.call(ctx, toolTimeout, hook, arg, inst.ctxValue(callContext{}))
	if err != nil {
		return "", err
	}
	return jsonString(out), nil
}

func findTool(m *Manifest, name string) *ToolSpec {
	for i := range m.Tools {
		if m.Tools[i].Name == name {
			return &m.Tools[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Model middleware

// Middlewares returns every enabled plugin middleware, ordered by priority.
func (m *Manager) Middlewares() []llm.Middleware {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var runtimes []*middlewareRuntime
	for _, inst := range m.loadedInstancesLocked() {
		for _, spec := range inst.Manifest.Middleware {
			runtimes = append(runtimes, newMiddlewareRuntime(inst, spec))
		}
	}
	sortMiddleware(runtimes)
	out := make([]llm.Middleware, 0, len(runtimes))
	for _, r := range runtimes {
		out = append(out, r)
	}
	return out
}

type middlewareRuntime struct {
	inst    *InstanceState
	spec    MiddlewareSpec
	timeout time.Duration
}

func newMiddlewareRuntime(inst *InstanceState, spec MiddlewareSpec) *middlewareRuntime {
	timeout := time.Duration(spec.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	return &middlewareRuntime{inst: inst, spec: spec, timeout: timeout}
}

func sortMiddleware(list []*middlewareRuntime) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0; j-- {
			a, b := list[j-1], list[j]
			if a.spec.Priority < b.spec.Priority {
				break
			}
			if a.spec.Priority == b.spec.Priority && a.inst.Manifest.ID <= b.inst.Manifest.ID {
				break
			}
			list[j-1], list[j] = list[j], list[j-1]
		}
	}
}

func (mw *middlewareRuntime) Name() string {
	return mw.inst.Manifest.ID + ":" + mw.spec.Name
}

func (mw *middlewareRuntime) Matches(provider, model string) bool {
	match := mw.spec.Match
	if len(match.Providers) > 0 && !matchAny(match.Providers, provider) {
		return false
	}
	if len(match.Models) > 0 && !matchAny(match.Models, model) {
		return false
	}
	return true
}

func matchAny(patterns []string, value string) bool {
	for _, p := range patterns {
		if p == "*" || p == value {
			return true
		}
		if strings.HasSuffix(p, "*") && strings.HasPrefix(value, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}

func (mw *middlewareRuntime) BeforeRequest(ctx context.Context, req *llm.MiddlewareRequest) error {
	if !mw.enabled("beforeRequest") {
		return nil
	}
	hook, err := mw.inst.hook("middleware", mw.spec.Name, "beforeRequest")
	if err != nil {
		return err
	}
	payload := map[string]any{
		"provider": req.Provider, "model": req.Model, "system": req.System,
		"messages": messagesToAny(req.Messages), "iteration": req.Iteration,
	}
	out, err := mw.inst.pool.call(ctx, mw.timeout, hook, payload, mw.inst.ctxValue(callContext{}))
	if err != nil {
		return err
	}
	patch, ok := out.(map[string]any)
	if !ok {
		return nil
	}
	if s, ok := patch["system"].(string); ok {
		req.System = s
	}
	if s, ok := patch["systemAppend"].(string); ok && s != "" {
		if req.System != "" {
			req.System += "\n\n"
		}
		req.System += s
	}
	if raw, ok := patch["messages"].([]any); ok {
		if msgs, ok := anyToMessages(raw); ok {
			req.Messages = msgs
		}
	}
	return nil
}

func (mw *middlewareRuntime) OnChunk(ctx context.Context, chunk llm.MiddlewareChunk) (llm.MiddlewareChunk, error) {
	if !mw.enabled("onChunk") {
		return chunk, nil
	}
	hook, err := mw.inst.hook("middleware", mw.spec.Name, "onChunk")
	if err != nil {
		return chunk, err
	}
	extra := map[string]any{"channel": chunk.Channel, "index": chunk.Index}
	out, err := mw.inst.pool.call(ctx, mw.timeout, hook, chunk.Text, mw.inst.ctxValue(callContext{Extra: extra}))
	if err != nil {
		return chunk, err
	}
	if s, ok := out.(string); ok {
		chunk.Text = s
	}
	return chunk, nil
}

func (mw *middlewareRuntime) AfterResponse(ctx context.Context, resp *llm.ChatResponse) error {
	if !mw.enabled("afterResponse") {
		return nil
	}
	hook, err := mw.inst.hook("middleware", mw.spec.Name, "afterResponse")
	if err != nil {
		return err
	}
	payload := map[string]any{"text": resp.Content, "reasoning": resp.Reasoning}
	out, err := mw.inst.pool.call(ctx, mw.timeout, hook, payload, mw.inst.ctxValue(callContext{}))
	if err != nil {
		return err
	}
	patch, ok := out.(map[string]any)
	if !ok {
		return nil
	}
	if s, ok := patch["text"].(string); ok {
		resp.Content = s
	}
	if s, ok := patch["reasoning"].(string); ok {
		resp.Reasoning = s
	}
	return nil
}

func (mw *middlewareRuntime) OnError(ctx context.Context, cause error) (llm.MiddlewareErrorResult, bool) {
	if !mw.enabled("onError") {
		return llm.MiddlewareErrorResult{}, false
	}
	hook, err := mw.inst.hook("middleware", mw.spec.Name, "onError")
	if err != nil {
		return llm.MiddlewareErrorResult{}, false
	}
	payload := map[string]any{"message": cause.Error()}
	out, err := mw.inst.pool.call(ctx, mw.timeout, hook, payload, mw.inst.ctxValue(callContext{}))
	if err != nil {
		return llm.MiddlewareErrorResult{}, false
	}
	patch, ok := out.(map[string]any)
	if !ok {
		return llm.MiddlewareErrorResult{}, false
	}
	res := llm.MiddlewareErrorResult{}
	res.Message, _ = patch["message"].(string)
	res.Retry, _ = patch["retry"].(bool)
	return res, true
}

func (mw *middlewareRuntime) enabled(hook string) bool {
	if len(mw.spec.Hooks) == 0 {
		return true
	}
	for _, h := range mw.spec.Hooks {
		if h == hook {
			return true
		}
	}
	return false
}

func messagesToAny(messages []llm.ChatMessage) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content})
	}
	return out
}

func anyToMessages(raw []any) ([]llm.ChatMessage, bool) {
	out := make([]llm.ChatMessage, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if role == "" {
			return nil, false
		}
		out = append(out, llm.ChatMessage{Role: role, Content: content})
	}
	return out, true
}

// ApplyToolResult runs every enabled plugin's tool-result rewriter in order.
func (m *Manager) ApplyToolResult(ctx context.Context, name, arguments, result string) string {
	m.mu.RLock()
	insts := m.loadedInstancesLocked()
	m.mu.RUnlock()
	for _, inst := range insts {
		if !inst.Manifest.ToolResult {
			continue
		}
		hook, err := inst.hook("onToolResult")
		if err != nil {
			continue
		}
		payload := map[string]any{"name": name, "arguments": arguments, "result": result}
		out, err := inst.pool.call(ctx, eventTimeout, hook, payload, inst.ctxValue(callContext{}))
		if err != nil {
			logLine(inst.Manifest.ID, "warn", "onToolResult 失败: "+err.Error())
			continue
		}
		if s, ok := out.(string); ok {
			result = s
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Events

type eventMsg struct {
	name    string
	payload map[string]any
}

// Emit delivers an event to every plugin that subscribes to it. Delivery is
// fire-and-forget: a slow plugin never blocks the caller.
func (m *Manager) Emit(name string, payload map[string]any) {
	// The send is non-blocking, so holding the read lock keeps the channel
	// replacement in load/unload from racing with it.
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, inst := range m.loadedInstancesLocked() {
		if !subscribes(inst.Manifest, name) || inst.eventCh == nil {
			continue
		}
		select {
		case inst.eventCh <- eventMsg{name: name, payload: payload}:
		default:
			logLine(inst.Manifest.ID, "warn", "事件队列已满，丢弃 "+name)
		}
	}
}

func subscribes(m *Manifest, name string) bool {
	for _, e := range m.Events {
		if e == name {
			return true
		}
	}
	return false
}

func (in *InstanceState) eventLoop() {
	for {
		select {
		case ev := <-in.eventCh:
			in.dispatchEvent(ev)
		case <-in.stopCh:
			return
		}
	}
}

func (in *InstanceState) dispatchEvent(ev eventMsg) {
	hook, err := in.hook("events", "onEvent")
	if err != nil {
		return
	}
	_, err = in.pool.call(context.Background(), eventTimeout, hook, ev.name, ev.payload, in.ctxValue(callContext{}))
	if err != nil {
		logLine(in.Manifest.ID, "warn", "事件 "+ev.name+" 处理失败: "+err.Error())
	}
}

// ---------------------------------------------------------------------------
// Slash commands

// CommandResult is what a command handler asks the UI to do.
type CommandResult struct {
	Draft   string
	Message string
}

// RunCommand renders a prompt command or calls an action handler.
func (m *Manager) RunCommand(ctx context.Context, pluginID, name, args, selection, conversation string) (CommandResult, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return CommandResult{}, fmt.Errorf("插件未加载: %s", pluginID)
	}
	var spec *CommandSpec
	for i := range inst.Manifest.Commands {
		if inst.Manifest.Commands[i].Name == name {
			spec = &inst.Manifest.Commands[i]
			break
		}
	}
	if spec == nil {
		return CommandResult{}, fmt.Errorf("命令不存在: %s", name)
	}
	if spec.Kind == "" || spec.Kind == "prompt" {
		text := strings.ReplaceAll(spec.Template, "{selection}", selection)
		text = strings.ReplaceAll(text, "{args}", args)
		return CommandResult{Draft: text}, nil
	}
	hook, err := inst.hook("commands", spec.Handler)
	if err != nil {
		return CommandResult{}, err
	}
	extra := map[string]any{"selection": selection, "draft": selection}
	out, err := inst.pool.call(ctx, commandTimeout, hook, args, inst.ctxValue(callContext{Conversation: conversation, Extra: extra}))
	if err != nil {
		return CommandResult{}, err
	}
	res := CommandResult{}
	if patch, ok := out.(map[string]any); ok {
		res.Draft, _ = patch["draft"].(string)
		res.Message, _ = patch["message"].(string)
	} else if s, ok := out.(string); ok {
		res.Draft = s
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Context providers

// ContextItem is one @ suggestion returned by a provider.
type ContextItem struct {
	ID     string
	Label  string
	Detail string
}

// SearchContext asks one provider for suggestions.
func (m *Manager) SearchContext(ctx context.Context, pluginID, providerID, query, conversation string) ([]ContextItem, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return nil, fmt.Errorf("插件未加载: %s", pluginID)
	}
	spec := findContext(inst.Manifest, providerID)
	if spec == nil {
		return nil, fmt.Errorf("上下文提供者不存在: %s", providerID)
	}
	hook, err := inst.hook("contextProviders", spec.ID, spec.Search)
	if err != nil {
		return nil, err
	}
	out, err := inst.pool.call(ctx, contextTimeout, hook, query, inst.ctxValue(callContext{Conversation: conversation}))
	if err != nil {
		return nil, err
	}
	raw, _ := out.([]any)
	items := make([]ContextItem, 0, len(raw))
	for _, r := range raw {
		item, ok := r.(map[string]any)
		if !ok {
			continue
		}
		id, _ := item["id"].(string)
		label, _ := item["label"].(string)
		detail, _ := item["detail"].(string)
		if id == "" {
			continue
		}
		if label == "" {
			label = id
		}
		items = append(items, ContextItem{ID: id, Label: label, Detail: detail})
	}
	return items, nil
}

// ResolveContext fetches the body of one chosen context item.
func (m *Manager) ResolveContext(ctx context.Context, pluginID, providerID, itemID, conversation string) (string, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return "", fmt.Errorf("插件未加载: %s", pluginID)
	}
	spec := findContext(inst.Manifest, providerID)
	if spec == nil {
		return "", fmt.Errorf("上下文提供者不存在: %s", providerID)
	}
	hook, err := inst.hook("contextProviders", spec.ID, spec.Resolve)
	if err != nil {
		return "", err
	}
	out, err := inst.pool.call(ctx, resolveTimeout, hook, itemID, inst.ctxValue(callContext{Conversation: conversation}))
	if err != nil {
		return "", err
	}
	text := ""
	if patch, ok := out.(map[string]any); ok {
		text, _ = patch["text"].(string)
	} else if s, ok := out.(string); ok {
		text = s
	}
	limit := spec.MaxChars
	if limit <= 0 {
		limit = 8000
	}
	if limit > 32000 {
		limit = 32000
	}
	if len(text) > limit {
		text = text[:limit] + "\n…(内容已截断)"
	}
	return text, nil
}

func findContext(m *Manifest, id string) *ContextSpec {
	for i := range m.Contexts {
		if m.Contexts[i].ID == id {
			return &m.Contexts[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Trigger types

// EvaluateTrigger asks a plugin trigger type when a task should run next.
func (m *Manager) EvaluateTrigger(ctx context.Context, pluginID, typ string, values map[string]string, lastRun, now time.Time) (time.Time, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return time.Time{}, fmt.Errorf("插件未加载: %s", pluginID)
	}
	var spec *TriggerSpec
	for i := range inst.Manifest.Triggers {
		if inst.Manifest.Triggers[i].Type == typ {
			spec = &inst.Manifest.Triggers[i]
			break
		}
	}
	if spec == nil || spec.Evaluate == "" {
		return time.Time{}, fmt.Errorf("触发器不存在或没有求值器: %s", typ)
	}
	hook, err := inst.hook("triggers", spec.Evaluate)
	if err != nil {
		return time.Time{}, err
	}
	last := ""
	if !lastRun.IsZero() {
		last = lastRun.Format(time.RFC3339)
	}
	out, err := inst.pool.call(ctx, triggerTimeout, hook, values, last, now.Format(time.RFC3339))
	if err != nil {
		return time.Time{}, err
	}
	patch, ok := out.(map[string]any)
	if !ok {
		return time.Time{}, fmt.Errorf("触发器返回值非法")
	}
	next, _ := patch["next"].(string)
	if next == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, next)
}

// ---------------------------------------------------------------------------
// Channel bridges

// ChannelInbound is a normalised message a plugin channel received.
type ChannelInbound struct {
	ChatID     string
	FromUser   string
	Text       string
	ReplyToken string
}

// ChannelRequest is what a webhook bridge hands to a plugin verifier.
type ChannelRequest struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    string
}

// VerifyChannel lets a plugin validate and normalise one inbound request.
func (m *Manager) VerifyChannel(ctx context.Context, pluginID, chType string, creds map[string]string, req ChannelRequest) (ChannelInbound, error) {
	inst, spec, err := m.channelSpec(pluginID, chType)
	if err != nil {
		return ChannelInbound{}, err
	}
	if spec.Verify == "" {
		return ChannelInbound{}, fmt.Errorf("渠道 %s 未声明 verify，不能使用 webhook 模式", chType)
	}
	hook, err := inst.hook("channels", spec.Type, spec.Verify)
	if err != nil {
		return ChannelInbound{}, err
	}
	out, err := inst.pool.call(ctx, channelTimeout, hook, map[string]any{
		"method": req.Method, "path": req.Path, "headers": req.Headers, "body": req.Body,
		"json": jsonOrNil(req.Body),
	}, inst.ctxValue(callContext{Extra: map[string]any{"channel": creds}}))
	if err != nil {
		return ChannelInbound{}, err
	}
	patch, ok := out.(map[string]any)
	if !ok {
		return ChannelInbound{}, fmt.Errorf("verify 返回值非法")
	}
	if okFlag, _ := patch["ok"].(bool); !okFlag {
		return ChannelInbound{}, fmt.Errorf("签名或请求校验失败")
	}
	in := ChannelInbound{}
	in.ChatID, _ = patch["chatId"].(string)
	in.FromUser, _ = patch["fromUser"].(string)
	in.Text, _ = patch["text"].(string)
	in.ReplyToken, _ = patch["replyToken"].(string)
	return in, nil
}

// ChannelPollMessage is one message a poll-mode channel returned.
type ChannelPollMessage struct {
	ChatID     string
	FromUser   string
	Text       string
	ReplyToken string
}

// PollChannel asks a poll-mode plugin channel for new messages.
func (m *Manager) PollChannel(ctx context.Context, pluginID, chType string, creds map[string]string) ([]ChannelPollMessage, error) {
	inst, spec, err := m.channelSpec(pluginID, chType)
	if err != nil {
		return nil, err
	}
	if spec.Poll == "" {
		return nil, fmt.Errorf("渠道 %s 未声明 poll", chType)
	}
	hook, err := inst.hook("channels", spec.Type, spec.Poll)
	if err != nil {
		return nil, err
	}
	out, err := inst.pool.call(ctx, channelTimeout, hook, inst.ctxValue(callContext{Extra: map[string]any{"channel": creds}}))
	if err != nil {
		return nil, err
	}
	raw, _ := out.([]any)
	msgs := make([]ChannelPollMessage, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		msg := ChannelPollMessage{}
		msg.ChatID, _ = m["chatId"].(string)
		msg.FromUser, _ = m["fromUser"].(string)
		msg.Text, _ = m["text"].(string)
		msg.ReplyToken, _ = m["replyToken"].(string)
		if strings.TrimSpace(msg.Text) != "" {
			msgs = append(msgs, msg)
		}
	}
	return msgs, nil
}

// SendChannel delivers markdown through a plugin channel.
func (m *Manager) SendChannel(ctx context.Context, pluginID, chType string, creds map[string]string, target, markdown string) error {
	inst, spec, err := m.channelSpec(pluginID, chType)
	if err != nil {
		return err
	}
	hook, err := inst.hook("channels", spec.Type, spec.Send)
	if err != nil {
		return err
	}
	_, err = inst.pool.call(ctx, channelTimeout, hook, target, markdown, inst.ctxValue(callContext{Extra: map[string]any{"channel": creds}}))
	return err
}

func (m *Manager) channelSpec(pluginID, chType string) (*InstanceState, *ChannelSpec, error) {
	inst, ok := m.Find(pluginID)
	if !ok {
		return nil, nil, fmt.Errorf("插件未加载: %s", pluginID)
	}
	for i := range inst.Manifest.Channels {
		if inst.Manifest.Channels[i].Type == chType {
			return inst, &inst.Manifest.Channels[i], nil
		}
	}
	return nil, nil, fmt.Errorf("渠道类型不存在: %s", chType)
}

func jsonOrNil(body string) any {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return nil
	}
	return v
}
