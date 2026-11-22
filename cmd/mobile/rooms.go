package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
)

func (a *app) roomList(c *ui.Context) {
	if a.offline() {
		a.offlineState(c)
		return
	}
	if len(a.rooms) == 0 {
		ui.Column(c).Grow(1).MinHeight(0).Justify(ui.Center).Children(func() {
			emptyState(c, "users", "还没有群聊", "创建一个多 Agent 群聊，让它们一起讨论。", func() {
				if primaryButton(c, "新建群聊").Clicked() {
					a.openRoomForm("")
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
				for _, room := range a.rooms {
					a.roomRow(c, room)
				}
			})
		})
		fabButton(c, "plus", "新建群聊", func() { a.openRoomForm("") })
	})
}

// roomRow is one group chat: its name, the members taking part and when it
// last moved.
func (a *app) roomRow(c *ui.Context, r mobileapi.Room) {
	t := c.Theme()
	title := firstNonEmpty(r.Name, "未命名")
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(12, 16).Gap(12).
		Role(ui.RoleButton).Label(title + ", " + roomMembersPreview(r))
	mobile.Ink(row)
	if mobile.Current() == mobile.Cupertino && row.Pressed() {
		row.Background(t.SurfacePressed)
	}
	row.Children(func() {
		listMark(c, "users", r.Running)
		ui.Column(c).Grow(1).Basis(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Text(c, roomMembersPreview(r)).FontSize(13).TextColor(t.TextMuted).
				MaxLines(1).Ellipsis("…")
		})
		ui.Column(c).AlignItems(ui.End).Shrink(0).Gap(4).Children(func() {
			if !r.Updated.IsZero() {
				ui.Text(c, shortTime(r.Updated)).FontSize(11).TextColor(t.TextMuted)
			}
			if r.Running {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent).Shrink(0)
					ui.Text(c, "生成中").FontSize(10).TextColor(t.Accent)
				})
			} else if r.Sessions > 0 {
				ui.Text(c, fmt.Sprintf("%d 个会话", r.Sessions)).FontSize(10).TextColor(t.TextMuted)
			}
		})
	})
	if row.Clicked() {
		a.openRoom(r.ID)
	}
}

// roomMembersPreview names the members taking part, a few of them and the
// rest as a count.
func roomMembersPreview(r mobileapi.Room) string {
	names := make([]string, 0, len(r.Members))
	for _, m := range r.Members {
		names = append(names, firstNonEmpty(m.Name, m.Agent))
	}
	if len(names) == 0 {
		return "无成员"
	}
	if len(names) > 3 {
		return fmt.Sprintf("%s 等 %d 个成员", strings.Join(names[:3], "、"), len(names))
	}
	return strings.Join(names, "、")
}

func (a *app) openRoom(id string) {
	for _, r := range a.rooms {
		if r.ID == id {
			a.update(func() {
				a.screen = screenRoom
				a.roomID = id
				a.roomName = r.Name
				a.roomSess = nil
			})
			a.loadRoomSessions(id)
			return
		}
	}
	a.errorf("群聊不存在: %s", id)
}

func (a *app) loadRoomSessions(id string) {
	if a.client == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		sessions, err := a.client.RoomSessions(ctx, id)
		if err != nil {
			a.errorf("读取群聊会话失败: %v", err)
			return
		}
		a.update(func() {
			if a.roomID == id {
				a.roomSess = sessions
			}
		})
	}()
}

func (a *app) roomView(c *ui.Context) {
	t := c.Theme()
	room := a.findRoom(a.roomID)
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		if room != nil {
			a.roomHeader(c, *room)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(a.roomSess) == 0 {
				ui.Text(c, "还没有群聊会话，点“新会话”开始。").FontSize(13).
					TextColor(t.TextMuted).Padding(16)
				return
			}
			mobile.Section(c, "会话", func() {
				for _, s := range a.roomSess {
					a.roomSessionRow(c, s)
				}
			})
			// Deleting a group takes the whole group with it, so the button
			// sits at the end of the list rather than in the bar.
			if room != nil {
				ui.Row(c).FillWidth().Padding(16, 16, 24, 16).Justify(ui.Center).Children(func() {
					if kit.Button(c, "删除群聊", kit.ButtonOptions{
						Variant: kit.Ghost, Foreground: t.Danger,
					}).Height(34).Padding(0, 12).Clicked() {
						a.deleteRoom(room.ID)
					}
				})
			}
		})
	})
}

// roomHeader shows what the group is: its members, where it works and the
// one action it wants.
func (a *app) roomHeader(c *ui.Context, r mobileapi.Room) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(12, 16, 8, 16).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			for _, m := range r.Members {
				m := m
				ui.Row(c).Gap(5).AlignItems(ui.Center).Padding(4, 8).
					Radius(10).Background(t.Surface).Children(func() {
					a.agentMark(c, m.Agent, 14)
					ui.Text(c, firstNonEmpty(m.Name, m.Agent)).FontSize(12).TextColor(t.Text).
						SingleLine().Ellipsis("…").MaxWidth(140)
				})
			}
		})
		if r.Workspace != "" {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons.Must("folder")).Size(13, 13).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, filepath.Base(r.Workspace)).FontSize(11).TextColor(t.TextMuted).
					SingleLine().Ellipsis("…")
			})
		}
		if primaryButton(c, "新会话").Clicked() {
			a.newRoomSession()
		}
	})
}

