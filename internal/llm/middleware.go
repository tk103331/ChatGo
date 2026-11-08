package llm

import (
	"context"
	"strings"
)

// MiddlewareRequest is the view of one model request a middleware may rewrite.
// Rewriting it never changes the saved conversation.
type MiddlewareRequest struct {
	Provider  string
	Model     string
	System    string
	Messages  []ChatMessage
	Iteration int
}

// MiddlewareChunk is one streamed delta. Channel is "content" or "reasoning".
type MiddlewareChunk struct {
	Channel string
	Index   int
	Text    string
}

// MiddlewareErrorResult is what an error middleware asks the host to do.
type MiddlewareErrorResult struct {
	Message string
	Retry   bool
}

// Middleware observes and rewrites model traffic. Plugin middleware implements
// this through the plugin manager; an empty chain keeps the previous behaviour.
type Middleware interface {
	Name() string
	Matches(provider, model string) bool
	BeforeRequest(ctx context.Context, req *MiddlewareRequest) error
	OnChunk(ctx context.Context, chunk MiddlewareChunk) (MiddlewareChunk, error)
	AfterResponse(ctx context.Context, resp *ChatResponse) error
	OnError(ctx context.Context, err error) (MiddlewareErrorResult, bool)
}

// MiddlewareChain is an ordered list of middleware.
type MiddlewareChain []Middleware

// For returns the middleware that match one provider/model pair, preserving
// order.
func (c MiddlewareChain) For(provider, model string) MiddlewareChain {
	if len(c) == 0 {
		return nil
	}
	out := make(MiddlewareChain, 0, len(c))
	for _, mw := range c {
		if mw != nil && mw.Matches(provider, model) {
			out = append(out, mw)
		}
	}
	return out
}

// BeforeRequest runs every request rewrite in order.
func (c MiddlewareChain) BeforeRequest(ctx context.Context, req *MiddlewareRequest) {
	for _, mw := range c {
		if err := mw.BeforeRequest(ctx, req); err != nil {
			continue
		}
	}
}

// OnChunk rewrites one delta through every middleware in order.
func (c MiddlewareChain) OnChunk(ctx context.Context, chunk MiddlewareChunk) MiddlewareChunk {
	for _, mw := range c {
		next, err := mw.OnChunk(ctx, chunk)
		if err != nil {
			continue
		}
		chunk = next
	}
	return chunk
}

// AfterResponse runs every response rewrite in order.
func (c MiddlewareChain) AfterResponse(ctx context.Context, resp *ChatResponse) {
	if resp == nil {
		return
	}
	for _, mw := range c {
		if err := mw.AfterResponse(ctx, resp); err != nil {
			continue
		}
	}
}

// OnError asks the middleware chain how to present an error. The first
// middleware that handles it decides the message and whether to retry once.
func (c MiddlewareChain) OnError(ctx context.Context, err error) (string, bool) {
	for _, mw := range c {
		res, ok := mw.OnError(ctx, err)
		if !ok {
			continue
		}
		return res.Message, res.Retry
	}
	return "", false
}

// ToolResultHook may rewrite a tool result before the model reads it.
type ToolResultHook func(ctx context.Context, name, arguments, result string) string

type toolResultHookKey struct{}

// WithToolResultHook attaches a result rewriter to ctx.
func WithToolResultHook(ctx context.Context, fn ToolResultHook) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, toolResultHookKey{}, fn)
}

func toolResultHookOf(ctx context.Context) ToolResultHook {
	fn, _ := ctx.Value(toolResultHookKey{}).(ToolResultHook)
	return fn
}

// splitSystem separates the leading system message from the turns, so a
// middleware can rewrite the prompt without rewriting history.
func splitSystem(messages []ChatMessage) (string, []ChatMessage) {
	system := ""
	rest := make([]ChatMessage, 0, len(messages))
	for _, m := range messages {
		if strings.EqualFold(m.Role, "system") && system == "" {
			system = m.Content
			continue
		}
		rest = append(rest, m)
	}
	return system, rest
}

// joinSystem puts the rewritten system prompt back in front of the turns.
func joinSystem(system string, messages []ChatMessage) []ChatMessage {
	if strings.TrimSpace(system) == "" {
		return messages
	}
	out := make([]ChatMessage, 0, len(messages)+1)
	out = append(out, ChatMessage{Role: "system", Content: system})
	out = append(out, messages...)
	return out
}

// applyRequestMiddleware rewrites messages before a request and returns the
// request view for the response side.
func applyRequestMiddleware(ctx context.Context, mws MiddlewareChain, provider, model string, messages []ChatMessage) ([]ChatMessage, *MiddlewareRequest) {
	if len(mws) == 0 {
		return messages, nil
	}
	system, rest := splitSystem(messages)
	req := &MiddlewareRequest{Provider: provider, Model: model, System: system, Messages: rest}
	mws.BeforeRequest(ctx, req)
	return joinSystem(req.System, req.Messages), req
}
