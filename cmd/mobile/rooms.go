package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/mobile"

	"chatgo/internal/mobileapi"
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
	t := c.Theme()
	ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mobile.Section(c, "群聊", func() {
			for _, room := range a.rooms {
				r := room
				row := mobile.ListRow(c, mobile.RowOptions{
					Title:      firstNonEmpty(r.Name, "未命名"),
					Subtitle:   roomSubtitle(r),
					Disclosure: true,
					Leading: func() {
						listMark(c, "users", r.Running)
					},
					Trailing: func() {
						if r.Running {
							ui.Text(c, "生成中").FontSize(11).TextColor(t.Accent)
						}
					},
				})
				if row.Clicked() {
					a.openRoom(r.ID)
				}
			}
		})
	})
}

func roomSubtitle(r mobileapi.Room) string {
	names := make([]string, 0, len(r.Members))
	for _, m := range r.Members {
		names = append(names, firstNonEmpty(m.Name, m.Agent))
	}
	if len(names) == 0 {
		return "无成员"
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
			ui.Column(c).FillWidth().Padding(8, 16, 4, 16).Gap(3).Children(func() {
				ui.Text(c, roomSubtitle(*room)).FontSize(12).TextColor(t.TextMuted).MaxLines(2)
				if room.Workspace != "" {
					ui.Text(c, room.Workspace).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
		}
		ui.Row(c).FillWidth().Padding(4, 16).Gap(6).AlignItems(ui.Center).Children(func() {
			if primaryButton(c, "新会话").Clicked() {
				a.newRoomSession()
			}
			if smallIconButton(c, "refresh-cw", "刷新").Clicked() {
				a.loadRoomSessions(a.roomID)
			}
			ui.Spacer(c)
			if room != nil && kit.Button(c, "删除群聊", kit.ButtonOptions{
				Variant: kit.Ghost, Foreground: t.Danger,
			}).Height(34).Padding(0, 10).Clicked() {
				a.deleteRoom(room.ID)
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			if len(a.roomSess) == 0 {
				ui.Text(c, "还没有群聊会话，点“新会话”开始。").FontSize(13).TextColor(t.TextMuted).Padding(16)
				return
			}
			mobile.Section(c, "会话", func() {
				for _, s := range a.roomSess {
					session := s
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:      firstNonEmpty(session.Label, session.ID),
						Subtitle:   shortTime(session.Updated),
						Disclosure: true,
					})
					if row.Clicked() {
						a.openChat(session.ID, a.roomID, a.roomName)
					}
				}
			})
		})
	})
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
					label := "选择"
					if selected {
						label = "已选"
					}
					row := mobile.ListRow(c, mobile.RowOptions{
						Title:    firstNonEmpty(agent.Name, agent.Ref),
						Subtitle: firstNonEmpty(agent.Kind, "chat"),
						Trailing: func() {
							ui.Text(c, label).FontSize(12).TextColor(c.Theme().Accent)
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
