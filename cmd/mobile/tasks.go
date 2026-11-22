package main

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
)

func (a *app) taskList(c *ui.Context) {
	if a.offline() {
		a.offlineState(c)
		return
	}
	if len(a.tasks) == 0 {
		ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
			emptyState(c, "clock", "还没有自动任务", "让桌面按时运行任务，手机查看结果。", func() {
				if primaryButton(c, "新建任务").Clicked() {
					a.openTaskForm("")
				}
			})
		})
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			// The bottom padding keeps the last row clear of the button
			// floating over the list.
			ui.Column(c).FillWidth().Padding(4, 0, 88, 0).Gap(6).Children(func() {
				for _, task := range a.tasks {
					a.taskRow(c, task)
				}
			})
		})
		fabButton(c, "plus", "新建任务", func() { a.openTaskForm("") })
	})
}

// taskRow is one scheduled task: its name, when it runs and when it last
// ran.
func (a *app) taskRow(c *ui.Context, task mobileapi.Task) {
	t := c.Theme()
	title := firstNonEmpty(task.Name, "未命名")
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(12, 16).Gap(12).
		Role(ui.RoleButton).Label(title + ", " + taskSubtitle(task))
	mobile.Ink(row)
	if mobile.Current() == mobile.Cupertino && row.Pressed() {
		row.Background(t.SurfacePressed)
	}
	row.Children(func() {
		listMark(c, "clock", task.Running)
		ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Text(c, taskSubtitle(task)).FontSize(13).TextColor(t.TextMuted).
				MaxLines(1).Ellipsis("…")
		})
		ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(4).Children(func() {
			if task.Running {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent).Shrink(0)
					ui.Text(c, "运行中").FontSize(10).TextColor(t.Accent)
				})
			} else if !task.Enabled {
				ui.Text(c, "已停用").FontSize(10).TextColor(t.TextMuted)
			}
			if !task.LastRun.IsZero() {
				ui.Text(c, shortTime(task.LastRun)).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
	if row.Clicked() {
		a.openTask(task.ID)
	}
}

// taskSubtitle says when the task runs and when it last ran.
func taskSubtitle(t mobileapi.Task) string {
	parts := []string{scheduleText(t.Schedule)}
	if t.LastRun.IsZero() {
		parts = append(parts, "未运行")
	} else {
		parts = append(parts, "上次 "+shortTime(t.LastRun))
	}
	return strings.Join(parts, " · ")
}

func scheduleText(s mobileapi.Schedule) string {
	clock := pad2(s.Hour) + ":" + pad2(s.Minute)
	switch s.Kind {
	case "weekly":
		return "每周 " + weekdayName(s.Weekday) + " " + clock
	case "monthly":
		return "每月 " + strconv.Itoa(s.Day) + " 日 " + clock
	case "manual":
		return "手动"
	default:
		return "每天 " + clock
	}
}

func pad2(v int) string {
	if v < 10 {
		return "0" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

func weekdayName(i int) string {
	names := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	if i < 0 || i >= len(names) {
		return "周一"
	}
	return names[i]
}

func (a *app) openTask(id string) {
	for i := range a.tasks {
		if a.tasks[i].ID == id {
			task := a.tasks[i]
			a.update(func() {
				a.screen = screenTask
				a.taskID = id
				a.task = &task
				a.taskSess = nil
			})
			a.loadTaskSessions(id)
			return
		}
	}
	a.errorf("任务不存在: %s", id)
}

func (a *app) loadTaskSessions(id string) {
	if a.client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sessions, err := a.client.TaskSessions(ctx, id)
		if err != nil {
			a.errorf("读取任务历史失败: %v", err)
			return
		}
		a.update(func() {
			if a.taskID == id {
				a.taskSess = sessions
			}
		})
	}()
}

func (a *app) taskView(c *ui.Context) {
	t := c.Theme()
	task := a.task
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if task == nil {
			ui.Text(c, "加载中…").Padding(16).TextColor(t.TextMuted)
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(12, 16, 16, 16).Gap(10).Children(func() {
				ui.Text(c, firstNonEmpty(task.Name, "未命名")).FontSize(19).FontWeight(600).
					SingleLine().Ellipsis("…")
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Padding(3, 8).Radius(8).
						Background(t.Surface).Children(func() {
						ui.Icon(c, icons.Must("clock")).Size(12, 12).TextColor(t.TextMuted).Shrink(0)
						ui.Text(c, scheduleText(task.Schedule)).FontSize(12).TextColor(t.Text)
					})
					if task.Running {
						ui.Row(c).Gap(4).AlignItems(ui.Center).Padding(3, 8).Radius(8).
							Background(t.Accent.Alpha(0.12)).Children(func() {
							ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent).Shrink(0)
							ui.Text(c, "运行中").FontSize(12).TextColor(t.Accent)
						})
					} else if !task.Enabled {
						ui.Text(c, "已停用").FontSize(12).TextColor(t.TextMuted)
					}
					if task.Agent != "" {
						ui.Text(c, a.agentLabel(task.Agent)).FontSize(12).TextColor(t.TextMuted).
							SingleLine().Ellipsis("…").MaxWidth(140)
					}
				})
				if task.Prompt != "" {
					ui.Text(c, task.Prompt).FontSize(13).TextColor(t.Text).MaxLines(4).Ellipsis("…")
				}
				if task.Workspace != "" {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, icons.Must("folder")).Size(13, 13).TextColor(t.TextMuted).Shrink(0)
						ui.Text(c, filepath.Base(task.Workspace)).FontSize(11).TextColor(t.TextMuted).
							SingleLine().Ellipsis("…")
					})
				}
				if task.LastNote != "" {
					ui.Text(c, "上次: "+task.LastNote).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
				}
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					if primaryButton(c, "立即运行").Clicked() {
						a.runTask(task.ID)
					}
					label := "停用"
					if !task.Enabled {
						label = "启用"
					}
					if secondaryButton(c, label).Clicked() {
						a.toggleTask(task)
					}
					if ghostButton(c, "编辑").Clicked() {
						a.openTaskForm(task.ID)
					}
				})
				ui.Text(c, "运行历史").FontSize(14).FontWeight(600)
				if len(a.taskSess) == 0 {
					ui.Text(c, "还没有运行记录").FontSize(13).TextColor(t.TextMuted)
				}
				for _, s := range a.taskSess {
					a.taskSessionRow(c, s)
				}
				// Deleting a task takes its history with it, so the button
				// sits at the end rather than in the bar.
				ui.Row(c).FillWidth().Padding(0, 0, 24, 0).Justify(ui.Center).Children(func() {
					if kit.Button(c, "删除任务", kit.ButtonOptions{
						Variant: kit.Ghost, Foreground: t.Danger,
					}).Height(34).Padding(0, 12).Clicked() {
						a.deleteTask(task.ID)
					}
				})
			})
		})
	})
}

