package main

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"
	"strconv"
	"strings"
	"time"

	"chatgo/internal/mobileapi"
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
	t := c.Theme()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mobile.Section(c, "任务", func() {
			for _, task := range a.tasks {
				tk := task
				row := mobile.ListRow(c, mobile.RowOptions{
					Title:      firstNonEmpty(tk.Name, "未命名"),
					Subtitle:   taskSubtitle(tk),
					Disclosure: true,
					Leading: func() {
						listMark(c, "clock", tk.Running)
					},
					Trailing: func() {
						if tk.Running {
							ui.Text(c, "运行中").FontSize(11).TextColor(t.Accent)
						} else if !tk.Enabled {
							ui.Text(c, "已停用").FontSize(11).TextColor(t.TextMuted)
						}
					},
				})
				if row.Clicked() {
					a.openTask(tk.ID)
				}
			}
		})
	})
}

func taskSubtitle(t mobileapi.Task) string {
	parts := []string{scheduleText(t.Schedule)}
	if t.LastRun.IsZero() {
		parts = append(parts, "未运行")
	} else {
		parts = append(parts, shortTime(t.LastRun))
		if t.LastNote != "" {
			parts = append(parts, t.LastNote)
		}
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
			ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
				ui.Text(c, task.Name).FontSize(20).FontWeight(600)
				ui.Text(c, scheduleText(task.Schedule)).FontSize(13).TextColor(t.TextMuted)
				if task.LastNote != "" {
					ui.Text(c, "上次: "+task.LastNote).FontSize(13).TextColor(t.TextMuted)
				}
				ui.Text(c, task.Prompt).FontSize(14).MaxLines(20)
				if task.Workspace != "" {
					ui.Text(c, task.Workspace).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
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
					ui.Spacer(c)
					if kit.Button(c, "删除", kit.ButtonOptions{
						Variant: kit.Ghost, Foreground: t.Danger,
					}).Height(34).Padding(0, 10).Clicked() {
						a.deleteTask(task.ID)
					}
				})
				ui.Text(c, "运行历史").FontSize(15).FontWeight(600)
				if len(a.taskSess) == 0 {
					ui.Text(c, "还没有运行记录").FontSize(13).TextColor(t.TextMuted)
				}
				for _, s := range a.taskSess {
					session := s
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:      firstNonEmpty(session.Label, session.ID),
						Subtitle:   shortTime(session.Updated),
						Disclosure: true,
					})
					if row.Clicked() {
						a.openChat(session.ID, "", "")
					}
				}
			})
		})
	})
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
		ui.Column(c).FillWidth().Padding(16).Gap(10).Children(func() {
			title := "新建任务"
			if a.taskEditing != "" {
				title = "编辑任务"
			}
			ui.Text(c, title).FontSize(17).FontWeight(600)
			ui.TextInput(c, &a.taskForm.Name).Placeholder("任务名称")
			ui.TextInput(c, &a.taskForm.Prompt).Placeholder("提示词").MaxLines(5)
			ui.Text(c, "Agent").FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, item := range append([]mobileapi.Agent{{Ref: "", Name: "Chat", Kind: "chat"}}, a.agents...) {
					if item.Disabled {
						continue
					}
					label := item.Name
					if a.taskForm.Agent == item.Ref {
						label = "● " + label
					}
					ref := item.Ref
					if secondaryButton(c, label).Clicked() {
						a.taskForm.Agent = ref
						a.invalidate()
					}
				}
			})
			ui.Text(c, "计划").FontSize(13).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, kind := range []struct{ key, label string }{
					{"daily", "每天"}, {"weekly", "每周"}, {"monthly", "每月"}, {"manual", "手动"},
				} {
					kind := kind
					label := kind.label
					if a.taskForm.Schedule.Kind == kind.key {
						label = "● " + label
					}
					if secondaryButton(c, label).Clicked() {
						a.taskForm.Schedule.Kind = kind.key
						a.invalidate()
					}
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &a.taskHour).Width(64).Placeholder("时")
				ui.TextInput(c, &a.taskMinute).Width(64).Placeholder("分")
				ui.Text(c, "时 / 分（0-23 / 0-59）").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			if a.taskForm.Schedule.Kind == "weekly" {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.taskWeekday).Width(64).Placeholder("周几")
					ui.Text(c, "0=周日 … 6=周六").FontSize(11).TextColor(c.Theme().TextMuted)
				})
			}
			if a.taskForm.Schedule.Kind == "monthly" {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.taskDay).Width(64).Placeholder("日")
					ui.Text(c, "每月执行日（1-31）").FontSize(11).TextColor(c.Theme().TextMuted)
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
