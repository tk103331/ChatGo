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
	"chatgo/pkg/models"
)

// roomForm edits one room; id is empty for a new one.
type roomForm struct {
	id         string
	name       string
	workspace  string
	activation string
	master     string
	allowSelf  bool
	members    []roomMemberInfo
}

func activationLabel(v string) string {
	switch v {
	case "natural":
		return "自然发话"
	case "pooled":
		return "轮流发言"
	case "mention":
		return "仅被 @"
	case "master":
		return "主持人调度"
	default:
		return "全员发言"
	}
}

func activationOptions() []kit.SelectOption[string] {
	return []kit.SelectOption[string]{
		{Value: "all", Label: "全员发言"},
		{Value: "natural", Label: "自然发话"},
		{Value: "pooled", Label: "轮流发言"},
		{Value: "mention", Label: "仅被 @ 时发言"},
		{Value: "master", Label: "主持人调度"},
	}
}

// refreshRooms reloads the room list.
func (a *app) refreshRooms() {
	var resp struct {
		Rooms []roomInfo `json:"rooms"`
	}
	if err := apiGet("/api/rooms", &resp); err != nil {
		a.win.Update(func() { a.status = "群聊列表：" + err.Error() })
		return
	}
	a.win.Update(func() { a.rooms = resp.Rooms })
}

// refreshRoomSessions reloads the open room's conversations.
func (a *app) refreshRoomSessions() {
	roomID := a.getRoomID()
	if roomID == "" {
		return
	}
	var resp struct {
		Sessions []roomSessionInfo `json:"sessions"`
	}
	if err := apiGet("/api/rooms/"+roomID+"/sessions", &resp); err != nil {
		a.win.Update(func() { a.status = "群聊会话：" + err.Error() })
		return
	}
	a.win.Update(func() {
		if a.getRoomID() != roomID {
			return
		}
		a.roomSessions = resp.Sessions
		cur := a.getRoomConv()
		if cur == "" && len(resp.Sessions) > 0 {
			a.setRoomConv(resp.Sessions[0].ID)
			go a.loadSession(resp.Sessions[0].ID, false)
		}
	})
}

// openRoom enters a room, newest conversation first.
func (a *app) openRoom(id string) {
	if id == "" {
		return
	}
	a.setRoomID(id)
	a.setRoomConv("")
	a.win.Update(func() {
		a.roomSessions = nil
		a.pending = nil
		a.attachments = nil
		a.draft = ""
		a.sidebarOpen = false
	})
	go func() {
		var resp struct {
			Sessions []roomSessionInfo `json:"sessions"`
		}
		if err := apiGet("/api/rooms/"+id+"/sessions", &resp); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		if len(resp.Sessions) == 0 {
			a.createRoomSession(id)
			return
		}
		a.win.Update(func() {
			if a.getRoomID() != id {
				return
			}
			a.roomSessions = resp.Sessions
			a.setRoomConv(resp.Sessions[0].ID)
			a.msgList.ScrollToEnd()
		})
		a.loadSession(resp.Sessions[0].ID, true)
	}()
}

// newRoomSession adds a blank conversation to the open room.
func (a *app) newRoomSession() {
	a.createRoomSession(a.getRoomID())
}

func (a *app) createRoomSession(roomID string) {
	if roomID == "" {
		return
	}
	go func() {
		var resp struct {
			Session roomSessionInfo `json:"session"`
		}
		if err := apiPost("/api/rooms/"+roomID+"/sessions", nil, &resp); err != nil {
			a.win.Update(func() { a.status = err.Error() })
			return
		}
		a.win.Update(func() {
			if a.getRoomID() != roomID {
				return
			}
			a.setRoomConv(resp.Session.ID)
			a.msgList.ScrollToEnd()
		})
		a.refreshRoomSessions()
		a.loadSession(resp.Session.ID, true)
	}()
}

