package autotask

import (
	"fmt"
	"strings"

	"chatgo/internal/config"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mdatetime "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/datetime"
	"git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/icons"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// editor is the create/edit page for one task.
func editor(c *mgo.Context, state *State, opts Options) {
	f := &state.Form
	u := mcore.Density(c).Unit()
	title := "新建任务"
	if f.Editing >= 0 {
		title = "编辑任务"
	}
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			if minput.Button(c, "返回", minput.ButtonOptions{Variant: minput.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				state.EditorOpen = false
			}
			mgo.Text(c, title).Font(mtheme.SerifFont).
				FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600).Grow(1)
			if minput.Button(c, "保存", minput.ButtonOptions{Variant: minput.Primary}).Clicked() {
				if Save(state, opts) {
					state.EditorOpen = false
				}
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().AlignItems(mgo.Center).Children(func() {
				mgo.Column(c).FillWidth().MaxWidth(760).Gap(u * 2).Children(func() {
					minput.TextInput(c, &f.Name, minput.TextInputOptions{Label: "名称", Placeholder: "任务名称"})
					agentOpts := []minput.SelectOption[string]{{Value: "", Label: opts.ChatAgentName}}
					for _, a := range opts.Config().Agents {
						agentOpts = append(agentOpts, minput.SelectOption[string]{Value: "agent:" + a.Name, Label: a.Name})
					}
					minput.Select(c, &f.Agent, agentOpts, minput.SelectOptions{Label: "Agent"})
					minput.TextArea(c, &f.Prompt, minput.TextAreaOptions{Label: "提示词", Placeholder: "每次运行发送给 Agent 的内容", MinHeight: 120})
					mgo.Row(c).Gap(u).AlignItems(mgo.End).Children(func() {
						in := minput.TextInput(c, &f.Workspace, minput.TextInputOptions{Label: "工作目录", Placeholder: "留空使用 Agent 工作目录 / 当前目录"})
						in.Element.Grow(1)
						if minput.Button(c, "选择", minput.ButtonOptions{Variant: minput.Secondary}).Clicked() {
							opts.PickDirectory("选择工作目录", func(path string) { f.Workspace = path })
						}
					})
					scheduleForm(c, f)
					minput.Switch(c, &f.Reuse, "复用会话（否则每次新建）", minput.SwitchOptions{})
					minput.Switch(c, &f.Enabled, "启用", minput.SwitchOptions{})
				})
			})
		})
	})
}

// scheduleForm edits the schedule fields on one row.
func scheduleForm(c *mgo.Context, f *Form) {
	u := mcore.Density(c).Unit()
	kinds := []minput.SelectOption[string]{{Value: "daily", Label: "每天"}, {Value: "weekly", Label: "每周"}, {Value: "monthly", Label: "每月"}}
	mgo.Row(c).Gap(u).AlignItems(mgo.End).Wrap().Children(func() {
		minput.Select(c, &f.Kind, kinds, minput.SelectOptions{Label: "频率"}).Element.Width(120)
		mdatetime.TimePicker(c, &f.Tod, mdatetime.TimePickerOptions{}).Element.Width(170)
		switch f.Kind {
		case "weekly":
			opts := []minput.SelectOption[int]{
				{Value: 1, Label: "周一"}, {Value: 2, Label: "周二"}, {Value: 3, Label: "周三"}, {Value: 4, Label: "周四"},
				{Value: 5, Label: "周五"}, {Value: 6, Label: "周六"}, {Value: 0, Label: "周日"},
			}
			minput.Select(c, &f.Weekday, opts, minput.SelectOptions{Label: "星期"}).Element.Width(120)
		case "monthly":
			opts := make([]minput.SelectOption[int], 0, 31)
			for d := 1; d <= 31; d++ {
				opts = append(opts, minput.SelectOption[int]{Value: d, Label: fmt.Sprintf("%d 日", d)})
			}
			minput.Select(c, &f.Day, opts, minput.SelectOptions{Label: "日期"}).Element.Width(120)
		}
	})
}

// ClearForm resets the form for a new task.
func ClearForm(state *State, opts Options) {
	state.Form = Form{Editing: -1, Kind: "daily", Tod: mdatetime.TimeOfDay{Hour: 9}, Day: 1, Enabled: true}
}

// LoadForm fills the form from the task at index i.
func LoadForm(state *State, opts Options, i int) {
	t := opts.Config().AutoTasks[i]
	d := t.Schedule.Day
	if d < 1 {
		d = 1
	}
	state.Form = Form{
		Editing: i, Name: t.Name, Agent: t.Agent, Prompt: t.Prompt, Workspace: t.Workspace,
		Kind: t.Schedule.Kind, Tod: mdatetime.TimeOfDay{Hour: t.Schedule.Hour, Minute: t.Schedule.Minute},
		Weekday: t.Schedule.Weekday, Day: d, Reuse: t.Reuse, Enabled: t.Enabled,
	}
}

// Save validates the form and writes the task into the config.
// It returns false when validation fails.
func Save(state *State, opts Options) bool {
	f := &state.Form
	if strings.TrimSpace(f.Name) == "" {
		opts.Fail("任务名称不能为空")
		return false
	}
	kind := f.Kind
	if kind == "" {
		kind = "daily"
	}
	d := f.Day
	if d < 1 {
		d = 1
	}
	sched := config.Schedule{
		Kind: kind, Hour: f.Tod.Hour, Minute: f.Tod.Minute, Weekday: f.Weekday, Day: d,
	}
	def := config.AutoTask{
		Name: f.Name, Agent: f.Agent, Prompt: f.Prompt, Workspace: f.Workspace, Schedule: sched, Reuse: f.Reuse, Enabled: f.Enabled,
	}
	cfg := opts.Config()
	if f.Editing >= 0 && f.Editing < len(cfg.AutoTasks) {
		def.ID = cfg.AutoTasks[f.Editing].ID
		def.ConvID = cfg.AutoTasks[f.Editing].ConvID
		def.LastRun = cfg.AutoTasks[f.Editing].LastRun
		def.LastNote = cfg.AutoTasks[f.Editing].LastNote
		cfg.AutoTasks[f.Editing] = def
	} else {
		def.ID = opts.NewID()
		cfg.AutoTasks = append(cfg.AutoTasks, def)
	}
	_ = opts.SaveConfig()
	opts.Toast("已保存任务 "+def.Name, mcore.SeveritySuccess)
	return true
}

// Delete removes the task at index i after confirmation.
func Delete(state *State, opts Options, i int) {
	cfg := opts.Config()
	if i < 0 || i >= len(cfg.AutoTasks) {
		return
	}
	name := cfg.AutoTasks[i].Name
	opts.AskConfirm("删除任务", "确定删除「"+name+"」？", func() {
		cfg.AutoTasks = append(cfg.AutoTasks[:i], cfg.AutoTasks[i+1:]...)
		_ = opts.SaveConfig()
		opts.Toast("任务已删除", mcore.SeveritySuccess)
	})
}
