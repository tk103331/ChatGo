package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/models"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
)

type acpClient struct{ cw *ChatWindow }

var _ acp.Client = acpClient{}

type acpUpdate struct {
	kind   string
	id     string
	name   string
	text   string
	args   string
	result string
	status string
}

func parseACPUpdate(u acp.SessionUpdate) (acpUpdate, bool) {
	switch {
	case u.AgentMessageChunk != nil:
		if t := u.AgentMessageChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "text", text: t.Text}, true
		}
	case u.AgentThoughtChunk != nil:
		if t := u.AgentThoughtChunk.Content.Text; t != nil && t.Text != "" {
			return acpUpdate{kind: "reasoning", text: t.Text}, true
		}
	case u.ToolCall != nil:
		return acpUpdate{
			kind:   "tool",
			id:     string(u.ToolCall.ToolCallId),
			name:   u.ToolCall.Title,
			args:   acpArgsJSON(u.ToolCall.RawInput),
			result: acpContentText(u.ToolCall.Content),
			status: string(u.ToolCall.Status),
		}, true
	case u.ToolCallUpdate != nil:
		title := ""
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		return acpUpdate{
			kind:   "tool",
			id:     string(u.ToolCallUpdate.ToolCallId),
			name:   title,
			args:   acpArgsJSON(u.ToolCallUpdate.RawInput),
			result: acpContentText(u.ToolCallUpdate.Content),
			status: status,
		}, true
	}
	return acpUpdate{}, false
}

func acpArgsJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := string(b)
	if s == "null" || s == "{}" || s == `""` {
		return ""
	}
	return s
}

func acpContentText(items []acp.ToolCallContent) string {
	var b strings.Builder
	for _, it := range items {
		if it.Content != nil {
			if t := it.Content.Content.Text; t != nil && t.Text != "" {
				b.WriteString(t.Text)
				b.WriteByte('\n')
			}
		}
		if it.Diff != nil {
			fmt.Fprintf(&b, "diff: %s\n", it.Diff.Path)
		}
	}
	return strings.TrimSpace(b.String())
}

func (a acpClient) SessionUpdate(ctx context.Context, n acp.SessionNotification) error {
	sess := n.SessionId
	if m := n.Update.CurrentModeUpdate; m != nil {
		a.cw.postMain(func() { a.cw.acpSetModeState(sess, m.CurrentModeId) })
		return nil
	}
	if cfg := n.Update.ConfigOptionUpdate; cfg != nil {
		opts := cfg.ConfigOptions
		a.cw.postMain(func() { a.cw.acpSetConfigState(sess, opts) })
		return nil
	}
	if ac := n.Update.AvailableCommandsUpdate; ac != nil {
		cmds := ac.AvailableCommands
		a.cw.acp.mu.Lock()
		if rt := a.cw.acp.convs[a.cw.acp.convBySess[sess]]; rt != nil {
			rt.commands = cmds
		}
		a.cw.acp.mu.Unlock()
		a.cw.postMain(func() { a.cw.invalidate() })
		return nil
	}
	upd, ok := parseACPUpdate(n.Update)
	if !ok {
		return nil
	}
	if upd.kind == "text" {
		a.cw.acp.mu.Lock()
		if b := a.cw.acp.channelReply[sess]; b != nil {
			b.WriteString(upd.text)
		}
		a.cw.acp.mu.Unlock()
	}
	a.cw.postMain(func() {
		a.cw.acp.mu.Lock()
		convID := a.cw.acp.convBySess[sess]
		a.cw.acp.mu.Unlock()
		if convID != "" {
			a.cw.acpApplyToConv(convID, upd)
		}
	})
	return nil
}

func (cw *ChatWindow) acpToolDuration(tc *models.ToolCall, id, status string) {
	if id == "" {
		return
	}
	switch status {
	case string(acp.ToolCallStatusPending), string(acp.ToolCallStatusInProgress):
		cw.acp.mu.Lock()
		if _, ok := cw.acp.toolStart[id]; !ok {
			cw.acp.toolStart[id] = time.Now()
		}
		cw.acp.mu.Unlock()
	case string(acp.ToolCallStatusCompleted), string(acp.ToolCallStatusFailed):
		cw.acp.mu.Lock()
		start, ok := cw.acp.toolStart[id]
		delete(cw.acp.toolStart, id)
		cw.acp.mu.Unlock()
		if ok {
			tc.DurationMs = time.Since(start).Milliseconds()
		}
	}
}

