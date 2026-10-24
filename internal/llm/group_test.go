package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgo/internal/config"
)

func TestGroupFallbackRoutes(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer failing.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := `{"id":"x","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`
		_, _ = w.Write([]byte("data: " + chunk + "\n\ndata: [DONE]\n\n"))
	}))
	defer ok.Close()

	providers := []config.Provider{
		{Name: "bad", Type: config.ProviderTypeCustom, APIType: "openai", BaseURL: failing.URL},
		{Name: "good", Type: config.ProviderTypeCustom, APIType: "openai", BaseURL: ok.URL},
	}
	group := config.ProviderGroup{
		Name:     "g",
		Strategy: config.StrategyFallback,
		Enabled:  true,
		Members:  []config.GroupMember{{Provider: "bad", Models: []string{"m"}}, {Provider: "good", Models: []string{"m"}}},
	}
	gc, err := NewGroupClient(group, providers)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		resp, err := gc.ChatNonBlocking(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		if resp == nil || resp.Content != "ok" {
			t.Fatalf("unexpected reply %+v", resp)
		}
	}
}

func TestGroupRoundRobinOrder(t *testing.T) {
	providers := []config.Provider{
		{Name: "a", Type: config.ProviderTypeCustom, APIType: "openai"},
		{Name: "b", Type: config.ProviderTypeCustom, APIType: "openai"},
	}
	group := config.ProviderGroup{
		Name:     "g",
		Strategy: config.StrategyRoundRobin,
		Members:  []config.GroupMember{{Provider: "a", Models: []string{"m"}}, {Provider: "b", Models: []string{"m"}}},
	}
	gc, err := NewGroupClient(group, providers)
	if err != nil {
		t.Fatal(err)
	}
	routes := gc.routes("")
	first := routes[0].provider
	second := routes[1].provider
	if first != "a" || second != "b" {
		t.Fatalf("round robin order %q,%q", first, second)
	}
}

func TestGroupAllMembersFail(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer failing.Close()
	providers := []config.Provider{{Name: "bad", Type: config.ProviderTypeCustom, APIType: "openai", BaseURL: failing.URL}}
	group := config.ProviderGroup{
		Name:    "g",
		Members: []config.GroupMember{{Provider: "bad", Models: []string{"m"}}},
	}
	gc, err := NewGroupClient(group, providers)
	if err != nil {
		t.Fatal(err)
	}
	_, err = gc.ChatNonBlocking(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "provider group") {
		t.Fatalf("expected group error, got %v", err)
	}
}

func TestGroupWithoutUsableMembers(t *testing.T) {
	group := config.ProviderGroup{Name: "g", Members: []config.GroupMember{{Provider: "missing"}}}
	if _, err := NewGroupClient(group, nil); err == nil {
		t.Fatal("expected error for group without usable members")
	}
}
