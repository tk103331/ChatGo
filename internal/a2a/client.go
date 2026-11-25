// Package a2a is the client side of the Agent2Agent protocol. It resolves an
// agent card, sends a user message, and turns the agent's messages, task
// updates and artifacts into the text a ChatGo conversation displays.
package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"
	"time"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"

	"chatgo/internal/config"
	"chatgo/internal/netx"
)

// A2UIMediaType marks a DataPart that carries A2UI envelopes.
const A2UIMediaType = "application/a2ui+json"

// sender is the part of a2aclient.Client this package uses. It is an
// interface so the event mapping can be tested without a server.
type sender interface {
	SendMessage(context.Context, *sdk.SendMessageRequest) (sdk.SendMessageResult, error)
	SendStreamingMessage(context.Context, *sdk.SendMessageRequest) iter.Seq2[sdk.Event, error]
	CancelTask(context.Context, *sdk.CancelTaskRequest) (*sdk.Task, error)
}

// Client is one configured remote agent.
type Client struct {
	conn   config.A2AConnection
	card   *sdk.AgentCard
	sender sender
	// legacy speaks the v0.3 wire format. Some agents publish a v1.0 card but
	// still reject v1.0 message roles, so a role-validation error retries over
	// this transport.
	legacy sender
}

// Info is the display information an agent card carries.
type Info struct {
	Name        string
	Description string
	URL         string
	Streaming   bool
}

// ChatRequest is one user turn. TaskID and ContextID continue an existing
// A2A task; both are empty for the first turn.
type ChatRequest struct {
	Text      string
	TaskID    sdk.TaskID
	ContextID string
	// A2UI holds renderer-to-agent envelopes, such as an action, sent as one
	// application/a2ui+json DataPart alongside the text.
	A2UI []any
	// OnA2UI receives each agent-to-renderer envelope while the task streams.
	OnA2UI func(json.RawMessage)
}

// Result is the text an A2A turn produced, plus the task identity the next
// turn should continue.
type Result struct {
	Text          string
	TaskID        sdk.TaskID
	ContextID     string
	State         sdk.TaskState
	InputRequired bool
	// A2UI holds every A2UI envelope the agent sent, in order.
	A2UI []json.RawMessage
}

// New resolves conn's agent card and opens a client to it. The HTTP client
// is left without a total timeout so a long-running agent is not cut off;
// the caller's context controls cancellation.
func New(ctx context.Context, conn config.A2AConnection) (*Client, error) {
	return newClient(ctx, conn, netx.Client(0))
}

func newClient(ctx context.Context, conn config.A2AConnection, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(conn.URL) == "" {
		return nil, fmt.Errorf("A2A 连接 %q 没有 URL", conn.Name)
	}
	headers := authHeaders(conn)

	// Card discovery gets its own bounded client: a server that never answers
	// should fail the turn, not hang the picker or the chat.
	cardClient := netx.Client(30 * time.Second)
	resolveOpts := make([]agentcard.ResolveOption, 0, len(headers))
	for k, v := range headers {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader(k, v))
	}
	resolver := agentcard.NewResolver(cardClient)
	resolver.CardParser = parseAgentCard
	card, err := resolver.Resolve(ctx, conn.URL, resolveOpts...)
	if err != nil {
		return nil, fmt.Errorf("获取 A2A Agent Card 失败：%w", err)
	}

	opts := []a2aclient.FactoryOption{
		a2aclient.WithJSONRPCTransport(httpClient),
		a2aclient.WithRESTTransport(httpClient),
		a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{Client: httpClient}),
		a2av0.WithRESTTransport(a2av0.RESTTransportConfig{Client: httpClient}),
		a2aclient.WithCallInterceptors(headerInterceptor{headers: headers}),
	}
	cli, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("连接 A2A Agent 失败：%w", err)
	}
	c := &Client{conn: conn, card: card, sender: cli}
	c.legacy = newLegacySender(ctx, card, opts)
	return c, nil
}

