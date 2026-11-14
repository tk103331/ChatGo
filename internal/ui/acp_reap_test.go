package ui

import (
	"testing"

	"chatgo/pkg/models"
)

// A CLI that dies mid-reply must leave no spinner behind: every running
// stream of its conversation ends, and an empty reply says what happened
// instead of waiting for a process that will never answer.
func TestACPDeadTurnFailsTheRunningReply(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	cm, err := models.NewConversationManagerAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cw.convManager = cm
	conv := &models.Conversation{ID: "c1", Title: "t", Agent: "acp:PiAgent",
		Messages: []models.Message{{ID: "r1", Role: "assistant"}}}
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatal(err)
	}
	cw.cur = conv
	cw.beginStream("c1", "r1", nil)

	cw.acpFailDeadTurns("c1", conv)

	if cw.streamingMsg("r1") {
		t.Fatal("the stream of a dead CLI should have ended")
	}
	if got := conv.Messages[0].Content; got != "⚠️ ACP 进程意外退出，请重试" {
		t.Fatalf("the empty reply should say the CLI exited, got %q", got)
	}
	// The conversation was saved with the failure, so a restart shows it.
	reloaded, err := cm.LoadConversation("c1")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Messages[0].Content == "" {
		t.Fatal("the failed reply was not saved")
	}
	// A stream of another conversation is left alone.
	cw.beginStream("c2", "r2", nil)
	cw.acpFailDeadTurns("c1", conv)
	if !cw.streamingMsg("r2") {
		t.Fatal("another conversation's stream must not be touched")
	}
}

// Only a session still registered died on its own: a reap after the app
// closed the connection itself (acpConvClose, acpShutdown) must not fail
// any turn.
func TestACPReapOnlyFailsTurnsOnUnexpectedExit(t *testing.T) {
	isolateHome(t)
	cw := testWindow()
	installQueue(t, cw)
	conv := &models.Conversation{ID: "c1", Title: "t",
		Messages: []models.Message{{ID: "r1", Role: "assistant"}}}
	cw.cur = conv
	cw.beginStream("c1", "r1", nil)
	// Registered: the exit was unexpected, the turn fails.
	rt := &acpConv{key: "c1", convID: "c1", sessionID: "s1"}
	cw.acp.convs["c1"] = rt
	cw.acpReap(rt)
	drainMain(cw)
	if cw.streamingMsg("r1") {
		t.Fatal("an unexpected exit should fail the running turn")
	}

	// Unregistered first: the app closed it, the turn keeps running as far
	// as the reap is concerned (the close path ends streams itself).
	cw.beginStream("c1", "r1", nil)
	cw.acpReap(rt)
	if !cw.streamingMsg("r1") {
		t.Fatal("a reap of an app-closed session must not touch streams")
	}
}
