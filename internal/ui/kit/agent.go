package kit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// AgentState is a tool call's state; the zero value is pending.
type AgentState int

const (
	AgentPending AgentState = iota
	AgentRunning
	AgentDone
	AgentFailed
)

func (s AgentState) String() string {
	switch s {
	case AgentPending:
		return "pending"
	case AgentRunning:
		return "running"
	case AgentDone:
		return "done"
	case AgentFailed:
		return "failed"
	}
	panic(fmt.Sprintf("kit: unknown AgentState %d", int(s)))
}

// agentWord is the word shown for s.
func agentWord(s AgentState) string {
	switch s {
	case AgentPending:
		return "待运行"
	case AgentRunning:
		return "运行中"
	case AgentDone:
		return "已完成"
	case AgentFailed:
		return "失败"
	}
	panic(fmt.Sprintf("kit: unknown AgentState %d", int(s)))
}

// agentTone returns the color and icon marking s.
func agentTone(k *mgo.Theme, s AgentState) (mgo.Color, *mgo.SVG) {
	switch s {
	case AgentRunning:
		return k.Accent, icons.Must("loader-circle")
	case AgentDone:
		return k.Success, icons.Must("circle-check")
	case AgentFailed:
		return k.Danger, icons.Must("circle-x")
	}
	return k.TextMuted, icons.Must("circle")
}

// agentMark draws s; running spins unless still.
func agentMark(c *mgo.Context, s AgentState, size float32, still bool) mgo.Element {
	tone, icon := agentTone(uikit.Tokens(c), s)
	if s == AgentRunning && !still {
		return uikit.Spinner(c, uikit.SpinnerOptions{Size: size, Color: tone, Label: agentWord(s)})
	}
	return mgo.Icon(c, icon).Size(size, size).TextColor(tone).Shrink(0)
}

// AgentDuration formats d like "320 ms" or "4.2 s".
func AgentDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f s", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%d m %02d s", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d h %02d m", int(d.Hours()), int(d.Minutes())%60)
}

// agentCaption is small muted text.
func agentCaption(c *mgo.Context, s string) mgo.Element {
	return mgo.Text(c, s).FontSize(uikit.FontSize(c, theme.CaptionSize)).
		TextColor(uikit.Tokens(c).TextMuted)
}

// ToolCall is one tool invocation of an assistant message.
type ToolCall struct {
	ID   string
	Name string
	// Args and Result show as JSON when valid.
	Args   string
	Result string
	// Error replaces Result when the call failed.
	Error    string
	State    AgentState
	Duration time.Duration
}

// ToolCallGroupOptions configure a ToolCallGroup.
type ToolCallGroupOptions struct {
	// Title replaces the built-in "N 次工具调用".
	Title string
	Still bool
}

// ToolCallGroup folds tool calls under a count, time and failure summary.
func ToolCallGroup(c *mgo.Context, open *bool, calls []ToolCall, opts ToolCallGroupOptions) ChoiceResult {
	if open == nil {
		panic("kit: ToolCallGroup needs open")
	}
	seen := make(map[string]bool, len(calls))
	var total time.Duration
	failed, running := 0, false
	for _, call := range calls {
		if call.ID == "" || seen[call.ID] {
			panic(fmt.Sprintf("kit: ToolCallGroup call IDs must be unique and set, got %q", call.ID))
		}
		seen[call.ID] = true
		total += call.Duration
		if call.State == AgentFailed {
			failed++
		}
		running = running || call.State == AgentRunning
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	title := opts.Title
	if title == "" {
		title = fmt.Sprintf("%d 次工具调用", len(calls))
	}
	state := AgentDone
	switch {
	case running:
		state = AgentRunning
	case failed > 0:
		state = AgentFailed
	}
	summary := title + " · " + AgentDuration(total)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d 个失败", failed)
	}
	return Collapsible(c, open, CollapsibleOptions{Title: summary, Trigger: func() {
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			agentMark(c, state, 16, opts.Still)
			mgo.Text(c, title).FontWeight(600).SingleLine()
			agentCaption(c, AgentDuration(total))
			if failed > 0 {
				mgo.Text(c, fmt.Sprintf("%d 个失败", failed)).TextColor(k.Danger).
					FontSize(uikit.FontSize(c, theme.CaptionSize+1)).SingleLine()
			}
		})
	}}, func() {
		for _, call := range calls {
			mgo.Column(c.Key(call.ID)).Children(func() {
				ToolCallCard(c, call, ToolCallCardOptions{Still: opts.Still})
			})
		}
	})
}

// ToolCallCardOptions configure a ToolCallCard.
type ToolCallCardOptions struct {
	// OpenArgs and OpenResult start the sections expanded.
	OpenArgs, OpenResult bool
	// Still stops the running animation.
	Still bool
}

type toolCallCardState struct{ args, result bool }

// ToolCallCard shows a tool call with foldable arguments and result.
func ToolCallCard(c *mgo.Context, call ToolCall, opts ToolCallCardOptions) ChoiceResult {
	if call.Name == "" {
		panic("kit: ToolCallCard needs a Name")
	}
	k := uikit.Tokens(c)
	u := uikit.Unit(c)
	var r ChoiceResult
	r.Element = mgo.Column(c).Shrink(0).Padding(u*3).Gap(u*2).Radius(theme.CardRadius).
		Background(k.Surface).Border(theme.BorderWidth, k.Border).
		Role(mgo.RoleGroup).Label(call.Name + ", " + agentWord(call.State))
	st := mgo.Local(r.Element, "chatgo.toolcall", func() toolCallCardState {
		return toolCallCardState{opts.OpenArgs, opts.OpenResult}
	})
	r.Element.Children(func() {
		mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).MinHeight(theme.MinHitSize).Children(func() {
			agentMark(c, call.State, 16, opts.Still)
			mgo.Text(c, call.Name).Font(theme.MonoFont).FontWeight(600).
				SingleLine().Grow(1).Shrink(1)
			agentCaption(c, agentWord(call.State))
			if call.State == AgentDone || call.State == AgentFailed {
				agentCaption(c, AgentDuration(call.Duration))
			}
		})
		if call.Args != "" {
			changed := Collapsible(c, &st.args, CollapsibleOptions{Title: "参数"}, func() {
				CodeBlock(c, agentPrettyJSON(call.Args), CodeBlockOptions{Language: "json"})
			}).Changed()
			r.changed = r.changed || changed
		}
		body, title := call.Result, "结果"
		if call.Error != "" {
			body, title = call.Error, "错误"
		}
		if body != "" {
			changed := Collapsible(c, &st.result, CollapsibleOptions{Title: title}, func() {
				if call.Error != "" {
					mgo.Text(c, call.Error).TextColor(k.Danger).Selectable()
					return
				}
				CodeBlock(c, agentPrettyJSON(body), CodeBlockOptions{Language: "json"})
			}).Changed()
			r.changed = r.changed || changed
		}
	})
	return r
}

// agentPrettyJSON indents s when it is JSON, and returns it unchanged
// otherwise, so a plain-text result still shows.
func agentPrettyJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(s), "", "  "); err != nil {
		return s
	}
	return buf.String()
}