// taskSessionRow is one run of a task: when it happened and what it said.
func (a *app) taskSessionRow(c *ui.Context, s mobileapi.RoomSession) {
	t := c.Theme()
	title := firstNonEmpty(s.Label, s.ID)
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(12, 0).Gap(12).
		Role(ui.RoleButton).Label(title)
	mobile.Ink(row)
	row.Children(func() {
		listMark(c, "message-square", false)
		ui.Text(c, title).FontSize(14).Grow(1).Basis(0).SingleLine().Ellipsis("…")
		if !s.Updated.IsZero() {
			ui.Text(c, shortTime(s.Updated)).FontSize(11).TextColor(t.TextMuted).Shrink(0)
		}
	})
	if row.Clicked() {
		a.openChat(s.ID, "", "")
	}
}

func (a *app) runTask(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := a.client.RunTask(ctx, id)
		if err != nil {
			a.errorf("运行任务失败: %v", err)
			return
		}
		a.loadTaskSessions(id)
		if res.SessionID != "" {
			a.openChat(res.SessionID, "", "")
		}
	}()
}

func (a *app) toggleTask(task *mobileapi.Task) {
	in := mobileapi.TaskInput{
		Name:      task.Name,
		Agent:     task.Agent,
		Prompt:    task.Prompt,
		Workspace: task.Workspace,
		Schedule:  task.Schedule,
		Reuse:     task.Reuse,
		Enabled:   !task.Enabled,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := a.client.UpdateTask(ctx, task.ID, in); err != nil {
			a.errorf("更新任务失败: %v", err)
			return
		}
		a.refreshTasks()
		a.openTask(task.ID)
	}()
}

func (a *app) deleteTask(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.client.DeleteTask(ctx, id); err != nil {
			a.errorf("删除任务失败: %v", err)
			return
		}
		a.update(func() {
			a.screen = screenTasks
			a.task = nil
			a.taskID = ""
		})
		a.refreshTasks()
	}()
}

func (a *app) openTaskForm(id string) {
	form := mobileapi.TaskInput{
		Schedule: mobileapi.Schedule{Kind: "daily", Hour: 9, Minute: 0},
		Enabled:  true,
	}
	if id != "" {
		for _, task := range a.tasks {
			if task.ID == id {
				form = mobileapi.TaskInput{
					Name:      task.Name,
					Agent:     task.Agent,
					Prompt:    task.Prompt,
					Workspace: task.Workspace,
					Schedule:  task.Schedule,
					Reuse:     task.Reuse,
					Enabled:   task.Enabled,
				}
				break
			}
		}
	}
	a.update(func() {
		a.taskFormOpen = true
		a.taskEditing = id
		a.taskForm = form
		a.taskHour = strconv.Itoa(form.Schedule.Hour)
		a.taskMinute = strconv.Itoa(form.Schedule.Minute)
		a.taskWeekday = strconv.Itoa(form.Schedule.Weekday)
		a.taskDay = strconv.Itoa(form.Schedule.Day)
	})
}

