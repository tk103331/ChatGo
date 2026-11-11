package webui

import (
	"context"
	"errors"

	"chatgo/pkg/models"
)

// liveTurn is the running turn's door to the browser: it applies conversation
// changes under the run's lock, and parks a question or permission request in
// the run state until a client answers it.
type liveTurn struct {
	rs *runState
}

func (l *liveTurn) Update(fn func(*models.Conversation)) {
	l.rs.mu.Lock()
	fn(l.rs.conv)
	l.rs.mu.Unlock()
}

// Ask publishes the questions and waits for an answer from any client.
func (l *liveTurn) Ask(ctx context.Context, items []AskItem) (string, error) {
	p := &pendingInteraction{
		info:   InteractionInfo{ID: newID(), Kind: "ask", Items: items},
		answer: make(chan string, 1),
	}
	l.rs.mu.Lock()
	l.rs.pending = p
	l.rs.mu.Unlock()
	defer l.clearPending(p)

	select {
	case answer := <-p.answer:
		return answer, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Permission publishes an ACP permission prompt and waits for a choice.
func (l *liveTurn) Permission(ctx context.Context, req PermissionRequest) (PermissionDecision, error) {
	p := &pendingInteraction{
		info:     InteractionInfo{ID: newID(), Kind: "permission", Permission: &req},
		decision: make(chan PermissionDecision, 1),
	}
	l.rs.mu.Lock()
	l.rs.pending = p
	l.rs.mu.Unlock()
	defer l.clearPending(p)

	select {
	case d := <-p.decision:
		return d, nil
	case <-ctx.Done():
		return PermissionDecision{Canceled: true}, ctx.Err()
	}
}

func (l *liveTurn) clearPending(p *pendingInteraction) {
	l.rs.mu.Lock()
	if l.rs.pending == p {
		l.rs.pending = nil
	}
	l.rs.mu.Unlock()
}

// pendingInteraction is the browser-facing question or permission of a run.
type pendingInteraction struct {
	info     InteractionInfo
	answer   chan string
	decision chan PermissionDecision
}

// interaction returns a copy of the pending question, for GET responses.
func (rs *runState) interaction() *InteractionInfo {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.pending == nil {
		return nil
	}
	info := rs.pending.info
	return &info
}

// answer hands a client's answer to the waiting turn.
func (rs *runState) answer(id, answer string) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	p := rs.pending
	if p == nil || p.info.ID != id {
		return errors.New("interaction not found")
	}
	if p.info.Kind != "ask" {
		return errors.New("interaction is not a question")
	}
	rs.pending = nil
	select {
	case p.answer <- answer:
	default:
	}
	return nil
}

// decide hands a client's permission choice to the waiting turn.
func (rs *runState) decide(id string, d PermissionDecision) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	p := rs.pending
	if p == nil || p.info.ID != id {
		return errors.New("interaction not found")
	}
	if p.info.Kind != "permission" {
		return errors.New("interaction is not a permission request")
	}
	rs.pending = nil
	select {
	case p.decision <- d:
	default:
	}
	return nil
}