// roomSessionRow is one conversation inside a group: its name and when it
// last moved.
func (a *app) roomSessionRow(c *ui.Context, s mobileapi.RoomSession) {
	t := c.Theme()
	title := firstNonEmpty(s.Label, s.ID)
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Padding(12, 16).Gap(12).
		Role(ui.RoleButton).Label(title)
	mobile.Ink(row)
	if mobile.Current() == mobile.Cupertino && row.Pressed() {
		row.Background(t.SurfacePressed)
	}
	row.Children(func() {
		listMark(c, "message-square", false)
		ui.Text(c, title).FontSize(15).Grow(1).Basis(0).SingleLine().Ellipsis("…")
		if !s.Updated.IsZero() {
			ui.Text(c, shortTime(s.Updated)).FontSize(11).TextColor(t.TextMuted).Shrink(0)
		}
	})
	if row.Clicked() {
		a.openChat(s.ID, a.roomID, a.roomName)
	}
}

func (a *app) newRoomSession() {
	if a.client == nil || a.roomID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		session, err := a.client.CreateRoomSession(ctx, a.roomID)
		if err != nil {
			a.errorf("新建群聊会话失败: %v", err)
			return
		}
		a.update(func() { a.roomSess = append([]mobileapi.RoomSession{session}, a.roomSess...) })
		a.openChat(session.ID, a.roomID, a.roomName)
	}()
}

func (a *app) findRoom(id string) *mobileapi.Room {
	for i := range a.rooms {
		if a.rooms[i].ID == id {
			return &a.rooms[i]
		}
	}
	return nil
}

func (a *app) deleteRoom(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.client.DeleteRoom(ctx, id); err != nil {
			a.errorf("删除群聊失败: %v", err)
			return
		}
		a.update(func() {
			a.screen = screenRooms
			a.roomID = ""
			a.roomName = ""
		})
		a.refreshRooms()
	}()
}

func (a *app) openRoomForm(id string) {
	form := mobileapi.RoomInput{}
	if id != "" {
		for _, r := range a.rooms {
			if r.ID == id {
				form = mobileapi.RoomInput{
					Name:               r.Name,
					Workspace:          r.Workspace,
					Activation:         r.Activation,
					Master:             r.Master,
					AllowSelfResponses: r.AllowSelfResponses,
					Members:            r.Members,
				}
				break
			}
		}
	}
	sel := map[string]bool{}
	for _, m := range form.Members {
		sel[m.Agent] = true
	}
	a.update(func() {
		a.roomFormOpen = true
		a.roomEditing = id
		a.roomFormName = form.Name
		a.roomFormSel = sel
	})
	if len(a.agents) == 0 && a.client != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if agents, err := a.client.Agents(ctx); err == nil {
				a.update(func() { a.agents = agents })
			}
		}()
	}
}

func (a *app) roomFormSheet(c *ui.Context) {
	mobile.Sheet(c, &a.roomFormOpen, mobile.SheetOptions{Detents: []float32{0.92}, Label: "新建群聊"}, func() {
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			title := "新建群聊"
			if a.roomEditing != "" {
				title = "编辑群聊"
			}
			ui.Text(c, title).FontSize(17).FontWeight(600)
			ui.TextInput(c, &a.roomFormName).Placeholder("群名称")
			ui.Text(c, "成员").FontSize(13).TextColor(c.Theme().TextMuted)
			ui.Scroll(c).MaxHeight(320).Children(func() {
				for _, agent := range a.agents {
					if agent.Disabled {
						continue
					}
					ref := agent.Ref
					selected := a.roomFormSel[ref]
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:    firstNonEmpty(agent.Name, agent.Ref),
						Subtitle: firstNonEmpty(agent.Kind, "chat"),
						Leading: func() {
							a.agentMark(c, ref, 20)
						},
						Trailing: func() {
							if selected {
								ui.Icon(c, icons.Must("check")).Size(16, 16).TextColor(c.Theme().Accent)
							}
						},
					})
					if row.Clicked() {
						a.roomFormSel[ref] = !selected
						a.invalidate()
					}
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				if primaryButton(c, "保存").Clicked() {
					a.createRoom()
				}
				if ghostButton(c, "取消").Clicked() {
					a.roomFormOpen = false
				}
			})
		})
	})
}

func (a *app) currentRoomActivation() string {
	if r := a.findRoom(a.roomEditing); r != nil {
		return r.Activation
	}
	return "all"
}

func (a *app) currentRoomMaster() string {
	if r := a.findRoom(a.roomEditing); r != nil {
		return r.Master
	}
	return ""
}

func (a *app) currentRoomSelfResponses() bool {
	if r := a.findRoom(a.roomEditing); r != nil {
		return r.AllowSelfResponses
	}
	return false
}

func (a *app) createRoom() {
	name := strings.TrimSpace(a.roomFormName)
	if name == "" {
		a.toast("群名称不能为空")
		return
	}
	var members []mobileapi.RoomMember
	for _, agent := range a.agents {
		if a.roomFormSel[agent.Ref] {
			members = append(members, mobileapi.RoomMember{
				Agent: agent.Ref,
				Name:  firstNonEmpty(agent.Name, agent.Ref),
			})
		}
	}
	if len(members) == 0 {
		a.toast("至少选择一个成员")
		return
	}
	in := mobileapi.RoomInput{
		Name:               name,
		Activation:         firstNonEmpty(a.currentRoomActivation(), "all"),
		Master:             a.currentRoomMaster(),
		Members:            members,
		AllowSelfResponses: a.currentRoomSelfResponses(),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var room mobileapi.Room
		var err error
		if a.roomEditing != "" {
			room, err = a.client.UpdateRoom(ctx, a.roomEditing, in)
		} else {
			room, err = a.client.CreateRoom(ctx, in)
		}
		if err != nil {
			a.errorf("保存群聊失败: %v", err)
			return
		}
		a.update(func() { a.roomFormOpen = false })
		a.refreshRooms()
		a.openRoom(room.ID)
	}()
}
