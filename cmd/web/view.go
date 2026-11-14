package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	"chatgo/internal/ui/uikit"
)

func (a *app) view(c *ui.Context) {
	uikit.Use(c, uikit.Settings{
		Mode:    uikit.System,
		Density: mtheme.DefaultDensity,
		Locale:  uikit.ZhCN,
	})
	t := c.Theme()
	w, _ := c.Size()
	narrow := w < 760
	if narrow {
		if a.sidebarOpen {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
				a.sidebar(c, t, true)
			})
			a.overlays(c)
			return
		}
		ui.Column(c).Fill().Background(t.Background).Children(func() {
			a.header(c, t, true)
			a.main(c, t)
		})
		a.overlays(c)
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, t, false)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.header(c, t, false)
			a.main(c, t)
		})
	})
	a.overlays(c)
}

// main draws the current section's conversation area.
func (a *app) main(c *ui.Context, t *ui.Theme) {
	switch a.getSection() {
	case secRooms:
		a.roomSessionMenu(c, t)
		if a.cur == nil || a.cur.RoomID == "" || a.cur.RoomID != a.getRoomID() {
			a.emptyConversation(c, t, "选择或新建一个群聊会话")
		} else {
			a.messages(c, t)
		}
		a.planPanel(c, t)
		a.interactionPanel(c, t)
		a.roomComposer(c, t)
	case secTasks:
		a.taskSessionMenu(c, t)
		if a.cur == nil || a.cur.TaskID == "" || a.cur.TaskID != a.getTaskID() {
			a.taskEmpty(c, t)
		} else {
			a.messages(c, t)
		}
		a.planPanel(c, t)
		a.interactionPanel(c, t)
		a.taskReadOnly(c, t)
	default:
		a.messages(c, t)
		a.planPanel(c, t)
		a.interactionPanel(c, t)
		a.composer(c, t)
	}
}

func (a *app) sidebar(c *ui.Context, t *ui.Theme, narrow bool) {
	u := uikit.Unit(c)
	col := ui.Column(c).Shrink(0).Background(t.Surface).
		Padding(u*2, u*2.5).Gap(u * 2)
	if narrow {
		col = col.Fill()
	} else {
		col = col.Width(280).BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(t.Border)
	}
	col.Children(func() {
		if narrow {
			ui.Row(c).Gap(u).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "ChatGo").Bold().Grow(1)
				if kit.IconButton(c, icons.Must("x"), "关闭列表", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
					a.sidebarOpen = false
				}
			})
		}
		a.sectionNav(c, t)
		switch a.getSection() {
		case secRooms:
			newBtn := kit.Button(c, "新建群聊", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("users"),
			})
			newBtn.FillWidth()
			if newBtn.Clicked() {
				a.newRoom()
			}
			a.roomList(c, t)
		case secTasks:
			newBtn := kit.Button(c, "新建任务", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("clock"),
			})
			newBtn.FillWidth()
			if newBtn.Clicked() {
				a.newTask()
			}
			a.taskList(c, t)
		default:
			newBtn := kit.Button(c, "新会话", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("message-square"),
			})
			newBtn.FillWidth()
			if newBtn.Clicked() {
				a.openNewSession()
			}
			search := kit.TextInput(c, &a.search, kit.TextInputOptions{
				Placeholder: "搜索会话", Label: "搜索会话",
			})
			search.Element.FillWidth()
			a.conversationList(c, t)
			a.sidebarFooter(c, t)
		}
	})
}