func (a *app) taskFormSheet(c *ui.Context) {
	mobile.Sheet(c, &a.taskFormOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "任务"}, func() {
		t := c.Theme()
		ui.Column(c).FillWidth().Padding(16).Gap(10).Children(func() {
			title := "新建任务"
			if a.taskEditing != "" {
				title = "编辑任务"
			}
			ui.Text(c, title).FontSize(17).FontWeight(600)
			ui.TextInput(c, &a.taskForm.Name).Placeholder("任务名称")
			ui.TextInput(c, &a.taskForm.Prompt).Placeholder("提示词").MaxLines(5)
			ui.Text(c, "Agent").FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, item := range append([]mobileapi.Agent{{Ref: "", Name: "Chat", Kind: "chat"}}, a.agents...) {
					if item.Disabled {
						continue
					}
					item := item
					selected := a.taskForm.Agent == item.Ref
					fg := t.Text
					if selected {
						fg = t.AccentText
					}
					chip := kit.Button(c, firstNonEmpty(item.Name, item.Ref), kit.ButtonOptions{
						Variant: chipVariant(selected),
						Leading: func() {
							a.agentMarkColor(c, item.Ref, 14, fg)
						},
					}).Height(34).Padding(0, 12)
					if chip.Clicked() {
						a.taskForm.Agent = item.Ref
						a.invalidate()
					}
				}
			})
			ui.Text(c, "计划").FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, kind := range []struct{ key, label string }{
					{"daily", "每天"}, {"weekly", "每周"}, {"monthly", "每月"}, {"manual", "手动"},
				} {
					kind := kind
					selected := a.taskForm.Schedule.Kind == kind.key
					chip := kit.Button(c, kind.label, kit.ButtonOptions{
						Variant: chipVariant(selected),
					}).Height(34).Padding(0, 12)
					if chip.Clicked() {
						a.taskForm.Schedule.Kind = kind.key
						a.invalidate()
					}
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.taskHour).Width(64).Placeholder("时")
				ui.TextInput(c, &a.taskMinute).Width(64).Placeholder("分")
				ui.Text(c, "0-23 时 / 0-59 分").FontSize(11).TextColor(t.TextMuted)
			})
			if a.taskForm.Schedule.Kind == "weekly" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c, &a.taskWeekday).Width(64).Placeholder("周几")
					ui.Text(c, "0 = 周日").FontSize(11).TextColor(t.TextMuted)
				})
			}
			if a.taskForm.Schedule.Kind == "monthly" {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.TextInput(c, &a.taskDay).Width(64).Placeholder("日")
					ui.Text(c, "1-31 日").FontSize(11).TextColor(t.TextMuted)
				})
			}
			ui.Row(c).Gap(8).Children(func() {
				if primaryButton(c, "保存").Clicked() {
					a.saveTaskForm()
				}
				if ghostButton(c, "取消").Clicked() {
					a.taskFormOpen = false
				}
			})
		})
	})
}

// chipVariant picks how a choice chip looks: the chosen one stands out.
func chipVariant(selected bool) kit.ButtonVariant {
	if selected {
		return kit.Primary
	}
	return kit.Secondary
}

func (a *app) saveTaskForm() {
	name := strings.TrimSpace(a.taskForm.Name)
	prompt := strings.TrimSpace(a.taskForm.Prompt)
	if name == "" || prompt == "" {
		a.toast("任务名称和提示词不能为空")
		return
	}
	hour, err := strconv.Atoi(strings.TrimSpace(a.taskHour))
	if err != nil {
		a.toast("小时必须是数字")
		return
	}
	minute, err := strconv.Atoi(strings.TrimSpace(a.taskMinute))
	if err != nil {
		a.toast("分钟必须是数字")
		return
	}
	a.taskForm.Name = name
	a.taskForm.Prompt = prompt
	a.taskForm.Schedule.Hour = hour
	a.taskForm.Schedule.Minute = minute
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		a.toast("时间超出范围")
		return
	}
	if a.taskForm.Schedule.Kind == "weekly" {
		weekday, err := strconv.Atoi(strings.TrimSpace(a.taskWeekday))
		if err != nil || weekday < 0 || weekday > 6 {
			a.toast("周几必须是 0-6")
			return
		}
		a.taskForm.Schedule.Weekday = weekday
	}
	if a.taskForm.Schedule.Kind == "monthly" {
		day, err := strconv.Atoi(strings.TrimSpace(a.taskDay))
		if err != nil || day < 1 || day > 31 {
			a.toast("执行日必须是 1-31")
			return
		}
		a.taskForm.Schedule.Day = day
	}
	editing := a.taskEditing
	in := a.taskForm
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		if editing == "" {
			_, err = a.client.CreateTask(ctx, in)
		} else {
			_, err = a.client.UpdateTask(ctx, editing, in)
		}
		if err != nil {
			a.errorf("保存任务失败: %v", err)
			return
		}
		a.update(func() { a.taskFormOpen = false })
		a.refreshTasks()
	}()
}
