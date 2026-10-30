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

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
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
	// kindOf is the tool category the adapter reports (read, edit, execute,
	// ...), shown on the tool row and used to pick its icon.
	kindOf string
	// plan is the agent's full execution plan, sent whole on every update.
	plan []models.PlanEntry
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
			kindOf: string(u.ToolCall.Kind),
		}, true
	case u.Plan != nil:
		entries := make([]models.PlanEntry, 0, len(u.Plan.Entries))
		for _, e := range u.Plan.Entries {
			entries = append(entries, models.PlanEntry{
				Content:  e.Content,
				Priority: string(e.Priority),
				Status:   string(e.Status),
			})
		}
		return acpUpdate{kind: "plan", plan: entries}, true
	case u.ToolCallUpdate != nil:
		title := ""
		if u.ToolCallUpdate.Title != nil {
			title = *u.ToolCallUpdate.Title
		}
		status := ""
		if u.ToolCallUpdate.Status != nil {
			status = string(*u.ToolCallUpdate.Status)
		}
		kindOf := ""
		if u.ToolCallUpdate.Kind != nil {
			kindOf = string(*u.ToolCallUpdate.Kind)
		}
		return acpUpdate{
			kind:   "tool",
			id:     string(u.ToolCallUpdate.ToolCallId),
			name:   title,
			args:   acpArgsJSON(u.ToolCallUpdate.RawInput),
			result: acpContentText(u.ToolCallUpdate.Content),
			status: status,
			kindOf: kindOf,
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
		greeting := a.cw.acp.greeting[sess]
		b := a.cw.acp.channelReply[sess]
		a.cw.acp.mu.Unlock()
		// An adapter's self-introduction arrives before the first prompt;
		// writing it to the conversation, or to a channel reply, would show a
		// message nobody asked for.
		if greeting {
			return nil
		}
		if b != nil {
			b.WriteString(upd.text)
		}
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
	if upd.kind == "plan" {
		// A plan update carries the whole list, so it replaces the stored one
		// rather than appending to it.
		conv.Plan = upd.plan
		_ = cw.convManager.SaveConversation(conv)
		cw.invalidate()
		return
	}
	n := len(conv.Messages)
	if n == 0 || conv.Messages[n-1].Role != "assistant" {
		conv.Messages = append(conv.Messages,
			conv.NewAgentMessage(cw.newID(), conv.Agent, authorNameOf(conv.Agent), time.Now()))
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
					if upd.kindOf != "" {
						if tc.Metadata == nil {
							tc.Metadata = map[string]interface{}{}
						}
						tc.Metadata["acp_kind"] = upd.kindOf
					}
					if upd.status == string(acp.ToolCallStatusFailed) {
						tc.Error = "failed"
					}
					cw.acpToolDuration(tc, upd.id, upd.status)
					cw.acpSyncToolSegment(last, tc)
					if upd.status != "" {
						cw.acpBatchTick(conv, last.ID, upd.id, upd.name, upd.status)
					}
					cw.invalidate()
					return
				}
			}
		}
		if upd.name == "" {
			return
		}
		tc := models.ToolCall{ID: upd.id, Name: upd.name, Arguments: upd.args, Result: upd.result, Status: upd.status}
		if upd.kindOf != "" {
			tc.Metadata = map[string]interface{}{"acp_kind": upd.kindOf}
		}
		if upd.status == string(acp.ToolCallStatusFailed) {
			tc.Error = "failed"
		}
		last.ToolCalls = append(last.ToolCalls, tc)
		idx := len(last.ToolCalls) - 1
		cw.acpToolDuration(&last.ToolCalls[idx], upd.id, upd.status)
		cw.acpSyncToolSegment(last, &last.ToolCalls[idx])
		if upd.status != "" {
			cw.acpBatchTick(conv, last.ID, upd.id, upd.name, upd.status)
		}
	}
	cw.invalidate()
}

