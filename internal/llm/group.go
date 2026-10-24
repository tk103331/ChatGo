package llm

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"

	"chatgo/internal/config"

	"github.com/cloudwego/eino/components/tool"
)

// groupEndpoint is one provider + model pair the group may send a request to.
type groupEndpoint struct {
	provider string
	model    string
	client   *Client
	react    *ReactClient
}

// GroupClient routes every request over the endpoints of a provider group,
// following the group's routing strategy.
type GroupClient struct {
	group     config.ProviderGroup
	endpoints []groupEndpoint
	mu        sync.Mutex
	cursor    int
}

// NewGroupClient builds a client for every selected provider model.
func NewGroupClient(group config.ProviderGroup, providers []config.Provider) (*GroupClient, error) {
	endpoints, err := buildEndpoints(group, providers, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GroupClient{group: group, endpoints: endpoints}, nil
}

// buildEndpoints resolves the group's members to concrete clients. When tools
// and an agent config are given, React clients are built instead.
func buildEndpoints(group config.ProviderGroup, providers []config.Provider, einoTools []tool.BaseTool, agentConfig *ReactAgentConfig) ([]groupEndpoint, error) {
	byName := make(map[string]config.Provider, len(providers))
	for _, p := range providers {
		byName[p.Name] = p
	}
	out := make([]groupEndpoint, 0, len(group.Members))
	for _, member := range group.Members {
		p, ok := byName[member.Provider]
		if !ok {
			continue
		}
		for _, model := range member.Models {
			if model == "" {
				continue
			}
			p.Model = model
			endpoint := groupEndpoint{provider: member.Provider, model: model}
			if agentConfig != nil {
				rc, err := NewReactClientWithEinoTools(p, einoTools, agentConfig)
				if err != nil {
					continue
				}
				endpoint.react = rc
			} else {
				c, err := NewClient(p)
				if err != nil {
					continue
				}
				endpoint.client = c
			}
			out = append(out, endpoint)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider group %q has no usable members", group.Name)
	}
	return out, nil
}

// Chat sends the request to the endpoints in routing order until one answers.
// An endpoint that already streamed output is not retried elsewhere, because
// its chunks were delivered to the caller.
func (g *GroupClient) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string), onReasoning ...func(string)) (*ChatResponse, error) {
	var lastErr error
	for _, endpoint := range g.routes("") {
		var mu sync.Mutex
		delivered := false
		chunk := func(s string) {
			mu.Lock()
			delivered = true
			mu.Unlock()
			if onChunk != nil {
				onChunk(s)
			}
		}
		resp, err := endpoint.client.Chat(ctx, messages, chunk, onReasoning...)
		if err == nil {
			return resp, nil
		}
		mu.Lock()
		streamed := delivered
		mu.Unlock()
		if streamed || ctx.Err() != nil {
			return nil, g.wrap(endpoint, err)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable members")
	}
	return nil, fmt.Errorf("provider group %q: %w", g.group.Name, lastErr)
}

// ChatNonBlocking sends a non-streaming request through the group.
func (g *GroupClient) ChatNonBlocking(ctx context.Context, messages []ChatMessage) (*ChatResponse, error) {
	return g.Chat(ctx, messages, nil)
}

func (g *GroupClient) wrap(endpoint groupEndpoint, err error) error {
	return fmt.Errorf("provider group %q member %q (%s): %w", g.group.Name, endpoint.provider, endpoint.model, err)
}

// routes returns the endpoints in the order this request should try them.
func (g *GroupClient) routes(model string) []groupEndpoint {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.routesLocked(model)
}

func (g *GroupClient) routesLocked(model string) []groupEndpoint {
	n := len(g.endpoints)
	if n == 0 {
		return nil
	}
	order := g.group.Routes(model, func(int) int {
		switch g.group.EffectiveStrategy() {
		case config.StrategyRoundRobin:
			start := g.cursor
			g.cursor = (g.cursor + 1) % n
			return start
		case config.StrategyRandom:
			return rand.IntN(n)
		default:
			return 0
		}
	})
	byKey := make(map[string]groupEndpoint, len(g.endpoints))
	for _, endpoint := range g.endpoints {
		byKey[endpoint.provider+"\x00"+endpoint.model] = endpoint
	}
	out := make([]groupEndpoint, 0, n)
	for _, member := range order {
		for _, selected := range member.Models {
			if endpoint, ok := byKey[member.Provider+"\x00"+selected]; ok {
				out = append(out, endpoint)
			}
		}
	}
	return out
}

// GroupReactClient routes React agent runs over the endpoints of a group,
// trying the next endpoint when one fails before delivering any output.
type GroupReactClient struct {
	group     config.ProviderGroup
	endpoints []groupEndpoint
	mu        sync.Mutex
	cursor    int
}

// NewGroupReactClient builds a React client for every selected provider model.
func NewGroupReactClient(group config.ProviderGroup, providers []config.Provider, einoTools []tool.BaseTool, agentConfig *ReactAgentConfig) (*GroupReactClient, error) {
	endpoints, err := buildEndpoints(group, providers, einoTools, agentConfig)
	if err != nil {
		return nil, err
	}
	return &GroupReactClient{group: group, endpoints: endpoints}, nil
}

// Chat runs the agent on the endpoints in routing order until one answers.
// Tool calls and streamed text already reported make a run non-retryable.
func (g *GroupReactClient) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string), onToolCall ToolCallCallback, onReasoning ...func(string)) (*ChatResponse, error) {
	var lastErr error
	for _, endpoint := range g.routes() {
		var mu sync.Mutex
		delivered := false
		chunk := func(s string) {
			mu.Lock()
			delivered = true
			mu.Unlock()
			if onChunk != nil {
				onChunk(s)
			}
		}
		toolCall := func(info ToolCallInfo) {
			mu.Lock()
			delivered = true
			mu.Unlock()
			if onToolCall != nil {
				onToolCall(info)
			}
		}
		resp, err := endpoint.react.Chat(ctx, messages, chunk, toolCall, onReasoning...)
		if err == nil {
			return resp, nil
		}
		mu.Lock()
		streamed := delivered
		mu.Unlock()
		if streamed || ctx.Err() != nil {
			return nil, fmt.Errorf("provider group %q member %q (%s): %w", g.group.Name, endpoint.provider, endpoint.model, err)
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable members")
	}
	return nil, fmt.Errorf("provider group %q: %w", g.group.Name, lastErr)
}

func (g *GroupReactClient) routes() []groupEndpoint {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := len(g.endpoints)
	if n == 0 {
		return nil
	}
	order := g.group.Routes("", func(int) int {
		switch g.group.EffectiveStrategy() {
		case config.StrategyRoundRobin:
			start := g.cursor
			g.cursor = (g.cursor + 1) % n
			return start
		case config.StrategyRandom:
			return rand.IntN(n)
		default:
			return 0
		}
	})
	byKey := make(map[string]groupEndpoint, len(g.endpoints))
	for _, endpoint := range g.endpoints {
		byKey[endpoint.provider+"\x00"+endpoint.model] = endpoint
	}
	out := make([]groupEndpoint, 0, n)
	for _, member := range order {
		for _, selected := range member.Models {
			if endpoint, ok := byKey[member.Provider+"\x00"+selected]; ok {
				out = append(out, endpoint)
			}
		}
	}
	return out
}
