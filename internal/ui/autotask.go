package ui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"

	"github.com/cloudwego/eino/components/tool"
	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// autoForm is the auto-task editor's working copy.
type autoForm struct {
	editing   int
	name      string
	agent     string
	prompt    string
	workspace string
	kind      string
	tod       kit.TimeOfDay
	weekday   int
	day       int
	reuse     bool
	enabled   bool
}

// autoTasksPage lists auto tasks, or the editor page.
func (cw *ChatWindow) autoTasksPage(c *mgo.Context) {
	if cw.autoTaskEditorOpen {
		cw.autoTaskEditor(c)
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Children(func() {
			mgo.Column(c).Grow(1).Gap(u / 2).Children(func() {
				mgo.Text(c, "自动任务").
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Text(c, "按计划自动运行 Agent，支持每天 / 每周 / 每月").TextColor(k.TextMuted).FontSize(caption)
			})
			if kit.Button(c, "新建任务", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAutoForm()
				cw.autoTaskEditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(cw.cfg.AutoTasks) == 0 {
				cw.autoTaskEmptyState(c)
				return
			}
			cardRows(c, 2, len(cw.cfg.AutoTasks), func(i int) { cw.autoTaskCard(c, i) })
		})
	})
	cw.autoTaskConvsDialog(c)
}

// autoTaskEmptyState shows a centered placeholder when no tasks exist.
func (cw *ChatWindow) autoTaskEmptyState(c *mgo.Context) {
	emptyState(c, emptyStateOptions{
		Icon:  "clock",
		Title: "还没有自动任务",
		Hint:  "创建任务后，Agent 会按计划自动执行",
		Actions: func() {
			if kit.Button(c, "新建任务", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAutoForm()
				cw.autoTaskEditorOpen = true
			}
		},
	})
}

// autoTaskCard shows one task with status, schedule, and controls.
func (cw *ChatWindow) autoTaskCard(c *mgo.Context, i int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	t := cw.cfg.AutoTasks[i]
	running := cw.autoRunning[t.ID]

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

	cardShell(c, "task:"+t.ID, func() {
		cardTitleRow(c, func() {
			mgo.Icon(c, icons.Must(dotIcon)).Size(16, 16).TextColor(dotCol).Shrink(0)
			cardTitle(c, t.Name)
			enabled := t.Enabled
			if mgo.Switch(c, &enabled).Changed() {
				cw.cfg.AutoTasks[i].Enabled = enabled
				_ = config.SaveConfig(cw.cfg)
			}
		})

		cardMetaRow(c, func() {
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Shrink(0).Children(func() {
				mgo.Icon(c, icons.Must("clock")).Size(14, 14).TextColor(k.TextMuted)
				mgo.Text(c, scheduleText(t.Schedule)).FontSize(caption).TextColor(k.TextMuted)
			})
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Shrink(0).Children(func() {
				mgo.Icon(c, icons.Must("bot")).Size(14, 14).TextColor(k.TextMuted)
				mgo.Text(c, agentLabel(t.Agent)).FontSize(caption).TextColor(k.TextMuted)
			})
			mgo.Text(c, reuseLabel(t.Reuse)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
		})

		if running {
			mgo.Row(c).Gap(u / 2).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, "运行中…").FontSize(caption).TextColor(k.Accent)
			})
		} else {
			cardMetaRow(c, func() {
				statusIcon, statusText, statusColor := "circle", "尚未运行", k.TextMuted
				switch {
				case strings.HasPrefix(t.LastNote, "失败"):
					statusIcon, statusText, statusColor = "circle-x", "失败", k.Danger
				case strings.HasPrefix(t.LastNote, "成功"):
					statusIcon, statusText, statusColor = "circle-check", "成功", k.Success
				}
				mgo.Icon(c, icons.Must(statusIcon)).Size(14, 14).TextColor(statusColor).Shrink(0)
				mgo.Text(c, statusText).FontSize(caption).TextColor(statusColor).Shrink(0)
				if !t.LastRun.IsZero() {
					mgo.Text(c, "上次 "+t.LastRun.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				}
				from := t.LastRun
				if from.IsZero() {
					from = time.Now()
				}
				if nr := nextRun(t.Schedule, from); !nr.IsZero() {
					mgo.Text(c, "下次 "+nr.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				}
			})
		}

		cardActionRow(c, func() {
			if kit.Button(c, "运行", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("play")}).Clicked() {
				cw.startAutoTask(i)
			}
			if kit.Button(c, "会话", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("message-square")}).Clicked() {
				cw.openAutoTaskConvs(t.ID)
			}
			if kit.Button(c, "编辑", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				cw.loadAutoForm(i)
				cw.autoTaskEditorOpen = true
			}
			if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
				cw.deleteAutoTask(i)
			}
		})
	})
}

