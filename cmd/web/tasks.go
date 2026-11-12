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

// taskForm edits one scheduled task; id is empty for a new one.
type taskForm struct {
	id        string
	name      string
	prompt    string
	agent     string
	workspace string
	kind      string
	hour      string
	minute    string
	weekday   int
	day       string
	reuse     bool
	enabled   bool
}

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

func scheduleLabel(s scheduleInfo) string {
	switch s.Kind {
	case "daily":
		return fmt.Sprintf("每天 %02d:%02d", s.Hour, s.Minute)
	case "weekly":
		w := s.Weekday
		if w < 0 || w > 6 {
			w = 0
		}
		return fmt.Sprintf("每%s %02d:%02d", weekdays[w], s.Hour, s.Minute)
	case "monthly":
		return fmt.Sprintf("每月 %d 日 %02d:%02d", s.Day, s.Hour, s.Minute)
	default:
		return "手动"
	}
}

func scheduleKindOptions() []kit.SelectOption[string] {
	return []kit.SelectOption[string]{
		{Value: "daily", Label: "每天"},
		{Value: "weekly", Label: "每周"},
		{Value: "monthly", Label: "每月"},
	}
}

func weekdayOptions() []kit.SelectOption[int] {
	out := make([]kit.SelectOption[int], 0, 7)
	for i, name := range weekdays {
		out = append(out, kit.SelectOption[int]{Value: i, Label: name})
	}
	return out
}

func (a *app) refreshTasks() {
	var resp struct {
		Tasks []taskInfo `json:"tasks"`
	}
	if err := apiGet("/api/tasks", &resp); err != nil {
		a.win.Update(func() { a.status = "任务列表：" + err.Error() })
		return
	}
	a.win.Update(func() { a.tasks = resp.Tasks })
}

func (a *app) refreshTaskSessions() {
	taskID := a.getTaskID()
	if taskID == "" {
		return
	}
	var resp struct {
		Sessions []roomSessionInfo `json:"sessions"`
	}
	if err := apiGet("/api/tasks/"+taskID+"/sessions", &resp); err != nil {
		a.win.Update(func() { a.status = "任务会话：" + err.Error() })
		return
	}
	a.win.Update(func() {
		if a.getTaskID() != taskID {
			return
		}
		a.taskSessions = resp.Sessions
		if a.getTaskConv() == "" && len(resp.Sessions) > 0 && a.getSection() == secTasks {
			a.setTaskConv(resp.Sessions[0].ID)
			go a.loadSession(resp.Sessions[0].ID, false)
		}
	})
}

// openTask shows a task and its newest run.
func (a *app) openTask(id string) {
	if id == "" {
		return
	}
	a.setTaskID(id)
	a.setTaskConv("")
	a.win.Update(func() {
		a.taskSessions = nil
		a.pending = nil
		a.draft = ""
		a.sidebarOpen = false
	})
	go func() {
		var resp struct {
			Sessions []roomSessionInfo `json:"sessions"`
		}
		if err := apiGet("/api/tasks/"+id+"/sessions", &resp); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.win.Update(func() {
			if a.getTaskID() != id {
				return
			}
			a.taskSessions = resp.Sessions
			if len(resp.Sessions) > 0 {
				a.setTaskConv(resp.Sessions[0].ID)
				a.msgList.ScrollToEnd()
			}
		})
		if len(resp.Sessions) > 0 {
			a.loadSession(resp.Sessions[0].ID, true)
		}
	}()
}

func (a *app) currentTask() *taskInfo {
	for i := range a.tasks {
		if a.tasks[i].ID == a.getTaskID() {
			return &a.tasks[i]
		}
	}
	return nil
}

