package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"

	"chatgo/pkg/models"
)

// messageRow renders one message as a flat, frameless row - the same
// immersive layout the rooms use - with the human on the right and the
// assistant on the left, actions appearing under the body on hover.
func (cw *ChatWindow) messageRow(c *mgo.Context, i int, msg models.Message) {
	if cw.editingIdx == i {
		cw.editMessage(c, i, msg)
		return
	}
	streaming := cw.streamingMsg(msg.ID)
	role := roleOf(msg)
	u := mcore.Density(c).Unit()
	user := role == kit.MessageUser
	avatar := func() {
		mgo.Box(c).Shrink(0).Children(func() {
			if user {
				cw.roomUserMark(c, 24)
			} else {
				cw.agentAvatar(c)()
			}
		})
	}
	row := mgo.Row(c).FillWidth().Gap(u*2).AlignItems(mgo.Start).
		Padding(u, u*2).Role(mgo.RoleGroup).Label(msg.Role)
	if user {
		row.Justify(mgo.End)
	}
	row.Children(func() {
		if !user {
			avatar()
		}
		col := mgo.Column(c).MinWidth(0).Gap(u)
		if user {
			col.AlignItems(mgo.End).MaxWidth(720).Shrink(1)
		} else {
			col.Grow(1)
		}
		col.Children(func() {
			mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, cw.messageNameLabel(msg, role)).FontWeight(600).SingleLine().
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				if user {
					mgo.Text(c, "我").FontWeight(600).SingleLine().
						FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				}
				mgo.Text(c, msg.Timestamp.Format("15:04")).SingleLine().
					FontFeatures("tnum").TextColor(mcore.Tokens(c).TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			})
			switch {
			case roomIsError(msg):
				cw.roomErrorBlock(c, msg)
			case len(msg.Segments) > 0:
				cw.messageSegments(c, msg, streaming)
			default:
				cw.messageLegacy(c, msg, streaming)
			}
			// The actions row stands at a fixed height so the message never
			// jumps when the icons appear on hover. Rewinding to a
			// checkpoint stays visible without hover, because it is a
			// destructive action users must be able to find.
			cw.messageActions(c, i, msg, streaming, row.Hovered())
		})
		if user {
			avatar()
		}
	})
}

// messageNameLabel names the speaker of an assistant or system message.
func (cw *ChatWindow) messageNameLabel(msg models.Message, role kit.MessageRole) string {
	if msg.AuthorName != "" {
		return msg.AuthorName
	}
	if role == kit.MessageAssistant {
		if m := cw.turnModelLabel(msg); m != "" {
			return m
		}
		return "助手"
	}
	return msg.Role
}

// messageSegments renders an assistant reply as one foldable execution
// process plus the final answer. While the turn runs the process timeline is
// open, listing every thinking block, tool call and intermediate message —
// each thinking and tool row folded to its one-line summary. When the turn
// ends the process collapses to its counts — thought N times, called M tools
// — leaving the last text as the only message body.
func (cw *ChatWindow) messageSegments(c *mgo.Context, msg models.Message, streaming bool) {
	segs := msg.Segments
	final := -1
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i].Kind == "text" && strings.TrimSpace(segs[i].Text) != "" {
			final = i
			break
		}
	}
	// The process holds every segment except the final text: the thinking,
	// the tool calls and the intermediate messages, in the order they ran.
	procSegs := segs
	if final >= 0 {
		procSegs = append(append([]models.Segment(nil), segs[:final]...), segs[final+1:]...)
	}
	proc := segmentRound{segments: procSegs}
	thinking, tools := proc.counts()
	duration := cw.turnDuration(msg, streaming)
	if thinking > 0 || tools > 0 {
		// The fold follows the whole stream: it stays open while the answer
		// is still being written and folds itself away when the turn ends,
		// so a finished reply reads as the answer with every layer of
		// process closed.
		fold := mgo.Local(c.Root(), "process:"+msg.ID, func() reasoningFold { return reasoningFold{} })
		fold.follow(streaming)
		live := streaming && (final < 0 || final < len(segs)-1)
		cw.processTimeline(c, &fold.open, msg, proc, live, thinking, tools, duration)
	} else if duration != "" {
		mgo.Text(c, "用时 "+duration).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
			TextColor(mcore.Tokens(c).TextMuted)
	}
	if final >= 0 {
		kit.MarkdownView(c, segs[final].Text)
	}
	cw.a2uiMessage(c, msg)
	if cw.emptySegments(msg) && streaming {
		kit.TypingIndicator(c, "Assistant")
	}
}

