package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/config"
	"chatgo/internal/ui/icons"
	"chatgo/internal/ui/kit"
	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// roomChatView is the conversation of one room: the roster across the top,
// the messages, and the composer. It is built on its own rather than reusing
// chatView, which carries a session list, a model picker and a tool bar that
// all belong to a single-agent conversation.
func (cw *ChatWindow) roomChatView(c *mgo.Context) {
	r, ok := cw.cfg.FindRoom(cw.room.active)
	if !ok {
		cw.leaveRoom()
		return
	}
	conv := cw.roomConv()
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Fill().AlignItems(mgo.Stretch).Background(k.Background).Children(func() {
		cw.roomHeader(c, r)
		if conv == nil {
			mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Children(func() {
				mgo.Text(c, "无法打开群聊会话").TextColor(k.TextMuted)
			})
			return
		}
		mgo.Row(c).Grow(1).MinHeight(0).AlignItems(mgo.Stretch).Children(func() {
			cw.roomSessionColumn(c, conv)
			mgo.Column(c).Grow(1).MinWidth(0).AlignItems(mgo.Stretch).Children(func() {
				msgs := conv.Messages
				res := kit.MessageList(c, &cw.room.list, len(msgs), kit.MessageListOptions{
					ID:    func(i int) string { return msgs[i].ID },
					Date:  func(i int) time.Time { return msgs[i].Timestamp },
					Label: func(i int) string { return msgs[i].Role },
				}, func(i int) { cw.roomMessageRow(c, msgs[i]) })
				res.Element.Grow(1).MinHeight(0)
				mgo.Column(c).Shrink(0).Padding(u*3, u*4).BorderWidth(mtheme.BorderWidth, 0, 0, 0).
					BorderColor(k.Border).Children(func() { cw.roomComposer(c, r) })
			})
			switch {
			case cw.room.settingsOpen:
				cw.roomSettingsPanel(c)
			case cw.room.detail:
				cw.roomDetailPanel(c, r, conv)
			}
		})
	})
}

// roomSessionColumn is the left-hand session list of the active room: one
// row per conversation, newest first, and a button starting a fresh one.
func (cw *ChatWindow) roomSessionColumn(c *mgo.Context, conv *models.Conversation) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(220).FillHeight().Shrink(0).AlignItems(mgo.Stretch).
		BorderWidth(0, mtheme.BorderWidth, 0, 0).BorderColor(k.Border).
		Background(k.Surface).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Padding(u*2, u*2).Children(func() {
			mgo.Text(c, "会话").FontWeight(600).Grow(1).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			if mcore.IconAction(c, icons.Must("plus"), "新建会话").Clicked() {
				cw.newRoomSession()
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().Gap(u/2).Padding(0, u).Children(func() {
				for _, s := range cw.room.sessions {
					cw.roomSessionRow(c, s, conv != nil && conv.ID == s.ID)
				}
			})
		})
	})
}

// roomSessionRow is one conversation in the session column.
func (cw *ChatWindow) roomSessionRow(c *mgo.Context, s roomSession, current bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).Key("session:"+s.ID).FillWidth().
		Padding(u, u*2).Radius(mtheme.ControlRadius).Label(s.Label)
	if current {
		b.Background(k.Selection)
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	b.Children(func() {
		mgo.Column(c).Grow(1).MinWidth(0).Children(func() {
			mgo.Text(c, s.Label).SingleLine().Ellipsis("…").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			mgo.Text(c, s.Updated.Format("01-02 15:04")).SingleLine().
				FontSize(mcore.FontSize(c, mtheme.CaptionSize-1)).TextColor(k.TextMuted)
		})
	})
	if b.Clicked() {
		cw.switchRoomSession(s.ID)
	}
}

// roomSettingsPanel edits the room in a right-hand panel, with the same form
// the create dialog uses.
func (cw *ChatWindow) roomSettingsPanel(c *mgo.Context) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	f := &cw.room.form
	mgo.Column(c).Width(420).FillHeight().Shrink(0).AlignItems(mgo.Stretch).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).
		Background(k.Surface).Children(func() {
		mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Padding(u*2, u*3).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			mgo.Text(c, "群聊设置").FontWeight(600).Grow(1)
			if kit.Button(c, "保存", kit.ButtonOptions{Variant: kit.Primary}).Clicked() {
				cw.saveRoom()
				cw.room.settingsOpen = false
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭设置").Clicked() {
				cw.room.settingsOpen = false
			}
		})
		mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			mgo.Column(c).FillWidth().Gap(u * 2).Padding(u * 3).Children(func() {
				cw.roomFormFields(c, f)
			})
		})
	})
}