// runTask starts a task now.
func (a *app) runTask(task *taskInfo) {
	if task == nil || task.Running || a.isRunning() {
		return
	}
	id := task.ID
	a.status = "正在运行任务…"
	a.win.Update(func() {})
	go func() {
		var resp struct {
			SessionID string `json:"session_id"`
		}
		if err := apiPost("/api/tasks/"+id+"/run", nil, &resp); err != nil {
			a.win.Update(func() { a.status = "运行任务失败：" + err.Error() })
			return
		}
		a.win.Update(func() {
			if a.getTaskID() != id {
				return
			}
			if resp.SessionID != "" {
				a.setTaskConv(resp.SessionID)
				a.msgList.ScrollToEnd()
			}
			a.status = ""
			a.setRunning(true)
		})
		if resp.SessionID != "" {
			a.loadSession(resp.SessionID, false)
		}
		a.refreshTasks()
	}()
}

// toggleTask flips a task's enabled switch.
func (a *app) toggleTask(task *taskInfo, on bool) {
	if task == nil {
		return
	}
	in := taskInput{
		Name: task.Name, Agent: task.Agent, Prompt: task.Prompt,
		Workspace: task.Workspace, Schedule: task.Schedule,
		Reuse: task.Reuse, Enabled: on,
	}
	id := task.ID
	task.Enabled = on
	go func() {
		if err := apiPatch("/api/tasks/"+id, in, nil); err != nil {
			a.win.Update(func() { a.status = "更新任务失败：" + err.Error() })
		}
		a.refreshTasks()
	}()
}

func (a *app) stopTask() {
	id := a.getTaskConv()
	if id == "" {
		return
	}
	a.status = "正在停止…"
	a.win.Update(func() {})
	go func() {
		if err := apiPost(sessionPath(id)+"/cancel", nil, nil); err != nil {
			a.win.Update(func() { a.status = err.Error() })
		}
	}()
}

// taskList draws the task sidebar.
func (a *app) taskList(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Scroll(c).Grow(1).MinHeight(0).Gap(u / 2).Children(func() {
		if len(a.tasks) == 0 {
			ui.Text(c, "还没有自动任务，点上方新建一个").FontSize(caption).
				TextColor(t.TextMuted).Padding(u*2, u*2)
			return
		}
		for i := range a.tasks {
			task := a.tasks[i]
			selected := task.ID == a.getTaskID()
			row := ui.ButtonBase(c.Key("task:"+task.ID)).FillWidth().
				MinHeight(uikit.Density(c).ControlHeight()).Padding(u*1.5, u*2).
				Gap(u).AlignItems(ui.Start).Radius(mtheme.ControlRadius).
				Label(task.Name)
			if selected {
				row.Background(t.Selection)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Icon(c, icons.Must("clock")).Size(16, 16).
					TextColor(t.Accent).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
					ui.Row(c).FillWidth().Gap(u).AlignItems(ui.Center).Children(func() {
						name := ui.Text(c, task.Name).SingleLine().Ellipsis("…").
							Grow(1).MinWidth(0)
						if selected {
							name.TextColor(t.Accent)
						}
						if task.Running {
							ui.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).
								Background(t.Success)
						} else if task.Enabled {
							ui.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).
								Background(t.Border)
						}
					})
					sub := scheduleLabel(task.Schedule)
					if task.LastNote != "" {
						sub += " · " + task.LastNote
					}
					ui.Text(c, sub).FontSize(caption).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…")
				})
			})
			if row.Clicked() {
				a.openTask(task.ID)
			}
		}
	})
}

