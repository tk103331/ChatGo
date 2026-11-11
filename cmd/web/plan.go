package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// planPanel shows the agent's execution plan above the composer, folded by
// default to its progress line, like the desktop conversation.
func (a *app) planPanel(c *ui.Context, t *ui.Theme) {
	cur := a.current()
	if cur == nil || len(cur.Plan) == 0 {
		return
	}
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	key := "plan:" + cur.ID
	open, known := a.folds[key]
	if !known {
		open = true
	}
	done := 0
	for _, e := range cur.Plan {
		if e.Status == models.PlanCompleted {
			done++
		}
	}
	ui.Row(c).FillWidth().Shrink(0).Padding(u*2, u*5).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Radius(mtheme.ControlRadius).
			Background(t.Surface).Border(mtheme.BorderWidth, t.Border).
			Padding(u * 2).Gap(u).Children(func() {
			b := ui.ButtonBase(c).FillWidth().Gap(u*2).AlignItems(ui.Center).
				Padding(u/2, u).Radius(mtheme.ControlRadius).
				Role(ui.RoleGroup).Label(fmt.Sprintf("执行计划 %d/%d", done, len(cur.Plan)))
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() {
				name := "chevron-right"
				if open {
					name = "chevron-down"
				}
				ui.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).
					TextColor(t.TextMuted)
				ui.Text(c, "执行计划").FontWeight(600).FontSize(caption)
				ui.Textf(c, "%d/%d", done, len(cur.Plan)).Shrink(0).
					FontSize(caption).TextColor(t.TextMuted)
				ui.Spacer(c)
				a.planProgress(c, t, done, len(cur.Plan))
			})
			if b.Clicked() {
				a.folds[key] = !open
			}
			if !open {
				return
			}
			ui.Scroll(c).FillWidth().Shrink(0).MaxHeight(u * 40).Children(func() {
				ui.Column(c).FillWidth().Gap(u / 2).Children(func() {
					for i, e := range cur.Plan {
						a.planRow(c, t, i, e)
					}
				})
			})
		})
	})
}

// planProgress is the thin bar that turns the counter into a glance.
func (a *app) planProgress(c *ui.Context, t *ui.Theme, done, total int) {
	u := uikit.Unit(c)
	if total == 0 {
		return
	}
	ui.Box(c).Width(u * 10).Height(3).Shrink(0).Radius(1.5).
		Background(t.Border).Draw(func(p *ui.Painter, r ui.Rect) {
		if done == 0 {
			return
		}
		frac := float32(done) / float32(total)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}, t.Success, 1.5)
	})
}

// planRow is one task: its state mark, its text, and a high-priority marker.
func (a *app) planRow(c *ui.Context, t *ui.Theme, i int, e models.PlanEntry) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Row(c).Key(fmt.Sprintf("plan:%d", i)).FillWidth().Gap(u*2).
		AlignItems(ui.Start).Padding(u/2, u).Children(func() {
		switch e.Status {
		case models.PlanCompleted:
			ui.Icon(c, icons.Must("circle-check")).Size(14, 14).Shrink(0).
				TextColor(t.Success)
		case models.PlanInProgress:
			uikit.Spinner(c, uikit.SpinnerOptions{Size: 12})
		default:
			ui.Icon(c, icons.Must("circle")).Size(14, 14).Shrink(0).
				TextColor(t.TextMuted)
		}
		text := ui.Text(c, e.Content).Shrink(1).MinWidth(0).FontSize(caption).MaxLines(3)
		switch e.Status {
		case models.PlanCompleted, models.PlanPending:
			text.TextColor(t.TextMuted)
		}
		if e.Priority == "high" && e.Status != models.PlanCompleted {
			ui.Box(c).Size(5, 5).Shrink(0).Radius(2.5).Background(t.Danger)
		}
	})
}
