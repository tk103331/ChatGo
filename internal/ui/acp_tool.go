package ui

import (
	"path/filepath"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/toolview"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// acpToolKind reads the tool category an adapter reported, stored on the
// call's metadata when the update arrived.
func acpToolKind(tc models.ToolCall) string {
	if tc.Metadata != nil {
		if v, ok := tc.Metadata["acp_kind"].(string); ok {
			return v
		}
	}
	return ""
}

// acpToolKindLabel names a tool category for display.
func acpToolKindLabel(kind string) string {
	return toolview.Label(kind)
}

// acpToolKindIcon picks the icon a category reads as.
func acpToolKindIcon(kind string) string {
	return toolview.Icon(kind)
}

// toolCallRows renders a message's tool calls as foldable rows: the
// category mark, a one-line summary, and the call's input and output inside
// the fold. An ACP call reports its category and title; a built-in call is
// read from its name and arguments. The text is muted so a tool trace reads
// as apparatus beside the message, not as part of it.
func (cw *ChatWindow) toolCallRows(c *mgo.Context, msg models.Message, calls []models.ToolCall) {
	isACP := strings.HasPrefix(msg.AuthorID, "acp:")
	for i := range calls {
		tc := calls[i]
		if tc.Name == "" && tc.Arguments == "" {
			continue
		}
		open := mgo.Local(c.Root(), "tool:"+tc.ID, func() bool { return false })
		kind := acpToolKind(tc)
		summary := tc.Name
		if !isACP {
			kind = builtinToolKind(tc.Name, tc.Arguments)
			if s := toolArgSummary(tc.Arguments); s != "" {
				summary = s
			}
		}
		label := acpToolKindLabel(kind)
		filePath := cw.toolReadPath(tc, kind)
		if filePath != "" {
			summary = cw.workspaceRel(filePath)
		}
		if summary == "" {
			summary = label
			label = ""
		}
		cw.toolRow(c, tc, kind, label, summary, filePath, open)
	}
}

// toolRow is one tool call: a clickable line and, while open, its payload.
// The category mark doubles as the fold control: at rest it names the tool,
// and under the pointer it turns into the fold arrow, so the row carries no
// permanent disclosure chrome.
func (cw *ChatWindow) toolRow(c *mgo.Context, tc models.ToolCall, kind, label, summary, filePath string, open *bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	title := summary
	if label != "" {
		title = label + " · " + summary
	}
	b := mgo.ButtonBase(c).FillWidth().Gap(u*2).AlignItems(mgo.Center).
		Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(mgo.RoleGroup).Label(title)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		// The fold arrow replaces the tool mark under the pointer and stays
		// once open, so the way back is always visible.
		if b.Hovered() || *open {
			name := "chevron-right"
			if *open {
				name = "chevron-down"
			}
			mgo.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
		} else {
			mgo.Icon(c, icons.Must(acpToolKindIcon(kind))).Size(14, 14).
				Shrink(0).TextColor(k.TextMuted)
		}
		if label != "" {
			mgo.Text(c, label).Shrink(0).SingleLine().FontWeight(600).
				FontSize(caption).TextColor(k.TextMuted)
		}
		mgo.Row(c).Grow(1).MinWidth(0).Children(func() {
			p := mgo.Text(c, summary).MinWidth(0).Shrink(1).SingleLine().Ellipsis("…").
				FontSize(caption).TextColor(k.TextMuted)
			if filePath != "" {
				p.Font(mtheme.MonoFont).Cursor(mgo.CursorPointer).Tooltip("在新标签页打开")
				if p.Hovered() {
					p.TextColor(k.Accent).Underline()
				}
				if p.Clicked() {
					cw.openPreview(filePath)
				}
			}
		})
		cw.acpToolState(c, tc)
	})
	if b.Clicked() {
		*open = !*open
	}
	if *open {
		cw.acpToolDetail(c, tc)
	}
}