// newLegacySender builds a v0.3-compatible client when the card advertises
// v1.0. It is used only after the v1.0 request is rejected for its message
// role, which means the agent did not process the request.
func newLegacySender(ctx context.Context, card *sdk.AgentCard, opts []a2aclient.FactoryOption) sender {
	hasV1 := false
	legacyCard := *card
	legacyCard.SupportedInterfaces = make([]*sdk.AgentInterface, 0, len(card.SupportedInterfaces))
	for _, iface := range card.SupportedInterfaces {
		if iface == nil {
			continue
		}
		cp := *iface
		if cp.ProtocolVersion == sdk.Version {
			hasV1 = true
		}
		cp.ProtocolVersion = a2av0.Version
		legacyCard.SupportedInterfaces = append(legacyCard.SupportedInterfaces, &cp)
	}
	if !hasV1 || len(legacyCard.SupportedInterfaces) == 0 {
		return nil
	}
	cli, err := a2aclient.NewFromCard(ctx, &legacyCard, opts...)
	if err != nil {
		return nil
	}
	return cli
}

// parseAgentCard reads a v1.0 card, falling back to the v0.3 shape when the
// card predates SupportedInterfaces. Most deployed agents still publish the
// older card, so accepting both is what makes one connection row work across
// the ecosystem.
func parseAgentCard(body []byte) (*sdk.AgentCard, error) {
	card, err := agentcard.DefaultCardParser(body)
	if err == nil && len(card.SupportedInterfaces) > 0 {
		return card, nil
	}
	legacy, legacyErr := a2av0.NewAgentCardParser()(body)
	if legacyErr == nil && len(legacy.SupportedInterfaces) > 0 {
		return legacy, nil
	}
	if err != nil {
		return nil, err
	}
	return card, nil
}

// Info reports the card's name and description, falling back to the config
// name when the card has none.
func (c *Client) Info() Info {
	name := strings.TrimSpace(c.card.Name)
	if name == "" {
		name = c.conn.Name
	}
	return Info{
		Name:        name,
		Description: strings.TrimSpace(c.card.Description),
		URL:         strings.TrimSpace(c.conn.URL),
		Streaming:   c.card.Capabilities.Streaming,
	}
}

// Card exposes the resolved card for callers that need its skills or
// capabilities. The card is read-only.
func (c *Client) Card() *sdk.AgentCard { return c.card }

// Chat sends one user message and returns the agent's text. When the agent
// advertises streaming and onChunk is set, text arrives incrementally;
// otherwise the turn is a single request.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onChunk func(string)) (Result, error) {
	if strings.TrimSpace(req.Text) == "" && len(req.A2UI) == 0 {
		return Result{}, fmt.Errorf("消息为空")
	}
	parts := make([]*sdk.Part, 0, 2)
	if strings.TrimSpace(req.Text) != "" {
		parts = append(parts, sdk.NewTextPart(req.Text))
	}
	if len(req.A2UI) > 0 {
		part := sdk.NewDataPart(req.A2UI)
		part.MediaType = A2UIMediaType
		if part.Metadata == nil {
			part.Metadata = map[string]any{}
		}
		part.Metadata["mimeType"] = A2UIMediaType
		parts = append(parts, part)
	}
	msg := sdk.NewMessage(sdk.MessageRoleUser, parts...)
	msg.TaskID = req.TaskID
	msg.ContextID = req.ContextID
	sendReq := &sdk.SendMessageRequest{Message: msg}

	res, err := c.chat(ctx, c.sender, sendReq, onChunk, req.OnA2UI)
	if err != nil && messageRoleRejected(err) {
		// Some agents mix a v1.0 response schema with v0.3 lowercase roles.
		// Retrying the same transport keeps their v1.0 events parseable.
		lowerReq := withMessageRole(sendReq, "user")
		res, err = c.chat(ctx, c.sender, lowerReq, onChunk, req.OnA2UI)
		if err == nil || !messageRoleRejected(err) {
			return res, err
		}
		if c.legacy != nil {
			return c.chat(ctx, c.legacy, sendReq, onChunk, req.OnA2UI)
		}
	}
	return res, err
}