// sectionNav switches between conversations, group chats and scheduled tasks.
func (a *app) sectionNav(c *ui.Context, t *ui.Theme) {
	sections := []string{secChat}
	segs := []kit.Segment{{Label: "会话", Icon: icons.Must("message-square")}}
	if a.cap(capRooms) {
		sections = append(sections, secRooms)
		segs = append(segs, kit.Segment{Label: "群聊", Icon: icons.Must("users")})
	}
	if a.cap(capTasks) {
		sections = append(sections, secTasks)
		segs = append(segs, kit.Segment{Label: "任务", Icon: icons.Must("clock")})
	}
	idx := 0
	for i, s := range sections {
		if s == a.getSection() {
			idx = i
		}
	}
	before := idx
	r := kit.SegmentedControl(c, &idx, segs, kit.SegmentedControlOptions{
		Label: "板块", Equal: true,
	})
	r.FillWidth()
	if idx == before || idx < 0 || idx >= len(sections) {
		return
	}
	a.selectSection(sections[idx])
	_ = t
}

func (a *app) conversationList(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	sessions := a.visibleSessions()
	ui.Scroll(c).Grow(1).MinHeight(0).Gap(u / 2).Children(func() {
		if len(sessions) == 0 {
			ui.Text(c, "没有匹配的会话").FontSize(caption).TextColor(t.TextMuted).
				Padding(u*2, u*2)
			return
		}
		for _, g := range groupSessions(sessions) {
			a.sessionGroup(c, t, g)
		}
	})
}

type sessionGroup struct {
	workspace string
	name      string
	sessions  []sessionSummary
}

// groupSessions keeps the newest session's project first, then lists every
// session of that project together even when another project sits between
// them by update time.
func groupSessions(sessions []sessionSummary) []sessionGroup {
	index := map[string]int{}
	groups := make([]sessionGroup, 0, 4)
	for _, s := range sessions {
		i, ok := index[s.Workspace]
		if !ok {
			i = len(groups)
			index[s.Workspace] = i
			groups = append(groups, sessionGroup{workspace: s.Workspace, name: projectName(s.Workspace)})
		}
		groups[i].sessions = append(groups[i].sessions, s)
	}
	return groups
}

func projectName(workspace string) string {
	w := strings.TrimRight(strings.TrimSpace(workspace), "/\\")
	if w == "" {
		return "未分组"
	}
	if i := strings.LastIndexAny(w, "/\\"); i >= 0 && i+1 < len(w) {
		return w[i+1:]
	}
	return w
}

func (a *app) sessionGroup(c *ui.Context, t *ui.Theme, g sessionGroup) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	open := !a.collapsedGroups[g.workspace]
	row := ui.ButtonBase(c.Key("group:"+g.workspace)).FillWidth().
		Padding(u, u*1.5).Gap(u).AlignItems(ui.Center).
		Radius(mtheme.ControlRadius).Label(g.name).Tooltip(g.workspace)
	if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Icon(c, icons.Must("folder")).Size(14, 14).
			TextColor(t.Accent).Shrink(0)
		ui.Text(c, g.name).FontSize(caption).FontWeight(600).
			TextColor(t.Text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
		ui.Text(c, fmt.Sprintf("%d", len(g.sessions))).FontSize(caption).
			TextColor(t.TextMuted).Shrink(0)
		arrow := "chevron-right"
		if open {
			arrow = "chevron-down"
		}
		ui.Icon(c, icons.Must(arrow)).Size(14, 14).
			TextColor(t.TextMuted).Shrink(0)
	})
	if row.Clicked() {
		a.collapsedGroups[g.workspace] = open
	}
	if !open {
		return
	}
	for _, s := range g.sessions {
		a.sessionRow(c, t, s, u*2)
	}
}

