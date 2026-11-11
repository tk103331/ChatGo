package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/toolview"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// messageRow draws one flat, frameless message row, matching the desktop
// conversation view: the person on the right, the assistant on the left, and
// the actions of a finished message under its body.
func (a *app) messageRow(c *ui.Context, _ int, m models.Message) {
	if m.ID != "" && a.editingID == m.ID {
		a.editMessage(c, m)
		return
	}
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	human := m.IsHuman()
	row := ui.Row(c).FillWidth().Gap(u*2).AlignItems(ui.Start).
		Padding(u, u*2).Role(ui.RoleGroup).Label(m.Role)
	if human {
		row.Justify(ui.End)
	}
	row.Children(func() {
		if !human {
			a.assistantAvatar(c)
		}
		col := ui.Column(c).MinWidth(0).Gap(u)
		if human {
			col.AlignItems(ui.End).MaxWidth(720).Shrink(1)
		} else {
			col.Grow(1)
		}
		col.Children(func() {
			ui.Row(c).Gap(u * 2).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.messageName(m)).FontWeight(600).SingleLine().
					FontSize(caption)
				if human {
					ui.Text(c, "我").FontWeight(600).SingleLine().FontSize(caption)
				}
				if !m.Timestamp.IsZero() {
					ui.Text(c, m.Timestamp.Format("15:04")).FontFeatures("tnum").
						TextColor(t.TextMuted).FontSize(caption)
				}
			})
			if len(m.Segments) > 0 {
				a.messageSegments(c, m)
			} else {
				a.messageLegacy(c, m)
			}
			a.messageActions(c, m, row.Hovered())
		})
		if human {
			a.userAvatar(c)
		}
	})
}

func (a *app) messageName(m models.Message) string {
	if strings.TrimSpace(m.AuthorName) != "" {
		return m.AuthorName
	}
	if m.Role == "assistant" {
		return "助手"
	}
	return m.Role
}

func (a *app) assistantAvatar(c *ui.Context) {
	ui.Icon(c, icons.Must("message-square")).Size(24, 24).
		TextColor(c.Theme().Accent).Shrink(0)
}

func (a *app) userAvatar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Size(24, 24).Shrink(0).Radius(12).
		Background(t.Selection).AlignItems(ui.Center).Justify(ui.Center).
		Children(func() {
			ui.Icon(c, icons.Must("users")).Size(14, 14).TextColor(t.Accent)
		})
}

// messageSegments renders an assistant reply as execution rounds plus the
// final answer, like the desktop conversation: a round's thinking and tool
// calls fold into one "执行过程" line, and the last text block stays visible.
func (a *app) messageSegments(c *ui.Context, m models.Message) {
	rounds := segmentRounds(m.Segments)
	final := -1
	for i := len(rounds) - 1; i >= 0; i-- {
		if rounds[i].hasText() {
			final = i
			break
		}
	}
	streaming := a.msgStreaming(m)
	duration := a.turnDuration(m, streaming)
	for i, round := range rounds {
		if !round.hasProcess() && !round.hasText() {
			continue
		}
		live := streaming && i == len(rounds)-1 && !round.hasText()
		showProcess := round.hasProcess() || (round.hasText() && i != final)
		key := fmt.Sprintf("round:%s:%d", m.ID, i)
		if showProcess {
			a.followFold(key, live)
			roundDuration := ""
			if i == len(rounds)-1 {
				roundDuration = duration
			}
			a.processBlock(c, key, m, round, live, i != final, roundDuration)
		}
		if i == final {
			if !showProcess && duration != "" {
				ui.Text(c, "用时 "+duration).FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
					TextColor(c.Theme().TextMuted)
			}
			if text, ok := round.text(); ok {
				kit.MarkdownView(c, text)
			}
		}
	}
	if a.emptySegments(m) && streaming {
		kit.TypingIndicator(c, "助手")
	}
}

// segmentRound is one completed or in-progress ReAct round: the thinking and
// tool calls leading to a text block, or a trailing round still running.
type segmentRound struct {
	segments []models.Segment
}

