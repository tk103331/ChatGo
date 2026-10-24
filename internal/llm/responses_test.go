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

// responsesServer serves one canned Responses reply and records the request.
func responsesServer(t *testing.T, body string, got *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/responses") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got != nil {
			_ = json.NewDecoder(r.Body).Decode(got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestResponsesGenerateReadsOutputAndUsage(t *testing.T) {
	var sent map[string]any
	srv := responsesServer(t, `{
	  "id": "resp_1", "object": "response", "model": "gpt-4.1", "status": "completed",
	  "output": [{"type":"message","id":"m1","role":"assistant","status":"completed",
	    "content":[{"type":"output_text","text":"你好","annotations":[]}]}],
	  "usage": {"input_tokens": 7, "output_tokens": 3, "total_tokens": 10,
	    "input_tokens_details": {"cached_tokens": 0},
	    "output_tokens_details": {"reasoning_tokens": 0}}
	}`, &sent)
	defer srv.Close()

	m, err := newResponsesChatModel(config.Provider{
		Name: "resp", Type: config.ProviderTypeCustom, APIType: "openai_responses",
		APIKey: "k", BaseURL: srv.URL, Model: "gpt-4.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := m.Generate(context.Background(), []*schema.Message{
		schema.SystemMessage("be brief"),
		schema.UserMessage("hi"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "你好" {
		t.Fatalf("content %q", msg.Content)
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		t.Fatal("usage missing")
	}
	if u := msg.ResponseMeta.Usage; u.PromptTokens != 7 || u.CompletionTokens != 3 || u.TotalTokens != 10 {
		t.Fatalf("usage %+v", u)
	}

	// The system message becomes instructions, not an input item.
	if sent["instructions"] != "be brief" {
		t.Fatalf("instructions %v", sent["instructions"])
	}
	items, _ := sent["input"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected one input item, got %v", sent["input"])
	}
	if sent["model"] != "gpt-4.1" {
		t.Fatalf("model %v", sent["model"])
	}
}

func TestResponsesGenerateReadsToolCalls(t *testing.T) {
	var sent map[string]any
	srv := responsesServer(t, `{
	  "id": "resp_2", "object": "response", "model": "gpt-4.1", "status": "completed",
	  "output": [{"type":"function_call","id":"fc1","call_id":"call_1",
	    "name":"get_weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}],
	  "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2,
	    "input_tokens_details": {"cached_tokens": 0},
	    "output_tokens_details": {"reasoning_tokens": 0}}
	}`, &sent)
	defer srv.Close()

	m, _ := newResponsesChatModel(config.Provider{
		Name: "resp", Type: config.ProviderTypeCustom, APIType: "openai_responses",
		APIKey: "k", BaseURL: srv.URL, Model: "gpt-4.1",
	})
	withTools, err := m.WithTools([]*schema.ToolInfo{{Name: "get_weather", Desc: "weather"}})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := withTools.Generate(context.Background(), []*schema.Message{schema.UserMessage("weather?")})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls %+v", msg.ToolCalls)
	}
	call := msg.ToolCalls[0]
	if call.ID != "call_1" || call.Function.Name != "get_weather" {
		t.Fatalf("unexpected call %+v", call)
	}
	if !strings.Contains(call.Function.Arguments, "Paris") {
		t.Fatalf("arguments %q", call.Function.Arguments)
	}
	// The tool must be declared in the request.
	tools, _ := sent["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools not sent: %v", sent["tools"])
	}
}

func TestResponsesStreamForwardsDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		events := []string{
			`{"type":"response.output_text.delta","sequence_number":1,"item_id":"m1","output_index":0,"content_index":0,"delta":"你"}`,
			`{"type":"response.output_text.delta","sequence_number":2,"item_id":"m1","output_index":0,"content_index":0,"delta":"好"}`,
			`{"type":"response.completed","sequence_number":3,"response":{"id":"r","object":"response","model":"gpt-4.1","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`,
		}
		for _, e := range events {
			_, _ = w.Write([]byte("data: " + e + "\n\n"))
		}
	}))
	defer srv.Close()

	m, _ := newResponsesChatModel(config.Provider{
		Name: "resp", Type: config.ProviderTypeCustom, APIType: "openai_responses",
		APIKey: "k", BaseURL: srv.URL, Model: "gpt-4.1",
	})
	sr, err := m.Stream(context.Background(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var usage *schema.TokenUsage
	for {
		msg, err := sr.Recv()
		if err != nil {
			break
		}
		if msg == nil {
			continue
		}
		text.WriteString(msg.Content)
		if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
			usage = msg.ResponseMeta.Usage
		}
	}
	if text.String() != "你好" {
		t.Fatalf("streamed %q", text.String())
	}
	if usage == nil || usage.TotalTokens != 6 {
		t.Fatalf("usage %+v", usage)
	}
}

// TestResponsesChatThroughClient covers the whole path: a custom provider with
// api_type openai_responses answering a Client.Chat call.
func TestResponsesChatThroughClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"response.output_text.delta","sequence_number":1,"item_id":"m","output_index":0,"content_index":0,"delta":"ok"}` + "\n\n"))
	}))
	defer srv.Close()

	c, err := NewClient(config.Provider{
		Name: "resp", Type: config.ProviderTypeCustom, APIType: "openai_responses",
		APIKey: "k", BaseURL: srv.URL, Model: "gpt-4.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var streamed strings.Builder
	resp, err := c.Chat(context.Background(), []ChatMessage{{Role: "user", Content: "hi"}},
		func(chunk string) { streamed.WriteString(chunk) })
	if err != nil {
		t.Fatal(err)
	}
	if streamed.String() != "ok" || resp.Content != "ok" {
		t.Fatalf("streamed %q resp %q", streamed.String(), resp.Content)
	}
}

func TestToolParametersRendersSchema(t *testing.T) {
	// A tool without parameters still needs a valid object schema.
	got, err := toolParameters(&schema.ToolInfo{Name: "noargs"})
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "object" {
		t.Fatalf("schema %v", got)
	}
	// A tool with parameters keeps them.
	info := &schema.ToolInfo{
		Name: "weather",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"city": {Type: schema.String, Desc: "city", Required: true},
		}),
	}
	got, err = toolParameters(info)
	if err != nil {
		t.Fatal(err)
	}
	props, ok := got["properties"].(map[string]any)
	if !ok || props["city"] == nil {
		t.Fatalf("properties missing: %v", got)
	}
}