func (a *app) sessionRow(c *ui.Context, t *ui.Theme, s sessionSummary, indent float32) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	title := strings.TrimSpace(s.Title)
	if title == "" {
		title = "未命名"
	}
	selected := a.getSelected() == s.ID
	row := ui.ButtonBase(c.Key(s.ID)).FillWidth().
		MinHeight(uikit.Density(c).ControlHeight()).Padding(u*1.5, u*2).
		PaddingStart(u*2 + indent).Gap(u).AlignItems(ui.Start).
		Radius(mtheme.ControlRadius).Label(title)
	if selected {
		row.Background(t.Selection)
	} else if row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	var menuBtn ui.Element
	row.Children(func() {
		ui.Icon(c, icons.Must("message-square")).Size(16, 16).
			TextColor(t.Accent).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
			ui.Row(c).FillWidth().Gap(u).AlignItems(ui.Center).Children(func() {
				name := ui.Text(c, title).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				if selected {
					name.TextColor(t.Accent)
				}
				if s.Running {
					ui.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).
						Background(t.Success)
				}
				ui.Text(c, kit.DateSeparatorWhen(s.UpdatedAt, uikit.DateOf(c.Now()))).
					FontSize(caption).TextColor(t.TextMuted).Shrink(0)
			})
			if sub := sessionSubtitle(s); sub != "" {
				ui.Text(c, sub).FontSize(caption).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…")
			}
		})
		if row.Hovered() || selected || a.menuFor == s.ID {
			menuBtn = kit.IconButton(c, icons.Must("ellipsis"), "会话操作",
				kit.IconButtonOptions{Variant: kit.Ghost}).AlignSelf(ui.Center)
		}
	})
	menuClicked := menuBtn.Valid() && menuBtn.Clicked()
	if menuClicked {
		a.menuFor = s.ID
		a.win.Update(func() {})
	}
	if menuBtn.Valid() {
		open := a.menuFor == s.ID
		kit.PopupMenuOpen(c, menuBtn, &open, func(m *kit.PopupMenu) {
			if m.Item("重命名", kit.PopupMenuItemOptions{}) {
				a.beginRename(s)
			}
			m.Separator()
			if m.Item("删除", kit.PopupMenuItemOptions{}) {
				a.beginDelete(s)
			}
		})
		if a.menuFor == s.ID && !open {
			a.menuFor = ""
		}
	}
	if row.Clicked() && !menuClicked {
		a.openSession(s.ID)
	}
}

func (a *app) sidebarFooter(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Column(c).Shrink(0).Padding(0, u*2).Gap(u/2).
		BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).Children(func() {
		label := "模型未就绪"
		provider, model := a.selectedModel()
		if provider != "" {
			label = fmt.Sprintf("%s / %s", provider, model)
		} else if a.meta.Error != "" {
			label = a.meta.Error
		}
		ui.Text(c, label).FontSize(caption).TextColor(t.TextMuted).
			SingleLine().Ellipsis("…")
		if a.status != "" {
			ui.Text(c, a.status).FontSize(caption).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…")
		}
	})
}

func (a *app) header(c *ui.Context, t *ui.Theme, narrow bool) {
	switch a.getSection() {
	case secRooms:
		a.roomHeader(c, t, narrow)
	case secTasks:
		a.taskHeader(c, t, narrow)
	default:
		a.chatHeader(c, t, narrow)
	}
}

func (a *app) chatHeader(c *ui.Context, t *ui.Theme, narrow bool) {
	u := uikit.Unit(c)
	title, sub := "ChatGo", "本地模型会话"
	if cur := a.current(); cur != nil {
		if strings.TrimSpace(cur.Title) != "" {
			title = cur.Title
		}
		sub = fmt.Sprintf("%d 条消息", len(cur.Messages))
	}
	if a.status != "" {
		sub = a.status
	}
	ui.Row(c).Shrink(0).Padding(u*3, u*5).Gap(u*2).AlignItems(ui.Center).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(t.Border).
		Background(t.Background).Children(func() {
		if narrow {
			if kit.IconButton(c, icons.Must("panel-left"), "会话列表", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.sidebarOpen = true
			}
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
			ui.Text(c, title).Bold().FontSize(15).SingleLine().Ellipsis("…")
			ui.Text(c, sub).FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
				TextColor(t.TextMuted).SingleLine().Ellipsis("…")
		})
		if a.isRunning() {
			if kit.Button(c, "停止", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("square"),
			}).Clicked() {
				a.stop()
			}
		}
	})
}