// sendRoom posts the draft into the open room and starts its turn.
func (a *app) sendRoom() {
	roomID := a.getRoomID()
	convID := a.getRoomConv()
	text := strings.TrimSpace(a.draft)
	attachments := append([]attachmentRef{}, a.attachments...)
	if roomID == "" || convID == "" || a.isRunning() || a.attaching || (text == "" && len(attachments) == 0) {
		return
	}
	a.draft = ""
	a.attachments = nil
	a.setRunning(true)
	a.status = "正在生成…"
	if a.cur != nil && a.cur.ID == convID {
		content := text
		for _, att := range attachments {
			content += "\n\n[附件 " + att.Name + "]"
		}
		a.cur.Messages = append(a.cur.Messages, models.Message{
			ID:   fmt.Sprintf("local-%d", time.Now().UnixNano()),
			Role: "user", Content: content,
			AuthorID: models.AuthorHuman, AuthorKind: models.AuthorKindHuman,
			Timestamp: time.Now(),
		})
	}
	a.msgList.ScrollToEnd()
	a.win.Update(func() {})
	body := map[string]any{"text": text}
	if len(attachments) > 0 {
		body["attachments"] = attachments
	}
	path := "/api/rooms/" + roomID + "/sessions/" + convID + "/messages"
	go func() {
		if err := apiPost(path, body, nil); err != nil {
			a.win.Update(func() {
				a.attachments = attachments
				a.setRunning(false)
				a.status = err.Error()
			})
			return
		}
		a.loadSession(convID, false)
	}()
}

// stopRoom cancels the running room turn.
func (a *app) stopRoom() {
	id := a.getRoomConv()
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

// roomList draws the room sidebar.
func (a *app) roomList(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	ui.Scroll(c).Grow(1).MinHeight(0).Gap(u / 2).Children(func() {
		if len(a.rooms) == 0 {
			ui.Text(c, "还没有群聊，点上方新建一个").FontSize(caption).
				TextColor(t.TextMuted).Padding(u*2, u*2)
			return
		}
		for _, room := range a.rooms {
			selected := room.ID == a.getRoomID()
			row := ui.ButtonBase(c.Key("room:"+room.ID)).FillWidth().
				MinHeight(uikit.Density(c).ControlHeight()).Padding(u*1.5, u*2).
				Gap(u).AlignItems(ui.Start).Radius(mtheme.ControlRadius).
				Label(room.Name)
			if selected {
				row.Background(t.Selection)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Icon(c, icons.Must("users")).Size(16, 16).
					TextColor(t.Accent).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
					ui.Row(c).FillWidth().Gap(u).AlignItems(ui.Center).Children(func() {
						name := ui.Text(c, room.Name).SingleLine().Ellipsis("…").
							Grow(1).MinWidth(0)
						if selected {
							name.TextColor(t.Accent)
						}
						if room.Running {
							ui.Box(c).Size(u*1.5, u*1.5).Shrink(0).Radius(u * 0.75).
								Background(t.Success)
						}
					})
					sub := fmt.Sprintf("%d 个成员 · %s", len(room.Members), activationLabel(room.Activation))
					ui.Text(c, sub).FontSize(caption).TextColor(t.TextMuted).
						SingleLine().Ellipsis("…")
				})
			})
			if row.Clicked() {
				a.openRoom(room.ID)
			}
		}
	})
}

