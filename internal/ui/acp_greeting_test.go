package ui

import (
	"strings"
	"sync"
	"testing"

	acp "github.com/coder/acp-go-sdk"

	"chatgo/pkg/models"
)

// acpTextUpdate builds the agent-text notification an adapter such as
// PiAgent sends to introduce itself.
func acpTextUpdate(sess acp.SessionId, text string) acp.SessionNotification {
	return acp.SessionNotification{
		SessionId: sess,
		Update: acp.SessionUpdate{AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
			Content: acp.ContentBlock{Text: &acp.ContentBlockText{Text: text}},
		}},
	}
}

// A greeting that arrives before the first prompt is dropped, so no stray
// assistant message shows up ahead of the real reply.
func TestACPGreetingIsDropped(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	cw.mode = modeChat
	conv := &models.Conversation{ID: "c1", Title: "t", Agent: "acp:PiAgent", Workspace: "/tmp/demo"}
	cw.cur = conv

	sess := acp.SessionId("sess-1")
	cw.acp.mu.Lock()
	cw.acp.convBySess[sess] = conv.ID
	cw.acp.convObj[conv.ID] = conv
	cw.acp.greeting[sess] = true
	cw.acp.mu.Unlock()

	client := acpClient{cw: cw}
	if err := client.SessionUpdate(t.Context(), acpTextUpdate(sess, "pi v1.3.0\n---\n## Skills\n")); err != nil {
		t.Fatalf("SessionUpdate: %v", err)
	}
	drainMain(cw)

	if len(conv.Messages) != 0 {
		t.Fatalf("the greeting became %d message(s), want none", len(conv.Messages))
	}

	// Once the first prompt clears the mark, the same update lands on the
	// conversation as the reply it belongs to.
	cw.acp.mu.Lock()
	delete(cw.acp.greeting, sess)
	cw.acp.mu.Unlock()
	if err := client.SessionUpdate(t.Context(), acpTextUpdate(sess, "真正的回复")); err != nil {
		t.Fatalf("SessionUpdate after prompt: %v", err)
	}
	drainMain(cw)
	if len(conv.Messages) != 1 {
		t.Fatalf("expected one assistant message, got %d", len(conv.Messages))
	}
	if got := conv.Messages[0].Content; got != "真正的回复" {
		t.Fatalf("content is %q, want the reply", got)
	}
}

// A greeting is dropped from a channel reply as well, so the adapter's
// self-introduction never reaches the channel.
func TestACPGreetingIsDroppedFromChannelReply(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	cw.mode = modeChat
	conv := &models.Conversation{ID: "c1", Title: "t", Agent: "acp:PiAgent", Workspace: "/tmp/demo"}
	cw.cur = conv

	sess := acp.SessionId("sess-2")
	cw.acp.mu.Lock()
	cw.acp.convBySess[sess] = conv.ID
	cw.acp.convObj[conv.ID] = conv
	cw.acp.greeting[sess] = true
	buf := &strings.Builder{}
	cw.acp.channelReply[sess] = buf
	cw.acp.mu.Unlock()

	client := acpClient{cw: cw}
	if err := client.SessionUpdate(t.Context(), acpTextUpdate(sess, "pi v1.3.0")); err != nil {
		t.Fatalf("SessionUpdate: %v", err)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("the channel reply holds %q, want nothing", got)
	}
	if len(conv.Messages) != 0 {
		t.Fatalf("the greeting became %d message(s), want none", len(conv.Messages))
	}
}

// Reaping a dead session clears its greeting mark, so a restarted CLI is not
// left muted.
func TestACPReapClearsGreeting(t *testing.T) {
	cw := testWindow()
	rt := &acpConv{convID: "c1", sessionID: acp.SessionId("sess-3")}
	cw.acp.mu.Lock()
	cw.acp.convs["c1"] = rt
	cw.acp.convBySess[rt.sessionID] = "c1"
	cw.acp.greeting[rt.sessionID] = true
	cw.acp.mu.Unlock()

	cw.acpReap(rt)

	cw.acp.mu.Lock()
	_, hasSess := cw.acp.convBySess[rt.sessionID]
	_, hasGreeting := cw.acp.greeting[rt.sessionID]
	cw.acp.mu.Unlock()
	if hasSess || hasGreeting {
		t.Fatalf("reap left session=%v greeting=%v, want both cleared", hasSess, hasGreeting)
	}
}

// drainMain runs the main-thread callbacks the client queued. The real
// window delivers them through window.Update; a test queues them and runs
// them on its own goroutine.
func drainMain(cw *ChatWindow) {
	if q := queueOf(cw); q != nil {
		q.drain()
	}
}

// queueOf is the main queue installed for cw by installQueue.
var (
	queueMu sync.Mutex
	queues  = map[*ChatWindow]*mainQueue{}
)

func queueOf(cw *ChatWindow) *mainQueue {
	queueMu.Lock()
	defer queueMu.Unlock()
	return queues[cw]
}

// installQueue routes cw's postMain callbacks into a queue the test drains,
// standing in for the window's main thread.
func installQueue(t *testing.T, cw *ChatWindow) *mainQueue {
	q := &mainQueue{}
	cw.dispatch = q.post
	queueMu.Lock()
	queues[cw] = q
	queueMu.Unlock()
	t.Cleanup(func() {
		queueMu.Lock()
		delete(queues, cw)
		queueMu.Unlock()
	})
	return q
}