// segmentRound is the ordered run of segments an execution process holds:
// thinking blocks, tool calls and intermediate messages, everything except
// the reply's final text.
type segmentRound struct {
	segments []models.Segment
}

func (r segmentRound) counts() (thinking, tools int) {
	for _, s := range r.segments {
		if s.Kind == "reasoning" && strings.TrimSpace(s.Text) != "" {
			thinking++
		}
		if s.Kind == "tool" && s.ToolCall != nil {
			tools++
		}
	}
	return thinking, tools
}

func (r segmentRound) lastProcessIndex() int {
	for i := len(r.segments) - 1; i >= 0; i-- {
		s := r.segments[i]
		if s.Kind == "reasoning" && strings.TrimSpace(s.Text) != "" {
			return i
		}
		if s.Kind == "tool" && s.ToolCall != nil {
			return i
		}
	}
	return -1
}

// turnDuration reports how long the turn that produced msg has been running,
// or ran when it finished.
func (cw *ChatWindow) turnDuration(msg models.Message, streaming bool) string {
	t, ok := cw.turnFor(msg)
	if !ok || t.StartedAt.IsZero() {
		return ""
	}
	end := t.EndedAt
	if end.IsZero() {
		if !streaming {
			return ""
		}
		end = time.Now()
	}
	if end.Before(t.StartedAt) {
		return ""
	}
	return kit.AgentDuration(end.Sub(t.StartedAt))
}

// processTimeline is the message's whole execution process behind one fold.
// The header counts the thinking blocks and tool calls and how long the turn
// ran, spinning while the process is still going; the body is a timeline of
// rows — every thinking block and tool call as a collapsed row carrying its
// one-line summary, the intermediate messages in place — so a long ReAct turn
// can be followed live and reads as a single summary line once it is done.
func (cw *ChatWindow) processTimeline(c *mgo.Context, open *bool, msg models.Message, proc segmentRound, live bool, thinking, tools int, duration string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	b := mgo.ButtonBase(c).FillWidth().Justify(mgo.Start).Gap(u*2).AlignItems(mgo.Center).
		Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(mgo.RoleGroup).Label("执行过程")
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		// The master fold shows its chevron at rest: it is the one control
		// that opens the whole trace.
		name := "chevron-right"
		if *open {
			name = "chevron-down"
		}
		mgo.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
		mgo.Text(c, fmt.Sprintf("思考 %d 次 · 工具调用 %d 次", thinking, tools)).
			FontSize(caption).TextColor(k.TextMuted)
		if duration != "" {
			mgo.Text(c, "用时 "+duration).FontSize(caption).TextColor(k.TextMuted)
		}
		if live {
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
		}
	})
	if b.Clicked() {
		*open = !*open
	}
	if !*open {
		return
	}
	mgo.Column(c).FillWidth().Children(func() {
		last := proc.lastProcessIndex()
		for j, seg := range proc.segments {
			// One timeline entry: a rail holding this entry's stretch of
			// the vertical thread, and the row itself beside it. Entries
			// stack without gaps, so the thread reads as one line running
			// through the whole trace.
			mgo.Row(c).FillWidth().Children(func() {
				rail := mgo.Box(c).Width(u * 2).FillHeight().Shrink(0)
				rail.Draw(func(p *mgo.Painter, r mgo.Rect) {
					p.Line(r.X+r.W/2, r.Y, r.X+r.W/2, r.Y+r.H, 1, k.Border)
				})
				mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
					switch {
					case seg.Kind == "reasoning" && strings.TrimSpace(seg.Text) != "":
						segLive := live && j == last
						// A thinking row starts folded — even the one being
						// written: its summary carries the newest thought,
						// and opening the body is a deliberate act.
						fold := mgo.Local(c.Root(), fmt.Sprintf("reason:%s:%d", msg.ID, j),
							func() reasoningFold { return reasoningFold{} })
						cw.reasoningBlock(c, &fold.open, seg.Text,
							cw.reasoningDuration(msg, proc, j, segLive), segLive)
					case seg.Kind == "tool" && seg.ToolCall != nil:
						cw.toolCalls(c, msg, []models.ToolCall{*seg.ToolCall})
					case seg.Kind == "text" && strings.TrimSpace(seg.Text) != "":
						// An intermediate message: shown in place while the
						// turn runs, and kept inside the fold once it ends.
						mgo.Box(c).FillWidth().Padding(u/2, 0).Children(func() {
							kit.MarkdownView(c, seg.Text)
						})
					}
				})
			})
		}
	})
}