// acpToolState marks a call's run: running, failed, or done with its
// duration. An ACP adapter reports its own status; a built-in call reports
// nothing until it finishes, so an empty call with no result yet is still
// running.
func (cw *ChatWindow) acpToolState(c *mgo.Context, tc models.ToolCall) {
	k := mcore.Tokens(c)
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	running := tc.Status == string(acp.ToolCallStatusInProgress) ||
		tc.Status == string(acp.ToolCallStatusPending) ||
		(tc.Status == "" && tc.Error == "" && tc.Result == "" && tc.DurationMs == 0)
	switch {
	case running:
		mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
	case tc.Error != "" || tc.Status == string(acp.ToolCallStatusFailed):
		mgo.Text(c, "失败").Shrink(0).SingleLine().FontSize(caption).TextColor(k.Danger)
	case tc.DurationMs > 0:
		mgo.Text(c, kit.AgentDuration(time.Duration(tc.DurationMs)*time.Millisecond)).Shrink(0).SingleLine().
			FontSize(caption).TextColor(k.TextMuted)
	}
}

// acpToolDetail is a tool call's fold: its input and output, each in a
// framed, scrollable block, because a tool result is often longer than the
// room the fold has.
func (cw *ChatWindow) acpToolDetail(c *mgo.Context, tc models.ToolCall) {
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	if strings.TrimSpace(tc.Arguments) == "" && strings.TrimSpace(tc.Result) == "" {
		mgo.Text(c, "（无输入输出）").FontSize(caption).TextColor(mcore.Tokens(c).TextMuted)
		return
	}
	mgo.Column(c).FillWidth().Gap(mcore.Density(c).Unit()).Children(func() {
		if args := strings.TrimSpace(tc.Arguments); args != "" {
			cw.toolDetailBlock(c, "输入", args, false)
		}
		if out := strings.TrimSpace(tc.Result); out != "" {
			if v, ok := parseView(out); ok {
				cw.renderView(c, v)
			} else {
				cw.toolDetailBlock(c, "输出", out, tc.Error != "")
			}
		}
	})
}

// toolDetailLines is how many lines of a tool payload stay visible before
// the block scrolls. Ten lines is enough to read a command's output without
// opening anything else.
const toolDetailLines = 10

// toolDetailBlock is one framed, scrollable section of a tool's fold. The
// frame makes it read as a payload rather than as prose, the height cap
// keeps a huge output from pushing the conversation away, and the copy
// button takes the whole body even when only part of it is visible.
func (cw *ChatWindow) toolDetailBlock(c *mgo.Context, title, body string, danger bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	border := k.Border
	if danger {
		border = k.Danger
	}
	mgo.Column(c).FillWidth().Gap(u / 2).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			mgo.Text(c, title).FontWeight(600).FontSize(caption).
				TextColor(k.TextMuted).Grow(1)
			if mcore.IconAction(c, icons.Must("copy"), "复制"+title).Clicked() {
				c.WriteClipboard(body)
				cw.toast("已复制", mcore.SeveritySuccess)
			}
		})
		mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * toolDetailLines).
			Children(func() {
				mgo.Column(c).FillWidth().Padding(u).Radius(mtheme.ControlRadius).
					Background(k.Surface).Border(mtheme.BorderWidth, border).Children(func() {
					text := mgo.Text(c, body).Font(mtheme.MonoFont).FontSize(caption).Selectable()
					if danger {
						text.TextColor(k.Danger)
					} else {
						text.TextColor(k.Text)
					}
				})
			})
	})
}

// builtinToolKind infers a tool's category from its name and arguments, the
// way an ACP adapter reports one explicitly.
func builtinToolKind(name, args string) string {
	return toolview.Kind(name, args)
}

// toolArgField reads one string field out of a tool call's JSON arguments.
func toolArgField(args, field string) string {
	return toolview.ArgField(args, field)
}

func (cw *ChatWindow) toolReadPath(tc models.ToolCall, kind string) string {
	if kind == "" {
		kind = builtinToolKind(tc.Name, tc.Arguments)
	}
	if acp.ToolKind(kind) != acp.ToolKindRead {
		return ""
	}
	if strings.EqualFold(toolArgField(tc.Arguments, "action"), "list") {
		return ""
	}
	lower := strings.ToLower(tc.Name)
	if strings.Contains(lower, "list") || strings.Contains(lower, "glob") {
		return ""
	}
	path := toolArgPath(tc.Arguments)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(cw.fileRoot(), path)
}

func toolArgPath(args string) string {
	return toolview.ArgPath(args)
}

// toolArgSummary is the one-line summary of a built-in call: its most
// telling argument, or its name when the arguments say nothing useful.
func toolArgSummary(args string) string {
	return toolview.ArgSummary(args)
}
