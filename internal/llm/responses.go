package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"chatgo/internal/config"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

// responsesChatModel speaks the OpenAI Responses API, which eino-ext does not
// cover. ChatGo keeps the conversation itself, so each call is stateless: the
// history is sent as input items rather than through previous_response_id.
type responsesChatModel struct {
	provider config.Provider
	client   openai.Client
	tools    []*schema.ToolInfo
}

var _ model.ToolCallingChatModel = (*responsesChatModel)(nil)

func newResponsesChatModel(p config.Provider) (*responsesChatModel, error) {
	opts := []option.RequestOption{option.WithAPIKey(p.APIKey)}
	if p.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(p.BaseURL))
	}
	return &responsesChatModel{provider: p, client: openai.NewClient(opts...)}, nil
}

func (m *responsesChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.tools = append([]*schema.ToolInfo(nil), tools...)
	return &cp, nil
}

func (m *responsesChatModel) BindTools(tools []*schema.ToolInfo) error {
	m.tools = append([]*schema.ToolInfo(nil), tools...)
	return nil
}

func (m *responsesChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	params, err := m.params(input, opts...)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Responses.New(ctx, params)
	if err != nil {
		return nil, m.wrap(err)
	}
	out := &schema.Message{Role: schema.Assistant, Content: resp.OutputText()}
	if calls := toolCallsOf(resp); len(calls) > 0 {
		out.ToolCalls = calls
	}
	attachResponsesUsage(out, resp.Usage)
	return out, nil
}

func (m *responsesChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	params, err := m.params(input, opts...)
	if err != nil {
		return nil, err
	}
	sr, sw := schema.Pipe[*schema.Message](0)
	go m.pump(ctx, params, sw)
	return sr, nil
}

// pump forwards the text deltas of a streamed response, then the usage of the
// completed one.
func (m *responsesChatModel) pump(ctx context.Context, params responses.ResponseNewParams, sw *schema.StreamWriter[*schema.Message]) {
	defer sw.Close()
	stream := m.client.Responses.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta.OfString == "" {
				continue
			}
			if sw.Send(&schema.Message{Role: schema.Assistant, Content: event.Delta.OfString}, nil) {
				return
			}
		case "response.completed":
			msg := &schema.Message{Role: schema.Assistant}
			if calls := toolCallsOf(&event.Response); len(calls) > 0 {
				msg.ToolCalls = calls
			}
			attachResponsesUsage(msg, event.Response.Usage)
			if sw.Send(msg, nil) {
				return
			}
		}
	}
	if err := stream.Err(); err != nil && !errors.Is(err, io.EOF) {
		sw.Send(nil, m.wrap(err))
	}
}

// params builds the request from the eino messages: system messages become the
// instructions, the rest become input items.
func (m *responsesChatModel) params(input []*schema.Message, opts ...model.Option) (responses.ResponseNewParams, error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	modelName := m.provider.Model
	if options.Model != nil && *options.Model != "" {
		modelName = *options.Model
	}
	params := responses.ResponseNewParams{Model: shared.ResponsesModel(modelName)}

	var instructions []string
	items := responses.ResponseInputParam{}
	for _, msg := range input {
		if msg == nil {
			continue
		}
		switch msg.Role {
		case schema.System:
			if msg.Content != "" {
				instructions = append(instructions, msg.Content)
			}
		case schema.Assistant:
			if msg.Content != "" {
				items = append(items, responses.ResponseInputItemParamOfMessage(
					msg.Content, responses.EasyInputMessageRoleAssistant))
			}
		case schema.Tool:
			items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(
				msg.ToolCallID, msg.Content))
		default:
			items = append(items, responses.ResponseInputItemParamOfMessage(
				msg.Content, responses.EasyInputMessageRoleUser))
		}
	}
	if len(instructions) > 0 {
		params.Instructions = openai.String(strings.Join(instructions, "\n\n"))
	}
	params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: items}

	tools := options.Tools
	if len(tools) == 0 {
		tools = m.tools
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		schemaMap, err := toolParameters(t)
		if err != nil {
			return params, err
		}
		params.Tools = append(params.Tools, responses.ToolParamOfFunction(t.Name, schemaMap, false))
	}
	if options.MaxTokens != nil && *options.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(*options.MaxTokens))
	}
	if options.Temperature != nil {
		params.Temperature = openai.Float(float64(*options.Temperature))
	}
	if options.TopP != nil {
		params.TopP = openai.Float(float64(*options.TopP))
	}
	return params, nil
}

// toolParameters renders a tool's schema as the plain map the SDK expects.
func toolParameters(t *schema.ToolInfo) (map[string]any, error) {
	if t.ParamsOneOf == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}, nil
	}
	js, err := t.ParamsOneOf.ToJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("工具 %s 的参数无法转换: %w", t.Name, err)
	}
	if js == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}, nil
	}
	raw, err := json.Marshal(js)
	if err != nil {
		return nil, fmt.Errorf("工具 %s 的参数无法序列化: %w", t.Name, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("工具 %s 的参数无法解析: %w", t.Name, err)
	}
	return out, nil
}

// toolCallsOf collects the function calls of a response.
func toolCallsOf(resp *responses.Response) []schema.ToolCall {
	if resp == nil {
		return nil
	}
	var out []schema.ToolCall
	for _, item := range resp.Output {
		if item.Type != "function_call" {
			continue
		}
		index := len(out)
		out = append(out, schema.ToolCall{
			Index: &index,
			ID:    item.CallID,
			Type:  "function",
			Function: schema.FunctionCall{
				Name:      item.Name,
				Arguments: item.Arguments,
			},
		})
	}
	return out
}

// attachResponsesUsage copies the token counts onto the eino message.
func attachResponsesUsage(msg *schema.Message, usage responses.ResponseUsage) {
	if msg == nil || usage.TotalTokens == 0 {
		return
	}
	if msg.ResponseMeta == nil {
		msg.ResponseMeta = &schema.ResponseMeta{}
	}
	msg.ResponseMeta.Usage = &schema.TokenUsage{
		PromptTokens:     int(usage.InputTokens),
		CompletionTokens: int(usage.OutputTokens),
		TotalTokens:      int(usage.TotalTokens),
	}
}

func (m *responsesChatModel) wrap(err error) error {
	return setupError(m.provider, err)
}

// takeUsage keeps the first non-zero usage seen in a stream, because some
// protocols only report it on the last chunk and some repeat it on every one.
func takeUsage(usage **schema.TokenUsage, msg *schema.Message) {
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	seen := msg.ResponseMeta.Usage
	if seen.TotalTokens == 0 && seen.PromptTokens == 0 && seen.CompletionTokens == 0 {
		return
	}
	*usage = seen
}