// segmentRounds splits ordered segments at non-empty text blocks. A trailing
// run without text is the round currently being generated.
func segmentRounds(segs []models.Segment) []segmentRound {
	var rounds []segmentRound
	start := 0
	for i, seg := range segs {
		if seg.Kind == "text" && strings.TrimSpace(seg.Text) != "" {
			rounds = append(rounds, segmentRound{segments: segs[start : i+1]})
			start = i + 1
		}
	}
	if start < len(segs) {
		rounds = append(rounds, segmentRound{segments: segs[start:]})
	}
	return rounds
}

func (r segmentRound) hasText() bool {
	_, ok := r.text()
	return ok
}

func (r segmentRound) text() (string, bool) {
	for i := len(r.segments) - 1; i >= 0; i-- {
		s := r.segments[i]
		if s.Kind == "text" && strings.TrimSpace(s.Text) != "" {
			return s.Text, true
		}
	}
	return "", false
}

func (r segmentRound) hasProcess() bool {
	for _, s := range r.segments {
		if s.Kind == "reasoning" && strings.TrimSpace(s.Text) != "" {
			return true
		}
		if s.Kind == "tool" && s.ToolCall != nil {
			return true
		}
	}
	return false
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

// processBlock folds one round's thinking and tool calls, with a summary of
// how many times the model thought and how many tools it ran.
func (a *app) processBlock(c *ui.Context, key string, m models.Message, round segmentRound, live, includeText bool, duration string) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	thinking, tools := round.counts()
	open := a.folds[key]
	b := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Gap(u*2).
		AlignItems(ui.Center).Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(ui.RoleGroup).Label("执行过程")
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		if b.Hovered() || open {
			name := "chevron-right"
			if open {
				name = "chevron-down"
			}
			ui.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		} else {
			ui.Icon(c, icons.Must("brain")).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		}
		ui.Text(c, "执行过程").FontSize(caption).TextColor(t.TextMuted)
		ui.Textf(c, "思考 %d 次 · 工具 %d 次", thinking, tools).
			FontSize(caption).TextColor(t.TextMuted)
		if duration != "" {
			ui.Text(c, "用时 "+duration).FontSize(caption).TextColor(t.TextMuted)
		}
		if live {
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 10})
		}
	})
	if b.Clicked() {
		a.folds[key] = !open
	}
	if !open {
		return
	}
	ui.Column(c).FillWidth().Gap(u).Padding(u).Children(func() {
		last := round.lastProcessIndex()
		for j, seg := range round.segments {
			switch seg.Kind {
			case "reasoning":
				if strings.TrimSpace(seg.Text) == "" {
					continue
				}
				a.reasoningBlock(c, fmt.Sprintf("%s:r%d", key, j), seg.Text,
					live && j == last, true)
			case "tool":
				if seg.ToolCall != nil {
					a.toolCalls(c, m, []models.ToolCall{*seg.ToolCall}, fmt.Sprintf("%s:t%d", key, j))
				}
			case "text":
				if includeText && strings.TrimSpace(seg.Text) != "" {
					kit.MarkdownView(c, seg.Text)
				}
			}
		}
	})
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

func (a *app) messageLegacy(c *ui.Context, m models.Message) {
	if m.Role == "assistant" && strings.TrimSpace(m.Reasoning) != "" {
		key := "reason:" + m.ID
		streaming := a.msgStreaming(m)
		a.followFold(key, streaming)
		a.reasoningBlock(c, key, m.Reasoning, streaming, false)
	}
	if len(m.ToolCalls) > 0 {
		a.toolCalls(c, m, m.ToolCalls, "tool:"+m.ID)
	}
	switch {
	case strings.TrimSpace(m.Content) != "":
		a.textBlock(c, m, m.Content)
	case a.msgStreaming(m):
		kit.TypingIndicator(c, "助手")
	}
}

func (a *app) textBlock(c *ui.Context, m models.Message, text string) {
	if m.IsHuman() {
		ui.Text(c, text).Selectable().LineHeight(mtheme.LineHeight)
		return
	}
	kit.MarkdownView(c, text)
}

// followFold opens a fold while its content is streaming and folds it again
// once the run finishes, the way the desktop conversation does.
func (a *app) followFold(key string, live bool) {
	if prev, ok := a.foldLive[key]; !ok || prev != live {
		a.folds[key] = live
	}
	a.foldLive[key] = live
}

