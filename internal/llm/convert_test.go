package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgo/internal/config"

	"github.com/cloudwego/eino/schema"
)

func TestRelayFormatOfProvider(t *testing.T) {
	cases := map[string]string{
		"":                 "openai",
		"openai":           "openai",
		"claude":           "claude",
		"gemini":           "gemini",
		"openai_responses": "openai_responses",
	}
	for apiType, want := range cases {
		p := config.Provider{Type: config.ProviderTypeCustom, APIType: apiType}
		got, err := relayFormatOfProvider(p)
		if err != nil {
			t.Fatalf("%s: %v", apiType, err)
		}
		if string(got) != want {
			t.Fatalf("%s: got %s want %s", apiType, got, want)
		}
	}
	if _, err := relayFormatOfProvider(config.Provider{Type: config.ProviderTypeCustom, APIType: "nope"}); err == nil {
		t.Fatal("expected error for unknown api type")
	}
}

func TestConvertedClaudeStream(t *testing.T) {
	jsonMode := false
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"claude-3","usage":{"input_tokens":3,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
			`{"type":"message_stop"}`,
		}
		for _, e := range events {
			_, _ = w.Write([]byte("event: x\ndata: " + e + "\n\n"))
		}
		_ = jsonMode
	}))
	defer server.Close()

	p := config.Provider{Name: "c", Type: config.ProviderTypeCustom, APIType: "claude", APIKey: "k", BaseURL: server.URL, Model: "claude-3"}
	m, err := newConvertedChatModel(p)
	if err != nil {
		t.Fatal(err)
	}
	sr, err := m.Stream(context.Background(), []*schema.Message{
		schema.SystemMessage("be brief"),
		schema.UserMessage("hi"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for {
		msg, err := sr.Recv()
		if err != nil {
			break
		}
		if msg != nil {
			text.WriteString(msg.Content)
		}
	}
	if text.String() != "Hello world" {
		t.Fatalf("got %q", text.String())
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("expected system moved out of messages, got %v", gotBody["messages"])
	}
	if gotBody["system"] == nil {
		t.Fatalf("expected system field, got %v", gotBody)
	}
	if gotBody["max_tokens"] == nil {
		t.Fatalf("expected max_tokens, got %v", gotBody)
	}
	if gotBody["stream"] != true {
		t.Fatalf("expected stream true, got %v", gotBody["stream"])
	}
}

func TestConvertedGeminiNonStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models/gemini-pro:generateContent") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "k" {
			t.Errorf("missing api key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6}}`))
	}))
	defer server.Close()

	p := config.Provider{Name: "g", Type: config.ProviderTypeCustom, APIType: "gemini", APIKey: "k", BaseURL: server.URL, Model: "gemini-pro"}
	m, err := newConvertedChatModel(p)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := m.Generate(context.Background(), []*schema.Message{schema.UserMessage("ping")})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "pong" {
		t.Fatalf("got %q", msg.Content)
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil || msg.ResponseMeta.Usage.TotalTokens != 6 {
		t.Fatalf("usage not converted: %+v", msg.ResponseMeta)
	}
}

func TestConvertedToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","id":"tu_1","name":"get_weather","input":{"city":"Paris"}}],"stop_reason":"tool_use","model":"claude-3","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()

	p := config.Provider{Name: "c", Type: config.ProviderTypeCustom, APIType: "claude", APIKey: "k", BaseURL: server.URL, Model: "claude-3"}
	m, err := newConvertedChatModel(p)
	if err != nil {
		t.Fatal(err)
	}
	tools := []*schema.ToolInfo{{Name: "get_weather", Desc: "weather"}}
	withTools, err := m.WithTools(tools)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := withTools.Generate(context.Background(), []*schema.Message{schema.UserMessage("weather in paris?")})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("tool call not converted: %+v", msg.ToolCalls)
	}
	if !strings.Contains(msg.ToolCalls[0].Function.Arguments, "Paris") {
		t.Fatalf("unexpected arguments %q", msg.ToolCalls[0].Function.Arguments)
	}
}
