package llm

import (
	"chatgo/internal/config"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// ReactClient wraps a React Agent for tool-enabled conversations
type ReactClient struct {
	agent  *react.Agent
	model  model.ToolCallingChatModel
	tools  *compose.ToolsNodeConfig
	config *ReactAgentConfig
}

// ReactAgentConfig holds configuration for the React Agent
type ReactAgentConfig struct {
	MaxStep            int
	MessageModifier    func(ctx context.Context, input []*schema.Message) []*schema.Message
	ToolReturnDirectly map[string]struct{}
	SystemPrompt       string
	Timeout            time.Duration
	EnableRetry        bool
	MaxRetryAttempts   int
}

// ToolCallInfo contains information about a tool call
type ToolCallInfo struct {
	ID        string // Unique identifier for this tool call
	Name      string
	Arguments string
	Result    string
	Error     error
	Duration  time.Duration
}

// ToolCallCallback is called when a tool is invoked
type ToolCallCallback func(info ToolCallInfo)

// ToolDefinition defines a tool for the React Agent
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]*schema.ParameterInfo
	Handler     func(ctx context.Context, arguments string) (string, error)
}

// NewReactClient creates a new React Agent client
func NewReactClient(provider config.Provider, tools []ToolDefinition, agentConfig *ReactAgentConfig) (*ReactClient, error) {
	ctx := context.Background()

	// First, create the base chat model
	baseClient, err := NewClient(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create base client: %w", err)
	}

	// Check if the model supports tool calling
	toolableModel, ok := baseClient.model.(model.ToolCallingChatModel)
	if !ok {
		return nil, fmt.Errorf("model %s does not support tool calling", provider.Type)
	}

	// Convert tool definitions to Eino tools
	einoTools := make([]tool.BaseTool, len(tools))
	for i, toolDef := range tools {
		einoTools[i] = newToolWrapper(toolDef)
	}

	return createReactClientWithTools(ctx, toolableModel, einoTools, agentConfig)
}

// NewReactClientWithEinoTools creates a new React Agent client with pre-built Eino tools
func NewReactClientWithEinoTools(provider config.Provider, einoTools []tool.BaseTool, agentConfig *ReactAgentConfig) (*ReactClient, error) {
	ctx := context.Background()

	// First, create the base chat model
	baseClient, err := NewClient(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to create base client: %w", err)
	}

	// Check if the model supports tool calling
	toolableModel, ok := baseClient.model.(model.ToolCallingChatModel)
	if !ok {
		return nil, fmt.Errorf("model %s does not support tool calling", provider.Type)
	}

	return createReactClientWithTools(ctx, toolableModel, einoTools, agentConfig)
}

// createReactClientWithTools creates a ReactClient with given Eino tools
func createReactClientWithTools(ctx context.Context, toolableModel model.ToolCallingChatModel, einoTools []tool.BaseTool, agentConfig *ReactAgentConfig) (*ReactClient, error) {
	// Build tools config
	toolsConfig := &compose.ToolsNodeConfig{
		Tools: einoTools,
	}

	// Set default message modifier if system prompt is provided
	if agentConfig != nil && agentConfig.SystemPrompt != "" && agentConfig.MessageModifier == nil {
		agentConfig.MessageModifier = func(ctx context.Context, input []*schema.Message) []*schema.Message {
			res := make([]*schema.Message, 0, len(input)+1)
			res = append(res, schema.SystemMessage(agentConfig.SystemPrompt))
			res = append(res, input...)
			return res
		}
	}

	// Create agent config
	cfg := &react.AgentConfig{
		ToolCallingModel: toolableModel,
		ToolsConfig:      *toolsConfig,
	}

	// Apply optional configurations
	if agentConfig != nil {
		if agentConfig.MaxStep > 0 {
			cfg.MaxStep = agentConfig.MaxStep
		}
		if agentConfig.MessageModifier != nil {
			cfg.MessageModifier = agentConfig.MessageModifier
		}
		if agentConfig.ToolReturnDirectly != nil {
			cfg.ToolReturnDirectly = agentConfig.ToolReturnDirectly
		}
	}

	// Create the agent
	agent, err := react.NewAgent(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create react agent: %w", err)
	}

	log.Printf("[React Agent] Created successfully with %d tools", len(einoTools))
	if agentConfig != nil {
		if agentConfig.Timeout > 0 {
			log.Printf("[React Agent] Timeout configured: %v", agentConfig.Timeout)
		}
		if agentConfig.EnableRetry {
			log.Printf("[React Agent] Retry enabled with max attempts: %d", agentConfig.MaxRetryAttempts)
		}
	}

	return &ReactClient{
		agent:  agent,
		model:  toolableModel,
		tools:  toolsConfig,
		config: agentConfig,
	}, nil
}