// roomHeader is the open room's title and controls.
func (a *app) roomHeader(c *ui.Context, t *ui.Theme, narrow bool) {
	u := uikit.Unit(c)
	caption := uikit.FontSize(c, mtheme.CaptionSize)
	room := a.currentRoom()
	title, sub := "群聊", "选择一个群聊"
	if room != nil {
		title = room.Name
		sub = fmt.Sprintf("%d 个成员 · %s", len(room.Members), activationLabel(room.Activation))
	}
	if a.isRunning() {
		sub = "正在生成…"
	}
	ui.Row(c).Shrink(0).Padding(u*3, u*5).Gap(u*2).AlignItems(ui.Center).
		BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(t.Border).
		Background(t.Background).Children(func() {
		if narrow {
			if kit.IconButton(c, icons.Must("panel-left"), "群聊列表", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.sidebarOpen = true
			}
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(u / 2).Children(func() {
			ui.Text(c, title).Bold().FontSize(15).SingleLine().Ellipsis("…")
			ui.Text(c, sub).FontSize(caption).TextColor(t.TextMuted).
				SingleLine().Ellipsis("…")
		})
		if a.isRunning() {
			if kit.Button(c, "停止", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("square"),
			}).Clicked() {
				a.stopRoom()
			}
		}
		if room != nil {
			if kit.IconButton(c, icons.Must("history"), "群聊会话", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.roomSessionsOpen = !a.roomSessionsOpen
			}
			if kit.IconButton(c, icons.Must("settings"), "群聊设置", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.editRoom(room)
			}
			if kit.IconButton(c, icons.Must("circle-x"), "删除群聊", kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
				a.beginDeleteRoom()
			}
		}
	})
}

// roomSessionMenu lists the room's conversations under the header.
func (a *app) roomSessionMenu(c *ui.Context, t *ui.Theme) {
	if !a.roomSessionsOpen || a.getRoomID() == "" {
		return
	}
	// A menu anchored to the header's history button would need its element;
	// a compact card under the header keeps the same actions without a
	// second anchor.
	u := uikit.Unit(c)
	ui.Row(c).FillWidth().Shrink(0).Padding(u, u*5).Gap(u).Wrap().
		AlignItems(ui.Center).Children(func() {
		if kit.Button(c, "新会话", kit.ButtonOptions{
			Variant: kit.Secondary, Icon: icons.Must("plus"),
		}).Clicked() {
			a.roomSessionsOpen = false
			a.newRoomSession()
		}
		for _, s := range a.roomSessions {
			selected := s.ID == a.getRoomConv()
			b := kit.Button(c, s.Label, kit.ButtonOptions{
				Variant: kit.Ghost,
			})
			if selected {
				b.Background(t.Selection)
			}
			if b.Clicked() {
				a.roomSessionsOpen = false
				a.setRoomConv(s.ID)
				go a.loadSession(s.ID, true)
			}
		}
	})
}

func (a *app) currentRoom() *roomInfo {
	for i := range a.rooms {
		if a.rooms[i].ID == a.getRoomID() {
			return &a.rooms[i]
		}
	}
	return nil
}

