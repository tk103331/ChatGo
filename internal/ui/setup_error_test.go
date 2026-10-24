package ui

import (
	"strings"
	"testing"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"
)

// TestModelSetupErrorStaysOutOfUnrelatedTabs guards the bug where a model
// problem was written to cw.status, which the shortcut tab also rendered: the
// deepseek API key error showed up under 快捷键.
func TestModelSetupErrorStaysOutOfUnrelatedTabs(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{
		{Name: "DeepSeek", Type: "deepseek", Model: "deepseek-chat", Enabled: true},
	}
	cw.provider = "DeepSeek"
	cw.model = "deepseek-chat"
	cw.setupProvider("DeepSeek")

	msg := cw.modelSetupError()
	if msg == "" {
		t.Fatal("a provider without an API key should report a setup error")
	}
	if !strings.Contains(msg, "API Key") {
		t.Fatalf("the message should name the API key, got %q", msg)
	}
	for _, leak := range []string{"authToken", "DEEPSEEK_API_KEY", "environment variable"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("the message leaks SDK wording %q: %s", leak, msg)
		}
	}
	if cw.status != "" {
		t.Fatalf("a model problem must not land in cw.status, got %q", cw.status)
	}

	// The shortcut tab renders only its own failures.
	cw.section = sectionSettings
	cw.settings.tab = 4
	if img := mgo.Render(cw.view, 1000, 700, 1); img == nil {
		t.Fatal("shortcut tab render failed")
	}
	if cw.settings.shortcut.err != "" {
		t.Fatalf("the shortcut tab should have no error of its own, got %q", cw.settings.shortcut.err)
	}
}

func TestModelSetupErrorClearsWhenConfigured(t *testing.T) {
	cw := testWindow()
	cw.cfg.Providers = []config.Provider{
		{Name: "OpenAI", Type: "openai", Model: "gpt-4o", Enabled: true},
	}
	cw.provider, cw.model = "OpenAI", "gpt-4o"
	cw.setupProvider("OpenAI")
	if cw.modelSetupError() == "" {
		t.Fatal("expected an error before the key is set")
	}
	cw.cfg.Providers[0].APIKey = "sk-test"
	cw.setupProvider("OpenAI")
	if got := cw.modelSetupError(); got != "" {
		t.Fatalf("the error should clear once configured, got %q", got)
	}
	if cw.llmClient == nil {
		t.Fatal("a configured provider should build a client")
	}
}