// Chat sends a chat completion request with streaming support using React Agent
func (c *ReactClient) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string), onToolCall ToolCallCallback) (*ChatResponse, error) {
	// Apply timeout if configured
	if c.config != nil && c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	// Convert messages to eino format
	einoMessages := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		einoMessages[i] = &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		}
	}

	// If streaming callback is provided, use Stream
	if onChunk != nil || onToolCall != nil {
		return c.chatWithStream(ctx, einoMessages, onChunk, onToolCall)
	}

	// Otherwise use Generate
	return c.chatWithoutStream(ctx, einoMessages)
}

// chatWithStream sends a streaming chat completion request via React Agent
func (c *ReactClient) chatWithStream(ctx context.Context, messages []*schema.Message, onChunk func(string), onToolCall ToolCallCallback) (*ChatResponse, error) {
	// Create stream reader
	streamReader, err := c.agent.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	defer streamReader.Close()

	var fullContent strings.Builder
	toolCallTracker := make(map[string]*ToolCallInfo)

	// Read from stream
	for {
		chunk, err := streamReader.Recv()
		if err != nil {
			if err == io.EOF {
				log.Printf("[React Agent] Stream ended (EOF)")
				break
			}
			return nil, fmt.Errorf("failed to receive from stream: %w", err)
		}

		if chunk == nil {
			continue
		}

		// Debug: log chunk details
		log.Printf("[React Agent] Received chunk - Role: %s, ContentLen: %d, ToolCalls: %d",
			chunk.Role, len(chunk.Content), len(chunk.ToolCalls))

		// Handle content
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			if onChunk != nil {
				onChunk(chunk.Content)
			}
		}

		// Handle tool calls
		if len(chunk.ToolCalls) > 0 && onToolCall != nil {
			for _, tc := range chunk.ToolCalls {
				// Track tool call
				info := &ToolCallInfo{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
				toolCallTracker[tc.ID] = info

				// Log tool call start
				log.Printf("[React Agent] Tool call started: %s (ID: %s), args: %s", tc.Function.Name, tc.ID, tc.Function.Arguments)

				// Notify callback
				onToolCall(*info)
			}
		}

		// Handle tool call results (ToolMessage role indicates a tool result)
		if chunk.Role == schema.Tool && chunk.ToolCallID != "" && onToolCall != nil {
			if info, exists := toolCallTracker[chunk.ToolCallID]; exists {
				info.Result = chunk.Content
				log.Printf("[React Agent] Tool call completed: %s (ID: %s), result length: %d", info.Name, info.ID, len(info.Result))

				// Notify callback with result
				onToolCall(*info)
			}
		}
	}

	return &ChatResponse{
		Content: fullContent.String(),
		Done:    true,
	}, nil
}

// chatWithoutStream sends a non-streaming chat completion request via React Agent
func (c *ReactClient) chatWithoutStream(ctx context.Context, messages []*schema.Message) (*ChatResponse, error) {
	// Generate response
	response, err := c.agent.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := ""
	if response != nil {
		content = response.Content
	}

	return &ChatResponse{
		Content: content,
		Done:    true,
	}, nil
}

