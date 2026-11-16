package ui

// End-to-end regression for the first-reply greeting bug: acpChatStart used
// to clear the session's greeting mark only after Prompt returned, so every
// session's first streamed reply was dropped as a "greeting". The test runs
// a real turn against the acp-go-sdk example agent, which streams its
// answer while the Prompt call is still blocking — the exact timing the bug
// needs.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// exampleAgentBin builds the SDK's example agent once for the test binary's
// run and returns its path. It speaks a full turn: streamed text, a tool
// call, a permission request.
func exampleAgentBin(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("CHATGO_TEST_ACP_AGENT"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin // prebuilt by the developer or CI
		}
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "example-agent")
	build := exec.Command("go", "build", "-o", bin,
		"github.com/coder/acp-go-sdk/example/agent")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Skipf("the example agent cannot be built (no module graph in sandbox): %v: %s", err, out)
	}
	return bin
}

// The first turn's streamed text must land on the reply: the greeting mark
// has to clear before the Prompt request is sent, not after it returns.
func TestACPFirstReplySurvivesTheGreetingWindow(t *testing.T) {
	bin := exampleAgentBin(t)
	cw := testWindow()
	installQueue(t, cw)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.mode = modeChat
	// The connection runs the example agent, which answers from its own
	// process with no model behind it.
	cw.cfg.ACPAgents = []config.ACPConnection{{
		Name: "ExampleAgent", Command: bin, Args: nil,
	}}

	conv := &models.Conversation{ID: "c1", Title: "t", Agent: "acp:ExampleAgent",
		Workspace: t.TempDir()}
	conv.Messages = append(conv.Messages, models.Message{
		ID: "reply-1", Role: "assistant", AuthorID: "acp:ExampleAgent",
		AuthorName: "ExampleAgent", Timestamp: time.Now(),
	})
	cw.cur = conv

	cw.acpChatStart(conv, "ExampleAgent", 0, "hello")

	// The turn streams for a few seconds; wait for the reply to carry text.
	// The greeting bug left Content empty forever, so the wait fails on
	// timeout rather than fast.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		q := queueOf(cw)
		if q != nil {
			q.drain()
		}
		if len(conv.Messages) > 0 && strings.Contains(conv.Messages[len(conv.Messages)-1].Content, "demo only") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The session process is cleaned up after the check.
	defer cw.acpShutdown()

	last := conv.Messages[len(conv.Messages)-1]
	if !strings.Contains(last.Content, "demo only") {
		t.Fatalf("the first reply was dropped by the greeting window; content = %q", last.Content)
	}
	if len(last.Segments) == 0 {
		t.Fatal("the first reply should carry its streamed segments")
	}
	// The mark itself is gone, so a later turn is never muted either.
	cw.acp.mu.Lock()
	stillMarked := false
	for _, v := range cw.acp.greeting {
		if v {
			stillMarked = true
		}
	}
	cw.acp.mu.Unlock()
	if stillMarked {
		t.Fatal("the greeting mark should be cleared once the turn ran")
	}
}
