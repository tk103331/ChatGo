package ui

import (
	"strings"

	mcore "chatgo/internal/ui/uikit"

	"chatgo/pkg/models"
)

// checkpointEnabled reports whether the experimental auto-checkpoint feature
// is on. AgentCheckpoint holds "turn" or "batch"; anything else is off.
func (cw *ChatWindow) checkpointEnabled() bool {
	return cw.cfg.AgentCheckpoint == "turn" || cw.cfg.AgentCheckpoint == "batch"
}

// checkpointCommit is the current assistant message's commit; the message ID
// is unique in the conversation, so the chat page can locate the checkpoint
// of a given reply with a single --grep.
func checkpointCommit(msgID, summary string) string {
	return "checkpoint msg=" + msgID + ": " + summary
}

// autoCommitCheckpoint commits the conversation workspace's current changes
// as one checkpoint tied to msgID. It does nothing when the feature is off,
// the conversation has no workspace, or nothing changed since the last
// commit. The commit needs git identity; when the repository has none, one
// is supplied per-command so the commit never fails silently.
func (cw *ChatWindow) autoCommitCheckpoint(conv *models.Conversation, msgID string, tools []string) {
	if !cw.checkpointEnabled() {
		return
	}
	if conv == nil || conv.Workspace == "" {
		return
	}
	if gitOut(conv.Workspace, "status", "--porcelain") == "" {
		return
	}
	summary := strings.Join(tools, ", ")
	if summary == "" {
		summary = "agent checkpoint"
	}
	if _, err := gitRun(conv.Workspace, "add", "-A"); err != nil {
		return
	}
	args := []string{"commit", "-m", checkpointCommit(msgID, summary)}
	if gitOut(conv.Workspace, "config", "user.name") == "" {
		args = append([]string{"-c", "user.name=ChatGo", "-c", "user.email=chatgo@local"}, args...)
	}
	if _, err := gitRun(conv.Workspace, args...); err != nil {
		return
	}
	cw.toast("已生成 checkpoint", mcore.SeveritySuccess)
}

// findCheckpointCommit returns the newest checkpoint commit created for
// msgID in dir, or "" when that reply never committed its changes.
func findCheckpointCommit(dir, msgID string) string {
	out := gitOut(dir, "log", "-1", "--grep="+checkpointGrep(msgID), "--format=%H")
	return out
}

// checkpointGrep is the --grep pattern that matches only the checkpoint of a
// given message, anchored so the pattern cannot match a check in someone
// else's commit subject.
func checkpointGrep(msgID string) string {
	return "checkpoint msg=" + msgID
}

// resetToCheckpoint resets dir's current branch hard to sha, discarding any
// work after the checkpoint.
func resetToCheckpoint(dir, sha string) error {
	_, err := gitRun(dir, "reset", "--hard", sha)
	return err
}