// roomHeader names the room, shows the directory it is bound to, and draws
// the roster with the queue order of the turn being generated.
func (cw *ChatWindow) roomHeader(c *mgo.Context, r config.Room) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).FillWidth().Shrink(0).Gap(u).Padding(u*2, u*4).
		Background(k.Surface).BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
		mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).Children(func() {
			if kit.Button(c, "返回", kit.ButtonOptions{Variant: kit.Ghost, Icon: icons.Must("chevron-left")}).Clicked() {
				cw.leaveRoom()
			}
			mgo.Column(c).Grow(1).Children(func() {
				mgo.Text(c, r.Name).FontWeight(600).SingleLine()
				mgo.Text(c, roomSummary(r)).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted).SingleLine()
			})
			if kit.Button(c, "轨迹", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("eye")}).Clicked() {
				cw.room.detail = !cw.room.detail
				cw.room.settingsOpen = false
			}
			if kit.Button(c, "设置", kit.ButtonOptions{Variant: kit.Secondary, Icon: icons.Must("sliders-horizontal")}).Clicked() {
				if cw.room.settingsOpen {
					cw.room.settingsOpen = false
				} else {
					cw.loadRoomFormByID(r.ID)
					cw.room.editorOpen = false
					cw.room.settingsOpen = true
					cw.room.detail = false
				}
			}
		})
		mgo.Row(c).Gap(u).Wrap().AlignItems(mgo.Center).Children(func() {
			for _, m := range r.Members {
				cw.roomRosterChip(c, m)
			}
		})
	})
}

// roomRosterChip is one member in the header, badged with its place in the
// running turn's queue so the user can see who is still to speak. Without it
// a serial turn looks stalled while the later members wait.
func (cw *ChatWindow) roomRosterChip(c *mgo.Context, m config.RoomMember) {
	u := mcore.Density(c).Unit()
	k := mcore.Tokens(c)
	speaking := cw.room.speaking == m.Agent
	queued := cw.roomQueuePos(m.Agent)
	passed := cw.room.passed[m.Agent]
	row := mgo.Row(c).Gap(u).AlignItems(mgo.Center).Padding(u/2, u, u/2, u).Radius(mtheme.ControlRadius)
	switch {
	case speaking:
		row.Background(k.Selection)
	case m.Muted:
		row.Background(k.Surface)
	default:
		row.Background(k.SurfaceHover)
	}
	row.Children(func() {
		cw.roomMemberAvatar(c, m.Agent, 18)
		label := roomMemberName(m)
		text := mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).SingleLine()
		if m.Muted {
			text.TextColor(k.TextMuted)
		}
		switch {
		case speaking:
			mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
		case queued > 0:
			mgo.Text(c, fmt.Sprintf("%d", queued)).
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		case passed:
			// A member that chose not to speak this turn: without a mark
			// the user cannot tell it apart from one that never ran.
			mgo.Text(c, "·").TextColor(k.TextMuted)
		}
	})
}

// roomQueuePos is a member's 1-based place in the queue, or 0 when it is not
// waiting.
func (cw *ChatWindow) roomQueuePos(agent string) int {
	for i, a := range cw.room.queue {
		if a == agent {
			return i + 1
		}
	}
	return 0
}

