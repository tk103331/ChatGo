package ui

import (
	"context"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"chatgo/internal/config"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// The web/phone runtime runs an A2A turn against the configured agent.
func TestWebRunA2AReply(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	card := &sdk.AgentCard{
		Name:               "Test Agent",
		Version:            "1.0",
		Capabilities:       sdk.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		SupportedInterfaces: []*sdk.AgentInterface{
			sdk.NewAgentInterface(srv.URL+"/invoke", sdk.TransportProtocolJSONRPC),
		},
	}
	executor := a2asrv.AgentExecutorFunc(func(context.Context, *a2asrv.ExecutorContext) iter.Seq2[sdk.Event, error] {
		return func(yield func(sdk.Event, error) bool) {
			yield(sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("a2a ok")), nil)
		}
	})
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(executor)))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))

	cw := testWindow()
	cw.cfg.A2AAgents = []config.A2AConnection{{Name: "Local", URL: srv.URL}}
	conv := &models.Conversation{
		ID: "c1", Title: "t", Agent: "a2a:Local",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "hi"},
			{ID: "r1", Role: "assistant"},
		},
	}
	if err := cw.webRunA2A(context.Background(), conv, 1, webui.RunRequest{Agent: "a2a:Local"}, aguiLive{conv: conv}); err != nil {
		t.Fatal(err)
	}
	if got := conv.Messages[1].Content; got != "a2a ok" {
		t.Fatalf("reply = %q", got)
	}
}