// messageLegacy renders a message that predates segments. Its reasoning has
// no start time to be measured against, so the fold shows a summary only.
func (cw *ChatWindow) messageLegacy(c *mgo.Context, msg models.Message, streaming bool) {
	if msg.Role == "assistant" && strings.TrimSpace(msg.Reasoning) != "" {
		fold := mgo.Local(c.Root(), "reason:"+msg.ID, func() reasoningFold { return reasoningFold{} })
		fold.follow(streaming)
		cw.reasoningBlock(c, &fold.open, msg.Reasoning, "", streaming)
	}
	if len(msg.ToolCalls) > 0 {
		cw.toolCalls(c, msg, msg.ToolCalls)
	}
	switch {
	case strings.TrimSpace(msg.Content) != "":
		kit.MarkdownView(c, msg.Content)
	case streaming:
		kit.TypingIndicator(c, "Assistant")
	}
	cw.a2uiMessage(c, msg)
}

// emptySegments reports whether a message has no visible text, thinking or tool
// content yet.
func (cw *ChatWindow) emptySegments(msg models.Message) bool {
	if len(msg.A2UI) > 0 {
		return false
	}
	for _, s := range msg.Segments {
		if s.Kind == "tool" && s.ToolCall != nil {
			return false
		}
		if strings.TrimSpace(s.Text) != "" {
			return false
		}
	}
	return true
}

type reasoningFold struct {
	open bool
	live bool
}

func (f *reasoningFold) follow(live bool) {
	if live != f.live {
		f.open = live
	}
	f.live = live
}

// reasoningSummary picks the thinking's newest non-empty line as the fold's
// one-line summary: while streaming it is the thought being written, and
// once done it reads as where the thinking ended up.
func reasoningSummary(text string) string {
	summary := ""
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			summary = t
		}
	}
	return truncate(summary, 48)
}

// reasoningDuration times one thinking block: from its own start to the
// next segment's, to the turn's end, or to now while the block still streams.
// Empty when the segment predates start times, so old conversations show a
// summary without a made-up number.
func (cw *ChatWindow) reasoningDuration(msg models.Message, round segmentRound, j int, live bool) string {
	if j >= len(round.segments) {
		return ""
	}
	start := round.segments[j].At
	if start.IsZero() {
		return ""
	}
	var end time.Time
	if j+1 < len(round.segments) {
		// The next block's start is where this thinking stopped, whether
		// what followed was a tool call or the answer itself.
		end = round.segments[j+1].At
	} else if t, ok := cw.turnFor(msg); ok && !t.EndedAt.IsZero() {
		end = t.EndedAt
	} else if live {
		end = time.Now()
	}
	if end.IsZero() || end.Before(start) {
		return ""
	}
	return kit.AgentDuration(end.Sub(start))
}