// roomMessageRow draws one message. The main list stays compact so a room
// with several verbose members remains scannable: a finished reply shows only
// its final text, and a reply being generated is one live status line with
// stop and detail actions. The full process lives in the detail panel.
// roomMessageRow draws one message as a flat row with no bubble frame,
// because a room is read as a meeting log. Members sit on the left and the
// human on the right, which keeps who-said-what readable without frames.
// The main list stays compact: a finished reply shows only its final text,
// a generating one is a live status line. Actions sit by the time stamp.
func (cw *ChatWindow) roomMessageRow(c *mgo.Context, msg models.Message) {
	role := roleOf(msg)
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	streaming := cw.streamingMsg(msg.ID)
	user := role == kit.MessageUser
	name := msg.AuthorName
	if user {
		name = "我"
	}
	avatar := func() {
		mgo.Box(c).Shrink(0).Children(func() {
			if user {
				cw.roomUserMark(c, 24)
			} else {
				cw.roomMemberAvatar(c, msg.AuthorID, 24)
			}
		})
	}
	row := mgo.Row(c).FillWidth().Gap(u*2).AlignItems(mgo.Start).
		Padding(u, u*2).Role(mgo.RoleGroup).Label(name)
	if user {
		row.Justify(mgo.End)
	}
	row.Children(func() {
		if !user {
			avatar()
		}
		col := mgo.Column(c).MinWidth(0).Gap(u)
		if user {
			col.AlignItems(mgo.End).MaxWidth(720).Shrink(1)
		} else {
			col.Grow(1)
		}
		col.Children(func() {
			mgo.Row(c).Gap(u * 2).AlignItems(mgo.Center).Children(func() {
				mgo.Text(c, name).FontWeight(600).SingleLine().
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
				mgo.Text(c, msg.Timestamp.Format("15:04")).SingleLine().
					FontFeatures("tnum").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			})
			cw.roomQuotePreview(c, msg.QuoteID)
			if streaming {
				cw.roomStreamingLine(c, msg)
			} else {
				cw.roomFinalText(c, msg)
			}
			// The actions row always occupies its height, so the message
			// does not jump when the icons appear on hover.
			cw.roomMessageActions(c, msg, streaming, row.Hovered())
		})
		if user {
			avatar()
		}
	})
}

// roomMessageActions is the actions row of one message: it always stands at
// the height of an action button so the message never jumps, and draws the
// icons only while the row is hovered (or while generating, so the stop
// control is always reachable).
func (cw *ChatWindow) roomMessageActions(c *mgo.Context, msg models.Message, streaming, hovered bool) {
	role := roleOf(msg)
	u := mcore.Density(c).Unit()
	show := streaming || hovered
	mgo.Row(c).Gap(u * 2).FillWidth().AlignItems(mgo.Center).
		MinHeight(mcore.Density(c).ControlHeight()).Children(func() {
		if show && streaming {
			cw.roomMiniAction(c, "square", "停止生成", func() { cw.stopRoomTurn() })
		}
		if show && !streaming && role != kit.MessageSystem {
			cw.roomMiniAction(c, "reply", "引用回复", func() { cw.room.quoteID = msg.ID })
		}
		if show && role == kit.MessageAssistant {
			cw.roomMiniAction(c, "eye", "查看当轮详情", func() {
				cw.room.detail = true
				cw.room.detailTurnID = msg.ID
				cw.room.detailAgent = msg.AuthorID
			})
		}
		if show && !streaming {
			cw.roomMiniAction(c, "copy", "复制消息", func() {
				c.WriteClipboard(roomQuoteSummary(msg))
			})
		}
	})
}

// roomMiniAction is a small icon action, smaller than the standard hit size
// so a row of them stays tidy.
func (cw *ChatWindow) roomMiniAction(c *mgo.Context, iconName, label string, fn func()) {
	k := mcore.Tokens(c)
	b := mgo.ButtonBase(c).Size(20, 20).Radius(mtheme.ControlRadius).Label(label)
	if b.Hovered() {
		b.Background(k.SurfaceHover).TextColor(k.Text)
	}
	b.Children(func() {
		mgo.Icon(c, icons.Must(iconName)).Size(14, 14).TextColor(k.TextMuted)
	})
	if b.Clicked() {
		fn()
	}
}

// roomUserMark is the human's round mark in the flat message rows, where
// every speaker needs a face for the eye to anchor on.
func (cw *ChatWindow) roomUserMark(c *mgo.Context, size float32) {
	k := mcore.Tokens(c)
	mgo.Row(c).Size(size, size).Shrink(0).Radius(size / 2).
		Background(k.Selection).AlignItems(mgo.Center).Justify(mgo.Center).Children(func() {
		mgo.Icon(c, icons.Must("users")).Size(size*0.6, size*0.6).TextColor(k.Accent)
	})
}

