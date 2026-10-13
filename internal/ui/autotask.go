package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdatetime "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/datetime"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// autoForm is the auto-task editor's working copy.
type autoForm struct {
	editing   int
	name      string
	agent     string
	prompt    string
	workspace string
	kind      string
	tod       mdatetime.TimeOfDay
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
				mgo.Text(c, "自动任务").Font(mtheme.SerifFont).
					FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Text(c, "按计划自动运行 Agent，支持每天 / 每周 / 每月").TextColor(k.TextMuted).FontSize(caption)
			})
			if minput.Button(c, "新建任务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.clearAutoForm()
				cw.autoTaskEditorOpen = true
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).Gap(u * 2).MaxWidth(900).Children(func() {
				if len(cw.cfg.AutoTasks) == 0 {
					cw.autoTaskEmptyState(c)
				} else {
					for i := range cw.cfg.AutoTasks {
						cw.autoTaskCard(c, i)
					}
				}
				cw.autoTaskConvs(c)
			})
		})
	})
}

// autoTaskEmptyState shows a centered placeholder when no tasks exist.
func (cw *ChatWindow) autoTaskEmptyState(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).
		Padding(u * 8, 0).Children(func() {
		mgo.Icon(c, icons.Must("clock")).Size(48, 48).TextColor(k.TextMuted)
		mgo.Text(c, "还没有自动任务").Font(mtheme.SerifFont).
			FontSize(mcore.FontSize(c, mtheme.H3Size)).TextColor(k.TextMuted)
		mgo.Text(c, "创建任务后，Agent 会按计划自动执行").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		if minput.Button(c, "新建任务", minput.ButtonOptions{Variant: minput.Primary, Icon: icons.Must("plus")}).Clicked() {
			cw.clearAutoForm()
			cw.autoTaskEditorOpen = true
		}
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

	mgo.Column(c).Gap(u).Padding(u*3).FillWidth().Background(k.Surface).
		Border(mtheme.BorderWidth, k.Border).Radius(mtheme.CardRadius).Children(func() {

		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Icon(c, icons.Must(dotIcon)).Size(16, 16).TextColor(dotCol).Shrink(0)
			mgo.Text(c, t.Name).FontWeight(600).Grow(1).SingleLine().MinWidth(0).Ellipsis("…")
			enabled := t.Enabled
			if minput.Switch(c, &enabled, "", minput.SwitchOptions{}).Changed() {
				cw.cfg.AutoTasks[i].Enabled = enabled
				_ = config.SaveConfig(cw.cfg)
			}
		})

		mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
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
		} else if !t.LastRun.IsZero() {
			mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				if t.LastNote != "" {
					mgo.Text(c, t.LastNote).FontSize(caption).TextColor(dotCol).Shrink(0)
				}
				mgo.Text(c, "上次: "+t.LastRun.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				nr := nextRun(t.Schedule, t.LastRun)
				if !nr.IsZero() {
					mgo.Text(c, "下次: "+nr.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
				}
			})
		} else {
			nr := nextRun(t.Schedule, time.Now())
			if !nr.IsZero() {
				mgo.Text(c, "下次: "+nr.Format("01-02 15:04")).FontSize(caption).TextColor(k.TextMuted)
			}
		}

		mgo.Row(c).Gap(u).Children(func() {
			if minput.Button(c, "运行", minput.ButtonOptions{Variant: minput.Secondary, Icon: icons.Must("play")}).Clicked() {
				cw.startAutoTask(i)
			}
			if minput.Button(c, "编辑", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
				cw.loadAutoForm(i)
				cw.autoTaskEditorOpen = true
			}
			if minput.Button(c, "删除", minput.ButtonOptions{Variant: minput.Danger}).Clicked() {
				cw.deleteAutoTask(i)
			}
		})
	})
}

// autoTaskConvs lists the conversations produced by tasks.
func (cw *ChatWindow) autoTaskConvs(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	caption := mcore.FontSize(c, mtheme.CaptionSize)
	var convs []models.Conversation
	for _, conv := range cw.taskConvs {
		convs = append(convs, conv)
	}
	if len(convs) == 0 {
		return
	}
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Padding(0, 0, u, 0).Children(func() {
		mgo.Icon(c, icons.Must("message-square")).Size(18, 18).TextColor(k.TextMuted)
		mgo.Text(c, "任务会话").FontWeight(600)
	})
	for _, conv := range convs {
		conv := conv
		b := mgo.ButtonBase(c).FillWidth().Justify(mgo.Start).Gap(u).MinHeight(mcore.Density(c).ControlHeight()).
			Padding(u, u*2).Radius(mtheme.ControlRadius)
		b.Children(func() {
			mgo.Icon(c, icons.Must("message-square")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
			mgo.Text(c, conv.Title).SingleLine().Grow(1).MinWidth(0).Ellipsis("…")
			mgo.Text(c, taskNameOf(cw, conv.TaskID)).FontSize(caption).TextColor(k.TextMuted).Shrink(0)
			mgo.Icon(c, icons.Must("chevron-right")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		})
		if b.Clicked() {
			cw.openConversation(conv.ID)
		}
	}
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
			if minput.Button(c, "返回", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.autoTaskEditorOpen = false
			}
			mgo.Text(c, title).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if cw.saveAutoTask() {
					cw.autoTaskEditorOpen = false
				}
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).MaxWidth(760).Gap(u * 2).Children(func() {
				minput.TextInput(c, &f.name, minput.TextInputOptions{Label: "名称", Placeholder: "任务名称"})
				agentOpts := []minput.SelectOption[string]{{Value: "", Label: chatAgentName}}
				for _, a := range cw.cfg.Agents {
					agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
				}
				minput.Select(c, &f.agent, agentOpts, minput.SelectOptions{Label: "Agent"})
				minput.TextArea(c, &f.prompt, minput.TextAreaOptions{Label: "提示词", Placeholder: "每次运行发送给 Agent 的内容", MinHeight: 120})
				mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
					in := minput.TextInput(c, &f.workspace, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
					in.Element.Grow(1)
					if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
						cw.pickDirectory("选择工作目录", func(path string) { f.workspace = path })
					}
				})
				cw.autoScheduleForm(c, f)
				minput.Switch(c, &f.reuse, "复用会话（否则每次新建）", minput.SwitchOptions{})
				minput.Switch(c, &f.enabled, "启用", minput.SwitchOptions{})
			})
		})
	})
}

