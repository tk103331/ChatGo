package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"
)

// headCount counts the commits on the repository's current branch.
func headCount(t *testing.T, dir string) int {
	t.Helper()
	out := gitOut(dir, "rev-list", "--count", "HEAD")
	if out == "" {
		return 0
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		t.Fatalf("rev-list count %q: %v", out, err)
	}
	return n
}

// checkpointWindow builds a window with the checkpoint level set.
func checkpointWindow(t *testing.T, level string) *ChatWindow {
	t.Helper()
	cw := testWindow()
	cw.cfg.AgentCheckpoint = level
	return cw
}

// writeFile creates a file and commits nothing, leaving it as a working-tree
// change the checkpoint can pick up.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointOffCommitsNothing(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "off")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.autoCommitCheckpoint(conv, "m1", []string{"edit"})
	if headCount(t, dir) != before {
		t.Fatalf("off level still committed; commits %d -> %d", before, headCount(t, dir))
	}
}

func TestCheckpointBatchCommitsOnce(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.autoCommitCheckpoint(conv, "m1", []string{"edit"})
	if headCount(t, dir) != before+1 {
		t.Fatalf("expected one commit, got %d -> %d", before, headCount(t, dir))
	}
	msg := gitOut(dir, "log", "-1", "--format=%s")
	if !strings.Contains(msg, "checkpoint msg=m1") {
		t.Fatalf("commit subject %q lacks the checkpoint marker", msg)
	}
}

func TestCheckpointSkipsCleanTree(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}

	before := headCount(t, dir)
	cw.autoCommitCheckpoint(conv, "m1", []string{"edit"})
	if headCount(t, dir) != before {
		t.Fatalf("clean tree still committed; commits %d -> %d", before, headCount(t, dir))
	}
}

func TestCheckpointFallsBackIdentity(t *testing.T) {
	dir := newTestRepo(t)
	_, _ = gitRun(dir, "config", "--unset", "user.name")
	_, _ = gitRun(dir, "config", "--unset", "user.email")
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.autoCommitCheckpoint(conv, "m1", []string{"edit"})
	if headCount(t, dir) != before+1 {
		t.Fatalf("identity fallback did not commit; %d -> %d", before, headCount(t, dir))
	}
}

// TestBatchTickParallelOneCommit drives one parallel batch (two tools opened
// before either finishes) and expects a single checkpoint when both end.
func TestBatchTickParallelOneCommit(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	cw.acp.convs[conv.ID] = &acpConv{convID: conv.ID, batch: map[string]bool{}}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.acpBatchTick(conv, "m1", "t1", "read", "in_progress")
	cw.acpBatchTick(conv, "m1", "t2", "edit", "in_progress")
	cw.acpBatchTick(conv, "m1", "t1", "read", "completed")
	if headCount(t, dir) != before {
		t.Fatalf("half-finished batch committed early: %d -> %d", before, headCount(t, dir))
	}
	cw.acpBatchTick(conv, "m1", "t2", "edit", "completed")
	if headCount(t, dir) != before+1 {
		t.Fatalf("parallel batch should commit once, got %d -> %d", before, headCount(t, dir))
	}
}

// TestBatchTickSerialTwoBatches expects a checkpoint per finished single-tool
// batch, matching one tool call followed by the agent reading the result.
func TestBatchTickSerialTwoBatches(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	cw.acp.convs[conv.ID] = &acpConv{convID: conv.ID, batch: map[string]bool{}}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.acpBatchTick(conv, "m1", "t1", "edit", "in_progress")
	cw.acpBatchTick(conv, "m1", "t1", "edit", "completed")
	if headCount(t, dir) != before+1 {
		t.Fatalf("first serial batch should commit, got %d -> %d", before, headCount(t, dir))
	}
	writeFile(t, dir, "b.txt", "b2\n")
	cw.acpBatchTick(conv, "m1", "t2", "edit", "in_progress")
	cw.acpBatchTick(conv, "m1", "t2", "edit", "completed")
	if headCount(t, dir) != before+2 {
		t.Fatalf("second serial batch should commit, got %d -> %d", before, headCount(t, dir))
	}
}