// roomStreamingLine is the one-line live status of a generating reply: what
// the member is doing now. Its actions sit by the time stamp above.
func (cw *ChatWindow) roomStreamingLine(c *mgo.Context, msg models.Message) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Gap(u * 2).AlignItems(mgo.Center).Children(func() {
		mcore.Spinner(c, mcore.SpinnerOptions{Size: 12})
		mgo.Text(c, roomStreamStatus(msg)).
			Grow(1).MinWidth(0).SingleLine().Ellipsis("…").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	})
}

// roomStreamStatus is the one-line live status of a generating reply: its
// newest visible activity, prefixed by what kind of work it is. Text is
// shown as itself; thinking and tool calls are labelled.
func roomStreamStatus(msg models.Message) string {
	for i := len(msg.Segments) - 1; i >= 0; i-- {
		seg := msg.Segments[i]
		switch seg.Kind {
		case "tool":
			if seg.ToolCall != nil {
				return "工具执行：" + seg.ToolCall.Name
			}
		case "text":
			if t := strings.TrimSpace(seg.Text); t != "" {
				return roomLastLine(t)
			}
		case "reasoning":
			if t := strings.TrimSpace(seg.Text); t != "" {
				return "思考中：" + roomLastLine(t)
			}
		}
	}
	if t := strings.TrimSpace(msg.Content); t != "" {
		return roomLastLine(t)
	}
	if t := strings.TrimSpace(msg.Reasoning); t != "" {
		return "思考中：" + roomLastLine(t)
	}
	return "生成中…"
}

// roomLastLine is the newest non-blank line of a streaming text, which reads
// as "what it is saying right now".
func roomLastLine(t string) string {
	lines := strings.Split(t, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" {
			return s
		}
	}
	return t
}

// roomFinalText shows a finished reply as its final text only. The reply of
// a multi-step turn is its last text segment; everything before it is the
// journey, kept for the detail panel.
func (cw *ChatWindow) roomFinalText(c *mgo.Context, msg models.Message) {
	k := mcore.Tokens(c)
	if t := roomFinalTextOf(msg); t != "" {
		kit.MarkdownView(c, t)
	} else {
		mgo.Text(c, "（无文字回复）").TextColor(k.TextMuted).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
	}
}

// roomFinalTextOf is the last text segment of a message, or its plain
// content for one that predates segments.
func roomFinalTextOf(msg models.Message) string {
	for i := len(msg.Segments) - 1; i >= 0; i-- {
		if msg.Segments[i].Kind == "text" {
			if t := strings.TrimSpace(msg.Segments[i].Text); t != "" {
				return t
			}
		}
	}
	return strings.TrimSpace(msg.Content)
}

// roomDetailPanel is the right-hand view of one turn, in two parts: the
// execution graph of the members on top, and below it every member's full
// process - thinking, tool calls and text. Clicking a node narrows the lower
// half to that member; clicking it again lifts the filter.
func (cw *ChatWindow) roomDetailPanel(c *mgo.Context, r config.Room, conv *models.Conversation) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Column(c).Width(420).FillHeight().Shrink(0).AlignItems(mgo.Stretch).
		BorderWidth(0, 0, 0, mtheme.BorderWidth).BorderColor(k.Border).
		Background(k.Surface).Children(func() {
		start, end := roomTurnBounds(conv, cw.room.detailTurnID)
		mgo.Row(c).FillWidth().Shrink(0).AlignItems(mgo.Center).Padding(u*2, u*3).
			BorderWidth(0, 0, mtheme.BorderWidth, 0).BorderColor(k.Border).Children(func() {
			title := "本轮详情"
			if end < len(conv.Messages) {
				title = "历史轮详情"
			}
			mgo.Text(c, title).FontWeight(600).Grow(1)
			if end < len(conv.Messages) {
				if kit.Button(c, "回到最新", kit.ButtonOptions{Variant: kit.Ghost}).Clicked() {
					cw.room.detailTurnID = ""
					cw.room.detailAgent = ""
				}
			}
			if mcore.IconAction(c, icons.Must("x"), "关闭详情").Clicked() {
				cw.room.detail = false
			}
		})
		nodes := cw.roomGraphNodes(r, conv, start, end)
		if len(nodes) == 0 {
			mgo.Column(c).Grow(1).AlignItems(mgo.Center).Justify(mgo.Center).Children(func() {
				mgo.Text(c, "本轮还没有成员发言").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			})
			return
		}
		cw.roomTurnGraph(c, conv, nodes, start, cw.room.detailAgent)
		mgo.Box(c).FillWidth().Height(mtheme.BorderWidth).Shrink(0).Background(k.Border)
		cw.roomDetailReplies(c, conv, start, end)
	})
}