// roomComposer is the room's message input.
func (a *app) roomComposer(c *ui.Context, t *ui.Theme) {
	u := uikit.Unit(c)
	if a.getRoomID() == "" || a.getRoomConv() == "" {
		ui.Row(c).Shrink(0).Padding(u*3, u*5).
			BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "选择或新建一个群聊会话").TextColor(t.TextMuted)
		})
		return
	}
	ui.Column(c).Shrink(0).Padding(u*3, u*5).Gap(u).
		BorderWidth(mtheme.BorderWidth, 0, 0, 0).BorderColor(t.Border).
		Background(t.Background).Children(func() {
		a.attachmentChips(c, t)
		r := kit.PromptComposer(c, &a.draft, kit.PromptComposerOptions{
			Placeholder: "向群里发消息，Enter 发送，Shift+Enter 换行",
			MaxHeight:   220,
			Box:         true,
			Tools: func() {
				ui.Row(c).Gap(u).AlignItems(ui.Center).Wrap().Children(func() {
					a.contextButton(c, t)
					a.attachmentButton(c, t, &a.attachOpen)
				})
			},
			Actions: func() {
				r := kit.SendButton(c, kit.SendButtonOptions{
					Streaming: a.isRunning(),
					Disabled:  (strings.TrimSpace(a.draft) == "" && len(a.attachments) == 0) || a.attaching,
				})
				if r.Sent() {
					a.sendRoom()
				}
				if r.Stopped() {
					a.stopRoom()
				}
			},
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
		if r.Submitted() {
			a.sendRoom()
		}
	})
}

// newRoom opens the editor for a new room.
func (a *app) newRoom() {
	a.roomForm = &roomForm{activation: "all"}
	a.roomEditorOpen = true
	a.roomEditorClosing = false
	a.win.Update(func() {})
}

// editRoom opens the editor for an existing room.
func (a *app) editRoom(room *roomInfo) {
	form := &roomForm{
		id: room.ID, name: room.Name, workspace: room.Workspace,
		activation: room.Activation, master: room.Master,
		allowSelf: room.AllowSelfResponses,
		members:   append([]roomMemberInfo{}, room.Members...),
	}
	a.roomForm = form
	a.roomEditorOpen = true
	a.roomEditorClosing = false
	a.win.Update(func() {})
}

// roomEditor is the create/edit dialog.
func (a *app) roomEditor(c *ui.Context) {
	if a.roomForm == nil || (!a.roomEditorOpen && !a.roomEditorClosing) {
		return
	}
	f := a.roomForm
	wasOpen := a.roomEditorOpen
	title := "新建群聊"
	if f.id != "" {
		title = "编辑群聊"
	}
	kit.Dialog(c, &a.roomEditorOpen, kit.DialogOptions{
		Title: title,
		Actions: func() {
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Secondary}).Clicked() {
				a.roomEditorOpen = false
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				a.saveRoom()
			}
		},
	}, func() {
		in := kit.TextInput(c, &f.name, kit.TextInputOptions{
			Placeholder: "群名称", Label: "群名称",
		})
		in.Element.FillWidth()
		uikit.AutoFocus(c, in.Input)

		wsOpts := []kit.SelectOption[string]{{Value: "", Label: "不绑定工作区"}}
		for _, ws := range sortedWorkspaces(a.workspaces) {
			wsOpts = append(wsOpts, kit.SelectOption[string]{Value: ws, Label: ws})
		}
		ws := kit.Select(c, &f.workspace, wsOpts, kit.SelectOptions{Label: "工作区", Placeholder: "选择工作区"})
		ws.Element.FillWidth()

		act := kit.Select(c, &f.activation, activationOptions(), kit.SelectOptions{Label: "发言方式"})
		act.Element.FillWidth()

		master := kit.TextInput(c, &f.master, kit.TextInputOptions{
			Placeholder: "主持人模型，例如 OpenAI::gpt-4o（可留空）", Label: "主持人模型",
		})
		master.Element.FillWidth()
		allow := f.allowSelf
		if kit.SwitchRow(c, &allow, "允许连续发言", kit.SwitchRowOptions{
			Hint: "同一个成员可以连续两轮发言",
		}).Changed() {
			f.allowSelf = allow
		}

		ui.Row(c).FillWidth().Gap(uikit.Unit(c)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "成员").FontWeight(600).Grow(1)
			if kit.Button(c, "添加成员", kit.ButtonOptions{
				Variant: kit.Secondary, Icon: icons.Must("plus"),
			}).Clicked() {
				agent := ""
				if len(a.agents) > 0 {
					agent = a.agents[0].Ref
				}
				f.members = append(f.members, roomMemberInfo{Agent: agent})
				a.win.Update(func() {})
			}
		})
		removeMember := -1
		for i := range f.members {
			m := &f.members[i]
			ui.Column(c).FillWidth().Gap(uikit.Unit(c)/2).
				Padding(uikit.Unit(c), uikit.Unit(c)).
				Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, c.Theme().Border).
				Children(func() {
					ui.Row(c).FillWidth().Gap(uikit.Unit(c)).AlignItems(ui.Center).Children(func() {
						opts := make([]kit.SelectOption[string], 0, len(a.agents)+1)
						if m.Agent != "" {
							found := false
							for _, ag := range a.agents {
								if ag.Ref == m.Agent {
									found = true
									break
								}
							}
							if !found {
								opts = append(opts, kit.SelectOption[string]{Value: m.Agent, Label: m.Agent})
							}
						}
						for _, ag := range a.agents {
							label := ag.Name
							if ag.Kind == "acp" {
								label += " · ACP"
							}
							opts = append(opts, kit.SelectOption[string]{Value: ag.Ref, Label: label})
						}
						sel := kit.Select(c, &m.Agent, opts, kit.SelectOptions{Label: "成员 Agent"})
						sel.Element.Grow(1)
						if kit.IconButton(c, icons.Must("x"), "移除成员",
							kit.IconButtonOptions{Variant: kit.Ghost}).Clicked() {
							removeMember = i
						}
					})
					ui.Row(c).FillWidth().Gap(uikit.Unit(c)).AlignItems(ui.Center).Children(func() {
						name := kit.TextInput(c, &m.Name, kit.TextInputOptions{
							Placeholder: "群内昵称（可留空）", Label: "群内昵称",
						})
						name.Element.Grow(1)
						muted := m.Muted
						if kit.SwitchRow(c, &muted, "静音", kit.SwitchRowOptions{Compact: true}).Changed() {
							m.Muted = muted
						}
					})
				})
		}
		if removeMember >= 0 && removeMember < len(f.members) {
			f.members = append(f.members[:removeMember], f.members[removeMember+1:]...)
			a.win.Update(func() {})
		}
	})
	switch {
	case a.roomEditorOpen:
		a.roomEditorClosing = false
	case wasOpen:
		a.roomEditorClosing = true
	default:
		a.roomEditorClosing = false
		a.roomForm = nil
	}
}

