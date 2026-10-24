package llm

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"chatgo/internal/config"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/qwen"
	einoopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
)

// modelTypeName names the concrete chat model a provider was built with, so a
// test can assert which protocol client is in use.
func modelTypeName(c *Client) string {
	if c == nil || c.model == nil {
		return "<nil>"
	}
	return reflect.TypeOf(c.model).String()
}

// TestProviderTypesUseTheirOwnClient guards the regression where every
// non-openai provider type was routed through a conversion layer and failed
// with "unsupported custom api_type".
func TestProviderTypesUseTheirOwnClient(t *testing.T) {
	want := map[string]any{
		"openai":   &einoopenai.Client{},
		"deepseek": &deepseek.ChatModel{},
		"qwen":     &qwen.ChatModel{},
		"claude":   &claude.ChatModel{},
		"ollama":   &ollama.ChatModel{},
		"gemini":   &gemini.ChatModel{},
	}
	for typ, exemplar := range want {
		p := config.Provider{Name: typ, Type: typ, APIKey: "test-key", Model: "test-model"}
		c, err := NewClient(p)
		if err != nil {
			t.Fatalf("provider type %q failed to build: %v", typ, err)
		}
		if got, expect := modelTypeName(c), reflect.TypeOf(exemplar).String(); got != expect {
			t.Fatalf("provider type %q built %s, want %s", typ, got, expect)
		}
	}
}

// TestCustomAPITypeSelectsProtocolClient checks a custom provider is built
// with the client of the protocol it declares, with no conversion in between.
func TestCustomAPITypeSelectsProtocolClient(t *testing.T) {
	cases := map[string]any{
		"openai":           &einoopenai.Client{},
		"claude":           &claude.ChatModel{},
		"gemini":           &gemini.ChatModel{},
		"openai_responses": &responsesChatModel{},
	}
	for apiType, exemplar := range cases {
		p := config.Provider{
			Name: "c", Type: config.ProviderTypeCustom, APIType: apiType,
			APIKey: "k", Model: "m", BaseURL: "https://example.invalid/v1",
		}
		c, err := NewClient(p)
		if err != nil {
			t.Fatalf("api_type %q failed to build: %v", apiType, err)
		}
		if got, expect := modelTypeName(c), reflect.TypeOf(exemplar).String(); got != expect {
			t.Fatalf("api_type %q built %s, want %s", apiType, got, expect)
		}
	}
}

// TestUnknownAPITypeIsRejected keeps an unsupported api_type from silently
// falling back to another protocol.
func TestUnknownAPITypeIsRejected(t *testing.T) {
	p := config.Provider{
		Name: "c", Type: config.ProviderTypeCustom, APIType: "no-such-protocol",
		APIKey: "k", Model: "m",
	}
	_, err := NewClient(p)
	if err == nil {
		t.Fatal("an unknown api_type must be rejected")
	}
	if !strings.Contains(err.Error(), "no-such-protocol") {
		t.Fatalf("the error should name the api_type, got %v", err)
	}
}

// TestNewClientRejectsMissingAPIKey guards the raw SDK message users saw:
// "failed to create deepseek client: authToken is empty and DEEPSEEK_API_KEY
// environment variable is not set".
func TestNewClientRejectsMissingAPIKey(t *testing.T) {
	for _, typ := range []string{"openai", "deepseek", "qwen", "claude", "anthropic", "gemini"} {
		_, err := NewClient(config.Provider{Name: "线上", Type: typ, Model: "m"})
		if err == nil {
			t.Fatalf("%s without an API key must fail", typ)
		}
		msg := err.Error()
		for _, leak := range []string{"authToken", "DEEPSEEK_API_KEY", "environment variable", "genai.ClientConfig", "failed to create"} {
			if strings.Contains(msg, leak) {
				t.Fatalf("%s message leaks SDK wording %q: %s", typ, leak, msg)
			}
		}
		if !strings.Contains(msg, "线上") {
			t.Fatalf("%s message should name the provider: %s", typ, msg)
		}
	}
}

func TestSetupErrorKeepsOneLine(t *testing.T) {
	p := config.Provider{Name: "代理", Type: "openai"}
	err := setupError(p, errors.New("boom happened\nstack line 1\nstack line 2"))
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("a multi-line SDK error must be trimmed: %q", err)
	}
	if !strings.Contains(err.Error(), "代理") || !strings.Contains(err.Error(), "boom happened") {
		t.Fatalf("unexpected message %q", err)
	}
	// A credential failure is reported as such, whatever the SDK calls it.
	err = setupError(p, errors.New("invalid api key provided"))
	if !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("a credential error should mention the API key: %q", err)
	}
	if setupError(p, nil) != nil {
		t.Fatal("a nil error must stay nil")
	}
}