// roomDetailReplies is the lower half: the full process of every member of
// the turn in speaking order, or of the picked member alone.
func (cw *ChatWindow) roomDetailReplies(c *mgo.Context, conv *models.Conversation, start, end int) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	picked := cw.room.detailAgent
	shown := 0
	mgo.Scroll(c).Grow(1).MinHeight(0).Children(func() {
		mgo.Column(c).FillWidth().Gap(u * 3).Padding(u * 3).Children(func() {
			for _, msg := range conv.Messages[start:end] {
				if msg.AuthorKind != models.AuthorKindAgent || msg.AuthorID == "" {
					continue
				}
				if picked != "" && msg.AuthorID != picked {
					continue
				}
				shown++
				mgo.Column(c).Key("detail:" + msg.ID).FillWidth().Gap(u).Children(func() {
					cw.roomDetailReply(c, msg)
				})
			}
			if shown == 0 {
				mgo.Text(c, "该成员本轮没有回复").TextColor(k.TextMuted).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize))
			}
		})
	})
}

// roomTurnGraph is the upper half of the detail panel: the turn's prompt,
// then one clickable node per member, joined by a rail in speaking order.
// Clicking the prompt lifts the member filter, the way the graph's root
// covers the whole turn.
func (cw *ChatWindow) roomTurnGraph(c *mgo.Context, conv *models.Conversation, nodes []roomGraphNode, start int, pickedAgent string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Scroll(c).FillWidth().Shrink(0).MaxHeight(260).Children(func() {
		mgo.Column(c).FillWidth().Padding(u*2, u*3).Children(func() {
			if prompt := roomTurnPrompt(conv, start); prompt != "" {
				b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Center).
					Padding(u, u/2).Radius(mtheme.ControlRadius).Label("本轮提问 " + prompt)
				if pickedAgent == "" {
					b.Background(k.Selection)
				} else if b.Hovered() {
					b.Background(k.SurfaceHover)
				}
				b.Children(func() {
					mgo.Icon(c, icons.Must("message-square")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
					mgo.Text(c, prompt).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
						FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
				})
				if b.Clicked() {
					cw.room.detailAgent = ""
				}
			}
			for i, n := range nodes {
				cw.roomGraphRow(c, n, i == len(nodes)-1, n.Agent == pickedAgent)
			}
		})
	})
}

// roomGraphRow is one node: the rail segment, the state dot, the member and
// its state. Clicking it pins the lower half to this member's reply.
func (cw *ChatWindow) roomGraphRow(c *mgo.Context, n roomGraphNode, last, picked bool) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Stretch).
		Radius(mtheme.ControlRadius).Label(n.Name + " " + roomNodeWord(n.State))
	if picked {
		b.Background(k.Selection)
	} else if b.Hovered() {
		b.Background(k.SurfaceHover)
	}
	dot := roomNodeColor(k, n.State)
	b.Children(func() {
		rail := mgo.Box(c).Width(u * 3).FillHeight().Shrink(0)
		rail.Draw(func(p *mgo.Painter, rect mgo.Rect) {
			x := rect.X + rect.W/2
			mid := rect.Y + rect.H/2
			p.Line(x, rect.Y, x, mid-5, 1.5, k.Border)
			if !last {
				p.Line(x, mid+5, x, rect.Y+rect.H, 1.5, k.Border)
			}
			p.Fill(mgo.Rect{X: x - 4, Y: mid - 4, W: 8, H: 8}, dot, 4)
		})
		mgo.Row(c).Grow(1).MinWidth(0).Gap(u).AlignItems(mgo.Center).
			Padding(u, 0).Children(func() {
			cw.roomMemberAvatar(c, n.Agent, 20)
			mgo.Text(c, n.Name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
				FontSize(mcore.FontSize(c, mtheme.CaptionSize)).FontWeight(600)
			if n.State == roomNodeSpeaking {
				mcore.Spinner(c, mcore.SpinnerOptions{Size: 10})
			}
			if word := roomNodeWord(n.State); word != "" {
				mgo.Text(c, word).Shrink(0).
					FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
			}
		})
	})
	if b.Clicked() {
		if cw.room.detailAgent == n.Agent {
			cw.room.detailAgent = ""
		} else {
			cw.room.detailAgent = n.Agent
		}
	}
}

