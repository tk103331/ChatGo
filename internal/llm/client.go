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

	if err := provider.Validate(); err != nil {
		return nil, err
	}

	// A custom provider declares which protocol its endpoint speaks, so it is
	// built with the client for that protocol rather than its own type name.
	switch provider.EffectiveAPIType() {
	case config.ProviderTypeOpenAI, config.ProviderTypeCustom, config.ProviderTypeMagpie:
		// OpenAI, magpie and custom providers use OpenAI-compatible API
		cfg := &openai.Config{
			APIKey: provider.APIKey,
			Model:  provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		} else if provider.Type == config.ProviderTypeMagpie {
			// An empty base URL would send the request to api.openai.com,
			// so the local gateway's own address stands in.
			cfg.BaseURL = config.MagpieBaseURL
		}
		client, err := openai.NewClient(ctx, cfg)
		if err != nil {
			return nil, setupError(provider, err)
		}
		chatModel = client

	case config.ProviderTypeAnthropic, config.ProviderTypeClaude:
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
			return nil, setupError(provider, err)
		}

	case config.ProviderTypeOllama:
		// Ollama - no APIKey needed
		cfg := &ollama.ChatModelConfig{
			Model: provider.Model,
		}
		if provider.BaseURL != "" {
			cfg.BaseURL = provider.BaseURL
		}
		chatModel, err = ollama.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, setupError(provider, err)
		}

	case config.ProviderTypeQwen:
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
			return nil, setupError(provider, err)
		}

	case config.ProviderTypeDeepSeek:
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
			return nil, setupError(provider, err)
		}

	case config.APITypeOpenAIResponses:
		// The OpenAI Responses API, which eino-ext does not cover.
		chatModel, err = newResponsesChatModel(provider)
		if err != nil {
			return nil, setupError(provider, err)
		}

	case config.ProviderTypeGemini:
		// Google Gemini - need to create genai client first
		genaiCfg := &genai.ClientConfig{APIKey: provider.APIKey}
		if provider.BaseURL != "" {
			genaiCfg.HTTPOptions.BaseURL = provider.BaseURL
		}
		genaiClient, err := genai.NewClient(ctx, genaiCfg)
		if err != nil {
			return nil, setupError(provider, err)
		}
		cfg := &gemini.Config{
			Client: genaiClient,
			Model:  provider.Model,
		}
		chatModel, err = gemini.NewChatModel(ctx, cfg)
		if err != nil {
			return nil, setupError(provider, err)
		}

	default:
		return nil, fmt.Errorf("不支持的模型类型：%s", provider.EffectiveAPIType())
	}

	return &Client{
		provider: provider,
		model:    chatModel,
	}, nil
}

// setupError turns an upstream SDK failure into a message that names the
// provider and, for the common case of a missing credential, says so plainly
// instead of quoting the SDK's environment-variable wording.
func setupError(provider config.Provider, err error) error {
	if err == nil {
		return nil
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "api key") || strings.Contains(low, "apikey") ||
		strings.Contains(low, "authtoken") || strings.Contains(low, "api_key"):
		return fmt.Errorf("模型「%s」的 API Key 无效或缺失，请在设置 → 模型中检查", provider.Name)
	default:
		return fmt.Errorf("模型「%s」初始化失败：%s", provider.Name, firstLine(err.Error()))
	}
}

// firstLine keeps an error to its first line so a multi-line SDK dump does not
// take over the status bar.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string // user, assistant, system
	Content string
}

// ChatResponse represents the response from a chat completion
type ChatResponse struct {
	Content   string
	Reasoning string // Model thinking process, if any
	Done      bool

	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Chat sends a chat completion request with streaming support
func (c *Client) Chat(ctx context.Context, messages []ChatMessage, onChunk func(string), onReasoning ...func(string)) (*ChatResponse, error) {
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
		return c.chatWithStream(ctx, einoMessages, onChunk, onReasoning)
	}

	// Otherwise use Generate
	return c.chatWithoutStream(ctx, einoMessages)
}

// chatWithStream sends a streaming chat completion request
func (c *Client) chatWithStream(ctx context.Context, messages []*schema.Message, onChunk func(string), onReasoning []func(string)) (*ChatResponse, error) {
	// Create stream reader
	streamReader, err := c.model.Stream(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	defer streamReader.Close()

	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var usage *schema.TokenUsage

	// Read from stream
	for {
		chunk, err := streamReader.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to receive from stream: %w", err)
		}

		if chunk == nil {
			continue
		}
		takeUsage(&usage, chunk)
		if chunk.Content != "" {
			fullContent.WriteString(chunk.Content)
			onChunk(chunk.Content)
		}
		if chunk.ReasoningContent != "" {
			fullReasoning.WriteString(chunk.ReasoningContent)
			for _, cb := range onReasoning {
				if cb != nil {
					cb(chunk.ReasoningContent)
				}
			}
		}
	}

	return newChatResponse(fullContent.String(), fullReasoning.String(), usage), nil
}

// chatWithoutStream sends a non-streaming chat completion request
func (c *Client) chatWithoutStream(ctx context.Context, messages []*schema.Message) (*ChatResponse, error) {
	// Generate response
	response, err := c.model.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := ""
	reasoning := ""
	var usage *schema.TokenUsage
	if response != nil {
		content = response.Content
		reasoning = response.ReasoningContent
		if response.ResponseMeta != nil {
			usage = response.ResponseMeta.Usage
		}
	}

	return newChatResponse(content, reasoning, usage), nil
}

// newChatResponse builds a ChatResponse, copying token usage when present.
func newChatResponse(content, reasoning string, usage *schema.TokenUsage) *ChatResponse {
	resp := &ChatResponse{
		Content:   content,
		Reasoning: reasoning,
		Done:      true,
	}
	if usage != nil {
		resp.PromptTokens = usage.PromptTokens
		resp.CompletionTokens = usage.CompletionTokens
		resp.TotalTokens = usage.TotalTokens
	}
	return resp
}

// ChatNonBlocking sends a chat completion request without streaming
func (c *Client) ChatNonBlocking(ctx context.Context, messages []ChatMessage) (*ChatResponse, error) {
	return c.Chat(ctx, messages, nil)
}
