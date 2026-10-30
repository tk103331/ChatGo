package ui

import (
	"fmt"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// planPanel shows the agent's execution plan above the composer, as a card
// that stays in view while the conversation scrolls. It folds when the list
// is long; an empty plan renders nothing.
func (cw *ChatWindow) planPanel(c *mgo.Context) {
	cw.planPanelFor(c, cw.cur)
}

// planPanelFor renders the plan of conv, so the room view can show the plan
// its own conversation carries.
func (cw *ChatWindow) planPanelFor(c *mgo.Context, conv *models.Conversation) {
	if conv == nil || len(conv.Plan) == 0 {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	plan := conv.Plan
	done := 0
	for _, e := range plan {
		if e.Status == models.PlanCompleted {
			done++
		}
	}
	open := mgo.Local(c.Root(), "plan:open:"+conv.ID, func() bool { return true })
	mgo.Column(c).FillWidth().Shrink(0).AlignItems(mgo.Start).Padding(u*2, u*4).Children(func() {
		mgo.Column(c).FillWidth().Shrink(0).
			Radius(mtheme.ControlRadius).Background(k.Surface).
			Border(mtheme.BorderWidth, k.Border).Padding(u * 2).Gap(u).Children(func() {
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				b := mgo.ButtonBase(c).Grow(1).Gap(u*2).AlignItems(mgo.Center).
					Padding(u/2, u).Radius(mtheme.ControlRadius).
					Role(mgo.RoleGroup).Label(fmt.Sprintf("执行计划 %d/%d", done, len(plan)))
				if b.Hovered() {
					b.Background(k.SurfaceHover)
				}
				b.Children(func() {
					name := "chevron-right"
					if *open {
						name = "chevron-down"
					}
					mgo.Icon(c, icons.Must(name)).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
					mgo.Text(c, "执行计划").FontWeight(600).FontSize(caption)
					mgo.Text(c, fmt.Sprintf("%d/%d", done, len(plan))).Shrink(0).
						FontSize(caption).TextColor(k.TextMuted)
					mgo.Spacer(c)
					cw.planProgress(c, done, len(plan))
				})
				if b.Clicked() {
					*open = !*open
				}
				if mcore.IconAction(c, icons.Must("x"), "关闭执行计划").Clicked() {
					cw.closePlan(conv)
				}
			})
			if !*open {
				return
			}
			mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(u * 40).Children(func() {
				mgo.Column(c).FillWidth().Gap(u / 2).Children(func() {
					for i, e := range plan {
						cw.planRow(c, i, e)
					}
				})
			})
		})
	})
}

// planProgress is the thin bar that turns the counter into a glance: how much
// of the job is behind the agent.
func (cw *ChatWindow) planProgress(c *mgo.Context, done, total int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	if total == 0 {
		return
	}
	mgo.Box(c).Width(u * 10).Height(3).Shrink(0).Radius(1.5).
		Background(k.Border).Draw(func(p *mgo.Painter, r mgo.Rect) {
		frac := float32(done) / float32(total)
		if frac > 0 {
			p.Fill(mgo.Rect{X: r.X, Y: r.Y, W: r.W * frac, H: r.H}, k.Success, 1.5)
		}
	})
}

// planRow is one task: its state mark, its text, and a high-priority marker.
func (cw *ChatWindow) planRow(c *mgo.Context, i int, e models.PlanEntry) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Row(c).Key(fmt.Sprintf("plan:%d", i)).FillWidth().Gap(u*2).
		AlignItems(mgo.Start).Padding(u/2, u).Children(func() {
		switch e.Status {
		case models.PlanCompleted:
			mgo.Icon(c, icons.Must("circle-check")).Size(14, 14).Shrink(0).TextColor(k.Success)
		case models.PlanInProgress:
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
		default:
			mgo.Icon(c, icons.Must("circle")).Size(14, 14).Shrink(0).TextColor(k.TextMuted)
		}
		text := mgo.Text(c, e.Content).Shrink(1).MinWidth(0).FontSize(caption).MaxLines(3)
		switch e.Status {
		case models.PlanCompleted, models.PlanPending:
			text.TextColor(k.TextMuted)
		}
		if e.Priority == "high" && e.Status != models.PlanCompleted {
			// A dot by the state mark, not a label at the far edge: the row
			// stays compact and the eye still catches the urgent task.
			mgo.Box(c).Size(5, 5).Shrink(0).Radius(2.5).Background(k.Danger)
		}
	})
}

// closePlan drops the conversation's plan, hiding the panel until the agent
// records a new one.
func (cw *ChatWindow) closePlan(conv *models.Conversation) {
	if conv == nil || len(conv.Plan) == 0 {
		return
	}
	conv.Plan = nil
	_ = cw.convManager.SaveConversation(conv)
	cw.invalidate()
}

// closeDonePlan drops a plan whose every entry is done. Sending a new message
// starts a new job, so a finished plan from the last one should not linger.
func (cw *ChatWindow) closeDonePlan(conv *models.Conversation) {
	if conv == nil || len(conv.Plan) == 0 {
		return
	}
	for _, e := range conv.Plan {
		if e.Status != models.PlanCompleted {
			return
		}
	}
	conv.Plan = nil
}