func withMessageRole(req *sdk.SendMessageRequest, role sdk.MessageRole) *sdk.SendMessageRequest {
	clone := *req
	if req.Message != nil {
		msg := *req.Message
		msg.Role = role
		clone.Message = &msg
	}
	return &clone
}

func (c *Client) chat(ctx context.Context, s sender, sendReq *sdk.SendMessageRequest, onChunk func(string), onA2UI func(json.RawMessage)) (Result, error) {
	if onChunk != nil && c.card.Capabilities.Streaming {
		res, err := c.chatStream(ctx, s, sendReq, onChunk, onA2UI)
		if err == nil {
			return res, nil
		}
		// A server that advertises streaming but fails before producing any
		// text may still answer a normal request. Retrying is safe because no
		// output reached the conversation yet.
		if strings.TrimSpace(res.Text) != "" {
			return res, err
		}
	}
	return c.chatOnce(ctx, s, sendReq, onA2UI)
}

// messageRoleRejected recognizes the schema rejection a v0.3 agent returns
// when it receives a v1.0 ROLE_USER/ROLE_AGENT enum.
func messageRoleRejected(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "role must be user") || strings.Contains(msg, "role must be agent")
}

// isAgentRole accepts both the v1.0 enum and the lowercase v0.3 value some
// agents still return.
func isAgentRole(role sdk.MessageRole) bool {
	switch strings.ToUpper(string(role)) {
	case "ROLE_AGENT", "AGENT":
		return true
	default:
		return false
	}
}

func (c *Client) chatOnce(ctx context.Context, s sender, req *sdk.SendMessageRequest, onA2UI func(json.RawMessage)) (Result, error) {
	resp, err := s.SendMessage(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("A2A 请求失败：%w", err)
	}
	res := Result{
		TaskID:    req.Message.TaskID,
		ContextID: req.Message.ContextID,
	}
	switch v := resp.(type) {
	case *sdk.Message:
		res.Text = partsText(v.Parts)
		res.A2UI = collectA2UI(v.Parts)
		res.TaskID = firstTaskID(v.TaskID, res.TaskID)
		res.ContextID = firstString(v.ContextID, res.ContextID)
	case *sdk.Task:
		applyTask(&res, v)
		res.Text = taskText(v)
		res.A2UI = taskA2UI(v)
	default:
		return res, fmt.Errorf("A2A 返回了未知响应类型 %T", resp)
	}
	if onA2UI != nil {
		for _, raw := range res.A2UI {
			onA2UI(raw)
		}
	}
	return res, taskError(res)
}