// openAutoTaskConvs snapshots the task's conversations for the dialog.
func (cw *ChatWindow) openAutoTaskConvs(taskID string) {
	var convs []models.Conversation
	for _, conv := range cw.taskConvs {
		if conv.TaskID == taskID {
			convs = append(convs, conv)
		}
	}
	cw.autoConvs = convs
	cw.autoConvsTaskID = taskID
	cw.autoConvsOpen = true
}

// autoTaskConvsDialog lists the conversations a task produced.
func (cw *ChatWindow) autoTaskConvsDialog(c *mgo.Context) {
	if !cw.autoConvsOpen {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	title := "任务会话"
	if name := taskNameOf(cw, cw.autoConvsTaskID); name != "" {
		title = name + " · 会话"
	}
	kit.Dialog(c, &cw.autoConvsOpen, kit.DialogOptions{
		Title: title,
		Width: 640,
	}, func() {
		mgo.Column(c).Gap(u * 2).Children(func() {
			if len(cw.autoConvs) == 0 {
				mgo.Text(c, "该任务还没有会话").TextColor(k.TextMuted)
				return
			}
			mgo.Scroll(c).MaxHeight(400).Children(func() {
				mgo.Column(c).Gap(u).Children(func() {
					for _, conv := range cw.autoConvs {
						conv := conv
						open := mgo.Local(c.Root(), "autoconv:"+conv.ID, func() bool { return false })
						last := ""
						if n := len(conv.Messages); n > 0 {
							last = strings.TrimSpace(conv.Messages[n-1].Content)
						}
						mgo.Column(c).FillWidth().Gap(u).Children(func() {
							mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
								trig := mgo.ButtonBase(c).Key(conv.ID).Grow(1).Basis(0).MinWidth(0).
									Gap(u).AlignItems(mgo.Center).Justify(mgo.Start).
									Padding(u, u*2).Radius(mtheme.ControlRadius).
									Label(conv.Title)
								if trig.Hovered() {
									trig.Background(k.SurfaceHover)
								}
								trig.Children(func() {
									arrow := "chevron-right"
									if *open {
										arrow = "chevron-down"
									}
									mgo.Icon(c, icons.Must(arrow)).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
									mgo.Icon(c, icons.Must("message-square")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
									mgo.Text(c, conv.Title).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
									switch conv.TaskStatus {
									case "failed":
										mgo.Icon(c, icons.Must("circle-x")).Size(14, 14).TextColor(k.Danger).Shrink(0)
										mgo.Text(c, "失败").FontSize(caption).TextColor(k.Danger).Shrink(0)
									case "success":
										mgo.Icon(c, icons.Must("circle-check")).Size(14, 14).TextColor(k.Success).Shrink(0)
										mgo.Text(c, "成功").FontSize(caption).TextColor(k.Success).Shrink(0)
									default:
										mgo.Icon(c, icons.Must("circle")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
										mgo.Text(c, "未运行").FontSize(caption).TextColor(k.TextMuted).Shrink(0)
									}
									mgo.Text(c, convLast(conv).Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
								})
								if trig.Clicked() && last != "" {
									*open = !*open
								}
								if kit.Button(c, "进入对话", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
									cw.autoConvsOpen = false
									cw.openConversationByID(conv.ID)
								}
							})
							if *open && last != "" {
								mgo.Column(c).FillWidth().Padding(u*2, u*3).Gap(u).
									Radius(mtheme.ControlRadius).Background(k.Surface).
									Children(func() {
										color := k.Text
										if strings.HasPrefix(last, "⚠️") {
											color = k.Danger
										}
										mgo.Text(c, last).FontSize(caption).TextColor(color).
											Font(mtheme.MonoFont).MaxLines(6)
									})
							}
						})
					}
				})
			})
		})
	})
}

// autoTaskEditor is the create/edit page for one task.
func (cw *ChatWindow) autoTaskEditor(c *mgo.Context) {
	f := &cw.autoForm
	u := mcore.Density(c).Unit()
	title := "新建任务"
	if f.editing >= 0 {
		title = "编辑任务"
	}
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if kit.Button(c, "返回", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.autoTaskEditorOpen = false
			}
			mgo.Text(c, title).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				if cw.saveAutoTask() {
					cw.autoTaskEditorOpen = false
				}
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().AlignItems(mgo.Center).Padding(0, 0, u*4, 0).Children(func() {
				mgo.Column(c).FillWidth().MaxWidth(760).Gap(u * 3).Children(func() {
					autoFormGroup(c, "基本信息", func() {
						formField(c, "名称", func() {
							kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "名称", Placeholder: "任务名称"})
						})
						formField(c, "Agent", func() {
							agentOpts := []kit.SelectOption[string]{{Value: "", Label: chatAgentName}}
							for _, a := range cw.cfg.Agents {
								agentOpts = append(agentOpts, kit.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
							}
							kit.Select(c, &f.agent, agentOpts, kit.SelectOptions{Label: "Agent"})
						})
						formField(c, "提示词", func() {
							kit.TextArea(c, &f.prompt, kit.TextAreaOptions{Label: "提示词", Placeholder: "每次运行发送给 Agent 的内容", MinHeight: 120})
						})
						formField(c, "工作目录", func() {
							mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.End).Children(func() {
								in := kit.TextInput(c, &f.workspace, kit.TextInputOptions{Label: "工作目录", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
								in.Element.Grow(1)
								if kit.Button(c, "选择", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
									cw.pickDirectory("选择工作目录", func(path string) { f.workspace = path })
								}
							})
						})
					})
					mgo.Divider(c)
					autoFormGroup(c, "调度", func() {
						cw.autoScheduleForm(c, f)
					})
					mgo.Divider(c)
					autoFormGroup(c, "运行选项", func() {
						switchRow(c, &f.reuse, "复用会话", "在同一会话中继续，否则每次运行新建会话")
						switchRow(c, &f.enabled, "启用任务", "关闭后不再按计划自动运行")
					})
				})
			})
		})
	})
}

