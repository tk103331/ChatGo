package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"chatgo/internal/config"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cloudwego/eino/schema"
)

// defaultMaxOutputTokens is used for protocols that require an explicit output
// token limit (Claude messages) when the caller has none.
const defaultMaxOutputTokens = 4096

var apiTypeRelayFormats = map[string]types.RelayFormat{
	"openai":           types.RelayFormatOpenAI,
	"openai_responses": types.RelayFormatOpenAIResponses,
	"claude":           types.RelayFormatClaude,
	"gemini":           types.RelayFormatGemini,
}

// relayFormatOfProvider maps a provider to the relay format spoken upstream.
func relayFormatOfProvider(p config.Provider) (types.RelayFormat, error) {
	apiType := p.EffectiveAPIType()
	format, ok := apiTypeRelayFormats[apiType]
	if !ok {
		return "", fmt.Errorf("unsupported custom api_type: %s", apiType)
	}
	return format, nil
}

// newConversionMeta builds the conversion context, enabling the Claude and
// Gemini thinking-suffix adapters and a default Claude max_tokens.
func newConversionMeta() *convmeta.Values {
	return &convmeta.Values{
		ChannelMetaAttached: true,
		Options: &convmeta.Options{
			Claude: convmeta.ClaudeOptions{
				ThinkingAdapterEnabled: true,
				DefaultMaxTokens: func(string) int {
					return defaultMaxOutputTokens
				},
			},
			Gemini: convmeta.GeminiOptions{
				ThinkingAdapterEnabled: true,
			},
		},
	}
}

// convertRequest writes eino messages as the provider's upstream request body.
func convertRequest(ctx context.Context, p config.Provider, model string, messages []*schema.Message, tools []*schema.ToolInfo) (any, types.RelayFormat, error) {
	target, err := relayFormatOfProvider(p)
	if err != nil {
		return nil, "", err
	}
	req, err := toOpenAIChatRequest(model, messages)
	if err != nil {
		return nil, "", err
	}
	maxTokens := uint(defaultMaxOutputTokens)
	req.MaxTokens = &maxTokens
	if len(tools) > 0 {
		req.Tools = toToolCallRequests(tools)
	}
	result, err := relayconvert.ConvertRequest(ctx, newConversionMeta(), target, req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert request to %s: %w", target, err)
	}
	return result.Value, target, nil
}

// toOpenAIChatRequest maps eino messages to the OpenAI Chat request the
// converters read from, keeping tools and tool calls intact.
func toOpenAIChatRequest(model string, messages []*schema.Message) (*dto.GeneralOpenAIRequest, error) {
	req := &dto.GeneralOpenAIRequest{Model: model}
	for _, m := range messages {
		if m == nil {
			continue
		}
		msg := dto.Message{Role: string(m.Role), Content: m.Content}
		if len(m.ToolCalls) > 0 {
			calls := make([]dto.ToolCallResponse, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, dto.ToolCallResponse{
					ID:   tc.ID,
					Type: tc.Type,
					Function: dto.FunctionResponse{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
			raw, err := json.Marshal(calls)
			if err != nil {
				return nil, fmt.Errorf("failed to encode tool calls: %w", err)
			}
			msg.ToolCalls = raw
		}
		if m.Role == schema.Tool && m.ToolCallID != "" {
			msg.ToolCallId = m.ToolCallID
		}
		req.Messages = append(req.Messages, msg)
	}
	return req, nil
}

// toToolCallRequests maps eino tool info to OpenAI function tool definitions.
func toToolCallRequests(tools []*schema.ToolInfo) []dto.ToolCallRequest {
	out := make([]dto.ToolCallRequest, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		var params any
		if t.ParamsOneOf != nil {
			if spec, err := t.ParamsOneOf.ToJSONSchema(); err == nil && spec != nil {
				params = spec
			}
		}
		out = append(out, dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        t.Name,
				Description: t.Desc,
				Parameters:  params,
			},
		})
	}
	return out
}

// convertResponse maps a converted upstream response back to an eino message.
func convertResponse(ctx context.Context, value any) (*schema.Message, error) {
	result, err := relayconvert.ConvertResponse(ctx, newConversionMeta(), types.RelayFormatOpenAI, value)
	if err != nil {
		return nil, fmt.Errorf("failed to convert response to openai chat: %w", err)
	}
	resp, ok := result.Value.(*dto.OpenAITextResponse)
	if !ok || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("converted response has no choices")
	}
	msg := openAIChoiceToSchema(resp.Choices[0])
	usage := result.Usage
	if usage == nil {
		usage = &resp.Usage
	}
	attachUsage(msg, usage)
	return msg, nil
}

// openAIChoiceToSchema maps one OpenAI chat choice to an eino message.
func openAIChoiceToSchema(choice dto.OpenAITextResponseChoice) *schema.Message {
	msg := &schema.Message{Role: schema.Assistant}
	switch content := choice.Content.(type) {
	case string:
		msg.Content = content
	case nil:
	default:
		raw, _ := json.Marshal(content)
		msg.Content = string(raw)
	}
	if choice.ReasoningContent != nil {
		msg.ReasoningContent = *choice.ReasoningContent
	}
	if len(choice.ToolCalls) > 0 {
		msg.ToolCalls = toolCallsToSchema(decodeToolCalls(choice.ToolCalls))
	}
	return msg
}

// openAIStreamDeltaToSchema maps one OpenAI chat stream chunk to an eino message.
func openAIStreamDeltaToSchema(resp *dto.ChatCompletionsStreamResponse) *schema.Message {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	delta := resp.Choices[0].Delta
	msg := &schema.Message{Role: schema.Assistant}
	if delta.Content != nil {
		msg.Content = *delta.Content
	}
	if delta.ReasoningContent != nil {
		msg.ReasoningContent = *delta.ReasoningContent
	}
	if len(delta.ToolCalls) > 0 {
		msg.ToolCalls = toolCallsToSchema(delta.ToolCalls)
	}
	return msg
}

func decodeToolCalls(raw json.RawMessage) (calls []dto.ToolCallResponse) {
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	return calls
}

func toolCallsToSchema(calls []dto.ToolCallResponse) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(calls))
	for _, c := range calls {
		index := 0
		if c.Index != nil {
			index = *c.Index
		}
		callType := ""
		if s, ok := c.Type.(string); ok {
			callType = s
		}
		out = append(out, schema.ToolCall{
			Index: &index,
			ID:    c.ID,
			Type:  callType,
			Function: schema.FunctionCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			},
		})
	}
	return out
}

// apiTypeErrorHint explains a failed converted call with the upstream protocol.
func apiTypeErrorHint(p config.Provider, err error) error {
	if err == nil {
		return nil
	}
	apiType := strings.TrimSpace(p.EffectiveAPIType())
	if apiType == "" || apiType == "openai" {
		return err
	}
	return fmt.Errorf("openai→%s: %w", apiType, err)
}
