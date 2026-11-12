package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	acp "github.com/coder/acp-go-sdk"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/webui"
	"chatgo/pkg/models"
)

// Rooms lists the configured group chats with their session counts.
func (r webRuntime) Rooms() []webui.RoomInfo {
	cw := r.cw
	return onMain(cw, func() []webui.RoomInfo {
		counts, updated := roomSessionStats(cw)
		out := make([]webui.RoomInfo, 0, len(cw.cfg.Rooms))
		for _, room := range cw.cfg.Rooms {
			info := roomInfoOf(room)
			info.Sessions = counts[room.ID]
			info.Updated = updated[room.ID]
			info.Running = cw.room.running == room.ID
			out = append(out, info)
		}
		return out
	})
}

func roomInfoOf(room config.Room) webui.RoomInfo {
	info := webui.RoomInfo{
		ID: room.ID, Name: room.Name, Workspace: room.Workspace,
		Activation: room.EffectiveActivation(), Master: room.Master,
		AllowSelfResponses: room.AllowSelfResponses,
	}
	for _, m := range room.Members {
		info.Members = append(info.Members, webui.RoomMemberInfo{
			Agent: m.Agent, Name: roomMemberName(m), Muted: m.Muted,
			Talkativeness: m.Talkativeness,
		})
	}
	if info.Members == nil {
		info.Members = []webui.RoomMemberInfo{}
	}
	return info
}

// roomSessionStats counts each room's conversations and their newest update.
func roomSessionStats(cw *ChatWindow) (map[string]int, map[string]time.Time) {
	counts := map[string]int{}
	updated := map[string]time.Time{}
	all, err := cw.convManager.ListConversations()
	if err != nil {
		return counts, updated
	}
	for _, conv := range all {
		if conv.RoomID == "" {
			continue
		}
		counts[conv.RoomID]++
		if conv.UpdatedAt.After(updated[conv.RoomID]) {
			updated[conv.RoomID] = conv.UpdatedAt
		}
	}
	return counts, updated
}

func (r webRuntime) CreateRoom(in webui.RoomInput) (webui.RoomInfo, error) {
	cw := r.cw
	type result struct {
		info webui.RoomInfo
		err  error
	}
	res := onMain(cw, func() result {
		room := roomFromInput(cw.newID(), in)
		cw.cfg.Rooms = append(cw.cfg.Rooms, room)
		if err := config.SaveConfig(cw.cfg); err != nil {
			cw.cfg.Rooms = cw.cfg.Rooms[:len(cw.cfg.Rooms)-1]
			return result{err: err}
		}
		return result{info: roomInfoOf(room)}
	})
	return res.info, res.err
}

func (r webRuntime) UpdateRoom(id string, in webui.RoomInput) (webui.RoomInfo, error) {
	cw := r.cw
	type result struct {
		info webui.RoomInfo
		err  error
	}
	res := onMain(cw, func() result {
		for i := range cw.cfg.Rooms {
			if cw.cfg.Rooms[i].ID != id {
				continue
			}
			old := cw.cfg.Rooms[i]
			room := roomFromInput(id, in)
			// The id, conversation and knowledge base outlive an edit, so
			// renaming or changing the roster keeps the existing history.
			room.ConvID, room.KBID = old.ConvID, old.KBID
			cw.cfg.Rooms[i] = room
			if err := config.SaveConfig(cw.cfg); err != nil {
				cw.cfg.Rooms[i] = old
				return result{err: err}
			}
			return result{info: roomInfoOf(room)}
		}
		return result{err: errRoomMissing}
	})
	return res.info, res.err
}

func roomFromInput(id string, in webui.RoomInput) config.Room {
	room := config.Room{
		ID: id, Name: strings.TrimSpace(in.Name),
		Workspace:  strings.TrimSpace(in.Workspace),
		Activation: in.Activation, Master: strings.TrimSpace(in.Master),
		AllowSelfResponses: in.AllowSelfResponses,
	}
	for _, m := range in.Members {
		agent := strings.TrimSpace(m.Agent)
		if agent == "" {
			continue
		}
		room.Members = append(room.Members, config.RoomMember{
			Agent: agent, Name: strings.TrimSpace(m.Name),
			Muted: m.Muted, Talkativeness: m.Talkativeness,
		})
	}
	return room
}