// reasoningBlock shows the model's thinking as a fold, with the same icon
// behaviour as a tool row: a brain mark at rest, the fold arrow under the
// pointer, and the arrow kept once open so the way back is always visible.
// The title carries the thinking's newest line as a summary and how long the
// block ran, so the fold answers "what did it think" without opening it.
func (cw *ChatWindow) reasoningBlock(c *mgo.Context, open *bool, reasoning string, duration string, live bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	b := mgo.ButtonBase(c).FillWidth().Justify(mgo.Start).Gap(u*2).AlignItems(mgo.Center).
		Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(mgo.RoleGroup).Label("思考过程")
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		if b.Hovered() || *open {
			name := "chevron-right"
			if *open {
				name = "chevron-down"
			}
			mgo.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
		} else {
			mgo.Icon(c, icons.Must("brain")).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
		}
		mgo.Text(c, "思考过程").FontSize(caption).TextColor(k.TextMuted)
		if summary := reasoningSummary(reasoning); summary != "" {
			mgo.Text(c, summary).FontSize(caption).TextColor(k.TextMuted).
				Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		}
		if duration != "" {
			mgo.Text(c, duration).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		}
		if live {
			// The reasoning segment is still growing: the title marks it
			// with a spinner so an all-thinking stream does not look stalled.
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
		}
	})
	if b.Clicked() {
		*open = !*open
	}
	if *open {
		// A framed body sets the thinking apart from the reply itself; the
		// height cap keeps a long trace from pushing the answer away. The copy
		// action floats over the frame instead of taking a row of its own.
		body := mgo.Box(c).FillWidth().Shrink(0)
		body.Children(func() {
			mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * toolDetailLines).
				Children(func() {
					mgo.Column(c).FillWidth().Padding(u).Radius(mtheme.ControlRadius).
						Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Children(func() {
						kit.MarkdownView(c, reasoning)
					})
				})
			if body.Hovered() {
				mgo.Row(c).Absolute().Top(u / 2).Right(u / 2).Padding(u / 2).
					Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
					if mcore.IconAction(c, icons.Must("copy"), "复制思考过程").Clicked() {
						c.WriteClipboard(reasoning)
						cw.toast("已复制", mcore.SeveritySuccess)
					}
				})
			}
		})
	}
}

func (cw *ChatWindow) messageActions(c *mgo.Context, i int, msg models.Message, streaming, reveal bool) {
	if streaming {
		return
	}
	u := mcore.Density(c).Unit()
	mgo.Row(c).Gap(u).MinHeight(mcore.Density(c).ControlHeight()).AlignItems(mgo.Center).Children(func() {
		// All actions wait for the hover, so a finished reply reads as just
		// its text until the pointer enters the row.
		if !reveal {
			return
		}
		if mcore.IconAction(c, icons.Must("copy"), "复制").Tooltip("复制").Clicked() {
			c.WriteClipboard(msg.Content)
			cw.toast("已复制到剪贴板", mcore.SeveritySuccess)
		}
		if msg.Role == "user" {
			if mcore.IconAction(c, icons.Must("pencil"), "编辑").Tooltip("编辑").Clicked() {
				cw.editingIdx = i
				cw.editDraft = msg.Content
			}
			return
		}
		if mcore.IconAction(c, icons.Must("refresh-cw"), "重新生成").Tooltip("重新生成").Clicked() {
			cw.regenerate(i)
		}
		if cw.checkpointEnabled() && msg.Role == "assistant" && msg.ID != "" {
			if mcore.IconAction(c, icons.Must("history"), "回退到此点").Tooltip("回退到此点").Clicked() {
				cw.askConfirmAs("回退到此点",
					"将把代码回退到该回复完成时的 checkpoint。此回复之后的所有改动都会丢失，且不可撤销。",
					"回退", func() { cw.revertToMessage(msg.ID) })
			}
		}
		if t, ok := cw.turnFor(msg); ok {
			if t.Total > 0 {
				mgo.Text(c, tokenLabel(t)).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
					TextColor(mcore.Tokens(c).TextMuted).SingleLine()
			}
		}
	})
}