// taskHeader is the open task's title and controls.
func (a *app) taskHeader(c *ui.Context, t *ui.Theme, narrow bool) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	task := a.currentTask()
	title, sub := "自动任务", "选择一个任务"
	if task != nil {
		title = task.Name
		sub = scheduleLabel(task.Schedule)
		if task.Agent != "" {
			sub += " · " + agentLabel(task.Agent)
		}
		if task.LastNote != "" {
			sub += " · " + task.LastNote
		}
	}
	if a.isRunning() {
		sub = "正在运行…"
	}
	ui.Row(c).Shrink(0).Padding(u*3, u*5).Gap(u*2).AlignItems(ui.Center).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(t.Border).
		Background(t.Background).Children(func() {
		if narrow {
			if kit.IconButton(c, icons.Must("panel-left"), "任务列表", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.sidebarOpen = true
			}
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
			ui.Text(c, title).Bold().FontSize(15).SingleLine().Ellipsis("…")
			ui.Text(c, sub).FontSize(caption).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…")
		})
		if task != nil {
			enabled := task.Enabled
			if kit.SwitchRow(c, &enabled, "调度", kit.SwitchRowOptions{Compact: true}).Changed() {
				a.toggleTask(task, enabled)
			}
			if a.isRunning() {
				if kit.IconButton(c, icons.Must("square"), "停止", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
					a.stopTask()
				}
			} else if kit.IconButton(c, icons.Must("play"), "立即运行", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.runTask(task)
			}
			if kit.IconButton(c, icons.Must("history"), "运行记录", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.taskSessionsOpen = !a.taskSessionsOpen
			}
			if kit.IconButton(c, icons.Must("settings"), "编辑任务", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.editTask(task)
			}
			if kit.IconButton(c, icons.Must("circle-x"), "删除任务", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.taskDeleteOpen = true
			}
		}
	})
}

// taskSessionMenu lists a task's runs under the header.
func (a *app) taskSessionMenu(c *ui.Context, t *ui.Theme) {
	if !a.taskSessionsOpen || a.getTaskID() == "" {
		return
	}
	u := uikit.Unit(c)
	ui.Row(c).FillWidth().Shrink(0).Padding(u, u*5).Gap(u).Wrap().
		AlignItems(ui.Center).Children(func() {
		if len(a.taskSessions) == 0 {
			ui.Text(c, "还没有运行记录").FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
				TextColor(t.TextMuted)
			return
		}
		for _, s := range a.taskSessions {
			selected := s.ID == a.getTaskConv()
			b := kit.Button(c, s.Label, kit.ButtonOptions{Variant: kit.Ghost})
			if selected {
				b.Background(t.Selection)
			}
			if b.Clicked() {
				a.taskSessionsOpen = false
				a.setTaskConv(s.ID)
				go a.loadSession(s.ID, true)
			}
		}
	})
}

// taskEmpty is the task page with no run selected.
func (a *app) taskEmpty(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	task := a.currentTask()
	if task == nil {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(u * 2).
			Padding(u * 8).Children(func() {
			ui.Icon(c, icons.Must("clock")).Size(32, 32).TextColor(t.Accent)
			ui.Text(c, "自动任务").Bold().FontSize(22)
			ui.Text(c, "选择一个任务查看运行记录，或立即运行一次").TextColor(t.TextMuted)
		})
		return
	}
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(u * 3).
		Padding(u * 8).Children(func() {
		ui.Icon(c, icons.Must("clock")).Size(32, 32).TextColor(t.Accent)
		ui.Text(c, task.Name).Bold().FontSize(22)
		ui.Text(c, scheduleLabel(task.Schedule)).TextColor(t.TextMuted)
		ui.Text(c, task.Prompt).MaxLines(6).TextColor(t.TextMuted).
			FontSize(caption).MaxWidth(560)
		if kit.Button(c, "立即运行", kit.ButtonOptions{
			Variant: kit.Primary, Icon: icons.Must("play"),
		}).Clicked() {
			a.runTask(task)
		}
	})
}

// taskReadOnly explains that a task conversation has no composer.
func (a *app) taskReadOnly(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	ui.Row(c).Shrink(0).Padding(u*3, u*5).
		BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "自动任务的运行记录为只读，可立即重新运行或编辑任务").
			TextColor(t.TextMuted).FontSize(uikit.FontSize(c, mtheme.CaptionSize))
	})
}