// UpdateTools updates the tools available to the agent at runtime
func (c *ReactClient) UpdateTools(tools []ToolDefinition) error {
	// Convert tool definitions to Eino tools
	einoTools := make([]tool.BaseTool, len(tools))
	for i, toolDef := range tools {
		einoTools[i] = newToolWrapper(toolDef)
	}

	// Build new tools config
	toolsConfig := &compose.ToolsNodeConfig{
		Tools: einoTools,
	}

	// Update tools in the model
	toolInfos := make([]*schema.ToolInfo, len(einoTools))
	for i, t := range einoTools {
		info, err := t.Info(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get tool info for tool %d: %w", i, err)
		}
		toolInfos[i] = info
	}

	updatedModel, err := c.model.WithTools(toolInfos)
	if err != nil {
		return fmt.Errorf("failed to update tools in model: %w", err)
	}

	// Recreate agent with updated model and tools
	cfg := &react.AgentConfig{
		ToolCallingModel: updatedModel,
		ToolsConfig:      *toolsConfig,
	}

	// Apply existing configurations
	if c.config != nil {
		if c.config.MaxStep > 0 {
			cfg.MaxStep = c.config.MaxStep
		}
		if c.config.MessageModifier != nil {
			cfg.MessageModifier = c.config.MessageModifier
		}
		if c.config.ToolReturnDirectly != nil {
			cfg.ToolReturnDirectly = c.config.ToolReturnDirectly
		}
	}

	// Create new agent
	agent, err := react.NewAgent(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("failed to recreate agent with new tools: %w", err)
	}

	// Update client
	c.agent = agent
	c.tools = toolsConfig
	c.model = updatedModel

	log.Printf("[React Agent] Tools updated successfully: %d tools", len(tools))
	return nil
}

// toolWrapper wraps a ToolDefinition as an Eino InvokableTool
type toolWrapper struct {
	info        *schema.ToolInfo
	handler     func(ctx context.Context, arguments string) (string, error)
	retry       bool
	maxAttempts int
}

func newToolWrapper(def ToolDefinition) *toolWrapper {
	// Build parameters map
	params := make(map[string]*schema.ParameterInfo)
	for name, param := range def.Parameters {
		params[name] = param
	}

	return &toolWrapper{
		info: &schema.ToolInfo{
			Name:        def.Name,
			Desc:        def.Description,
			ParamsOneOf: schema.NewParamsOneOfByParams(params),
		},
		handler:     def.Handler,
		retry:       false,
		maxAttempts: 1,
	}
}

// newToolWrapperWithRetry creates a tool wrapper with retry support
func newToolWrapperWithRetry(def ToolDefinition, maxAttempts int) *toolWrapper {
	wrapper := newToolWrapper(def)
	wrapper.retry = maxAttempts > 1
	wrapper.maxAttempts = maxAttempts
	return wrapper
}

// Info returns the tool information
func (w *toolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.info, nil
}

// InvokableRun executes the tool (required by InvokableTool interface)
func (w *toolWrapper) InvokableRun(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
	var lastErr error
	attempts := w.maxAttempts
	if attempts < 1 {
		attempts = 1
	}

	for i := 0; i < attempts; i++ {
		start := time.Now()
		result, err := w.handler(ctx, arguments)
		duration := time.Since(start)

		if err == nil {
			log.Printf("[Tool] %s executed successfully in %v (attempt %d/%d)", w.info.Name, duration, i+1, attempts)
			return result, nil
		}

		lastErr = err
		log.Printf("[Tool] %s failed on attempt %d/%d: %v", w.info.Name, i+1, attempts, err)

		// If context is cancelled or deadline exceeded, don't retry
		if ctx.Err() != nil {
			return "", fmt.Errorf("tool %s execution cancelled: %w", w.info.Name, ctx.Err())
		}

		// If retry is disabled or this is the last attempt, return error
		if !w.retry || i == attempts-1 {
			break
		}

		// Wait before retry (exponential backoff)
		backoff := time.Duration(i+1) * 100 * time.Millisecond
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("tool %s execution cancelled during retry: %w", w.info.Name, ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	// Return detailed error
	return "", fmt.Errorf("tool %s failed after %d attempts: %w (args: %s)",
		w.info.Name, attempts, lastErr, truncateString(arguments, 100))
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// StreamableRun executes the tool with streaming (optional, returns not supported)
func (w *toolWrapper) StreamableRun(ctx context.Context, arguments string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported for this tool")
}