// turnFor returns the turn whose reply is msg.
// revertToMessage resets the conversation workspace to the checkpoint of the
// reply with the given message ID, discarding every change after it.
func (cw *ChatWindow) revertToMessage(msgID string) {
	if cw.cur == nil {
		return
	}
	dir := cw.cur.Workspace
	if dir == "" {
		cw.toast("该会话没有工作区", mcore.SeverityWarning)
		return
	}
	sha := findCheckpointCommit(dir, msgID)
	if sha == "" {
		cw.toast("该回复没有 checkpoint", mcore.SeverityWarning)
		return
	}
	if err := resetToCheckpoint(dir, sha); err != nil {
		cw.toast("回退失败："+err.Error(), mcore.SeverityError)
		return
	}
	cw.toast("已回退到该 checkpoint", mcore.SeveritySuccess)
	cw.invalidate()
}

func (cw *ChatWindow) turnFor(msg models.Message) (models.Turn, bool) {
	if cw.cur == nil {
		return models.Turn{}, false
	}
	if t := turnOf(cw.cur, msg.ID); t != nil {
		return *t, true
	}
	return models.Turn{}, false
}

// turnModelLabel names the model a turn was generated with.
func (cw *ChatWindow) turnModelLabel(msg models.Message) string {
	t, ok := cw.turnFor(msg)
	if !ok || (t.Model == "" && t.Provider == "") {
		return ""
	}
	return modelLabel(modelRef(t.Provider, t.Model))
}

// tokenLabel formats a turn's usage as "↑input ↓output = total".
func tokenLabel(t models.Turn) string {
	return fmt.Sprintf("↑%s ↓%s = %s tokens", formatCount(t.Input), formatCount(t.Output), formatCount(t.Total))
}

func roleOf(msg models.Message) kit.MessageRole {
	switch msg.Role {
	case "user":
		return kit.MessageUser
	case "system":
		return kit.MessageSystem
	default:
		return kit.MessageAssistant
	}
}