// autoScheduleForm edits the schedule fields on one row, each under its own
// label so the row reads without guessing which field is the kind and which
// the time.
func (cw *ChatWindow) autoScheduleForm(c *mgo.Context, f *autoForm) {
	u := mcore.Density(c).Unit()
	kinds := []kit.SelectOption[string]{{Value: "daily", Label: "每天"}, {Value: "weekly", Label: "每周"}, {Value: "monthly", Label: "每月"}}
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Start).Wrap().Children(func() {
		formField(c, "频率", func() {
			kit.Select(c, &f.kind, kinds, kit.SelectOptions{Label: "频率"}).Element.Width(140)
		})
		switch f.kind {
		case "weekly":
			opts := []kit.SelectOption[int]{
				{Value: 1, Label: "周一"}, {Value: 2, Label: "周二"}, {Value: 3, Label: "周三"}, {Value: 4, Label: "周四"},
				{Value: 5, Label: "周五"}, {Value: 6, Label: "周六"}, {Value: 0, Label: "周日"},
			}
			formField(c, "星期", func() {
				kit.Select(c, &f.weekday, opts, kit.SelectOptions{Label: "星期"}).Element.Width(140)
			})
		case "monthly":
			opts := make([]kit.SelectOption[int], 0, 31)
			for d := 1; d <= 31; d++ {
				opts = append(opts, kit.SelectOption[int]{Value: d, Label: fmt.Sprintf("%d 日", d)})
			}
			formField(c, "日期", func() {
				kit.Select(c, &f.day, opts, kit.SelectOptions{Label: "日期"}).Element.Width(140)
			})
		}
		formField(c, "时间", func() {
			kit.TimePicker(c, &f.tod, kit.TimePickerOptions{Label: "时间"}).Element.Width(190)
		})
	})
}

