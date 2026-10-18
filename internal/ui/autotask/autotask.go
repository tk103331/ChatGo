// Package autotask renders the auto-task page: a list of scheduled tasks with
// status, schedule and controls, plus a create/edit editor and a conversations
// dialog. The State struct is owned by the host window; data and side effects
// flow through the Options callbacks.
package autotask

import (
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdatetime "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/datetime"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	moverlay "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/overlay"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Form is the auto-task editor's working copy.
type Form struct {
	Editing   int
	Name      string
	Agent     string
	Prompt    string
	Workspace string
	Kind      string
	Tod       mdatetime.TimeOfDay
	Weekday   int
	Day       int
	Reuse     bool
	Enabled   bool
}

// State is the auto-task page's mutable state, owned by the host window.
type State struct {
	Form        Form
	EditorOpen  bool
	Running     map[string]bool
	ConvsOpen   bool
	ConvsTaskID string
	Convs       []models.Conversation
}

// Options supplies the page with host data and actions.
type Options struct {
	Config           func() *config.Config
	ChatAgentName    string
	ConvLast         func(conv models.Conversation) time.Time
	SnapshotConvs    func(taskID string) []models.Conversation
	SaveConfig       func() error
	Fail             func(msg string)
	Toast            func(msg string, sev mcore.Severity)
	AskConfirm       func(title, desc string, action func())
	PickDirectory    func(title string, cb func(string))
	NewID            func() string
	OpenConversation func(id string)
	StartTask        func(i int)
	Invalidate       func()
}

// View renders the auto-task page: the task list or the editor.
func View(c *mgo.Context, state *State, opts Options) {
	if state.EditorOpen {
		editor(c, state, opts)
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Column(c).Grow(1).Gap(u / 2).Children(func() {
				mgo.Text(c, "自动任务").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Text(c, "按计划自动运行 Agent，支持每天 / 每周 / 每月").TextColor(k.TextMuted).FontSize(caption)
			})
			if minput.Button(c, "新建任务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				ClearForm(state, opts)
				state.EditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(opts.Config().AutoTasks) == 0 {
				emptyState(c, state, opts)
				return
			}
			mgo.Column(c).FillWidth().Grow(1).AlignItems(mgo.Center).Children(func() {
				mgo.Column(c).FillWidth().MaxWidth(900).Gap(u * 2).Children(func() {
					for i := range opts.Config().AutoTasks {
						card(c, state, opts, i)
					}
				})
			})
		})
	})
	convsDialog(c, state, opts)
}

// emptyState shows a centered placeholder when no tasks exist.
func emptyState(c *mgo.Context, state *State, opts Options) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u*2).
		Padding(u*8, 0).Children(func() {
		mgo.Icon(c, icons.Must("clock")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有自动任务").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "创建任务后，Agent 会按计划自动执行").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Button(c, "新建任务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
			ClearForm(state, opts)
			state.EditorOpen = true
		}
	})
}

// card shows one task with status, schedule, and controls.
func card(c *mgo.Context, state *State, opts Options, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	t := opts.Config().AutoTasks[i]
	running := state.Running[t.ID]

	var dotIcon string
	var dotCol mgo.Color
	switch {
	case running:
		dotIcon, dotCol = "loader-circle", k.Accent
	case strings.HasPrefix(t.LastNote, "失败"):
		dotIcon, dotCol = "circle-x", k.Danger
	case strings.HasPrefix(t.LastNote, "成功"):
		dotIcon, dotCol = "circle-check", k.Success
	case t.Enabled:
		dotIcon, dotCol = "circle", k.Success
	default:
		dotIcon, dotCol = "circle", k.TextMuted
	}

	mgo.Column(c).Gap(u).Padding(u*3).FillWidth().Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).Children(func() {

		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Icon(c, icons.Must(dotIcon)).Size(16, 16).TextColor(dotCol).Shrink(0)
			mgo.Text(c, t.Name).FontWeight(600).Grow(1).SingleLine().MinWidth(0).Ellipsis("…")
			enabled := t.Enabled
			if minput.Switch(c, &enabled, "", minput.SwitchOptions{}).Changed() {
				opts.Config().AutoTasks[i].Enabled = enabled
				_ = opts.SaveConfig()
			}
		})

		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Shrink(0).Children(func() {
				mgo.Icon(c, icons.Must("clock")).Size(14, 14).TextColor(k.TextMuted)
				mgo.Text(c, ScheduleText(t.Schedule)).FontSize(caption).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Shrink(0).Children(func() {
				mgo.Icon(c, icons.Must("bot")).Size(14, 14).TextColor(k.TextMuted)
				mgo.Text(c, AgentLabel(t.Agent, opts.ChatAgentName)).FontSize(caption).TextColor(k.TextMuted)
			})
			mgo.Text(c, ReuseLabel(t.Reuse)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})

		if running {
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "运行中…").FontSize(caption).TextColor(k.Accent)
			})
		} else if !t.LastRun.IsZero() {
			mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				if t.LastNote != "" {
					mgo.Text(c, t.LastNote).FontSize(caption).TextColor(dotCol).Shrink(0)
				}
				mgo.Text(c, "上次: "+t.LastRun.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				nr := NextRun(t.Schedule, t.LastRun)
				if !nr.IsZero() {
					mgo.Text(c, "下次: "+nr.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				}
			})
		} else {
			nr := NextRun(t.Schedule, time.Now())
			if !nr.IsZero() {
				mgo.Text(c, "下次: "+nr.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted)
			}
		}

		mgo.Row(c).Gap(u).Children(func() {
			if minput.Button(c, "运行", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("play")}).Clicked() {
				opts.StartTask(i)
			}
			if minput.Button(c, "会话", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("message-square")}).Clicked() {
				OpenConvs(state, opts, t.ID)
			}
			if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				LoadForm(state, opts, i)
				state.EditorOpen = true
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				Delete(state, opts, i)
			}
		})
	})
}