func (r webRuntime) DeleteRoom(id string) error {
	cw := r.cw
	return onMain(cw, func() error {
		idx := -1
		for i := range cw.cfg.Rooms {
			if cw.cfg.Rooms[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return errRoomMissing
		}
		cw.cfg.Rooms = append(cw.cfg.Rooms[:idx], cw.cfg.Rooms[idx+1:]...)
		if err := config.SaveConfig(cw.cfg); err != nil {
			return err
		}
		if cw.room.active == id {
			cw.leaveRoom()
		}
		if all, err := cw.convManager.ListConversations(); err == nil {
			for _, conv := range all {
				if conv.RoomID == id {
					_ = cw.convManager.DeleteConversation(conv.ID)
				}
			}
		}
		return nil
	})
}

func (r webRuntime) RoomSessions(id string) []webui.RoomSessionInfo {
	cw := r.cw
	return onMain(cw, func() []webui.RoomSessionInfo {
		all, err := cw.convManager.ListConversations()
		if err != nil {
			return nil
		}
		out := make([]webui.RoomSessionInfo, 0, 4)
		for _, conv := range all {
			if conv.RoomID != id {
				continue
			}
			out = append(out, webui.RoomSessionInfo{
				ID: conv.ID, Label: roomSessionLabel(conv), Updated: conv.UpdatedAt,
			})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
		return out
	})
}

func (r webRuntime) CreateRoomSession(id string) (webui.RoomSessionInfo, error) {
	cw := r.cw
	type result struct {
		info webui.RoomSessionInfo
		err  error
	}
	res := onMain(cw, func() result {
		room, ok := cw.cfg.FindRoom(id)
		if !ok {
			return result{err: errRoomMissing}
		}
		conv, err := cw.convManager.CreateConversation(room.Name, "", "")
		if err != nil {
			return result{err: err}
		}
		conv.RoomID = id
		conv.Workspace = room.Workspace
		if err := cw.convManager.SaveConversation(conv); err != nil {
			return result{err: err}
		}
		return result{info: webui.RoomSessionInfo{
			ID: conv.ID, Label: roomSessionLabel(*conv), Updated: conv.UpdatedAt,
		}}
	})
	return res.info, res.err
}

// RunRoom runs one room turn: the selected members answer in order, each
// seeing the replies already posted.
func (r webRuntime) RunRoom(ctx context.Context, roomID string, conv *models.Conversation, text string, live webui.Live) error {
	cw := r.cw
	type setup struct {
		room   config.Room
		ok     bool
		master masterChatFunc
	}
	st := onMain(cw, func() setup {
		room, ok := cw.cfg.FindRoom(roomID)
		if !ok {
			return setup{}
		}
		room.KBID = cw.ensureRoomKBID(room.ID)
		s := setup{room: room, ok: true}
		if room.EffectiveActivation() == config.ActivationMaster {
			s.master = cw.masterClient(room)
		}
		return s
	})
	if !st.ok {
		return errRoomMissing
	}
	room := st.room
	// Mark the room busy on the window too, so a send from the desktop cannot
	// start a competing turn while the browser's own run is streaming.
	onMain(cw, func() struct{} {
		cw.room.running = room.ID
		return struct{}{}
	})
	defer func() {
		onMain(cw, func() struct{} {
			if cw.room.running == room.ID {
				cw.room.running = ""
			}
			return struct{}{}
		})
	}()

	// Archive the message that opened the turn, as the desktop does.
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if conv.Messages[i].IsHuman() {
			human := conv.Messages[i]
			go cw.roomArchiveMessage(room.KBID, human)
			break
		}
	}

	members := cw.roomSpeakers(room, conv, text)
	if len(members) == 0 {
		return nil
	}
	if room.EffectiveActivation() == config.ActivationMaster {
		var self string
		members, self = cw.webMasterNarrow(ctx, room, conv, members, st.master, live)
		if self != "" {
			return nil
		}
	}
	spoken := map[string]bool{}
	pending := append([]config.RoomMember(nil), members...)
	for len(pending) > 0 {
		if ctx.Err() != nil {
			break
		}
		m := pending[0]
		pending = pending[1:]
		if spoken[m.Agent] {
			continue
		}
		spoken[m.Agent] = true
		handoff, _ := cw.webRunRoomMember(ctx, room, conv, m, live)
		for _, agent := range handoff {
			if spoken[agent] || agent == m.Agent {
				continue
			}
			for _, member := range room.ActiveMembers() {
				if member.Agent == agent {
					pending = append(pending, member)
					break
				}
			}
		}
	}
	return ctx.Err()
}

// webMasterNarrow asks the moderator who speaks, headlessly: the placeholder
// carries its assignment or its own answer, and a pick of everyone withdraws
// the placeholder.
func (cw *ChatWindow) webMasterNarrow(ctx context.Context, room config.Room, conv *models.Conversation, members []config.RoomMember, chat masterChatFunc, live webui.Live) ([]config.RoomMember, string) {
	replyID := cw.newID()
	live.Update(func(c *models.Conversation) {
		msg := c.NewAgentMessage(replyID, masterAuthorID, "主持人", time.Now())
		c.Messages = append(c.Messages, msg)
	})
	history := ""
	if q := roomLastHumanText(conv); q != "" {
		history = cw.roomHistorySearch(room.KBID, q, 6)
	}
	picked, selfReply := masterPick(ctx, chat, conv, members, history)
	live.Update(func(c *models.Conversation) {
		i := roomIndexOf(c, replyID)
		if i < 0 {
			return
		}
		switch {
		case selfReply != "":
			c.Messages[i].Content = selfReply
			appendSegmentText(&c.Messages[i], "text", selfReply)
		case len(picked) != len(members):
			text := masterAssignText(picked)
			c.Messages[i].Content = text
			appendSegmentText(&c.Messages[i], "text", text)
		default:
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
		}
	})
	if selfReply != "" {
		return nil, selfReply
	}
	return picked, ""
}

// webRunRoomMember generates one member's reply into the live conversation.
func (cw *ChatWindow) webRunRoomMember(ctx context.Context, room config.Room, conv *models.Conversation, m config.RoomMember, live webui.Live) (handoff []string, pass bool) {
	if strings.HasPrefix(m.Agent, "acp:") {
		return cw.webRunRoomACPMember(ctx, room, conv, m, live)
	}
	name := roomMemberName(m)
	var (
		handoffMu   sync.Mutex
		toolHandoff []string
	)
	sendTool := cw.webRoomSendTool(room, m, live, &handoffMu, &toolHandoff)
	env := &builtinToolEnv{
		ask: func(actx context.Context, items []*askQuestionItem) (string, error) {
			qs := make([]webui.AskItem, 0, len(items))
			for _, it := range items {
				qs = append(qs, webui.AskItem{Question: it.question, Options: it.options})
			}
			return live.Ask(actx, qs)
		},
	}
	type built struct {
		client     *llm.Client
		react      *llm.ReactClient
		group      *llm.GroupClient
		groupReact *llm.GroupReactClient
		err        error
	}
	b := onMain(cw, func() built {
		client, react, group, groupReact, err := cw.taskClientWithEnvModel(
			m.Agent, room.Workspace, "", env, conv,
			cw.roomHistoryTool(room.KBID), sendTool)
		return built{client: client, react: react, group: group, groupReact: groupReact, err: err}
	})
	if b.err != nil {
		cw.webRoomAppendError(conv, m, name, b.err, live)
		return nil, false
	}
	msgs := onMain(cw, func() []llm.ChatMessage { return cw.buildRoomMessages(conv, m, name) })
	replyID := cw.newID()
	live.Update(func(c *models.Conversation) {
		reply := c.NewAgentMessage(replyID, m.Agent, name, time.Now())
		reply.QuoteID = roomTurnPromptID(c)
		c.Messages = append(c.Messages, reply)
	})
	onChunk := func(chunk string) {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				c.Messages[i].Content += chunk
				appendSegmentText(&c.Messages[i], "text", chunk)
			}
		})
	}
	onReasoning := func(chunk string) {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				c.Messages[i].Reasoning += chunk
				appendSegmentText(&c.Messages[i], "reasoning", chunk)
			}
		})
	}
	onTool := func(info llm.ToolCallInfo) {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				applyToolCall(&c.Messages[i], info)
				appendSegmentTool(&c.Messages[i], info)
			}
		})
	}
	var (
		resp *llm.ChatResponse
		err  error
	)
	switch {
	case b.groupReact != nil:
		resp, err = b.groupReact.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	case b.react != nil:
		resp, err = b.react.Chat(ctx, msgs, onChunk, onTool, onReasoning)
	case b.group != nil:
		resp, err = b.group.Chat(ctx, msgs, onChunk, onReasoning)
	default:
		resp, err = b.client.Chat(ctx, msgs, onChunk, onReasoning)
	}
	handoffMu.Lock()
	handoff = append(handoff, toolHandoff...)
	handoffMu.Unlock()
	live.Update(func(c *models.Conversation) {
		i := roomIndexOf(c, replyID)
		if i < 0 {
			return
		}
		msg := &c.Messages[i]
		if err != nil && msg.Content == "" {
			msg.Content = "⚠️ " + err.Error()
		} else if resp != nil && msg.Content == "" && resp.Content != "" {
			msg.Content = resp.Content
			appendSegmentText(msg, "text", resp.Content)
		}
		if resp != nil && msg.Reasoning == "" {
			msg.Reasoning = resp.Reasoning
		}
		if roomIsPass(msg.Content) {
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			pass = true
			return
		}
		if !strings.HasPrefix(msg.Content, "⚠️") {
			for _, h := range roomAtMentioned(room.ActiveMembers(), msg.Content) {
				if h.Agent != m.Agent {
					handoff = append(handoff, h.Agent)
				}
			}
			archived := *msg
			go cw.roomArchiveMessage(room.KBID, archived)
		}
	})
	return handoff, pass
}