// formField stacks a field's label just above its control, so each control
// reads as its own field while the group stays compact.
func formField(c *mgo.Context, label string, control func()) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Gap(u).Children(func() {
		fieldLabel(c, label)
		control()
	})
}

// fieldLabel shows a field's name above its control; the controls themselves
// carry only an accessibility label, so this is what the eye reads.
func fieldLabel(c *mgo.Context, text string) {
	mgo.Text(c, text).
		FontSize(mcore.FontSize(c, mtheme.CaptionSize)).
		TextColor(mcore.Tokens(c).TextMuted)
}

// switchRow shows a labeled switch, its name and hint at the start and the
// switch at the end, and reports a change.
func switchRow(c *mgo.Context, on *bool, label, hint string) bool {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	small := mcore.FontSize(c, mtheme.CaptionSize)
	changed := false
	mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Column(c).Grow(1).Gap(u / 2).Children(func() {
			mgo.Text(c, label)
			if hint != "" {
				mgo.Text(c, hint).FontSize(small).TextColor(k.TextMuted)
			}
		})
		changed = mgo.Switch(c, on).Label(label).Changed()
	})
	return changed
}

// autoFormGroup stacks a titled block of fields, separating the editor into
// readable sections.
func autoFormGroup(c *mgo.Context, title string, fields func()) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		mgo.Text(c, title).FontWeight(600)
		fields()
	})
}

func (cw *ChatWindow) clearAutoForm() {
	cw.autoForm = autoForm{editing: -1, kind: "daily", tod: kit.TimeOfDay{Hour: 9}, day: 1, enabled: true}
}

func (cw *ChatWindow) loadAutoForm(i int) {
	t := cw.cfg.AutoTasks[i]
	d := t.Schedule.Day
	if d < 1 {
		d = 1
	}
	cw.autoForm = autoForm{
		editing: i, name: t.Name, agent: t.Agent, prompt: t.Prompt, workspace: t.Workspace,
		kind: t.Schedule.Kind, tod: kit.TimeOfDay{Hour: t.Schedule.Hour, Minute: t.Schedule.Minute},
		weekday: t.Schedule.Weekday, day: d, reuse: t.Reuse, enabled: t.Enabled,
	}
}

func (cw *ChatWindow) saveAutoTask() bool {
	f := &cw.autoForm
	if strings.TrimSpace(f.name) == "" {
		cw.fail("任务名称不能为空")
		return false
	}
	kind := f.kind
	if kind == "" {
		kind = "daily"
	}
	d := f.day
	if d < 1 {
		d = 1
	}
	sched := config.Schedule{
		Kind: kind, Hour: f.tod.Hour, Minute: f.tod.Minute, Weekday: f.weekday, Day: d,
	}
	def := config.AutoTask{
		Name: f.name, Agent: f.agent, Prompt: f.prompt, Workspace: f.workspace, Schedule: sched, Reuse: f.reuse, Enabled: f.enabled,
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.AutoTasks) {
		def.ID = cw.cfg.AutoTasks[f.editing].ID
		def.ConvID = cw.cfg.AutoTasks[f.editing].ConvID
		def.LastRun = cw.cfg.AutoTasks[f.editing].LastRun
		def.LastNote = cw.cfg.AutoTasks[f.editing].LastNote
		cw.cfg.AutoTasks[f.editing] = def
	} else {
		def.ID = cw.newID()
		cw.cfg.AutoTasks = append(cw.cfg.AutoTasks, def)
	}
	_ = config.SaveConfig(cw.cfg)
	cw.toast("已保存任务 "+def.Name, mcore.SeveritySuccess)
	return true
}

