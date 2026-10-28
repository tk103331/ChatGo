package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// roomState is the group-chat section's own state. The section is a page of
// its own, not a mode of the chat view: a room has a roster and a turn that
// several members answer, which the one-agent chat composer cannot express.
type roomState struct {
	// active is the room being talked in, empty while the list is shown.
	active string
	// conv is the active room's conversation, held open the way the chat
	// view holds cw.cur. It must be one object for the whole visit: the
	// running turn appends streamed chunks to the object it captured, so a
	// copy re-read per frame would render none of them.
	conv *models.Conversation
	// editorOpen shows the room editor dialog over the list.
	editorOpen bool
	// settingsOpen shows the chat view's right-hand settings panel, which
	// edits the same form the dialog does.
	settingsOpen bool
	form         roomForm
	draft        string
	// running is the room whose turn is being generated, empty when idle.
	running string
	// queue is the members still waiting to speak in the running turn, in
	// order, so the roster can show who is up next.
	queue []string
	// speaking is the member generating right now.
	speaking string
	// passed records the members that chose silence this turn, keyed by
	// agent reference, so the roster can show they ran and declined.
	passed map[string]bool
	// cancel stops the running turn, including the members not yet reached.
	cancel context.CancelFunc
	// list keeps the room message list's own scroll place, separate from
	// the chat view's.
	list kit.MessageListState
	// detail shows the right-hand panel with every message in full, which
	// is where thinking and tool calls live once the main list is compact.
	detail bool
	// turnOrder is every member of the running turn in speaking order. The
	// queue alone cannot draw the graph: it only holds who is still waiting,
	// and the graph needs the finished members too.
	turnOrder []string
	// detailAgent is the member whose reply the detail panel's lower half
	// shows alone, picked by clicking a graph node; empty shows every reply.
	detailAgent string
	// detailTurnID anchors the detail panel to the turn containing this
	// message; empty follows the newest turn.
	detailTurnID string
	// sessions lists the active room's conversations, newest first, for the
	// session column.
	sessions []roomSession
	// turnConv is the conversation the running turn writes into. Switching
	// sessions must hand this object back, not a disk copy: the turn appends
	// streamed chunks to it, and a copy would show none of them.
	turnConv *models.Conversation
	// masterChat moderates under ActivationMaster, resolved on the main
	// thread before the turn goroutine starts.
	masterChat masterChatFunc
	// handoff collects the agents @-mentioned by a member's reply or its
	// room_send call, for the running turn to wake next.
	handoff []string
	// quoteID is the message the draft replies to, empty for a plain
	// message. It is cleared when the draft is sent or the quote dropped.
	quoteID string
}

// roomSession is one conversation of a room in the session column.
type roomSession struct {
	ID      string
	Label   string
	Updated time.Time
}

// roomForm edits one room. editing is the index into Config.Rooms, or -1 for
// a room being created.
type roomForm struct {
	editing    int
	name       string
	workspace  string
	activation string
	master     string
	allowSelf  bool
	members    []config.RoomMember
}

// roomsPage renders the group-chat section: the room cards or the active
// room's conversation, with the create/edit dialog floating over either.
func (cw *ChatWindow) roomsPage(c *mgo.Context) {
	if cw.room.active != "" {
		cw.roomChatView(c)
	} else {
		cw.roomListPage(c)
	}
	cw.roomEditor(c)
}

func (cw *ChatWindow) roomListPage(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().Padding(u * 4).Gap(u * 3).Children(func() {
		mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
			mgo.Column(c).Grow(1).Children(func() {
				mgo.Text(c, "群聊").FontSize(mcore.FontSize(c, mtheme.H2Size)).FontWeight(600)
				mgo.Text(c, "把多个 Agent 拉进一个群，让它们围绕同一个问题各自发言。").
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(mcore.Tokens(c).TextMuted)
			})
			if kit.Button(c, "新建群聊", kit.ButtonOptions{Variant: kit.Primary, Icon: icons.Must("plus")}).Clicked() {
				cw.newRoomForm()
			}
		})
		if len(cw.cfg.Rooms) == 0 {
			cw.roomEmptyState(c)
			return
		}
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Row(c).FillWidth().Gap(u*2).Wrap().AlignItems(mgo.Start).
				Padding(0, 0, u*4, 0).Children(func() {
				for i := range cw.cfg.Rooms {
					cw.roomCard(c, i)
				}
			})
		})
	})
}

func (cw *ChatWindow) roomEmptyState(c *mgo.Context) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Gap(u * 2).Children(func() {
		mgo.Icon(c, icons.Must("users")).Size(40, 40).TextColor(k.TextMuted)
		mgo.Text(c, "还没有群聊").FontWeight(600)
		mgo.Text(c, "新建一个群，选几个 Agent 进来，绑定一个工作目录。").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
	})
}