// newTask opens the editor for a new task.
func (a *app) newTask() {
	agent := ""
	if len(a.agents) > 0 {
		agent = a.agents[0].Ref
	}
	ws := ""
	if len(a.workspaces) > 0 {
		ws = sortedWorkspaces(a.workspaces)[0]
	}
	a.taskForm = &taskForm{
		agent: agent, workspace: ws, kind: "daily",
		hour: "09", minute: "00", day: "1", enabled: true,
	}
	a.taskEditorOpen = true
	a.taskEditorClosing = false
	a.win.Update(func() {})
}

func (a *app) editTask(task *taskInfo) {
	a.taskForm = &taskForm{
		id: task.ID, name: task.Name, prompt: task.Prompt,
		agent: task.Agent, workspace: task.Workspace,
		kind: task.Schedule.Kind, hour: fmt.Sprintf("%02d", task.Schedule.Hour),
		minute:  fmt.Sprintf("%02d", task.Schedule.Minute),
		weekday: task.Schedule.Weekday,
		day:     strconv.Itoa(max(1, task.Schedule.Day)),
		reuse:   task.Reuse, enabled: task.Enabled,
	}
	if a.taskForm.kind == "" {
		a.taskForm.kind = "daily"
	}
	a.taskEditorOpen = true
	a.taskEditorClosing = false
	a.win.Update(func() {})
}

// taskEditor is the create/edit dialog.
func (a *app) taskEditor(c *ui.Context) {
	if a.taskForm == nil || (!a.taskEditorOpen && !a.taskEditorClosing) {
		return
	}
	f := a.taskForm
	wasOpen := a.taskEditorOpen
	title := "新建自动任务"
	if f.id != "" {
		title = "编辑自动任务"
	}
	kit.Dialog(c, &a.taskEditorOpen, kit.DialogOptions{
		Title: title,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				a.taskEditorOpen = false
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				a.saveTask()
			}
		},
	}, func() {
		in := kit.TextInput(c, &f.name, kit.TextInputOptions{
			Placeholder: "任务名称", Label: "任务名称",
		})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)

		area := kit.TextArea(c, &f.prompt, kit.TextAreaOptions{
			Label: "提示词", Placeholder: "每次运行交给 Agent 的提示词",
			MinHeight: 90, MaxHeight: 180,
		})
		area.Element.FillWidth()

		agentOpts := []kit.SelectOption[string]{{Value: "", Label: "Chat"}}
		for _, ag := range a.agents {
			label := ag.Name
			if ag.Kind == "acp" {
				label += " · ACP"
			}
			agentOpts = append(agentOpts, kit.SelectOption[string]{Value: ag.Ref, Label: label})
		}
		agent := kit.Select(c, &f.agent, agentOpts, kit.SelectOptions{Label: "Agent"})
		agent.Element.FillWidth()

		wsOpts := []kit.SelectOption[string]{{Value: "", Label: "默认工作区"}}
		for _, ws := range sortedWorkspaces(a.workspaces) {
			wsOpts = append(wsOpts, kit.SelectOption[string]{Value: ws, Label: ws})
		}
		ws := kit.Select(c, &f.workspace, wsOpts, kit.SelectOptions{Label: "工作区"})
		ws.Element.FillWidth()

		kind := kit.Select(c, &f.kind, scheduleKindOptions(), kit.SelectOptions{Label: "调度"})
		kind.Element.FillWidth()
		ui.Row(c).FillWidth().Gap(uikit.Unit(c)).AlignItems(ui.Center).Children(func() {
			hour := kit.TextInput(c, &f.hour, kit.TextInputOptions{
				Label: "小时", Placeholder: "09",
			})
			hour.Element.Width(64)
			ui.Text(c, ":").TextColor(c.Theme().TextMuted)
			minute := kit.TextInput(c, &f.minute, kit.TextInputOptions{
				Label: "分钟", Placeholder: "00",
			})
			minute.Element.Width(64)
			if f.kind == "weekly" {
				wd := kit.Select(c, &f.weekday, weekdayOptions(), kit.SelectOptions{Label: "星期"})
				wd.Element.Grow(1)
			}
			if f.kind == "monthly" {
				day := kit.TextInput(c, &f.day, kit.TextInputOptions{
					Label: "日", Placeholder: "1",
				})
				day.Element.Width(64)
			}
		})
		reuse := f.reuse
		if kit.SwitchRow(c, &reuse, "复用同一个会话", kit.SwitchRowOptions{
			Hint: "关闭时每次运行都会新建一个会话",
		}).Changed() {
			f.reuse = reuse
		}
		enabled := f.enabled
		if kit.SwitchRow(c, &enabled, "启用调度", kit.SwitchRowOptions{
			Hint: "关闭后仍可手动运行",
		}).Changed() {
			f.enabled = enabled
		}
	})
	switch {
	case a.taskEditorOpen:
		a.taskEditorClosing = false
	case wasOpen:
		a.taskEditorClosing = true
	default:
		a.taskEditorClosing = false
		a.taskForm = nil
	}
}