func (cw *ChatWindow) deleteAutoTask(i int) {
	if i < 0 || i >= len(cw.cfg.AutoTasks) {
		return
	}
	name := cw.cfg.AutoTasks[i].Name
	cw.askConfirm("删除任务", "确定删除「"+name+"」？", func() {
		cw.cfg.AutoTasks = append(cw.cfg.AutoTasks[:i], cw.cfg.AutoTasks[i+1:]...)
		_ = config.SaveConfig(cw.cfg)
		cw.toast("任务已删除", mcore.SeveritySuccess)
	})
}

// openConversation opens any conversation, including task ones, in chat.
func (cw *ChatWindow) openConversationByID(id string) {
	cw.selectConversation(id)
	cw.section = sectionChat
}

// scheduleText renders a schedule for display.
func scheduleText(s config.Schedule) string {
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

func agentLabel(agent string) string {
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		return name
	}
	return chatAgentName
}

func reuseLabel(reuse bool) string {
	if reuse {
		return "复用会话"
	}
	return "每次新建"
}

func taskNameOf(cw *ChatWindow, id string) string {
	for _, t := range cw.cfg.AutoTasks {
		if t.ID == id {
			return t.Name
		}
	}
	return ""
}

// nextRun is the first scheduled time strictly after from.
func nextRun(s config.Schedule, from time.Time) time.Time {
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

// autoTaskLoop checks due tasks every 30 seconds.
func (cw *ChatWindow) autoTaskLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		cw.autoTaskTick()
	}
}

func (cw *ChatWindow) autoTaskTick() {
	now := time.Now()
	for i := range cw.cfg.AutoTasks {
		t := cw.cfg.AutoTasks[i]
		if !t.Enabled || cw.autoRunning[t.ID] {
			continue
		}
		from := t.LastRun
		if from.IsZero() {
			from = now
		}
		if !nextRun(t.Schedule, from).After(now) {
			cw.startAutoTask(i)
		}
	}
}

