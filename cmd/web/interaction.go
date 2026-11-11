package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

// interactionPanel shows the question or permission a running turn waits on.
// The browser polls the session, so the card appears within one tick.
func (a *app) interactionPanel(c *ui.Context, t *ui.Theme) {
	p := a.pending
	if p == nil {
		return
	}
	if p.Kind == "permission" {
		a.permissionCard(c, t, p)
		return
	}
	a.askCard(c, t, p)
}

func (a *app) askCard(c *ui.Context, t *ui.Theme, p *interactionInfo) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	if a.askFor != p.ID {
		a.askFor = p.ID
		a.askTab = 0
		a.askAnswers = map[string]string{}
		a.askInputs = map[string]string{}
	}
	if a.askTab >= len(p.Items) {
		a.askTab = 0
	}
	if len(p.Items) == 0 {
		return
	}

	answered := func() bool {
		for i := range p.Items {
			if strings.TrimSpace(a.askAnswers[strconv.Itoa(i)]) == "" {
				return false
			}
		}
		return true
	}

	submitted, cancelled := false, false
	ui.Row(c).FillWidth().Shrink(0).Padding(u*2, u*5).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Radius(mtheme.ControlRadius).
			Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
			Padding(u * 2).Gap(u).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u).Children(func() {
				ui.Icon(c, icons.Must("message-square")).Size(14, 14).
					Shrink(0).TextColor(t.Accent)
				ui.Text(c, "Agent 提问").FontWeight(600).FontSize(caption).Grow(1)
				if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					cancelled = true
				}
			})
			tabs := make([]kit.Tab, 0, len(p.Items))
			for i := range p.Items {
				label := fmt.Sprintf("问题 %d", i+1)
				if strings.TrimSpace(a.askAnswers[strconv.Itoa(i)]) != "" {
					label += " ✓"
				}
				tabs = append(tabs, kit.Tab{Label: label})
			}
			if len(tabs) > 1 {
				kit.Tabs(c, &a.askTab, tabs, kit.TabsOptions{Label: "问题"})
			}
			idx := a.askTab
			it := p.Items[idx]
			key := strconv.Itoa(idx)
			ui.Column(c).FillWidth().Gap(u).Padding(u).Radius(mtheme.ControlRadius).
				Background(t.Background).Border(mtheme.BorderWidth, t.Border).Children(func() {
				ui.Text(c, it.Question).FontSize(uikit.FontSize(c, mtheme.BodySize)).
					TextColor(t.Text).LineHeight(1.5)
				for _, opt := range it.Options {
					opt := opt
					chosen := a.askAnswers[key] == opt
					b := ui.ButtonBase(c).FillWidth().Gap(u).AlignItems(ui.Center).
						Padding(u/2, u).Radius(mtheme.ControlRadius).
						Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
						Label(opt)
					if chosen {
						b.Background(t.Selection)
					} else if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					b.Children(func() {
						if chosen {
							ui.Icon(c, icons.Must("circle-check")).Size(14, 14).
								Shrink(0).TextColor(t.Success)
						}
						ui.Text(c, opt).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
							FontSize(caption)
					})
					if b.Clicked() {
						a.askAnswers[key] = opt
						a.askInputs[key] = opt
					}
				}
				value := a.askInputs[key]
				in := kit.TextInput(c, &value, kit.TextInputOptions{
					Label: "回答", Placeholder: "输入你的回答",
				})
				if in.Changed() {
					a.askInputs[key] = value
					a.askAnswers[key] = strings.TrimSpace(value)
				}
				if in.Submitted() {
					v := strings.TrimSpace(value)
					if v != "" {
						a.askAnswers[key] = v
						if a.askTab < len(p.Items)-1 {
							a.askTab++
						}
					}
				}
				if len(it.Options) > 0 {
					ui.Text(c, "或直接输入回答").FontSize(caption).TextColor(t.TextMuted)
				}
			})
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if kit.Button(c, "提交", kit.ButtonOptions{
					Variant: kit.Primary, Disabled: !answered(),
				}).Clicked() && answered() {
					submitted = true
				}
			})
		})
	})

	switch {
	case cancelled:
		a.respondInteraction("", "", true)
	case submitted:
		a.respondInteraction(a.askResult(p), "", false)
	}
}

// askResult formats the answers the way the desktop tool result does.
func (a *app) askResult(p *interactionInfo) string {
	if len(p.Items) == 1 {
		return a.askAnswers["0"]
	}
	var b strings.Builder
	for i, it := range p.Items {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "问题 %d：%s\n回答：%s", i+1, it.Question, a.askAnswers[strconv.Itoa(i)])
	}
	return b.String()
}

func (a *app) permissionCard(c *ui.Context, t *ui.Theme, p *interactionInfo) {
	req := p.Permission
	if req == nil {
		return
	}
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	selected := ""
	cancelled := false
	ui.Row(c).FillWidth().Shrink(0).Padding(u*2, u*5).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Radius(mtheme.ControlRadius).
			Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
			Padding(u * 2).Gap(u).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(u).Children(func() {
				ui.Icon(c, icons.Must("lock")).Size(14, 14).
					Shrink(0).TextColor(t.Warning)
				ui.Text(c, "Agent 请求权限").FontWeight(600).FontSize(caption).Grow(1)
				if kit.Button(c, "拒绝", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					cancelled = true
				}
			})
			ui.Column(c).FillWidth().Gap(u).Padding(u).Radius(mtheme.ControlRadius).
				Background(t.Background).Border(mtheme.BorderWidth, t.Border).Children(func() {
				ui.Text(c, req.Title).FontSize(uikit.FontSize(c, mtheme.BodySize)).
					TextColor(t.Text).LineHeight(1.5)
				if strings.TrimSpace(req.Detail) != "" {
					ui.Text(c, req.Detail).FontSize(caption).TextColor(t.TextMuted).MaxLines(4)
				}
				for _, opt := range req.Options {
					opt := opt
					icon, color := icons.Must("circle-check"), t.Success
					switch opt.Kind {
					case "allow_always":
						icon = icons.Must("lock-open")
					case "reject_once", "reject_always":
						icon, color = icons.Must("circle-x"), t.Warning
					}
					b := ui.ButtonBase(c).FillWidth().Gap(u).AlignItems(ui.Center).
						Padding(u/2, u).Radius(mtheme.ControlRadius).
						Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
						Label(opt.Name)
					if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					b.Children(func() {
						ui.Icon(c, icon).Size(14, 14).Shrink(0).TextColor(color)
						ui.Text(c, opt.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
							FontSize(caption)
					})
					if b.Clicked() {
						selected = opt.ID
					}
				}
			})
		})
	})
	switch {
	case cancelled:
		a.respondInteraction("", "", true)
	case selected != "":
		a.respondInteraction("", selected, false)
	}
}

// respondInteraction sends the answer and hides the card; the next poll
// confirms the turn moved on.
func (a *app) respondInteraction(answer, optionID string, canceled bool) {
	id := a.getSelected()
	p := a.pending
	if id == "" || p == nil {
		return
	}
	a.pending = nil
	a.interactionDone = p.ID
	a.askFor = ""
	body := map[string]any{"id": p.ID}
	switch {
	case p.Kind == "permission":
		body["option_id"] = optionID
		if canceled {
			body["canceled"] = true
		}
	default:
		body["answer"] = answer
		if canceled {
			body["canceled"] = true
		}
	}
	go func() {
		if err := apiPost(sessionPath(id)+"/interaction", body, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
		}
		a.loadSession(id, false)
	}()
}
