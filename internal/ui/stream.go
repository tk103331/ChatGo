package ui

import (
	"context"
	"strings"
)

// authorNameOf is the display name of a conversation-agent reference
// ("agent:Name", "acp:Name", "app:Name", or "" for the built-in chat). It is
// stamped onto a reply so the bubble and a group prompt can name the author
// without resolving the reference again later, by which time the agent may
// have been renamed or deleted.
func authorNameOf(agent string) string {
	switch {
	case strings.HasPrefix(agent, "agent:"):
		return strings.TrimPrefix(agent, "agent:")
	case strings.HasPrefix(agent, "acp:"):
		return strings.TrimPrefix(agent, "acp:")
	case strings.HasPrefix(agent, "app:"):
		if a, ok := appByID(strings.TrimPrefix(agent, "app:")); ok {
			return a.Title
		}
		return strings.TrimPrefix(agent, "app:")
	default:
		return chatAgentName
	}
}

// streamState is one reply being written. It is keyed in ChatWindow.streams
// by the id of the message it fills in, because a group conversation answers
// one turn with several replies at once and each needs its own cancel.
type streamState struct {
	convID string
	cancel context.CancelFunc
}

// beginStream records that the reply filling msgID is running, and replaces
// any stream already keyed there. Every caller runs on the main thread, which
// is what makes the bare map access safe: streams is read while drawing and
// written from postMain callbacks, all on that one thread.
func (cw *ChatWindow) beginStream(convID, msgID string, cancel context.CancelFunc) {
	if cw.streams == nil {
		cw.streams = map[string]*streamState{}
	}
	if old := cw.streams[msgID]; old != nil && old.cancel != nil {
		old.cancel()
	}
	cw.streams[msgID] = &streamState{convID: convID, cancel: cancel}
}

// endStream forgets the reply filling msgID, without cancelling: the caller
// is the finishing stream itself.
func (cw *ChatWindow) endStream(msgID string) {
	delete(cw.streams, msgID)
}

// streamingMsg reports whether the reply filling msgID is still running.
func (cw *ChatWindow) streamingMsg(msgID string) bool {
	return cw.streams[msgID] != nil
}

// streamingConv reports whether any reply in a conversation is running, which
// is what the composer and the session list ask about.
func (cw *ChatWindow) streamingConv(convID string) bool {
	if convID == "" {
		return false
	}
	for _, s := range cw.streams {
		if s.convID == convID {
			return true
		}
	}
	return false
}

// streamingCur reports whether the conversation on screen has a running reply.
func (cw *ChatWindow) streamingCur() bool {
	if cw.cur == nil {
		return false
	}
	return cw.streamingConv(cw.cur.ID)
}

// cancelConvStreams stops every reply running in a conversation and forgets
// them. Stopping is per-conversation, not global, so a group reply elsewhere
// keeps running when the user stops the conversation on screen.
func (cw *ChatWindow) cancelConvStreams(convID string) {
	for id, s := range cw.streams {
		if s.convID != convID {
			continue
		}
		if s.cancel != nil {
			s.cancel()
		}
		delete(cw.streams, id)
	}
}