// OpenConvs snapshots the task's conversations for the dialog.
func OpenConvs(state *State, opts Options, taskID string) {
	state.Convs = opts.SnapshotConvs(taskID)
	state.ConvsTaskID = taskID
	state.ConvsOpen = true
}

// convsDialog lists the conversations a task produced.
func convsDialog(c *mgo.Context, state *State, opts Options) {
	if !state.ConvsOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	title := "任务会话"
	if name := TaskNameOf(opts.Config(), state.ConvsTaskID); name != "" {
		title = name + " · 会话"
	}
	moverlay.Dialog(c, &state.ConvsOpen, moverlay.DialogOptions{
		Title: title,
		Width: 520,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if len(state.Convs) == 0 {
				mgo.Text(c, "该任务还没有会话").TextColor(k.TextMuted)
				return
			}
			mgo.Scroll(c).MaxHeight(400).Children(func() {
				mgo.Column(c).Gap(u).Children(func() {
					for _, conv := range state.Convs {
						conv := conv
						b := mgo.ButtonBase(c).FillWidth().Justify(mgo.Start).Gap(u).MinHeight(mcore.Density(c).ControlHeight()).
							Padding(u, u*2).Radius(mtheme.ControlRadius)
						b.Children(func() {
							mgo.Icon(c, icons.Must("message-square")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
							mgo.Text(c, conv.Title).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
							mgo.Text(c, opts.ConvLast(conv).Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
							mgo.Icon(c, icons.Must("chevron-right")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
						})
						if b.Clicked() {
							state.ConvsOpen = false
							opts.OpenConversation(conv.ID)
						}
					}
				})
			})
		})
	})
}

// ScheduleText renders a schedule for display.
func ScheduleText(s config.Schedule) string {
	hm := fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
	switch s.Kind {
	case "weekly":
		days := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
		d := ""
		if s.Weekday >= 0 && s.Weekday < len(days) {
			d = days[s.Weekday]
		}
		return "每周" + d + " " + hm
	case "monthly":
		return fmt.Sprintf("每月%d日 %s", s.Day, hm)
	default:
		return "每天 " + hm
	}
}

// AgentLabel renders an agent reference for display.
func AgentLabel(agent, chatName string) string {
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		return name
	}
	return chatName
}

// ReuseLabel renders the reuse flag for display.
func ReuseLabel(reuse bool) string {
	if reuse {
		return "复用会话"
	}
	return "每次新建"
}

// TaskNameOf finds a task's name by its ID.
func TaskNameOf(cfg *config.Config, id string) string {
	for _, t := range cfg.AutoTasks {
		if t.ID == id {
			return t.Name
		}
	}
	return ""
}

// NextRun is the first scheduled time strictly after from.
func NextRun(s config.Schedule, from time.Time) time.Time {
	h, m := s.Hour, s.Minute
	switch s.Kind {
	case "weekly":
		t := time.Date(from.Year(), from.Month(), from.Day(), h, m, 0, 0, from.Location())
		for int(t.Weekday()) != s.Weekday || !t.After(from) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	case "monthly":
		day := s.Day
		if day < 1 {
			day = 1
		}
		t := time.Date(from.Year(), from.Month(), day, h, m, 0, 0, from.Location())
		if !t.After(from) {
			t = time.Date(from.Year(), from.Month()+1, day, h, m, 0, 0, from.Location())
		}
		return t
	default:
		t := time.Date(from.Year(), from.Month(), from.Day(), h, m, 0, 0, from.Location())
		if !t.After(from) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	}
}
