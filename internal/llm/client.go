package llm

import (
	"chatgo/internal/config"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	"github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"google.golang.org/genai"
)

// Client represents an LLM client using eino
type Client struct {
	provider config.Provider
	model    model.ChatModel
}

// NewClient creates a new LLM client using eino
func NewClient(provider config.Provider) (*Client, error) {
	var chatModel model.ChatModel
	var err error

	ctx := context.Background()

	switch provider.Type {
	case "openai", "custom":
		// OpenAI and custom providers use OpenAI-compatible API
		cfg := &openai.Config{
			APIKey: provider.APIKey,
			Model:  provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		}
		client, err := openai.NewClient(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create openai client: %w", err)
		}
		chatModel = client

	case "anthropic", "claude":
		// Anthropic Claude
		cfg := &claude.Config{
			APIKey: provider.APIKey,
			Model:  provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = &provider.BaseURL
		}
		chatModel, err = claude.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create claude client: %w", err)
		}

	case "ollama":
		// Ollama - no APIKey needed
		cfg := &ollama.ChatModelConfig{
			Model: provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		}
		chatModel, err = ollama.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create ollama client: %w", err)
		}

	case "qwen":
		// Alibaba Qwen
		cfg := &qwen.ChatModelConfig{
			APIKey: provider.APIKey,
			Model:  provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		}
		chatModel, err = qwen.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create qwen client: %w", err)
		}

	case "deepseek":
		// DeepSeek
		cfg := &deepseek.ChatModelConfig{
			APIKey: provider.APIKey,
			Model:  provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		}
		chatModel, err = deepseek.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create deepseek client: %w", err)
		}

	case "gemini":
		// Google Gemini - need to create genai client first
		genaiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: provider.APIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create genai client: %w", err)
		}
		cfg := &gemini.Config{
			Client: genaiClient,
			Model:  provider.Model,
		}
		chatModel, err = gemini.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create gemini client: %w", err)
		}

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", provider.Type)
	}

	return &Client{
		provider: provider,
		model:    chatModel,
	}, nil
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string // user, assistant, system
	Content string
}

// ChatResponse represents the response from a chat completion
type ChatResponse struct {
	Content string
	Done    bool
}

// Chat sends a chat completion request with streaming support
func (c *Client) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string)) (*ChatResponse, error) {
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

// chatWithStream sends a streaming chat completion request
func (c *Client) chatWithStream(ctx context.Context, messages []*schema.Message, onChunk func(string)) (*ChatResponse, error) {
	// Create stream reader
	streamReader, err := c.model.Stream(ctx, messages)
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

// chatWithoutStream sends a non-streaming chat completion request
func (c *Client) chatWithoutStream(ctx context.Context, messages []*schema.Message) (*ChatResponse, error) {
	// Generate response
	response, err := c.model.Generate(ctx, messages)
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

// ChatNonBlocking sends a chat completion request without streaming
func (c *Client) ChatNonBlocking(ctx context.Context, messages []ChatMessage) (*ChatResponse, error) {
	return c.Chat(ctx, messages, nil)
}