// reasoningBlock shows the model's thinking as a fold, with a copy action
// floating over the body.
func (a *app) reasoningBlock(c *ui.Context, key, text string, live, defaultOpen bool) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	open, known := a.folds[key]
	if !known {
		open = defaultOpen
	}
	b := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Gap(u*2).
		AlignItems(ui.Center).Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(ui.RoleGroup).Label("思考过程")
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		if b.Hovered() || open {
			name := "chevron-right"
			if open {
				name = "chevron-down"
			}
			ui.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		} else {
			ui.Icon(c, icons.Must("brain")).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		}
		ui.Text(c, "思考过程").FontSize(caption).TextColor(t.TextMuted)
		if live {
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 10})
		}
	})
	if b.Clicked() {
		a.folds[key] = !open
	}
	if !open {
		return
	}
	body := ui.Box(c).FillWidth().Shrink(0)
	body.Children(func() {
		ui.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * 8).
			Children(func() {
				ui.Column(c).FillWidth().Padding(u).Radius(mtheme.ControlRadius).
					Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
					Children(func() {
						kit.MarkdownView(c, text)
					})
			})
		if body.Hovered() {
			ui.Row(c).Absolute().Top(u / 2).Right(u / 2).Padding(u / 2).
				Radius(mtheme.ControlRadius).Background(t.Surface).Children(func() {
				if uikit.IconAction(c, icons.Must("copy"), "复制思考过程").
					Tooltip("复制思考过程").Clicked() {
					c.WriteClipboard(text)
					a.flash("已复制")
				}
			})
		}
	})
}

// toolCalls renders a message's tool calls, one foldable row each.
func (a *app) toolCalls(c *ui.Context, m models.Message, calls []models.ToolCall, prefix string) {
	isACP := strings.HasPrefix(m.AuthorID, "acp:")
	for i := range calls {
		tc := calls[i]
		if tc.Name == "" && tc.Arguments == "" {
			continue
		}
		kind := toolview.Kind(tc.Name, tc.Arguments)
		if isACP {
			if k := acpKindOf(tc); k != "" {
				kind = k
			}
		}
		summary := tc.Name
		if !isACP {
			if s := toolview.ArgSummary(tc.Arguments); s != "" {
				summary = s
			}
		}
		label := toolview.Label(kind)
		if summary == "" {
			summary = label
			label = ""
		}
		key := prefix
		if tc.ID != "" {
			key = prefix + ":" + tc.ID
		} else {
			key = fmt.Sprintf("%s:%d", prefix, i)
		}
		a.toolRow(c, key, tc, kind, label, summary)
	}
}

// acpKindOf reads the tool category an ACP adapter reported, stored on the
// call's metadata when the update arrived.
func acpKindOf(tc models.ToolCall) string {
	if tc.Metadata != nil {
		if v, ok := tc.Metadata["acp_kind"].(string); ok {
			return v
		}
	}
	return ""
}

// toolRow is one tool call: a clickable line and, while open, its payload.
func (a *app) toolRow(c *ui.Context, key string, tc models.ToolCall, kind, label, summary string) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	title := summary
	if label != "" {
		title = label + " · " + summary
	}
	open := a.folds[key]
	b := ui.ButtonBase(c).FillWidth().Gap(u*2).AlignItems(ui.Center).
		Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(ui.RoleGroup).Label(title)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		if b.Hovered() || open {
			name := "chevron-right"
			if open {
				name = "chevron-down"
			}
			ui.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		} else {
			ui.Icon(c, icons.Must(toolview.Icon(kind))).Size(14, 14).
				Shrink(0).TextColor(t.TextMuted)
		}
		if label != "" {
			ui.Text(c, label).Shrink(0).SingleLine().FontWeight(600).
				FontSize(caption).TextColor(t.TextMuted)
		}
		ui.Text(c, summary).Grow(1).MinWidth(0).Shrink(1).SingleLine().
			Ellipsis("…").FontSize(caption).TextColor(t.TextMuted)
		a.toolState(c, tc)
	})
	if b.Clicked() {
		a.folds[key] = !open
	}
	if open {
		a.toolDetail(c, tc)
	}
}

