package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// messageRow draws one flat, frameless message row, matching the desktop
// conversation view: the person on the right, the assistant on the left.
func (a *app) messageRow(c *ui.Context, _ int, m models.Message) {
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

func (a *app) messageSegments(c *ui.Context, m models.Message) {
	for i, seg := range m.Segments {
		switch seg.Kind {
		case "reasoning":
			if strings.TrimSpace(seg.Text) != "" {
				a.reasoningBlock(c, fmt.Sprintf("reason:%s:%d", m.ID, i), seg.Text)
			}
		case "tool":
			if seg.ToolCall != nil {
				a.toolCalls(c, []models.ToolCall{*seg.ToolCall})
			}
		case "text":
			if strings.TrimSpace(seg.Text) != "" {
				a.textBlock(c, m, seg.Text)
			}
		}
	}
	if a.emptySegments(m) && a.isRunning() {
		kit.TypingIndicator(c, "助手")
	}
}

func (a *app) messageLegacy(c *ui.Context, m models.Message) {
	if m.Role == "assistant" && strings.TrimSpace(m.Reasoning) != "" {
		a.reasoningBlock(c, "reason:"+m.ID, m.Reasoning)
	}
	if len(m.ToolCalls) > 0 {
		a.toolCalls(c, m.ToolCalls)
	}
	switch {
	case strings.TrimSpace(m.Content) != "":
		a.textBlock(c, m, m.Content)
	case a.isRunning():
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

func (a *app) reasoningBlock(c *ui.Context, key, text string) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	open := a.folds[key]
	b := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Gap(u*2).
		AlignItems(ui.Center).Padding(u/2, u/2).Radius(mtheme.ControlRadius).
		Role(ui.RoleGroup).Label("思考过程")
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		switch {
		case open:
			ui.Icon(c, icons.Must("chevron-down")).Size(14, 14).
				TextColor(t.TextMuted).Shrink(0)
		case b.Hovered():
			ui.Icon(c, icons.Must("chevron-right")).Size(14, 14).
				TextColor(t.TextMuted).Shrink(0)
		default:
			ui.Icon(c, icons.Must("brain")).Size(14, 14).
				TextColor(t.TextMuted).Shrink(0)
		}
		ui.Text(c, "思考过程").FontSize(caption).TextColor(t.TextMuted)
	})
	if b.Clicked() {
		a.folds[key] = !open
	}
	if !open {
		return
	}
	ui.Scroll(c).FillWidth().Shrink(0).MaxHeight(caption * 1.6 * 8).
		Children(func() {
			ui.Column(c).FillWidth().Padding(u).Radius(mtheme.ControlRadius).
				Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
				Children(func() {
					kit.MarkdownView(c, text)
				})
		})
}

func (a *app) toolCalls(c *ui.Context, calls []models.ToolCall) {
	t := c.Theme()
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	for _, tc := range calls {
		ui.Column(c).FillWidth().Padding(u, u*2.5).
			Radius(mtheme.ControlRadius).Background(t.Surface).
			Border(mtheme.BorderWidth, t.Border).Gap(u / 2).Children(func() {
			ui.Row(c).Gap(u).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons.Must("wrench")).Size(14, 14).
					TextColor(t.TextMuted)
				ui.Textf(c, "工具 · %s", tc.Name).FontSize(caption).Bold().
					SingleLine().Ellipsis("…")
			})
			if strings.TrimSpace(tc.Arguments) != "" {
				ui.Text(c, clip(tc.Arguments, 240)).FontSize(caption).
					TextColor(t.TextMuted).MaxLines(3)
			}
			if strings.TrimSpace(tc.Result) != "" {
				ui.Text(c, clip(tc.Result, 480)).FontSize(caption).
					TextColor(t.TextMuted).MaxLines(5)
			}
			if strings.TrimSpace(tc.Error) != "" {
				ui.Text(c, tc.Error).FontSize(caption).TextColor(t.Danger)
			}
		})
	}
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