// startAutoTask builds the client and runs one task off the main thread.
func (cw *ChatWindow) startAutoTask(i int) {
	if i < 0 || i >= len(cw.cfg.AutoTasks) {
		return
	}
	t := cw.cfg.AutoTasks[i]
	if cw.autoRunning[t.ID] {
		return
	}
	client, react, groupClient, groupReact, err := cw.taskClient(t.Agent, t.Workspace)
	if err != nil {
		cw.cfg.AutoTasks[i].LastRun = time.Now()
		cw.cfg.AutoTasks[i].LastNote = "失败: " + err.Error()
		_ = config.SaveConfig(cw.cfg)
		cw.toast("任务失败: "+err.Error(), mcore.SeverityError)
		return
	}
	conv, err := cw.ensureTaskConv(&cw.cfg.AutoTasks[i])
	if err != nil {
		cw.cfg.AutoTasks[i].LastRun = time.Now()
		cw.cfg.AutoTasks[i].LastNote = "失败: " + err.Error()
		_ = config.SaveConfig(cw.cfg)
		return
	}
	conv.Messages = append(conv.Messages, conv.NewHumanMessage(cw.newID(), t.Prompt, time.Now()))
	msgs := buildMessages(conv, len(conv.Messages))
	if sys := cw.taskSystemPrompt(t.Agent); sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: sys}}, msgs...)
	}
	cw.autoRunning[t.ID] = true
	go func() {
		reply := ""
		reasoning := ""
		var runErr error
		ctx := context.Background()
		switch {
		case groupReact != nil:
			resp, err := groupReact.Chat(ctx, msgs, nil, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		case react != nil:
			resp, err := react.Chat(ctx, msgs, nil, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		case groupClient != nil:
			resp, err := groupClient.Chat(ctx, msgs, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		default:
			resp, err := client.Chat(ctx, msgs, nil)
			if resp != nil {
				reply = resp.Content
				reasoning = resp.Reasoning
			}
			runErr = err
		}
		cw.postMain(func() {
			if runErr != nil {
				reply = "⚠️ " + runErr.Error()
				cw.cfg.AutoTasks[i].LastNote = "失败: " + runErr.Error()
				conv.TaskStatus = "failed"
			} else {
				cw.cfg.AutoTasks[i].LastNote = "成功"
				conv.TaskStatus = "success"
			}
			m := conv.NewAgentMessage(cw.newID(), t.Agent, authorNameOf(t.Agent), time.Now())
			m.Content, m.Reasoning = reply, reasoning
			conv.Messages = append(conv.Messages, m)
			_ = cw.convManager.SaveConversation(conv)
			cw.cfg.AutoTasks[i].LastRun = time.Now()
			cw.autoRunning[t.ID] = false
			_ = config.SaveConfig(cw.cfg)
			cw.reloadConvs()
			cw.invalidate()
			cw.notifySessionComplete(conv, reply)
		})
	}()
}

// ensureTaskConv returns the task's conversation, creating one when needed.
func (cw *ChatWindow) ensureTaskConv(task *config.AutoTask) (*models.Conversation, error) {
	if task.Reuse && task.ConvID != "" {
		if conv, err := cw.convManager.LoadConversation(task.ConvID); err == nil {
			conv.TaskID = task.ID
			return conv, nil
		}
	}
	provName, model, _, ws, _ := cw.taskAgentSpec(task.Agent)
	if task.Workspace != "" {
		ws = task.Workspace
	}
	if p, ok := cw.findProvider(provName); ok && model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	conv, err := cw.convManager.CreateConversation(task.Name, provName, model)
	if err != nil {
		return nil, err
	}
	conv.TaskID = task.ID
	conv.Agent = task.Agent
	conv.Workspace = ws
	if task.Reuse {
		task.ConvID = conv.ID
	}
	_ = cw.convManager.SaveConversation(conv)
	return conv, nil
}

// taskClient builds a client for an agent without touching the active state.
func (cw *ChatWindow) taskClient(agent, workspace string, conv ...*models.Conversation) (*llm.Client, *llm.ReactClient, *llm.GroupClient, *llm.GroupReactClient, error) {
	var c *models.Conversation
	if len(conv) > 0 {
		c = conv[0]
	}
	return cw.taskClientWithChannel(agent, workspace, nil, nil, c)
}

func (cw *ChatWindow) taskClientWithChannel(agent, workspace string, ch *config.Channel, msgCtx *channelMsgContext, conv *models.Conversation, extra ...tool.BaseTool) (*llm.Client, *llm.ReactClient, *llm.GroupClient, *llm.GroupReactClient, error) {
	provName, model, sys, ws, toolIDs := cw.taskAgentSpec(agent)
	root := workspace
	if root == "" {
		root = ws
	}
	if root == "" {
		root = cw.fileRoot()
	}
	if g, ok := cw.cfg.FindProviderGroup(provName); ok {
		if model == "" {
			if ms := g.ModelsList(); len(ms) > 0 {
				model = ms[0]
			}
		}
		var env *builtinToolEnv
		if ch != nil {
			env = &builtinToolEnv{channel: ch, msgCtx: msgCtx}
		}
		einoTools := cw.buildEinoToolsEnv(toolIDs, root, conv, env)
		if ch != nil {
			einoTools = combineTools(einoTools, cw.buildChannelTools(ch, msgCtx))
		}
		einoTools = combineTools(einoTools, extra)
		if len(einoTools) > 0 {
			rc, err := llm.NewGroupReactClient(g, cw.cfg.Providers, einoTools, &llm.ReactAgentConfig{MaxStep: cw.cfg.ReactAgentMaxStep, SystemPrompt: sys})
			if err == nil {
				return nil, nil, nil, rc, nil
			}
			log.Printf("[channel-agent] NewGroupReactClient failed: %v", err)
		}
		gc, err := llm.NewGroupClient(g, cw.cfg.Providers)
		return nil, nil, gc, nil, err
	}
	p, ok := cw.findProvider(provName)
	if !ok {
		p, ok = cw.firstEnabled()
	}
	if !ok {
		if g, ok := cw.firstGroup(); ok {
			gc, err := llm.NewGroupClient(g, cw.cfg.Providers)
			return nil, nil, gc, nil, err
		}
		return nil, nil, nil, nil, fmt.Errorf("尚未配置模型")
	}
	if model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	p.Model = model
	var env *builtinToolEnv
	if ch != nil {
		env = &builtinToolEnv{channel: ch, msgCtx: msgCtx}
	}
	einoTools := cw.buildEinoToolsEnv(toolIDs, root, conv, env)
	if ch != nil {
		einoTools = combineTools(einoTools, cw.buildChannelTools(ch, msgCtx))
	}
	einoTools = combineTools(einoTools, extra)
	if len(einoTools) > 0 {
		rc, err := llm.NewReactClientWithEinoTools(p, einoTools, &llm.ReactAgentConfig{MaxStep: cw.cfg.ReactAgentMaxStep, SystemPrompt: sys})
		if err != nil {
			log.Printf("[channel-agent] NewReactClientWithEinoTools failed: %v (tools=%d)", err, len(einoTools))
		} else {
			return nil, rc, nil, nil, nil
		}
	}
	c, err := llm.NewClient(p)
	return c, nil, nil, nil, err
}

// taskSystemPrompt returns the system prompt an agent runs with.
func (cw *ChatWindow) taskSystemPrompt(agent string) string {
	_, _, sys, _, _ := cw.taskAgentSpec(agent)
	return sys
}

// taskAgentSpec resolves an agent's provider, model, prompt, workspace and tools.
func (cw *ChatWindow) taskAgentSpec(agent string) (provider, model, systemPrompt, workspace string, toolIDs []string) {
	if name, ok := strings.CutPrefix(agent, "agent:"); ok {
		for _, a := range cw.cfg.Agents {
			if a.Name != name {
				continue
			}
			if len(a.Models) > 0 {
				provider, model = splitModelRef(a.Models[0])
			}
			systemPrompt = a.SystemPrompt
			workspace = a.Workspace
			for _, t := range a.BuiltinTools {
				if builtinEnabled(cw.cfg, t) {
					toolIDs = append(toolIDs, "builtin:"+t)
				}
			}
			for _, s := range a.MCPServers {
				if mcpEnabled(cw.cfg, s) {
					toolIDs = append(toolIDs, "mcp:"+s)
				}
			}
			return
		}
	}
	provider = cw.cfg.Chat.Provider
	model = cw.cfg.Chat.Model
	for _, t := range cw.cfg.BuiltinTools {
		if t.Enabled {
			toolIDs = append(toolIDs, "builtin:"+t.Name)
		}
	}
	for _, s := range cw.cfg.MCPServers {
		if s.Enabled {
			toolIDs = append(toolIDs, "mcp:"+s.Name)
		}
	}
	return
}

func builtinEnabled(cfg *config.Config, name string) bool {
	for _, t := range cfg.BuiltinTools {
		if t.Name == name {
			return t.Enabled
		}
	}
	return false
}

func mcpEnabled(cfg *config.Config, name string) bool {
	for _, s := range cfg.MCPServers {
		if s.Name == name {
			return s.Enabled
		}
	}
	return false
}