func (c *Client) chatStream(ctx context.Context, s sender, req *sdk.SendMessageRequest, onChunk func(string), onA2UI func(json.RawMessage)) (Result, error) {
	res := Result{
		TaskID:    req.Message.TaskID,
		ContextID: req.Message.ContextID,
	}
	var text strings.Builder
	emitA2UI := func(raws []json.RawMessage) {
		for _, raw := range raws {
			res.A2UI = append(res.A2UI, raw)
			if onA2UI != nil {
				onA2UI(raw)
			}
		}
	}
	emit := func(s string) {
		if s == "" {
			return
		}
		text.WriteString(s)
		onChunk(s)
	}

	// Artifact updates can be deltas or full replacements. Keep the text seen
	// per artifact so a replacement never repeats what already rendered.
	artifactText := map[sdk.ArtifactID]string{}
	emittedArtifact := map[sdk.ArtifactID]string{}

	var streamErr error
	for event, err := range s.SendStreamingMessage(ctx, req) {
		if err != nil {
			streamErr = err
			break
		}
		switch v := event.(type) {
		case *sdk.Message:
			if isAgentRole(v.Role) {
				if s := partsText(v.Parts); s != "" && text.Len() == 0 {
					emit(s)
				}
			}
			res.TaskID = firstTaskID(v.TaskID, res.TaskID)
			res.ContextID = firstString(v.ContextID, res.ContextID)
			emitA2UI(collectA2UI(v.Parts))
		case *sdk.Task:
			applyTask(&res, v)
			if text.Len() == 0 {
				emit(taskText(v))
			}
			emitA2UI(taskA2UI(v))
		case *sdk.TaskStatusUpdateEvent:
			res.TaskID = firstTaskID(v.TaskID, res.TaskID)
			res.ContextID = firstString(v.ContextID, res.ContextID)
			res.State = v.Status.State
			if v.Status.State == sdk.TaskStateInputRequired {
				res.InputRequired = true
			}
			// "working" status messages are progress noise; only a question
			// or the final status carries text the conversation should keep.
			if text.Len() == 0 && v.Status.Message != nil &&
				isAgentRole(v.Status.Message.Role) &&
				(v.Status.State == sdk.TaskStateInputRequired || v.Status.State.Terminal()) {
				emit(partsText(v.Status.Message.Parts))
			}
			if v.Status.Message != nil {
				emitA2UI(collectA2UI(v.Status.Message.Parts))
			}
		case *sdk.TaskArtifactUpdateEvent:
			res.TaskID = firstTaskID(v.TaskID, res.TaskID)
			res.ContextID = firstString(v.ContextID, res.ContextID)
			if v.Artifact == nil {
				continue
			}
			id := v.Artifact.ID
			emitA2UI(collectA2UI(v.Artifact.Parts))
			next := partsText(v.Artifact.Parts)
			if v.Append {
				artifactText[id] += next
			} else {
				artifactText[id] = next
			}
			cur := artifactText[id]
			prev := emittedArtifact[id]
			switch {
			case strings.HasPrefix(cur, prev) && len(cur) > len(prev):
				emit(cur[len(prev):])
			case cur != prev:
				// A non-append update replaced the artifact. The earlier text
				// cannot be retracted from a chat bubble, so append only what
				// is new rather than repeating the whole artifact.
				emit(strings.TrimPrefix(cur, prev))
			}
			emittedArtifact[id] = cur
		}
	}
	res.Text = text.String()
	if streamErr != nil {
		return res, fmt.Errorf("A2A 流式请求失败：%w", streamErr)
	}
	return res, taskError(res)
}

func applyTask(res *Result, task *sdk.Task) {
	res.TaskID = firstTaskID(task.ID, res.TaskID)
	res.ContextID = firstString(task.ContextID, res.ContextID)
	res.State = task.Status.State
	if task.Status.State == sdk.TaskStateInputRequired {
		res.InputRequired = true
	}
}

// taskText is the final text of a task: the last agent message in its
// history, then the status message, then any artifacts.
func taskText(task *sdk.Task) string {
	for i := len(task.History) - 1; i >= 0; i-- {
		msg := task.History[i]
		if !isAgentRole(msg.Role) {
			continue
		}
		if s := partsText(msg.Parts); s != "" {
			return s
		}
	}
	if task.Status.Message != nil && isAgentRole(task.Status.Message.Role) {
		if s := partsText(task.Status.Message.Parts); s != "" {
			return s
		}
	}
	var b strings.Builder
	for _, artifact := range task.Artifacts {
		if artifact == nil {
			continue
		}
		b.WriteString(partsText(artifact.Parts))
	}
	return b.String()
}

func taskError(res Result) error {
	switch res.State {
	case sdk.TaskStateFailed, sdk.TaskStateRejected:
		return fmt.Errorf("A2A 任务结束于 %s", res.State)
	default:
		return nil
	}
}

// CancelTask asks the remote agent to stop a task. It is best effort: the
// task may already be terminal, and the caller has usually stopped waiting.
func (c *Client) CancelTask(ctx context.Context, id sdk.TaskID) error {
	if id == "" {
		return nil
	}
	_, err := c.sender.CancelTask(ctx, &sdk.CancelTaskRequest{ID: id})
	if err != nil && c.legacy != nil {
		if _, legacyErr := c.legacy.CancelTask(ctx, &sdk.CancelTaskRequest{ID: id}); legacyErr == nil {
			return nil
		}
	}
	return err
}