func (a *app) messages(c *ui.Context, t *ui.Theme) {
	cur := a.current()
	if cur == nil {
		a.emptyConversation(c, t, "选择左侧会话，或新建一个开始对话")
		return
	}
	msgs := cur.Messages
	if len(msgs) == 0 {
		a.emptyConversation(c, t, "发送第一条消息开始对话")
		return
	}
	res := kit.MessageList(c, &a.msgList, len(msgs), kit.MessageListOptions{
		ID: func(i int) string {
			if msgs[i].ID == "" {
				return fmt.Sprintf("idx-%d", i)
			}
			return msgs[i].ID
		},
		Date:  func(i int) time.Time { return msgs[i].Timestamp },
		Label: func(i int) string { return msgs[i].Role },
	}, func(i int) {
		a.messageRow(c, i, msgs[i])
	})
	res.Element.Grow(1).MinHeight(0)
}

func (a *app) emptyConversation(c *ui.Context, t *ui.Theme, text string) {
	u := uikit.Unit(c)
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).
		Gap(u * 2).Padding(u * 8).Children(func() {
		ui.Icon(c, icons.Must("message-square")).Size(32, 32).TextColor(t.Accent)
		ui.Text(c, "ChatGo").Bold().FontSize(22)
		ui.Text(c, text).TextColor(t.TextMuted)
	})
}

func (a *app) composer(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	if a.current() == nil {
		ui.Row(c).Shrink(0).Padding(u*3, u*5).
			BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "请选择或新建会话").TextColor(t.TextMuted)
		})
		return
	}
	if agent := a.sessionAgent(); agent != "" && !a.cap(capAgents) {
		ui.Row(c).FillWidth().Shrink(0).Padding(u*3, u*5).Gap(u).
			AlignItems(ui.Center).
			BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Icon(c, icons.Must("triangle-alert")).Size(16, 16).
				TextColor(t.TextMuted).Shrink(0)
			ui.Textf(c, "该会话由 %s 运行，当前 Web 运行时暂不支持继续对话", agentLabel(agent)).
				TextColor(t.TextMuted).FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
				Grow(1).MinWidth(0)
		})
		return
	}
	ui.Column(c).Shrink(0).Padding(u*3, u*5).
		BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).
		Background(t.Background).Gap(u).Children(func() {
		a.skillChips(c, t)
		a.attachmentChips(c, t)
		r := kit.PromptComposer(c, &a.draft, kit.PromptComposerOptions{
			Placeholder: "输入消息，Enter 发送，Shift+Enter 换行",
			MaxHeight:   220,
			Box:         true,
			Tools:       func() { a.composerTools(c, t) },
			Actions:     func() { a.composerActions(c, t) },
		})
		frame := r.Element
		frame.Radius(mtheme.ControlRadius).Padding(u*2, u*3).Cursor(ui.CursorText).
			Background(t.Surface).Border(mtheme.BorderWidth, t.Border)
		switch {
		case r.Input.Focused():
			frame.Border(mtheme.BorderWidth, t.Focus)
		case frame.Hovered():
			frame.Border(mtheme.BorderWidth, t.Border.Mix(t.Text, 0.35))
		}
		if frame.Clicked() {
			r.Input.Focus()
		}
		if files := frame.DroppedFiles(); len(files) > 0 {
			a.addDroppedFiles(files)
		}
		if frame.FileDragOver() {
			frame.Border(mtheme.FocusWidth, t.Focus).Background(t.Selection)
		}
		if a.cap(capSkills) && len(a.skills) > 0 && !strings.HasPrefix(a.agentRef, "acp:") {
			cmds := make([]kit.SlashCommand, 0, len(a.skills))
			for _, s := range a.skills {
				cmds = append(cmds, kit.SlashCommand{Name: s.Token, Description: s.Description})
			}
			if cmd, ok := kit.SlashCommandMenu(c, r, &a.draft, cmds).Chosen(); ok {
				a.draft = ""
				a.addSkill(cmd.Name)
				r.Input.Focus()
			}
		}
		if r.Submitted() {
			a.send()
		}
	})
}