// roomCard is one room in the grid: clicking the card enters its newest
// session, the pencil opens the editor dialog.
func (cw *ChatWindow) roomCard(c *mgo.Context, i int) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	r := cw.cfg.Rooms[i]
	b := mgo.ButtonBase(c).Key(fmt.Sprintf("room:%d", i)).Width(320).
		AlignItems(mgo.Stretch).Padding(u*3).Radius(mtheme.ControlRadius).
		Background(k.Surface).Border(mtheme.BorderWidth, k.Border).Label("进入 " + r.Name)
	if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
			mgo.Row(c).FillWidth().AlignItems(mgo.Center).Gap(u).Children(func() {
				mgo.Icon(c, icons.Must("users")).Size(20, 20).TextColor(k.Accent).Shrink(0)
				mgo.Text(c, r.Name).FontWeight(600).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if mcore.IconAction(c, icons.Must("pencil"), "编辑群聊").Clicked() {
					cw.loadRoomForm(i)
				}
			})
			mgo.Text(c, roomSummary(r)).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).MaxLines(2)
			if members := r.ActiveMembers(); len(members) > 0 {
				mgo.Row(c).Gap(u).Wrap().AlignItems(mgo.Center).Children(func() {
					for _, m := range members {
						cw.roomMemberChip(c, m)
					}
				})
			}
		})
	})
	if b.Clicked() {
		cw.openRoom(r.ID)
	}
}

// roomMemberChip is one member's avatar and name, drawn with the same mark
// the chat bubbles use so a member looks the same everywhere.
func (cw *ChatWindow) roomMemberChip(c *mgo.Context, m config.RoomMember) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Row(c).Gap(u).AlignItems(mgo.Center).Padding(u/2, u, u/2, u).
		Radius(mtheme.ControlRadius).Background(k.SurfaceHover).Children(func() {
		cw.roomMemberAvatar(c, m.Agent, 18)
		mgo.Text(c, roomMemberName(m)).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).SingleLine()
	})
}

// roomMemberAvatar draws a member's mark by its agent reference.
func (cw *ChatWindow) roomMemberAvatar(c *mgo.Context, agent string, size float32) {
	switch {
	case agent == masterAuthorID:
		mgo.Icon(c, icons.Must("bot")).Size(size, size).TextColor(mcore.Tokens(c).TextMuted)
	case strings.HasPrefix(agent, "acp:"):
		cw.acpLeading(c, strings.TrimPrefix(agent, "acp:"), size)
	case strings.HasPrefix(agent, "agent:"):
		cw.agentMarkByName(c, strings.TrimPrefix(agent, "agent:"), size)
	default:
		mgo.Icon(c, icons.Must(chatAgentIcon)).Size(size, size).TextColor(mcore.Tokens(c).Accent)
	}
}

func roomMemberName(m config.RoomMember) string {
	if n := strings.TrimSpace(m.Name); n != "" {
		return n
	}
	return authorNameOf(m.Agent)
}

func roomSummary(r config.Room) string {
	parts := []string{fmt.Sprintf("%d 个成员", len(r.ActiveMembers()))}
	parts = append(parts, config.ActivationName(r.EffectiveActivation()))
	if ws := strings.TrimSpace(r.Workspace); ws != "" {
		parts = append(parts, ws)
	} else {
		parts = append(parts, "未绑定工作目录")
	}
	return strings.Join(parts, " · ")
}

// roomEditor is the create/edit dialog over the room list. The same form
// also lives in the chat view's settings panel, so the fields are shared.
func (cw *ChatWindow) roomEditor(c *mgo.Context) {
	if !cw.room.editorOpen {
		return
	}
	f := &cw.room.form
	title := "新建群聊"
	if f.editing >= 0 {
		title = "编辑群聊"
	}
	kit.Dialog(c, &cw.room.editorOpen, kit.DialogOptions{
		Title: title,
		Width: 640,
		Actions: func() {
			if f.editing >= 0 {
				if kit.Button(c, "删除", kit.ButtonOptions{Variant: kit.Danger}).Clicked() {
					cw.deleteRoom()
				}
			}
			if kit.Button(c, "取消", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
				cw.room.editorOpen = false
			}
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveRoom()
			}
		},
	}, func() {
		u := mcore.Density(c).Unit()
		mgo.Scroll(c).FillWidth().MaxHeight(520).Children(func() {
			mgo.Column(c).FillWidth().Gap(u*2).Padding(0, u/2).Children(func() {
				cw.roomFormFields(c, f)
			})
		})
	})
}