// acpBatchTick tracks one tool call in the current batch: an unseen,
// still-running call joins the batch and its title is kept for the
// checkpoint summary; a call turning completed or failed leaves it. When
// the last call of a batch finishes, the batch's changes are committed.
func (cw *ChatWindow) acpBatchTick(conv *models.Conversation, msgID, id, name, status string) {
	if conv == nil {
		return
	}
	rt := cw.acpRuntimeFor(conv.ID)
	if rt == nil {
		return
	}
	terminal := status == string(acp.ToolCallStatusCompleted) ||
		status == string(acp.ToolCallStatusFailed)
	cw.acp.mu.Lock()
	if !rt.batch[id] {
		if !terminal {
			rt.batch[id] = true
			if name != "" {
				rt.batchSummary = append(rt.batchSummary, name)
			}
		}
		cw.acp.mu.Unlock()
		return
	}
	if !terminal {
		cw.acp.mu.Unlock()
		return
	}
	delete(rt.batch, id)
	empty := len(rt.batch) == 0
	summary := append([]string(nil), rt.batchSummary...)
	rt.batchSummary = nil
	cw.acp.mu.Unlock()
	// Only the "batch" level commits per batch; under "turn" the whole reply
	// is one checkpoint, committed by acpFlushTurn when the turn ends.
	if empty && cw.cfg.AgentCheckpoint == "batch" {
		cw.autoCommitCheckpoint(conv, msgID, summary)
	}
}

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
		a.cw.invalidate()
		a.cw.notifyAttention(a.cw.acpConvIDOf(req.SessionId), "Agent 请求权限", acpPermissionTitle(&req))
	})
	select {
	case r := <-reply:
		return r, nil
	case <-ctx.Done():
		a.cw.postMain(func() {
			if a.cw.acp.permissionCh == reply {
				a.cw.acp.permissionReq = nil
				a.cw.acp.permissionCh = nil
				a.cw.acp.permissionShow = false
				a.cw.invalidate()
			}
		})
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

func (cw *ChatWindow) acpConvIDOf(sess acp.SessionId) string {
	cw.acp.mu.Lock()
	defer cw.acp.mu.Unlock()
	return cw.acpConvIDOfLocked(sess)
}

// acpConvIDOfLocked is acpConvIDOf for callers already holding acp.mu.
func (cw *ChatWindow) acpConvIDOfLocked(sess acp.SessionId) string {
	key := cw.acp.convBySess[sess]
	if rt := cw.acp.convs[key]; rt != nil {
		return rt.convID
	}
	return key
}

func acpPermissionTitle(req *acp.RequestPermissionRequest) string {
	if req.ToolCall.Title != nil {
		return *req.ToolCall.Title
	}
	return "工具调用"
}

func (cw *ChatWindow) acpPermissionPanel(c *mgo.Context) {
	req := cw.acp.permissionReq
	if req == nil || !cw.acp.permissionShow || !cw.acpPermissionVisible(req) {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)

	var selected acp.PermissionOptionId
	denied := false
	mgo.Column(c).FillWidth().Shrink(0).AlignItems(mgo.Start).Padding(u*2, u*4).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).Radius(mtheme.ControlRadius).
			Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
			Padding(u * 2).Gap(u).Children(func() {
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("lock")).Size(14, 14).
					Shrink(0).TextColor(k.Warning)
				mgo.Text(c, "Agent 请求权限").FontWeight(600).FontSize(caption).Grow(1)
				if kit.Button(c, "拒绝", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					denied = true
				}
			})
			mgo.Column(c).FillWidth().Gap(u).Padding(u).Radius(mtheme.ControlRadius).
				Background(k.Background).Border(mtheme.BorderWidth, k.Border).Children(func() {
				mgo.Text(c, acpPermissionTitle(req)).FontSize(mcore.FontSize(c, mtheme.BodySize)).
					TextColor(k.Text).LineHeight(1.5)
				for _, opt := range req.Options {
					opt := opt
					icon, color := icons.Must("circle-check"), k.Success
					switch opt.Kind {
					case acp.PermissionOptionKindAllowAlways:
						icon = icons.Must("lock-open")
					case acp.PermissionOptionKindRejectOnce, acp.PermissionOptionKindRejectAlways:
						icon, color = icons.Must("circle-x"), k.Warning
					}
					b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Center).
						Padding(u/2, u).Radius(mtheme.ControlRadius).
						Background(k.Surface).Border(mtheme.BorderWidth, k.Border).
						Label(opt.Name)
					if b.Hovered() {
						b.Background(k.SurfaceHover)
					}
					b.Children(func() {
						mgo.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(color)
						mgo.Text(c, opt.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
							FontSize(caption)
					})
					if b.Clicked() {
						selected = opt.OptionId
					}
				}
			})
		})
	})
	switch {
	case denied:
		cw.resolvePermission(acpCancelled())
	case selected != "":
		cw.resolvePermission(acp.RequestPermissionResponse{
			Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: selected}},
		})
	}
}

func (cw *ChatWindow) acpPermissionVisible(req *acp.RequestPermissionRequest) bool {
	convID := cw.acpConvIDOf(req.SessionId)
	return convID == "" || (cw.cur != nil && cw.cur.ID == convID)
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