// isA2UIPart reports whether a DataPart carries A2UI envelopes.
func isA2UIPart(part *sdk.Part) bool {
	if part == nil {
		return false
	}
	if part.MediaType == A2UIMediaType {
		return true
	}
	if _, ok := part.Content.(sdk.Data); !ok {
		return false
	}
	if mime, _ := part.Metadata["mimeType"].(string); mime == A2UIMediaType {
		return true
	}
	return false
}

// collectA2UI returns the envelopes in a list of parts.
func collectA2UI(parts []*sdk.Part) []json.RawMessage {
	var out []json.RawMessage
	for _, part := range parts {
		if !isA2UIPart(part) {
			continue
		}
		data, ok := part.Content.(sdk.Data)
		if !ok {
			continue
		}
		raw, err := json.Marshal(data.Value)
		if err != nil {
			continue
		}
		out = append(out, flattenA2UI(raw)...)
	}
	return out
}

// taskA2UI collects envelopes from a task's history, status and artifacts.
func taskA2UI(task *sdk.Task) []json.RawMessage {
	if task == nil {
		return nil
	}
	var out []json.RawMessage
	for _, msg := range task.History {
		out = append(out, collectA2UI(msg.Parts)...)
	}
	if task.Status.Message != nil {
		out = append(out, collectA2UI(task.Status.Message.Parts)...)
	}
	for _, artifact := range task.Artifacts {
		out = append(out, collectA2UI(artifact.Parts)...)
	}
	return out
}

// flattenA2UI turns an array payload into individual envelopes, leaving a
// single envelope as it is.
func flattenA2UI(raw json.RawMessage) []json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '[' {
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err == nil {
			return list
		}
		return nil
	}
	return []json.RawMessage{raw}
}

func partsText(parts []*sdk.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part == nil {
			continue
		}
		switch v := part.Content.(type) {
		case sdk.Text:
			b.WriteString(string(v))
		case sdk.Data:
			if isA2UIPart(part) {
				continue
			}
			data, err := json.Marshal(v.Value)
			if err != nil {
				fmt.Fprintf(&b, "%v", v.Value)
			} else {
				b.Write(data)
			}
		case sdk.URL:
			if part.Filename != "" {
				fmt.Fprintf(&b, "[%s](%s)", part.Filename, string(v))
			} else {
				b.WriteString(string(v))
			}
		case sdk.Raw:
			if strings.HasPrefix(part.MediaType, "text/") {
				b.Write(v)
			} else if part.Filename != "" {
				fmt.Fprintf(&b, "[%s: %d bytes]", part.Filename, len(v))
			}
		}
	}
	return b.String()
}

func authHeaders(conn config.A2AConnection) map[string]string {
	out := make(map[string]string, len(conn.Headers)+1)
	for k, v := range conn.Headers {
		out[k] = v
	}
	hasAuth := false
	for k, v := range out {
		if !strings.EqualFold(k, "Authorization") {
			continue
		}
		if strings.TrimSpace(v) == "" {
			delete(out, k)
			continue
		}
		hasAuth = true
		break
	}
	if !hasAuth && strings.TrimSpace(conn.APIKey) != "" {
		out["Authorization"] = "Bearer " + strings.TrimSpace(conn.APIKey)
	}
	return out
}

func firstTaskID(v, fallback sdk.TaskID) sdk.TaskID {
	if v != "" {
		return v
	}
	return fallback
}

func firstString(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

type headerInterceptor struct {
	headers map[string]string
}

func (h headerInterceptor) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	for k, v := range h.headers {
		req.ServiceParams.Append(k, v)
	}
	return ctx, nil, nil
}

func (headerInterceptor) After(context.Context, *a2aclient.Response) error { return nil }