// roomFormFields is the room form itself: name, directory, members and the
// speaking rules, shared by the create dialog and the settings panel.
func (cw *ChatWindow) roomFormFields(c *mgo.Context, f *roomForm) {
	u := mcore.Density(c).Unit()
	kit.TextInput(c, &f.name, kit.TextInputOptions{Label: "群名称", Placeholder: "例如：方案评审"})
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.End).Children(func() {
		in := kit.TextInput(c, &f.workspace, kit.TextInputOptions{
			Label: "工作目录", Placeholder: "群内所有成员的文件与命令工具都在此目录下运行",
		})
		in.Element.Grow(1)
		if kit.Button(c, "选择", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("folder")}).Clicked() {
			cw.pickDirectory("选择群聊工作目录", func(path string) { f.workspace = path })
		}
	})
	mgo.Divider(c)
	cw.roomMemberFields(c, f)
	mgo.Divider(c)
	opts := make([]kit.SelectOption[string], 0, 4)
	for _, a := range config.RoomActivations() {
		opts = append(opts, kit.SelectOption[string]{Value: a, Label: config.ActivationName(a)})
	}
	kit.Select(c, &f.activation, opts, kit.SelectOptions{Label: "谁发言"})
	if f.activation == config.ActivationMaster {
		botMark := func() {
			mgo.Icon(c, icons.Must("bot")).Size(16, 16).Shrink(0).TextColor(mcore.Tokens(c).TextMuted)
		}
		mopts := []kit.SearchSelectOption[string]{{Value: "", Label: "当前模型（默认）", Leading: botMark}}
		for _, ref := range cw.allModelRefs() {
			mopts = append(mopts, kit.SearchSelectOption[string]{
				Value:   ref,
				Label:   cw.refLabel(ref),
				Leading: botMark,
			})
		}
		kit.SearchSelect(c, &f.master, mopts, kit.SearchSelectOptions[string]{
			Placeholder:       "主持模型（默认用当前模型）",
			SearchPlaceholder: "搜索模型",
			Label:             "主持模型",
		})
	}
	switchRow(c, &f.allowSelf, "允许连续发言",
		"关闭时，上一轮最后发言的成员本轮不再发言，避免一个成员自说自话")
}

// roomMemberFields lists the room's members and the agents that can be added.
func (cw *ChatWindow) roomMemberFields(c *mgo.Context, f *roomForm) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	mgo.Column(c).FillWidth().Gap(u * 2).Children(func() {
		if len(f.members) == 0 {
			mgo.Text(c, "还没有成员，从下面选 Agent 加入。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		}
		for i := range f.members {
			i := i
			m := f.members[i]
			mgo.Row(c).Key(fmt.Sprintf("member:%d", i)).FillWidth().Gap(u).AlignItems(mgo.Center).
				Padding(u).Radius(mtheme.ControlRadius).Background(k.Surface).Children(func() {
				cw.roomMemberAvatar(c, m.Agent, 20)
				mgo.Column(c).Grow(1).Children(func() {
					mgo.Text(c, roomMemberName(m)).FontWeight(600).SingleLine()
					mgo.Text(c, m.Agent).
						FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
				})
				if f.activation == config.ActivationNatural {
					talk := m.EffectiveTalkativeness()
					if kit.Select(c, &talk, roomTalkativenessOptions(), kit.SelectOptions{Label: "话痨度"}).Changed() {
						f.members[i].Talkativeness = talk
					}
				}
				muted := m.Muted
				if mgo.Switch(c, &muted).Changed() {
					f.members[i].Muted = muted
				}
				mgo.Text(c, "静音").
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).Shrink(0)
				if mcore.IconAction(c, icons.Must("x"), "移出群聊").Clicked() {
					f.members = append(f.members[:i], f.members[i+1:]...)
				}
			})
		}
		if avail := cw.roomCandidates(f); len(avail) > 0 {
			opts := make([]kit.SearchSelectOption[string], 0, len(avail))
			for _, agent := range avail {
				agent := agent
				opts = append(opts, kit.SearchSelectOption[string]{
					Value:   agent,
					Label:   authorNameOf(agent),
					Leading: func() { cw.roomMemberAvatar(c, agent, 18) },
				})
			}
			var pick string
			if kit.SearchSelect(c, &pick, opts, kit.SearchSelectOptions[string]{
				Placeholder:       "添加 Agent…",
				SearchPlaceholder: "搜索 Agent",
				Label:             "添加 Agent",
			}).Changed() && pick != "" {
				f.members = append(f.members, config.RoomMember{Agent: pick})
			}
		} else if len(cw.cfg.Agents) == 0 {
			mgo.Text(c, "还没有可用的 Agent，先到「Agent」页创建。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		} else {
			mgo.Text(c, "已加入全部可用 Agent。ACP Agent 暂不支持入群。").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		}
	})
}

// roomCandidates are the agent references not already in the room.
func (cw *ChatWindow) roomCandidates(f *roomForm) []string {
	taken := make(map[string]bool, len(f.members))
	for _, m := range f.members {
		taken[m.Agent] = true
	}
	// Only custom agents can join. An ACP agent is a coding CLI driven over
	// a session protocol, which taskAgentSpec cannot resolve: it would fall
	// through to the built-in chat and answer with a plain model instead of
	// the CLI the user picked.
	out := make([]string, 0, len(cw.cfg.Agents))
	for _, a := range cw.cfg.Agents {
		if ref := "agent:" + a.Name; !taken[ref] {
			out = append(out, ref)
		}
	}
	return out
}

// roomTalkativenessOptions are the speak-chance steps offered per member
// under the natural activation.
func roomTalkativenessOptions() []kit.SelectOption[int] {
	return []kit.SelectOption[int]{
		{Value: 10, Label: "很少 10%"},
		{Value: 25, Label: "偶尔 25%"},
		{Value: 50, Label: "一般 50%"},
		{Value: 75, Label: "常常 75%"},
		{Value: 100, Label: "总是 100%"},
	}
}