// toolState marks a call's run: running, failed, or done with its duration.
func (a *app) toolState(c *ui.Context, tc models.ToolCall) {
	t := c.Theme()
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	running := tc.Status == "in_progress" || tc.Status == "pending" ||
		(tc.Status == "" && tc.Error == "" && tc.Result == "" && tc.DurationMs == 0)
	switch {
	case running:
		uikit.Spinner(c, uikit.SpinnerOptions{Size: 10})
	case tc.Error != "" || tc.Status == "failed":
		ui.Text(c, "失败").Shrink(0).SingleLine().FontSize(caption).
			TextColor(t.Danger)
	case tc.DurationMs > 0:
		ui.Text(c, kit.AgentDuration(time.Duration(tc.DurationMs)*time.Millisecond)).
			Shrink(0).SingleLine().FontSize(caption).TextColor(t.TextMuted)
	}
}

// toolDetail is a tool call's fold: its input and output, each in a framed,
// scrollable block with a copy action.
func (a *app) toolDetail(c *ui.Context, tc models.ToolCall) {
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	if strings.TrimSpace(tc.Arguments) == "" && strings.TrimSpace(tc.Result) == "" {
		ui.Text(c, "（无输入输出）").FontSize(caption).
			TextColor(c.Theme().TextMuted)
		return
	}
	ui.Column(c).FillWidth().Gap(uikit.Unit(c)).Children(func() {
		if args := strings.TrimSpace(tc.Arguments); args != "" {
			a.toolDetailBlock(c, "输入", args, false)
		}
		if out := strings.TrimSpace(tc.Result); out != "" {
			a.toolDetailBlock(c, "输出", out, tc.Error != "")
		}
	})
}

// toolDetailLines is how many lines of a tool payload stay visible before the
// block scrolls.
const toolDetailLines = 10

func (a *app) toolDetailBlock(c *ui.Context, title, body string, danger bool) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	border := t.Border
	if danger {
		border = t.Danger
	}
	ui.Column(c).FillWidth().Gap(u / 2).Children(func() {
		ui.Row(c).FillWidth().Gap(u).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontWeight(600).FontSize(caption).
				TextColor(t.TextMuted).Grow(1)
			if uikit.IconAction(c, icons.Must("copy"), "复制"+title).Clicked() {
				c.WriteClipboard(body)
				a.flash("已复制")
			}
		})
		ui.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * toolDetailLines).
			Children(func() {
				ui.Column(c).FillWidth().Padding(u).Radius(mtheme.ControlRadius).
					Background(t.Surface).Border(mtheme.BorderWidth, border).
					Children(func() {
						text := ui.Text(c, body).Font(mtheme.MonoFont).
							FontSize(caption).Selectable()
						if danger {
							text.TextColor(t.Danger)
						} else {
							text.TextColor(t.Text)
						}
					})
			})
	})
}

// messageActions shows the actions of a finished message under its body:
// copy everywhere, edit on a prompt, regenerate on a reply, and the turn's
// token usage beside them.
func (a *app) messageActions(c *ui.Context, m models.Message, reveal bool) {
	if a.msgStreaming(m) {
		return
	}
	u := uikit.Unit(c)
	t := c.Theme()
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Row(c).Gap(u).MinHeight(uikit.Density(c).ControlHeight()).
		AlignItems(ui.Center).Children(func() {
		if !reveal {
			return
		}
		if uikit.IconAction(c, icons.Must("copy"), "复制").Tooltip("复制").Clicked() {
			c.WriteClipboard(m.Content)
			a.flash("已复制到剪贴板")
		}
		// Without the desktop runtime an agent session is read-only: the
		// plain web server cannot answer it, so replacing its history would
		// leave the agent in an inconsistent state.
		canReply := a.sessionAgent() == "" || a.cap(capAgents)
		if m.IsHuman() {
			if canReply {
				if uikit.IconAction(c, icons.Must("pencil"), "编辑").Tooltip("编辑").Clicked() {
					a.beginEdit(m)
				}
			}
			return
		}
		if canReply {
			if uikit.IconAction(c, icons.Must("refresh-cw"), "重新生成").
				Tooltip("重新生成").Clicked() {
				a.regenerate(m)
			}
		}
		if a.cap(capCheckpoints) && a.canRevert() {
			if uikit.IconAction(c, icons.Must("history"), "回退到此点").
				Tooltip("回退到此点").Clicked() {
				a.beginRevert(m)
			}
		}
		if turn, ok := a.turnFor(m); ok && turn.Total > 0 {
			ui.Text(c, tokenLabel(turn)).FontSize(caption).
				TextColor(t.TextMuted).SingleLine()
		}
	})
}

