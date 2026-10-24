package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"chatgo/internal/config"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const convertedMaxResponseBytes = 64 << 20

// convertedChatModel is an OpenAI Chat model whose upstream speaks another
// protocol: requests are converted to that protocol and every upstream chunk
// is converted back to OpenAI Chat shapes, which is what eino consumes.
type convertedChatModel struct {
	provider config.Provider
	format   types.RelayFormat
	client   *http.Client
	url      string
	headers  http.Header
	tools    []*schema.ToolInfo
}

var _ model.ToolCallingChatModel = (*convertedChatModel)(nil)

func newConvertedChatModel(p config.Provider) (*convertedChatModel, error) {
	format, err := relayFormatOfProvider(p)
	if err != nil {
		return nil, err
	}
	return &convertedChatModel{
		provider: p,
		format:   format,
		client:   &http.Client{},
		url:      convertedEndpoint(p, format),
		headers:  convertedHeaders(p, format),
	}, nil
}

// convertedEndpoint resolves the chat endpoint of a protocol from the base URL.
func convertedEndpoint(p config.Provider, format types.RelayFormat) string {
	base := strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")
	switch format {
	case types.RelayFormatGemini:
		modelName := p.Model
		if !strings.Contains(modelName, "/") {
			modelName = "models/" + modelName
		}
		if base == "" {
			base = "https://generativelanguage.googleapis.com/v1beta"
		}
		return base + "/" + modelName + ":generateContent"
	case types.RelayFormatClaude:
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return base + "/v1/messages"
	case types.RelayFormatOpenAIResponses:
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return base + "/responses"
	default:
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return base + "/chat/completions"
	}
}

// convertedHeaders returns the auth and content headers of a protocol.
func convertedHeaders(p config.Provider, format types.RelayFormat) http.Header {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	switch format {
	case types.RelayFormatGemini:
		if p.APIKey != "" {
			h.Set("x-goog-api-key", p.APIKey)
		}
		if p.BaseURL != "" {
			h.Set("Authorization", "Bearer "+p.APIKey)
		}
	case types.RelayFormatClaude:
		if p.APIKey != "" {
			h.Set("x-api-key", p.APIKey)
		}
		h.Set("anthropic-version", "2023-06-01")
	default:
		if p.APIKey != "" {
			h.Set("Authorization", "Bearer "+p.APIKey)
		}
	}
	return h
}

func (m *convertedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	var options *model.Options
	options = model.GetCommonOptions(options, opts...)
	payload, _, err := convertRequest(ctx, m.providerSnapshot(options), modelNameOf(options, m.provider), input, toolsFor(m, options))
	if err != nil {
		return nil, err
	}
	setStreamFlag(payload, false)
	value, err := m.doRequest(ctx, payload)
	if err != nil {
		return nil, err
	}
	return convertResponse(ctx, value)
}

func (m *convertedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	var options *model.Options
	options = model.GetCommonOptions(options, opts...)
	payload, _, err := convertRequest(ctx, m.providerSnapshot(options), modelNameOf(options, m.provider), input, toolsFor(m, options))
	if err != nil {
		return nil, err
	}
	setStreamFlag(payload, true)
	sr, sw := schema.Pipe[*schema.Message](0)
	go m.pumpStream(ctx, payload, sw)
	return sr, nil
}

func (m *convertedChatModel) BindTools(tools []*schema.ToolInfo) error {
	m.tools = append([]*schema.ToolInfo(nil), tools...)
	return nil
}

func (m *convertedChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.tools = append([]*schema.ToolInfo(nil), tools...)
	return &cp, nil
}

// toolsFor prefers the tools bound with WithTools over per-call options.
func toolsFor(m *convertedChatModel, options *model.Options) []*schema.ToolInfo {
	if options != nil && len(options.Tools) > 0 {
		return options.Tools
	}
	return m.tools
}

func modelNameOf(options *model.Options, p config.Provider) string {
	if options != nil && options.Model != nil && *options.Model != "" {
		return *options.Model
	}
	return p.Model
}

func (m *convertedChatModel) providerSnapshot(options *model.Options) config.Provider {
	p := m.provider
	p.Model = modelNameOf(options, m.provider)
	return p
}

// setStreamFlag marks the converted request as streaming or not.
func setStreamFlag(payload any, stream bool) {
	switch v := payload.(type) {
	case *dto.GeneralOpenAIRequest:
		v.Stream = &stream
	case *dto.ClaudeRequest:
		v.Stream = &stream
	}
}

func (m *convertedChatModel) doRequest(ctx context.Context, payload any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode %s request: %w", m.format, err)
	}
	resp, err := m.send(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, convertedMaxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiTypeErrorHint(m.provider, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncate(string(data), 600)))
	}
	return decodeUpstreamResponse(m.format, data)
}

