package ui

import (
	"strings"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

// Node states of the turn graph. A node is one member's slot in the running
// or finished turn; the state decides its color and badge.
const (
	roomNodeDone     = "done"
	roomNodeSpeaking = "speaking"
	roomNodeQueued   = "queued"
	roomNodePassed   = "passed"
	roomNodeIdle     = "idle"
)

// roomGraphNode is one member in the detail panel's execution graph.
type roomGraphNode struct {
	Agent string
	Name  string
	State string
	// MsgID is the member's reply in the current turn, empty for one that
	// has not spoken or chose silence.
	MsgID string
}

// roomTurnStart is the index of the current turn's first message: the last
// human message. A conversation with no human message yet is one turn.
func roomTurnStart(conv *models.Conversation) int {
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			return i
		}
	}
	return 0
}

// roomTurnBounds is the [start, end) range of the turn holding the anchor
// message: its human message up to the next one. An empty or unknown anchor
// means the newest turn, so the panel follows the conversation by default.
func roomTurnBounds(conv *models.Conversation, anchorID string) (int, int) {
	at := -1
	if anchorID != "" {
		for i := range conv.Messages {
			if conv.Messages[i].ID == anchorID {
				at = i
				break
			}
		}
	}
	if at < 0 {
		return roomTurnStart(conv), len(conv.Messages)
	}
	start := 0
	for i := at; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			start = i
			break
		}
	}
	end := len(conv.Messages)
	for i := at + 1; i < len(conv.Messages); i++ {
		if conv.Messages[i].IsHuman() {
			end = i
			break
		}
	}
	return start, end
}

// roomGraphNodes builds the execution graph of one turn: who ran, who is
// running, who still waits. The runtime state applies only to the newest
// turn of a running room; any other turn is read back from its messages, so
// history and restarts draw the same graph.
func (cw *ChatWindow) roomGraphNodes(r config.Room, conv *models.Conversation, start, end int) []roomGraphNode {
	replies := map[string]string{}
	for _, msg := range conv.Messages[start:end] {
		if msg.AuthorKind == models.AuthorKindAgent && msg.AuthorID != "" {
			replies[msg.AuthorID] = msg.ID
		}
	}
	live := cw.room.running == r.ID && end == len(conv.Messages)
	order := roomRepliedOrder(conv, start, end)
	if live {
		order = cw.room.turnOrder
	}
	nodes := make([]roomGraphNode, 0, len(order))
	for _, agent := range order {
		n := roomGraphNode{Agent: agent, Name: roomAgentLabel(r, agent), MsgID: replies[agent]}
		switch {
		case live && cw.room.speaking == agent:
			n.State = roomNodeSpeaking
		case live && cw.roomQueuePos(agent) > 0:
			n.State = roomNodeQueued
		case live && cw.room.passed[agent]:
			n.State = roomNodePassed
		case n.MsgID != "":
			n.State = roomNodeDone
		default:
			n.State = roomNodeIdle
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// roomRepliedOrder is the agents that answered inside the turn, in the order
// they spoke.
func roomRepliedOrder(conv *models.Conversation, start, end int) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, msg := range conv.Messages[start:end] {
		if msg.AuthorKind != models.AuthorKindAgent || msg.AuthorID == "" || seen[msg.AuthorID] {
			continue
		}
		seen[msg.AuthorID] = true
		out = append(out, msg.AuthorID)
	}
	return out
}

// roomAgentLabel is the member's display name in the room, falling back to
// the agent reference's own name for one no longer on the roster.
func roomAgentLabel(r config.Room, agent string) string {
	for _, m := range r.Members {
		if m.Agent == agent {
			return roomMemberName(m)
		}
	}
	return authorNameOf(agent)
}

// roomNodeWord is the state's reading on the node row.
func roomNodeWord(state string) string {
	switch state {
	case roomNodeSpeaking:
		return "发言中"
	case roomNodeQueued:
		return "排队中"
	case roomNodePassed:
		return "沉默"
	case roomNodeDone:
		return "已完成"
	default:
		return ""
	}
}

// roomTurnPrompt is the user message that opened the turn, its first line
// standing for the whole prompt at the head of the graph.
func roomTurnPrompt(conv *models.Conversation, start int) string {
	if start >= len(conv.Messages) || !conv.Messages[start].IsHuman() {
		return ""
	}
	for _, line := range strings.Split(conv.Messages[start].Content, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

// roomTurnPromptID is the id of the message that opened the current turn,
// which a member's reply quotes.
func roomTurnPromptID(conv *models.Conversation) string {
	start := roomTurnStart(conv)
	if start >= len(conv.Messages) || !conv.Messages[start].IsHuman() {
		return ""
	}
	return conv.Messages[start].ID
}
