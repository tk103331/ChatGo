package llm

import (
	"chatgo/internal/config"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// ReactClient wraps a React Agent for tool-enabled conversations
type ReactClient struct {
	agent   *react.Agent
	model   model.ToolCallingChatModel
	tools   *compose.ToolsNodeConfig
	config  *ReactAgentConfig
}

// ReactAgentConfig holds configuration for the React Agent
type ReactAgentConfig struct {
	MaxStep            int
	MessageModifier    func(ctx context.Context, input []*schema.Message) []*schema.Message
	ToolReturnDirectly map[string]struct{}
	SystemPrompt       string
}

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

	return &ReactClient{
		agent:  agent,
		model:  toolableModel,
		tools:  toolsConfig,
		config: agentConfig,
	}, nil
}

// Chat sends a chat completion request with streaming support using React Agent
func (c *ReactClient) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string)) (*ChatResponse, error) {
	// Convert messages to eino format
	einoMessages := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		einoMessages[i] = &schema.Message{
			Role:    schema.RoleType(msg.Role),
			Content: msg.Content,
		}
	}

	// If streaming callback is provided, use Stream
	if onChunk != nil {
		return c.chatWithStream(ctx, einoMessages, onChunk)
	}

	// Otherwise use Generate
	return c.chatWithoutStream(ctx, einoMessages)
}

// chatWithStream sends a streaming chat completion request via React Agent
func (c *ReactClient) chatWithStream(ctx context.Context, messages []*schema.Message, onChunk func(string)) (*ChatResponse, error) {
	// Create stream reader
	streamReader, err := c.agent.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	defer streamReader.Close()

	var fullContent strings.Builder

	// Read from stream
	for {
		chunk, err := streamReader.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to receive from stream: %w", err)
		}

		if chunk != nil && chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			onChunk(chunk.Content)
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

	// Update tools in the model
	toolInfos := make([]*schema.ToolInfo, len(einoTools))
	for i, t := range einoTools {
		info, _ := t.Info(context.Background())
		toolInfos[i] = info
	}

	_, err := c.model.WithTools(toolInfos)
	if err != nil {
		return fmt.Errorf("failed to update tools in model: %w", err)
	}

	return nil
}

// toolWrapper wraps a ToolDefinition as an Eino InvokableTool
type toolWrapper struct {
	info    *schema.ToolInfo
	handler func(ctx context.Context, arguments string) (string, error)
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
		handler: def.Handler,
	}
}

// Info returns the tool information
func (w *toolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.info, nil
}

// InvokableRun executes the tool (required by InvokableTool interface)
func (w *toolWrapper) InvokableRun(ctx context.Context, arguments string) (string, error) {
	return w.handler(ctx, arguments)
}

// StreamableRun executes the tool with streaming (optional, returns not supported)
func (w *toolWrapper) StreamableRun(ctx context.Context, arguments string) (*schema.StreamReader[string], error) {
	return nil, fmt.Errorf("streaming not supported for this tool")
}