func (a *app) saveTask() {
	f := a.taskForm
	if f == nil {
		return
	}
	if strings.TrimSpace(f.name) == "" || strings.TrimSpace(f.prompt) == "" {
		a.status = "请填写任务名称和提示词"
		return
	}
	hour, _ := strconv.Atoi(strings.TrimSpace(f.hour))
	minute, _ := strconv.Atoi(strings.TrimSpace(f.minute))
	day, _ := strconv.Atoi(strings.TrimSpace(f.day))
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		a.status = "时间需在 00:00 到 23:59 之间"
		return
	}
	if f.kind == "monthly" && (day < 1 || day > 31) {
		a.status = "每月执行日需在 1 到 31 之间"
		return
	}
	in := taskInput{
		Name: f.name, Agent: f.agent, Prompt: f.prompt,
		Workspace: f.workspace,
		Schedule: scheduleInfo{
			Kind: f.kind, Hour: hour, Minute: minute,
			Weekday: f.weekday, Day: day,
		},
		Reuse: f.reuse, Enabled: f.enabled,
	}
	id := f.id
	a.taskEditorOpen = false
	go func() {
		var out struct {
			Task taskInfo `json:"task"`
		}
		var err error
		if id == "" {
			err = apiPost("/api/tasks", in, &out)
		} else {
			err = apiPatch("/api/tasks/"+id, in, &out)
		}
		if err != nil {
			a.win.Update(func() { a.status = "保存任务失败：" + err.Error() })
			return
		}
		a.win.Update(func() { a.status = "" })
		a.refreshTasks()
		if id == "" && out.Task.ID != "" {
			a.openTask(out.Task.ID)
		}
	}()
}

// taskDeleteDialog confirms deleting the open task.
func (a *app) taskDeleteDialog(c *ui.Context) {
	if !a.taskDeleteOpen {
		return
	}
	wasOpen := a.taskDeleteOpen
	name := "该任务"
	if task := a.currentTask(); task != nil {
		name = "「" + task.Name + "」"
	}
	res := kit.AlertDialog(c, &a.taskDeleteOpen, kit.AlertDialogOptions{
		Title:        "删除自动任务",
		Description:  "将删除" + name + "的调度配置，已有运行记录会保留。",
		ConfirmLabel: "删除",
	})
	if res.Confirmed {
		a.commitDeleteTask()
	}
	if a.taskDeleteOpen || !wasOpen {
		return
	}
}

func (a *app) commitDeleteTask() {
	id := a.getTaskID()
	if id == "" {
		return
	}
	a.taskDeleteOpen = false
	go func() {
		if err := apiDelete("/api/tasks/"+id, nil); err != nil {
			a.win.Update(func() { a.status = "删除任务失败：" + err.Error() })
			return
		}
		a.win.Update(func() {
			a.setTaskID("")
			a.taskSessions = nil
			a.setTaskConv("")
			a.cur = nil
			a.setRunning(false)
			a.status = ""
		})
		a.refreshTasks()
	}()
}