// toolCalls renders a message's tool calls, one row each: merging them
// under a "N 次工具调用" group hides which tool did what, which a long ReAct
// turn needs when it is being followed.
func (cw *ChatWindow) toolCalls(c *mgo.Context, msg models.Message, calls []models.ToolCall) {
	cw.toolCallRows(c, msg, calls)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// editMessage edits a sent user message in place.
func (cw *ChatWindow) editMessage(c *mgo.Context, i int, msg models.Message) {
	r := kit.MessageEditor(c, &cw.editDraft, kit.MessageEditorOptions{Original: msg.Content})
	switch {
	case r.Saved():
		cw.applyEdit(i)
	case r.Canceled():
		cw.editingIdx = -1
	}
}

// applyEdit replaces an edited message, drops what followed and re-answers.
func (cw *ChatWindow) applyEdit(i int) {
	if cw.cur == nil {
		cw.editingIdx = -1
		return
	}
	conv := cw.cur
	cw.dropA2UIMessages(conv.Messages[i+1:])
	conv.Messages[i].Content = cw.editDraft
	conv.Messages = conv.Messages[:i+1]
	cw.editingIdx = -1
	_ = cw.convManager.SaveConversation(conv)
	cw.resendFrom(i + 1)
}

// regenerate drops an assistant reply and asks for a new one.
func (cw *ChatWindow) regenerate(i int) {
	if cw.cur == nil || i <= 0 {
		return
	}
	conv := cw.cur
	prev := conv.Messages[i]
	t := turnOf(conv, prev.ID)
	cw.dropA2UIMessages(conv.Messages[i:])
	conv.Messages = conv.Messages[:i]
	// The replaced reply's author stands: regenerating asks the same agent
	// again, not whichever one the window happens to have selected now.
	reply := conv.NewAgentMessage(cw.newID(), prev.AuthorID, prev.AuthorName, time.Now())
	if reply.AuthorName == "" {
		reply.AuthorID, reply.AuthorName = conv.Agent, authorNameOf(conv.Agent)
	}
	conv.Messages = append(conv.Messages, reply)
	if t != nil {
		t.ReplyMsgID = reply.ID
		t.Assistant = ""
		t.Input, t.Output, t.Total = 0, 0, 0
		t.EndedAt = time.Time{}
	}
	_ = cw.convManager.SaveConversation(conv)
	cw.streamFrom(conv, i)
}

// retargetTurn points a turn at a new reply and clears its finished state.
func (cw *ChatWindow) retargetTurn(conv *models.Conversation, oldReplyID, newReplyID string) {
	t := turnOf(conv, oldReplyID)
	if t == nil {
		return
	}
	t.ReplyMsgID = newReplyID
	t.Assistant = ""
	t.Input, t.Output, t.Total = 0, 0, 0
	t.EndedAt = time.Time{}
}

// resendFrom streams a new assistant reply appended at index i.
func (cw *ChatWindow) resendFrom(i int) {
	if cw.cur == nil || i == 0 {
		return
	}
	conv := cw.cur
	prev := conv.Messages[i-1]
	reply := conv.NewAgentMessage(cw.newID(), conv.Agent, authorNameOf(conv.Agent), time.Now())
	conv.Messages = append(conv.Messages, reply)
	cw.appendTurn(conv, prev, reply)
	cw.streamFrom(conv, i)
}

// appendTurn records a turn for an edited message's new reply.
func (cw *ChatWindow) appendTurn(conv *models.Conversation, userMsg, reply models.Message) {
	turn := models.Turn{
		ID:         cw.newID(),
		UserMsgID:  userMsg.ID,
		ReplyMsgID: reply.ID,
		User:       userMsg.Content,
		Provider:   cw.provider,
		Model:      cw.model,
		StartedAt:  time.Now(),
	}
	if strings.HasPrefix(conv.Agent, "acp:") {
		turn.Provider, turn.Model = strings.TrimPrefix(conv.Agent, "acp:"), ""
	} else if strings.HasPrefix(conv.Agent, "a2a:") {
		turn.Provider, turn.Model = "a2a", ""
	}
	conv.Turns = append(conv.Turns, turn)
}

func (cw *ChatWindow) streamFrom(conv *models.Conversation, i int) {
	replyID := conv.Messages[i].ID
	cw.scrollChatToEnd()
	if strings.HasPrefix(conv.Agent, "acp:") {
		cw.beginStream(conv.ID, replyID, nil)
		cw.invalidate()
		_ = cw.convManager.SaveConversation(conv)
		cw.acpChatStart(conv, strings.TrimPrefix(conv.Agent, "acp:"), i, lastUserText(conv, i))
		return
	}
	if strings.HasPrefix(conv.Agent, "a2a:") {
		ctx, cancel := context.WithCancel(context.Background())
		cw.beginStream(conv.ID, replyID, cancel)
		cw.invalidate()
		_ = cw.convManager.SaveConversation(conv)
		go cw.runA2AChat(ctx, conv, strings.TrimPrefix(conv.Agent, "a2a:"), i, lastUserText(conv, i))
		return
	}
	cw.applyTurnModelFor(conv, replyID)
	msgs := cw.chatMessages(conv, i)
	ctx, cancel := context.WithCancel(context.Background())
	cw.beginStream(conv.ID, replyID, cancel)
	cw.invalidate()
	go cw.runStream(ctx, conv, i, msgs)
}

// applyTurnModelFor rebuilds the client for the model a turn was recorded with.
func (cw *ChatWindow) applyTurnModelFor(conv *models.Conversation, replyID string) {
	t := turnOf(conv, replyID)
	if t == nil {
		return
	}
	cw.applyModel(t.Provider, t.Model)
}

// applyModel switches the active provider and model, rebuilding the client.
func (cw *ChatWindow) applyModel(provider, model string) {
	if provider == "" && model == "" {
		return
	}
	if provider == cw.provider && model == cw.model {
		return
	}
	if provider != "" {
		cw.provider = provider
	}
	if model != "" {
		cw.model = model
	}
	cw.setupProvider(cw.provider)
}

func lastUserText(conv *models.Conversation, before int) string {
	for j := before - 1; j >= 0; j-- {
		if conv.Messages[j].Role == "user" {
			return conv.Messages[j].Content
		}
	}
	return ""
}