func (cw *ChatWindow) acpApplyToConv(convID string, upd acpUpdate) {
	cw.acp.mu.Lock()
	conv := cw.acp.convObj[convID]
	cw.acp.mu.Unlock()
	if conv == nil {
		return
	}
	n := len(conv.Messages)
	if n == 0 || conv.Messages[n-1].Role != "assistant" {
		conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Timestamp: time.Now()})
	}
	last := &conv.Messages[len(conv.Messages)-1]
	switch upd.kind {
	case "text":
		last.Content += upd.text
		appendSegmentText(last, "text", upd.text)
	case "reasoning":
		last.Reasoning += upd.text
		appendSegmentText(last, "reasoning", upd.text)
	case "tool":
		if upd.id != "" {
			for i := range last.ToolCalls {
				if last.ToolCalls[i].ID == upd.id {
					tc := &last.ToolCalls[i]
					if upd.name != "" {
						tc.Name = upd.name
					}
					if upd.args != "" {
						tc.Arguments = upd.args
					}
					if upd.result != "" {
						tc.Result = upd.result
					}
					if upd.status != "" {
						tc.Status = upd.status
					}
					if upd.status == string(acp.ToolCallStatusFailed) {
						tc.Error = "failed"
					}
					cw.acpToolDuration(tc, upd.id, upd.status)
					cw.acpSyncToolSegment(last, tc)
					cw.invalidate()
					return
				}
			}
		}
		if upd.name == "" {
			return
		}
		tc := models.ToolCall{ID: upd.id, Name: upd.name, Arguments: upd.args, Result: upd.result, Status: upd.status}
		if upd.status == string(acp.ToolCallStatusFailed) {
			tc.Error = "failed"
		}
		last.ToolCalls = append(last.ToolCalls, tc)
		idx := len(last.ToolCalls) - 1
		cw.acpToolDuration(&last.ToolCalls[idx], upd.id, upd.status)
		cw.acpSyncToolSegment(last, &last.ToolCalls[idx])
	}
	cw.invalidate()
}

// acpSyncToolSegment keeps the ordered segment view in step with a tool call so
// an ACP reply renders interleaved (thinking, tool, text, ...) like the ReAct
// path instead of one collapsed group of every call.

func (cw *ChatWindow) acpSyncToolSegment(m *models.Message, tc *models.ToolCall) {
	if tc.ID == "" {
		return
	}
	for i := range m.Segments {
		s := &m.Segments[i]
		if s.Kind == "tool" && s.ToolCall != nil && s.ToolCall.ID == tc.ID {
			cp := *tc
			s.ToolCall = &cp
			return
		}
	}
	cp := *tc
	m.Segments = append(m.Segments, models.Segment{Kind: "tool", ToolCall: &cp})
}

func (a acpClient) RequestPermission(ctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	a.cw.acp.mu.Lock()
	auto := a.cw.acp.channelSess[p.SessionId]
	a.cw.acp.mu.Unlock()
	if auto {
		if r, ok := acpAutoApprove(p); ok {
			return r, nil
		}
	}
	reply := make(chan acp.RequestPermissionResponse, 1)
	req := p
	a.cw.postMain(func() {
		a.cw.acp.permissionReq = &req
		a.cw.acp.permissionCh = reply
		a.cw.acp.permissionShow = true
	})
	select {
	case r := <-reply:
		return r, nil
	case <-ctx.Done():
		return acpCancelled(), ctx.Err()
	}
}

func acpAutoApprove(p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, bool) {
	var id acp.PermissionOptionId
	for _, opt := range p.Options {
		if opt.Kind == acp.PermissionOptionKindAllowAlways {
			id = opt.OptionId
			break
		}
	}
	if id == "" {
		for _, opt := range p.Options {
			if opt.Kind == acp.PermissionOptionKindAllowOnce {
				id = opt.OptionId
				break
			}
		}
	}
	if id == "" {
		return acpCancelled(), false
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id}},
	}, true
}

func (a acpClient) ReadTextFile(ctx context.Context, p acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, fmt.Errorf("filesystem access is not supported")
}

func (a acpClient) WriteTextFile(ctx context.Context, p acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, fmt.Errorf("filesystem access is not supported")
}

func (a acpClient) CreateTerminal(ctx context.Context, p acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) KillTerminal(ctx context.Context, p acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) TerminalOutput(ctx context.Context, p acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) ReleaseTerminal(ctx context.Context, p acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, fmt.Errorf("terminal access is not supported")
}

func (a acpClient) WaitForTerminalExit(ctx context.Context, p acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, fmt.Errorf("terminal access is not supported")
}

func acpCancelled() acp.RequestPermissionResponse {
	return acp.RequestPermissionResponse{
		Outcome: acp.RequestPermissionOutcome{Cancelled: &acp.RequestPermissionOutcomeCancelled{}},
	}
}

// acpPermissionDialog asks the user to allow or deny a tool call.

func (cw *ChatWindow) acpPermissionDialog(c *mgo.Context) {
	if cw.acp.permissionReq == nil {
		return
	}
	req := cw.acp.permissionReq
	title := "工具调用"
	if req.ToolCall.Title != nil {
		title = *req.ToolCall.Title
	}
	moverlay.Dialog(c, &cw.acp.permissionShow, moverlay.DialogOptions{
		Title:       "Agent 请求权限",
		Description: title,
		Actions: func() {
			for _, opt := range req.Options {
				id := opt.OptionId
				if minput.Button(c, opt.Name, minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
					cw.resolvePermission(acp.RequestPermissionResponse{
						Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: id}},
					})
				}
			}
			if minput.Button(c, "拒绝", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				cw.resolvePermission(acpCancelled())
			}
		},
	}, func() {
		mgo.Text(c, "该操作需要你的授权：").TextColor(mcore.Tokens(c).TextMuted)
	})
	if !cw.acp.permissionShow && cw.acp.permissionReq != nil {
		cw.resolvePermission(acpCancelled())
	}
}

func (cw *ChatWindow) resolvePermission(r acp.RequestPermissionResponse) {
	if cw.acp.permissionCh != nil {
		cw.acp.permissionCh <- r
	}
	cw.acp.permissionReq = nil
	cw.acp.permissionCh = nil
	cw.acp.permissionShow = false
}

// acpConv is a conversation's live ACP session.