// skillChips shows the skills the next message carries.
func (a *app) skillChips(c *ui.Context, t *ui.Theme) {
	if len(a.skillsOn) == 0 {
		return
	}
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Row(c).FillWidth().Gap(u).Wrap().AlignItems(ui.Center).Children(func() {
		for i := range a.skillsOn {
			id := a.skillsOn[i]
			ui.Row(c).Shrink(0).Gap(u).AlignItems(ui.Center).
				Padding(u/2, u).Radius(mtheme.ControlRadius).
				Background(t.Surface).Border(mtheme.BorderWidth, t.Border).Children(func() {
				ui.Icon(c, icons.Must("sliders-horizontal")).Size(12, 12).
					TextColor(t.Accent).Shrink(0)
				ui.Text(c, a.skillName(id)).FontSize(caption).SingleLine().Ellipsis("…").MaxWidth(180)
				if uikit.IconAction(c, icons.Must("x"), "移除 Skill").Clicked() {
					a.skillsOn = append(a.skillsOn[:i], a.skillsOn[i+1:]...)
				}
			})
		}
	})
}

func (a *app) composerTools(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	ui.Row(c).Gap(u).AlignItems(ui.Center).Wrap().Children(func() {
		a.contextButton(c, t)
		// An ACP session brings its own runtime: its model and options show
		// here, the way the desktop's composer does, instead of the local
		// model and tool pickers.
		if a.cap(capACP) && strings.HasPrefix(a.agentRef, "acp:") && a.current() != nil {
			a.sessionACPControls(c, t)
			return
		}
		a.modelSelector(c, t)
		a.agentSelector(c, t)
		a.toolsButton(c, t)
		a.attachmentButton(c, t, &a.attachOpen)
	})
}

func (a *app) contextButton(c *ui.Context, t *ui.Theme) {
	b := kit.IconButton(c, icons.Must("plus"), "选择上下文", kit.IconButtonOptions{
		Variant: kit.Ghost,
	})
	if b.Clicked() {
		a.toggleContext()
	}
	kit.Popover(c, b, &a.contextOpen, kit.PopoverOptions{
		Title: "选择上下文", Width: 360, Up: true,
		Header: func() {
			in := kit.TextInput(c, &a.contextQuery, kit.TextInputOptions{
				Placeholder: "搜索文件、目录、图片…", Label: "搜索上下文",
			})
			in.Element.FillWidth()
			uikit.AutoFocus(c, in.Input)
		},
	}, func() {
		items := a.visibleContextFiles()
		if len(items) == 0 {
			ui.Text(c, "没有匹配的上下文").FontSize(uikit.FontSize(c, mtheme.CaptionSize)).
				TextColor(t.TextMuted).Padding(uikit.Unit(c) * 2)
			return
		}
		u := uikit.Unit(c)
		caption := uikit.FontSize(c, mtheme.CaptionSize)
		for _, item := range items {
			row := ui.ButtonBase(c).FillWidth().MinHeight(uikit.Density(c).ControlHeight()).
				Padding(u, u*2).Gap(u).AlignItems(ui.Center).Justify(ui.Start).
				Radius(mtheme.ControlRadius).Label(item.Label)
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				color := t.TextMuted
				if item.Kind == "directory" {
					color = t.Accent
				}
				ui.Icon(c, icons.Must(contextKindIcon(item.Kind))).Size(14, 14).
					TextColor(color).Shrink(0)
				ui.Text(c, item.Label).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				ui.Text(c, contextKindLabel(item.Kind)).FontSize(caption).
					TextColor(t.TextMuted).Shrink(0)
			})
			if row.Clicked() {
				a.insertContext(item)
				a.contextOpen = false
			}
		}
	})
}