// autoScheduleForm edits the schedule fields on one row.
func (cw *ChatWindow) autoScheduleForm(c *mgo.Context, f *autoForm) {
	u := mcore.Density(c).Unit()
	kinds := []minput.SelectOption[string]{{Value: "daily", Label: "每天"}, {Value: "weekly", Label: "每周"}, {Value: "monthly", Label: "每月"}}
	mgo.Row(c).Gap(u).AlignItems(mgo.End).Wrap().Children(func() {
		minput.Select(c, &f.kind, kinds, minput.SelectOptions{Label: "频率"}).Element.Width(120)
		mdatetime.TimePicker(c, &f.tod, mdatetime.TimePickerOptions{}).Element.Width(170)
		switch f.kind {
		case "weekly":
			opts := []minput.SelectOption[int]{
				{Value: 1, Label: "周一"}, {Value: 2, Label: "周二"}, {Value: 3, Label: "周三"}, {Value: 4, Label: "周四"},
				{Value: 5, Label: "周五"}, {Value: 6, Label: "周六"}, {Value: 0, Label: "周日"},
			}
			minput.Select(c, &f.weekday, opts, minput.SelectOptions{Label: "星期"}).Element.Width(120)
		case "monthly":
			opts := make([]minput.SelectOption[int], 0, 31)
			for d := 1; d <= 31; d++ {
				opts = append(opts, minput.SelectOption[int]{Value: d, Label: fmt.Sprintf("%d 日", d)})
			}
			minput.Select(c, &f.day, opts, minput.SelectOptions{Label: "日期"}).Element.Width(120)
		}
	})
}

func (cw *ChatWindow) clearAutoForm() {
	cw.autoForm = autoForm{editing: -1, kind: "daily", tod: mdatetime.TimeOfDay{Hour: 9}, day: 1, enabled: true}
}

func (cw *ChatWindow) loadAutoForm(i int) {
	t := cw.cfg.AutoTasks[i]
	d := t.Schedule.Day
	if d < 1 {
		d = 1
	}
	cw.autoForm = autoForm{
		editing: i, name: t.Name, agent: t.Agent, prompt: t.Prompt, workspace: t.Workspace,
		kind: t.Schedule.Kind, tod: mdatetime.TimeOfDay{Hour: t.Schedule.Hour, Minute: t.Schedule.Minute},
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
func (cw *ChatWindow) openConversation(id string) {
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
	client, react, err := cw.taskClient(t.Agent, t.Workspace)
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
	conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "user", Content: t.Prompt, Timestamp: time.Now()})
	msgs := buildMessages(conv, len(conv.Messages))
	if sys := cw.taskSystemPrompt(t.Agent); sys != "" {
		msgs = append([]llm.ChatMessage{{Role: "system", Content: sys}}, msgs...)
	}
	cw.autoRunning[t.ID] = true
	go func() {
		reply := ""
		var runErr error
		ctx := context.Background()
		if react != nil {
			resp, err := react.Chat(ctx, msgs, nil, nil)
			if resp != nil {
				reply = resp.Content
			}
			runErr = err
		} else {
			resp, err := client.Chat(ctx, msgs, nil)
			if resp != nil {
				reply = resp.Content
			}
			runErr = err
		}
		cw.postMain(func() {
			if runErr != nil {
				reply = "⚠️ " + runErr.Error()
				cw.cfg.AutoTasks[i].LastNote = "失败: " + runErr.Error()
			} else {
				cw.cfg.AutoTasks[i].LastNote = "成功"
			}
			conv.Messages = append(conv.Messages, models.Message{ID: cw.newID(), Role: "assistant", Content: reply, Timestamp: time.Now()})
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
func (cw *ChatWindow) taskClient(agent, workspace string) (*llm.Client, *llm.ReactClient, error) {
	provName, model, sys, ws, toolIDs := cw.taskAgentSpec(agent)
	root := workspace
	if root == "" {
		root = ws
	}
	if root == "" {
		root = cw.fileRoot()
	}
	p, ok := cw.findProvider(provName)
	if !ok {
		p, ok = cw.firstEnabled()
	}
	if !ok {
		return nil, nil, fmt.Errorf("尚未配置模型")
	}
	if model == "" {
		if ms := p.ModelsList(); len(ms) > 0 {
			model = ms[0]
		}
	}
	p.Model = model
	if len(toolIDs) > 0 {
		if rc, err := llm.NewReactClientWithEinoTools(p, cw.buildEinoTools(toolIDs, root), &llm.ReactAgentConfig{MaxStep: cw.cfg.ReactAgentMaxStep, SystemPrompt: sys}); err == nil {
			return nil, rc, nil
		}
	}
	c, err := llm.NewClient(p)
	return c, nil, err
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