// saveRoom posts the editor form.
func (a *app) saveRoom() {
	f := a.roomForm
	if f == nil {
		return
	}
	if strings.TrimSpace(f.name) == "" || len(f.members) == 0 {
		a.status = "请填写群名称并至少加入一个成员"
		return
	}
	input := roomInput{
		Name: f.name, Workspace: f.workspace, Activation: f.activation,
		Master: f.master, AllowSelfResponses: f.allowSelf,
		Members: append([]roomMemberInfo{}, f.members...),
	}
	id := f.id
	a.roomEditorOpen = false
	go func() {
		var out struct {
			Room roomInfo `json:"room"`
		}
		var err error
		if id == "" {
			err = apiPost("/api/rooms", input, &out)
		} else {
			err = apiPatch("/api/rooms/"+id, input, &out)
		}
		if err != nil {
			a.win.Update(func() { a.status = "保存群聊失败：" + err.Error() })
			return
		}
		a.win.Update(func() { a.status = "" })
		a.refreshRooms()
		if id == "" && out.Room.ID != "" {
			a.openRoom(out.Room.ID)
		}
	}()
}

// beginDeleteRoom asks before removing a room and its conversations.
func (a *app) beginDeleteRoom() {
	if a.getRoomID() == "" {
		return
	}
	a.roomDeleteOpen = true
	a.win.Update(func() {})
}

// roomDeleteDialog confirms deleting the open room.
func (a *app) roomDeleteDialog(c *ui.Context) {
	if !a.roomDeleteOpen {
		return
	}
	wasOpen := a.roomDeleteOpen
	name := "该群聊"
	if room := a.currentRoom(); room != nil {
		name = "「" + room.Name + "」"
	}
	res := kit.AlertDialog(c, &a.roomDeleteOpen, kit.AlertDialogOptions{
		Title:        "删除群聊",
		Description:  "将删除" + name + "及其全部会话记录，此操作不可撤销。",
		ConfirmLabel: "删除",
	})
	if res.Confirmed {
		a.commitDeleteRoom()
	}
	if a.roomDeleteOpen || !wasOpen {
		return
	}
}

func (a *app) commitDeleteRoom() {
	id := a.getRoomID()
	if id == "" {
		return
	}
	a.roomDeleteOpen = false
	go func() {
		if err := apiDelete("/api/rooms/"+id, nil); err != nil {
			a.win.Update(func() { a.status = "删除群聊失败：" + err.Error() })
			return
		}
		a.win.Update(func() {
			a.setRoomID("")
			a.roomSessions = nil
			a.setRoomConv("")
			a.cur = nil
			a.setRunning(false)
			a.status = ""
		})
		a.refreshRooms()
	}()
}