func (m *convertedChatModel) send(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, values := range m.headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, apiTypeErrorHint(m.provider, err)
	}
	return resp, nil
}

// decodeUpstreamResponse parses a protocol response body into its relay DTO.
func decodeUpstreamResponse(format types.RelayFormat, data []byte) (any, error) {
	switch format {
	case types.RelayFormatClaude:
		var resp dto.ClaudeResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to decode claude response: %w", err)
		}
		return &resp, nil
	case types.RelayFormatGemini:
		var resp dto.GeminiChatResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to decode gemini response: %w", err)
		}
		return &resp, nil
	case types.RelayFormatOpenAIResponses:
		var resp dto.OpenAIResponsesResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to decode responses response: %w", err)
		}
		return &resp, nil
	default:
		var resp dto.OpenAITextResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("failed to decode openai response: %w", err)
		}
		return &resp, nil
	}
}

// pumpStream reads the upstream SSE body, converts every event to OpenAI Chat
// and pushes the resulting deltas into the eino stream.
func (m *convertedChatModel) pumpStream(ctx context.Context, payload any, sw *schema.StreamWriter[*schema.Message]) {
	defer sw.Close()
	raw, err := json.Marshal(payload)
	if err != nil {
		sw.Send(nil, err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.url, bytes.NewReader(raw))
	if err != nil {
		sw.Send(nil, err)
		return
	}
	for k, values := range m.headers {
		for _, v := range values {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := m.client.Do(req)
	if err != nil {
		sw.Send(nil, apiTypeErrorHint(m.provider, err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, convertedMaxResponseBytes))
		sw.Send(nil, apiTypeErrorHint(m.provider, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, truncate(string(data), 600))))
		return
	}
	state, err := relayconvert.NewResponseStreamState(m.format, types.RelayFormatOpenAI, relayconvert.ResponseStreamOptions{
		Model:        m.provider.Model,
		IncludeUsage: true,
	})
	if err != nil {
		sw.Send(nil, err)
		return
	}
	err = readSSE(resp.Body, func(data []byte) bool {
		chunk, ok := decodeUpstreamStream(m.format, data)
		if !ok {
			return true
		}
		results, convErr := relayconvert.ConvertStreamResponseChunk(ctx, newConversionMeta(), state, chunk)
		if convErr != nil {
			sw.Send(nil, convErr)
			return false
		}
		for _, result := range results {
			out, ok := result.Value.(*dto.ChatCompletionsStreamResponse)
			if !ok {
				continue
			}
			if msg := openAIStreamDeltaToSchema(out); msg != nil {
				attachUsage(msg, out.Usage)
				if sw.Send(msg, nil) {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		sw.Send(nil, err)
		return
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(ctx, newConversionMeta(), state)
	if err != nil {
		sw.Send(nil, err)
		return
	}
	for _, result := range finalResults {
		out, ok := result.Value.(*dto.ChatCompletionsStreamResponse)
		if !ok || out.Usage == nil {
			continue
		}
		msg := &schema.Message{Role: schema.Assistant}
		attachUsage(msg, out.Usage)
		if sw.Send(msg, nil) {
			return
		}
	}
}

// decodeUpstreamStream parses one SSE payload of a protocol stream.
func decodeUpstreamStream(format types.RelayFormat, data []byte) (any, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) {
		return nil, false
	}
	switch format {
	case types.RelayFormatClaude:
		var chunk dto.ClaudeResponse
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil, false
		}
		return &chunk, true
	case types.RelayFormatGemini:
		var chunk dto.GeminiChatResponse
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil, false
		}
		return &chunk, true
	case types.RelayFormatOpenAIResponses:
		var chunk dto.ResponsesStreamResponse
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil, false
		}
		return &chunk, true
	default:
		var chunk dto.ChatCompletionsStreamResponse
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil, false
		}
		return &chunk, true
	}
}

// readSSE hands every event payload to fn until the body ends or fn stops it.
func readSSE(body io.Reader, fn func(data []byte) bool) error {
	reader := bufio.NewReaderSize(body, 64*1024)
	var data bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if data.Len() == 0 {
				continue
			}
			payload := append([]byte(nil), data.Bytes()...)
			data.Reset()
			if !fn(payload) {
				return nil
			}
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
}

// attachUsage records the upstream usage on the eino message meta.
func attachUsage(msg *schema.Message, usage *dto.Usage) {
	if msg == nil || usage == nil {
		return
	}
	if msg.ResponseMeta == nil {
		msg.ResponseMeta = &schema.ResponseMeta{}
	}
	msg.ResponseMeta.Usage = &schema.TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
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