func contextKindIcon(kind string) string {
	switch kind {
	case "directory":
		return "folder"
	case "image":
		return "image"
	case "doc":
		return "file-text"
	default:
		return "file"
	}
}

func contextKindLabel(kind string) string {
	switch kind {
	case "directory":
		return "目录"
	case "image":
		return "图片"
	case "doc":
		return "文档"
	default:
		return "文件"
	}
}

func (a *app) modelSelector(c *ui.Context, t *ui.Theme) {
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	// An ACP session runs its own CLI; it has no model to pick.
	if strings.HasPrefix(a.agentRef, "acp:") {
		return
	}
	// The enriched list offers search and provider marks.
	if len(a.models) > 0 {
		a.modelPickerButton(c, t, currentModelLabel(a.models, a.modelRef),
			a.models, &a.modelRef, func(ref string) { a.modelRef = ref },
			modelPickerOptions{Up: true, Variant: kit.Secondary})
		return
	}
	if len(a.meta.Models) > 0 {
		opts := make([]kit.SelectOption[string], 0, len(a.meta.Models))
		for _, m := range a.meta.Models {
			label := m.Model
			if m.Provider != "" {
				label = m.Provider + " · " + m.Model
			}
			opts = append(opts, kit.SelectOption[string]{
				Value: modelRefOf(m.Provider, m.Model), Label: label,
			})
		}
		cur := a.modelRef
		if cur == "" {
			cur = modelRefOf(a.meta.Provider, a.meta.Model)
		}
		r := kit.Select(c, &cur, opts, kit.SelectOptions{
			Label: "模型", Placeholder: "选择模型", MinWidth: 220,
		})
		r.Element.MaxWidth(260).MinWidth(0)
		if r.Changed() {
			a.modelRef = cur
		}
		return
	}
	label := "模型未就绪"
	if a.meta.Ready {
		label = fmt.Sprintf("%s / %s", a.meta.Provider, a.meta.Model)
	} else if a.meta.Error != "" {
		label = a.meta.Error
	}
	ui.Text(c, label).FontSize(caption).TextColor(t.TextMuted).
		SingleLine().Ellipsis("…").MaxWidth(320)
}

func (a *app) composerActions(c *ui.Context, _ *ui.Theme) {
	r := kit.SendButton(c, kit.SendButtonOptions{
		Streaming: a.isRunning(),
		Disabled:  (strings.TrimSpace(a.draft) == "" && len(a.attachments) == 0) || a.attaching,
	})
	if r.Sent() {
		a.send()
	}
	if r.Stopped() {
		a.stop()
	}
}

func (a *app) visibleSessions() []sessionSummary {
	q := strings.ToLower(strings.TrimSpace(a.search))
	if q == "" {
		return a.sessions
	}
	out := make([]sessionSummary, 0, len(a.sessions))
	for _, s := range a.sessions {
		if strings.Contains(strings.ToLower(s.Title), q) ||
			strings.Contains(strings.ToLower(s.Preview), q) ||
			strings.Contains(strings.ToLower(s.Agent), q) ||
			strings.Contains(strings.ToLower(s.Workspace), q) {
			out = append(out, s)
		}
	}
	return out
}

func sessionSubtitle(s sessionSummary) string {
	label := agentLabel(s.Agent)
	if s.Messages > 0 {
		return fmt.Sprintf("%s · %d 条", label, s.Messages)
	}
	return label
}

func agentLabel(agent string) string {
	switch {
	case strings.HasPrefix(agent, "acp:"):
		return strings.TrimPrefix(agent, "acp:")
	case strings.HasPrefix(agent, "agent:"):
		return strings.TrimPrefix(agent, "agent:")
	case strings.HasPrefix(agent, "app:"):
		return strings.TrimPrefix(agent, "app:")
	case agent == "":
		return "Chat"
	default:
		return agent
	}
}