func roomNodeColor(k *mgo.Theme, state string) mgo.Color {
	switch state {
	case roomNodeSpeaking:
		return k.Accent
	case roomNodeDone:
		return k.Success
	case roomNodePassed, roomNodeIdle:
		return k.TextMuted
	default:
		return k.Border
	}
}

// roomDetailReply is the picked member's reply rendered in full, the way the
// single-agent chat shows a message.
func (cw *ChatWindow) roomDetailReply(c *mgo.Context, msg models.Message) {
	streaming := cw.streamingMsg(msg.ID)
	kit.MessageHeader(c, kit.MessageHeaderOptions{Model: msg.AuthorName, Time: msg.Timestamp})
	if len(msg.Segments) > 0 {
		cw.messageSegments(c, msg, streaming)
	} else {
		cw.messageLegacy(c, msg, streaming)
	}
}

func (cw *ChatWindow) roomComposer(c *mgo.Context, r config.Room) {
	running := cw.room.running == r.ID
	cw.roomQuoteBar(c)
	in := composer(c, &cw.room.draft, composerOptions{
		Placeholder: "向群里所有成员提问，@成员名 点名回答，Enter 发送",
		Actions: func() {
			res := kit.SendButton(c, kit.SendButtonOptions{
				Streaming: running,
				Disabled:  !running && strings.TrimSpace(cw.room.draft) == "",
			})
			if res.Sent() {
				cw.sendRoomDraft(r)
			}
			if res.Stopped() {
				cw.stopRoomTurn()
			}
		},
	})
	mentionActive := cw.roomMentionPopover(c, in, r)
	if in.Submitted() && !running && !mentionActive {
		cw.sendRoomDraft(r)
	}
}

// roomMentionPopover floats the members matching the @-prefix being typed
// over the composer; a click or Enter completes the mention, the arrow keys
// move the highlight while focus stays in the draft, Escape abandons the
// mention. The open state is the draft's: it appears while an @ is being
// typed and goes away the moment the completion or any other edit ends it.
func (cw *ChatWindow) roomMentionPopover(c *mgo.Context, in kit.InputResult, r config.Room) bool {
	prefix, start, active := roomMentionQuery(cw.room.draft)
	matches := make([]config.RoomMember, 0, 4)
	if active {
		for _, m := range r.ActiveMembers() {
			name := roomMemberName(m)
			if prefix == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
				matches = append(matches, m)
			}
		}
	}
	open := mgo.Local(c.Root(), "room:mention:open", func() bool { return false })
	highlight := mgo.Local(c.Root(), "room:mention:highlight", func() int { return 0 })
	wasOpen := *open
	*open = active && len(matches) > 0
	if !*open {
		return false
	}
	if !wasOpen || *highlight >= len(matches) {
		*highlight = 0
	}
	complete := func(m config.RoomMember) {
		cw.room.draft = cw.room.draft[:start] + "@" + roomMemberName(m) + " "
		*open = false
	}
	switch {
	case in.Input.Shortcut(0, mgo.KeyDown):
		*highlight = (*highlight + 1) % len(matches)
	case in.Input.Shortcut(0, mgo.KeyUp):
		*highlight = (*highlight + len(matches) - 1) % len(matches)
	case in.Input.Shortcut(0, mgo.KeyTab):
		complete(matches[*highlight])
	case in.Input.Shortcut(0, mgo.KeyEscape):
		cw.room.draft = cw.room.draft[:start]
		*open = false
	case in.Submitted():
		complete(matches[*highlight])
	}
	if !*open {
		return true
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	kit.Popover(c, in.Element, open, kit.PopoverOptions{
		Label: "选择要 @ 的成员", Width: 180, MaxHeight: u * 60, Up: true,
	}, func() {
		mgo.Column(c).FillWidth().Gap(u / 2).Children(func() {
			for i, m := range matches {
				m := m
				name := roomMemberName(m)
				b := mgo.ButtonBase(c).FillWidth().Gap(u).AlignItems(mgo.Center).
					Padding(u, u*2).Radius(mtheme.ControlRadius).Label("@" + name)
				if i == *highlight {
					b.Background(k.Selection)
				} else if b.Hovered() {
					b.Background(k.SurfaceHover)
				}
				b.Children(func() {
					cw.roomMemberAvatar(c, m.Agent, 18)
					mgo.Text(c, name).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				})
				if b.Clicked() {
					complete(m)
				}
			}
		})
	})
	return true
}