// editMessage edits a sent user message in place.
func (a *app) editMessage(c *ui.Context, m models.Message) {
	r := kit.MessageEditor(c, &a.editDraft, kit.MessageEditorOptions{Original: m.Content})
	switch {
	case r.Saved():
		a.applyEdit(m)
	case r.Canceled():
		a.editingID = ""
		a.editDraft = ""
	}
}

// beginEdit opens the editor for a user message.
func (a *app) beginEdit(m models.Message) {
	a.editingID = m.ID
	a.editDraft = m.Content
	a.win.Update(func() {})
}

// applyEdit replaces the edited message on the server, which drops what
// followed it and answers it again.
func (a *app) applyEdit(m models.Message) {
	id := a.getSelected()
	text := strings.TrimSpace(a.editDraft)
	a.editingID = ""
	a.editDraft = ""
	if id == "" || m.ID == "" || text == "" {
		return
	}
	provider, model := a.selectedModel()
	body := map[string]any{"text": text}
	if provider != "" {
		body["provider"] = provider
		body["model"] = model
	}
	if len(a.skillsOn) > 0 {
		body["skills"] = a.skillsOn
	}
	a.skillsOn = nil
	a.status = "正在生成…"
	a.setRunning(true)
	go func() {
		if err := apiPost(messagePath(id, m.ID, "edit"), body, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			a.loadSession(id, false)
			return
		}
		a.win.Update(func() { a.msgList.ScrollToEnd() })
		a.loadSession(id, false)
		a.refreshSessions()
	}()
}

// regenerate drops an assistant reply on the server and asks for a new one.
func (a *app) regenerate(m models.Message) {
	id := a.getSelected()
	if id == "" || m.ID == "" {
		return
	}
	a.status = "正在生成…"
	a.setRunning(true)
	go func() {
		if err := apiPost(messagePath(id, m.ID, "regenerate"), nil, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			a.loadSession(id, false)
			return
		}
		a.win.Update(func() { a.msgList.ScrollToEnd() })
		a.loadSession(id, false)
		a.refreshSessions()
	}()
}

// canRevert reports whether the session has a workspace whose checkpoints can
// be rewound.
func (a *app) canRevert() bool {
	cur := a.current()
	return cur != nil && cur.Workspace != ""
}

// turnFor returns the turn whose reply is m.
func (a *app) turnFor(m models.Message) (models.Turn, bool) {
	cur := a.current()
	if cur == nil || m.ID == "" {
		return models.Turn{}, false
	}
	for i := range cur.Turns {
		if cur.Turns[i].ReplyMsgID == m.ID {
			return cur.Turns[i], true
		}
	}
	return models.Turn{}, false
}

// msgStreaming reports whether m is the reply still being generated.
func (a *app) msgStreaming(m models.Message) bool {
	if !a.isRunning() || m.IsHuman() {
		return false
	}
	if t, ok := a.turnFor(m); ok {
		return t.EndedAt.IsZero()
	}
	cur := a.current()
	if cur == nil || len(cur.Messages) == 0 {
		return false
	}
	last := cur.Messages[len(cur.Messages)-1]
	return last.ID == m.ID
}

// turnDuration reports how long the turn that produced m has been running,
// or ran when it finished.
func (a *app) turnDuration(m models.Message, streaming bool) string {
	t, ok := a.turnFor(m)
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

// tokenLabel formats a turn's usage as "↑input ↓output = total".
func tokenLabel(t models.Turn) string {
	return fmt.Sprintf("↑%s ↓%s = %s tokens", formatCount(t.Input), formatCount(t.Output), formatCount(t.Total))
}

// formatCount renders a token count with thousands separators and k/M suffixes.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func (a *app) emptySegments(m models.Message) bool {
	for _, s := range m.Segments {
		if s.Kind == "tool" && s.ToolCall != nil {
			return false
		}
		if strings.TrimSpace(s.Text) != "" {
			return false
		}
	}
	return true
}
