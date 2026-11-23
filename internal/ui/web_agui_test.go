package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgo/internal/config"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// aguiLive applies updates straight to the conversation under test.
type aguiLive struct{ conv *models.Conversation }

func (l aguiLive) Update(fn func(*models.Conversation)) { fn(l.conv) }
func (l aguiLive) Ask(context.Context, []webui.AskItem) (string, error) {
	return "", nil
}
func (l aguiLive) Permission(context.Context, webui.PermissionRequest) (webui.PermissionDecision, error) {
	return webui.PermissionDecision{}, nil
}

// The web/phone runtime runs an AG-UI turn through the configured service,
// streams the text and keeps the thread for the next turn.
func TestWebRunAGUIStreamsAndKeepsThread(t *testing.T) {
	var thread string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ThreadID string `json:"threadId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		thread = in.ThreadID
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"delta\":\"你\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"delta\":\"好\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\"}\n\n")
	}))
	defer srv.Close()

	cw := testWindow()
	cw.cfg.AGUIAgents = []config.AGUIConnection{{Name: "Local", URL: srv.URL}}
	conv := &models.Conversation{
		ID: "c1", Title: "t", Agent: "agui:Local",
		Messages: []models.Message{
			{ID: "u1", Role: "user", Content: "hi"},
			{ID: "r1", Role: "assistant"},
		},
	}
	if err := cw.webRunAGUI(context.Background(), conv, 1, webui.RunRequest{Agent: "agui:Local"}, aguiLive{conv: conv}); err != nil {
		t.Fatal(err)
	}
	if got := conv.Messages[1].Content; got != "你好" {
		t.Fatalf("reply = %q", got)
	}
	if conv.AGUIThreadID == "" || conv.AGUIThreadID != thread {
		t.Fatalf("thread = %q, service saw %q", conv.AGUIThreadID, thread)
	}
	if got := conv.Messages[1].Segments; len(got) == 0 || got[0].Kind != "text" || got[0].Text != "你好" {
		t.Fatalf("segments = %+v", conv.Messages[1].Segments)
	}

	// A second turn continues the same thread and a different agent resets it.
	if err := cw.webRunAGUI(context.Background(), conv, 1, webui.RunRequest{Agent: "agui:Other"}, aguiLive{conv: conv}); err == nil ||
		!strings.Contains(err.Error(), "未找到") {
		t.Fatalf("unknown agent err = %v", err)
	}
}