// roomMentionQuery reads an @-mention being typed at the end of the draft:
// the prefix after the trailing @, the byte offset of that @, and whether a
// mention is being typed at all. The @ must start a word, and the prefix
// stops at whitespace, so "a@b" or a completed "@名字 " offers nothing.
func roomMentionQuery(draft string) (prefix string, start int, active bool) {
	at := strings.LastIndex(draft, "@")
	if at < 0 {
		return "", 0, false
	}
	if at > 0 {
		before, _ := utf8.DecodeLastRuneInString(draft[:at])
		if !unicode.IsSpace(before) {
			return "", 0, false
		}
	}
	rest := draft[at+1:]
	if strings.ContainsFunc(rest, unicode.IsSpace) {
		return "", 0, false
	}
	return rest, at, true
}

// roomQuotePreview shows what a message replies to, above its own body, so
// a thread reads without hunting for the quoted message.
func (cw *ChatWindow) roomQuotePreview(c *mgo.Context, quoteID string) {
	if quoteID == "" {
		return
	}
	conv := cw.roomConv()
	if conv == nil {
		return
	}
	quoted, ok := roomFindMessage(conv, quoteID)
	if !ok {
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Gap(u).AlignItems(mgo.Center).
		Padding(u/2, u*2).Radius(mtheme.ControlRadius).
		Background(k.SurfaceHover).BorderWidth(0, 0, 0, u/2).BorderColor(k.Accent).Children(func() {
		mgo.Text(c, roomSpeakerName(quoted)).Shrink(0).FontWeight(600).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		mgo.Text(c, roomQuoteSummary(quoted)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
	})
}

// roomQuoteSummary is a quoted message in one line.
func roomQuoteSummary(msg models.Message) string {
	for _, line := range strings.Split(roomFinalTextOf(msg), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return "（无文字内容）"
}

// roomFindMessage looks a message up by id.
func roomFindMessage(conv *models.Conversation, id string) (models.Message, bool) {
	for i := range conv.Messages {
		if conv.Messages[i].ID == id {
			return conv.Messages[i], true
		}
	}
	return models.Message{}, false
}

// roomQuoteBar shows the message the draft replies to, above the composer,
// with a way to drop it.
func (cw *ChatWindow) roomQuoteBar(c *mgo.Context) {
	if cw.room.quoteID == "" {
		return
	}
	conv := cw.roomConv()
	if conv == nil {
		return
	}
	quoted, ok := roomFindMessage(conv, cw.room.quoteID)
	if !ok {
		cw.room.quoteID = ""
		return
	}
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Row(c).FillWidth().Shrink(0).Gap(u).AlignItems(mgo.Center).
		Margin(0, 0, u, 0).Padding(u, u*2).Radius(mtheme.ControlRadius).
		Background(k.SurfaceHover).BorderWidth(0, 0, 0, u/2).BorderColor(k.Accent).Children(func() {
		mgo.Icon(c, icons.Must("message-square")).Size(14, 14).TextColor(k.TextMuted).Shrink(0)
		mgo.Text(c, "回复 "+roomSpeakerName(quoted)).Shrink(0).FontWeight(600).
			FontSize(mcore.FontSize(c, mtheme.CaptionSize))
		mgo.Text(c, roomQuoteSummary(quoted)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").
			FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.TextMuted)
		if mcore.IconAction(c, icons.Must("x"), "取消引用").Clicked() {
			cw.room.quoteID = ""
		}
	})
}

// roomQuoteContext is the one-line core of a quoted message, enough for a
// member to know what a reply is answering without re-reading the thread. It
// falls back to the reasoning's opening line when there is no plain text.
func roomQuoteContext(content, reasoning string) (string, bool) {
	if s := roomQuoteSummary(models.Message{Content: content}); s != "（无文字内容）" {
		return s, true
	}
	for _, line := range strings.Split(reasoning, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t, true
		}
	}
	return "", false
}