// TestTurnLevelCommitsWholeReply checks that under "turn" a batch finishing
// does not commit, and the turn flush commits once with the reply's tools.
func TestTurnLevelCommitsWholeReply(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "turn")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	cw.acp.convs[conv.ID] = &acpConv{convID: conv.ID, batch: map[string]bool{}}
	conv.Messages = []models.Message{{
		ID: "m1", Role: "assistant",
		ToolCalls: []models.ToolCall{{ID: "t1", Name: "read"}, {ID: "t2", Name: "edit"}},
	}}
	writeFile(t, dir, "b.txt", "b\n")

	before := headCount(t, dir)
	cw.acpBatchTick(conv, "m1", "t1", "read", "in_progress")
	cw.acpBatchTick(conv, "m1", "t2", "edit", "in_progress")
	cw.acpBatchTick(conv, "m1", "t1", "read", "completed")
	cw.acpBatchTick(conv, "m1", "t2", "edit", "completed")
	if headCount(t, dir) != before {
		t.Fatalf("turn level committed per batch: %d -> %d", before, headCount(t, dir))
	}
	cw.acpFlushTurn(conv, "m1")
	if headCount(t, dir) != before+1 {
		t.Fatalf("turn flush should commit once, got %d -> %d", before, headCount(t, dir))
	}
	msg := gitOut(dir, "log", "-1", "--format=%s")
	if !strings.Contains(msg, "read, edit") {
		t.Fatalf("turn commit %q lacks the tool summary", msg)
	}
}

// TestCheckpointFindAndReset rolls two checkpoints, then resets to the first
// and expects the tree to match its state.
func TestCheckpointFindAndReset(t *testing.T) {
	dir := newTestRepo(t)
	cw := checkpointWindow(t, "batch")
	conv := &models.Conversation{ID: "c1", Workspace: dir}
	writeFile(t, dir, "b.txt", "b\n")
	cw.autoCommitCheckpoint(conv, "m1", []string{"edit"})
	sha1 := findCheckpointCommit(dir, "m1")
	if sha1 == "" {
		t.Fatal("no checkpoint for m1")
	}
	writeFile(t, dir, "b.txt", "c\n")
	writeFile(t, dir, "d.txt", "d\n")
	cw.autoCommitCheckpoint(conv, "m2", []string{"edit"})

	if got := findCheckpointCommit(dir, "m2"); got == "" || got == sha1 {
		t.Fatalf("m2 checkpoint missing or same as m1: %q vs %q", got, sha1)
	}
	if err := resetToCheckpoint(dir, sha1); err != nil {
		t.Fatalf("reset: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	if err != nil || string(data) != "b\n" {
		t.Fatalf("b.txt after reset = %q, want %q (err %v)", data, "b\n", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "d.txt")); !os.IsNotExist(err) {
		t.Fatalf("d.txt survived the reset: %v", err)
	}
}

// The chat page shows the rewind control on an assistant reply once the
// experimental checkpoint level is on, and hides it when off.
func TestChatShowsRewindButtonPerCheckpointLevel(t *testing.T) {
	for _, level := range []string{"turn", "batch"} {
		cw := checkpointWindow(t, level)
		cw.mode = modeChat
		cw.cur = &models.Conversation{ID: "c1", Title: "t",
			Messages: []models.Message{{ID: "m1", Role: "assistant", Content: "ok"}}}
		cw.convs = []models.Conversation{*cw.cur}
		cw.focusConvTab(cw.cur)
		tr := mgo.NewTester(cw.view, 1000, 700)
		for range 5 {
			tr.Frame()
		}
		if _, ok := tr.Find("回退到此点"); !ok {
			t.Fatalf("level %q: rewind control missing; texts: %v", level, tr.Texts())
		}
	}

	cw := checkpointWindow(t, "off")
	cw.mode = modeChat
	cw.cur = &models.Conversation{ID: "c1", Title: "t",
		Messages: []models.Message{{ID: "m1", Role: "assistant", Content: "ok"}}}
	cw.convs = []models.Conversation{*cw.cur}
	cw.focusConvTab(cw.cur)
	tr := mgo.NewTester(cw.view, 1000, 700)
	for range 5 {
		tr.Frame()
	}
	if _, ok := tr.Find("回退到此点"); ok {
		t.Fatalf("off level still shows the rewind control; texts: %v", tr.Texts())
	}
}