// webRunRoomACPMember runs one ACP member with its own session slot, writing
// its updates into the live room conversation.
func (cw *ChatWindow) webRunRoomACPMember(ctx context.Context, room config.Room, conv *models.Conversation, m config.RoomMember, live webui.Live) (handoff []string, pass bool) {
	name := roomMemberName(m)
	conn, ok := cw.findACPConn(strings.TrimPrefix(m.Agent, "acp:"))
	if !ok {
		cw.webRoomAppendError(conv, m, name, errRoomMissing, live)
		return nil, false
	}
	key := roomACPSessionKey(conv.ID, m.Agent)
	replyID := cw.newID()
	live.Update(func(c *models.Conversation) {
		reply := c.NewAgentMessage(replyID, m.Agent, name, time.Now())
		reply.QuoteID = roomTurnPromptID(c)
		c.Messages = append(c.Messages, reply)
	})
	cw.acp.mu.Lock()
	cw.acp.convObj[key] = conv
	cw.acp.webSink[key] = func(fn func(*models.Conversation)) { live.Update(fn) }
	cw.acp.webPerm[key] = func(pctx context.Context, p acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		opts := make([]webui.PermissionOption, 0, len(p.Options))
		for _, o := range p.Options {
			opts = append(opts, webui.PermissionOption{
				ID: string(o.OptionId), Name: o.Name, Kind: string(o.Kind),
			})
		}
		d, err := live.Permission(pctx, webui.PermissionRequest{Title: acpPermissionTitle(&p), Options: opts})
		if err != nil || d.Canceled || d.OptionID == "" {
			return acpCancelled(), nil
		}
		return acp.RequestPermissionResponse{Outcome: acp.RequestPermissionOutcome{Selected: &acp.RequestPermissionOutcomeSelected{OptionId: acp.PermissionOptionId(d.OptionID)}}}, nil
	}
	cw.acp.mu.Unlock()
	defer func() {
		cw.acp.mu.Lock()
		delete(cw.acp.convObj, key)
		delete(cw.acp.webSink, key)
		delete(cw.acp.webPerm, key)
		cw.acp.mu.Unlock()
	}()
	rt, err := cw.acpEnsureSessionAt(conv, conn, key)
	if err != nil {
		live.Update(func(c *models.Conversation) {
			if i := roomIndexOf(c, replyID); i >= 0 {
				c.Messages[i].Content = "⚠️ " + err.Error()
			}
		})
		return nil, false
	}
	prompt := cw.roomACPText(conv, m)
	_, perr := rt.conn.Prompt(ctx, acp.PromptRequest{
		SessionId: rt.sessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock(prompt)},
	})
	cw.acp.mu.Lock()
	delete(cw.acp.greeting, rt.sessionID)
	cw.acp.mu.Unlock()
	onMain(cw, func() struct{} { return struct{}{} })
	live.Update(func(c *models.Conversation) {
		i := roomIndexOf(c, replyID)
		if i < 0 {
			return
		}
		msg := &c.Messages[i]
		if perr != nil && msg.Content == "" {
			msg.Content = "⚠️ " + perr.Error()
		}
		if roomIsPass(msg.Content) {
			c.Messages = append(c.Messages[:i], c.Messages[i+1:]...)
			pass = true
			return
		}
		if !strings.HasPrefix(msg.Content, "⚠️") {
			for _, h := range roomAtMentioned(room.ActiveMembers(), msg.Content) {
				if h.Agent != m.Agent {
					handoff = append(handoff, h.Agent)
				}
			}
			archived := *msg
			go cw.roomArchiveMessage(room.KBID, archived)
		}
	})
	return handoff, pass
}

func (cw *ChatWindow) webRoomAppendError(conv *models.Conversation, m config.RoomMember, name string, err error, live webui.Live) {
	live.Update(func(c *models.Conversation) {
		msg := c.NewAgentMessage(cw.newID(), m.Agent, name, time.Now())
		msg.Content = "⚠️ " + err.Error()
		c.Messages = append(c.Messages, msg)
	})
}

// webRoomSendTool is the headless room_send: it posts a message into the live
// conversation and records the members it hands off to.
func (cw *ChatWindow) webRoomSendTool(room config.Room, who config.RoomMember, live webui.Live, mu *sync.Mutex, handoff *[]string) tool.BaseTool {
	return llm.ObserveTool(newBuiltinToolWrapper(llm.ToolDefinition{
		Name:        roomSendToolID,
		Description: roomSendToolDesc,
		Parameters: map[string]*schema.ParameterInfo{
			"content": {Type: schema.String, Desc: "消息内容，可包含 @成员名 交接任务，可附文档内容", Required: true},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var in struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			body := strings.TrimSpace(in.Content)
			if body == "" {
				return "", fmt.Errorf("缺少必填参数 content")
			}
			mentioned := roomAtMentioned(room.ActiveMembers(), body)
			var msg models.Message
			live.Update(func(c *models.Conversation) {
				msg = c.NewAgentMessage(cw.newID(), who.Agent, roomMemberName(who), time.Now())
				msg.Content = body
				c.Messages = append(c.Messages, msg)
			})
			mu.Lock()
			for _, m := range mentioned {
				if m.Agent != who.Agent {
					*handoff = append(*handoff, m.Agent)
				}
			}
			mu.Unlock()
			if msg.Content != "" {
				go cw.roomArchiveMessage(room.KBID, msg)
			}
			if len(mentioned) > 0 {
				names := make([]string, 0, len(mentioned))
				for _, m := range mentioned {
					names = append(names, roomMemberName(m))
				}
				return "消息已发送，已交接给：" + strings.Join(names, "、"), nil
			}
			return "消息已发送", nil
		},
	}))
}
