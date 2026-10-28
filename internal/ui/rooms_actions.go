package ui

import (
	"errors"
	"sort"
	"strings"

	"chatgo/internal/config"
	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/models"
)

// errRoomMissing reports a room id with no room behind it, which happens when
// a room is deleted while its page is open.
var errRoomMissing = errors.New("群聊不存在")

func (cw *ChatWindow) newRoomForm() {
	cw.room.form = roomForm{editing: -1, activation: config.ActivationAll}
	cw.room.editorOpen = true
}

func (cw *ChatWindow) loadRoomForm(i int) {
	if i < 0 || i >= len(cw.cfg.Rooms) {
		return
	}
	r := cw.cfg.Rooms[i]
	cw.room.form = roomForm{
		editing:    i,
		name:       r.Name,
		workspace:  r.Workspace,
		activation: r.EffectiveActivation(),
		master:     r.Master,
		allowSelf:  r.AllowSelfResponses,
		members:    append([]config.RoomMember(nil), r.Members...),
	}
	cw.room.editorOpen = true
}

// loadRoomFormByID opens the editor for a room by its id, for the header's
// settings button, which knows the room rather than its index.
func (cw *ChatWindow) loadRoomFormByID(id string) {
	for i := range cw.cfg.Rooms {
		if cw.cfg.Rooms[i].ID == id {
			cw.loadRoomForm(i)
			return
		}
	}
}

// saveRoom writes the form back to configuration, creating the room when the
// form was opened for a new one.
func (cw *ChatWindow) saveRoom() {
	f := &cw.room.form
	name := strings.TrimSpace(f.name)
	if name == "" {
		cw.toast("请填写群名称", mcore.SeverityWarning)
		return
	}
	if len(f.members) == 0 {
		cw.toast("请至少加入一个 Agent", mcore.SeverityWarning)
		return
	}
	r := config.Room{
		Name:               name,
		Workspace:          strings.TrimSpace(f.workspace),
		Activation:         f.activation,
		Master:             f.master,
		AllowSelfResponses: f.allowSelf,
		Members:            append([]config.RoomMember(nil), f.members...),
	}
	if f.editing >= 0 && f.editing < len(cw.cfg.Rooms) {
		old := cw.cfg.Rooms[f.editing]
		// The id and the conversation outlive an edit, so renaming a room
		// or changing its roster keeps the history it already has.
		r.ID, r.ConvID = old.ID, old.ConvID
		cw.cfg.Rooms[f.editing] = r
	} else {
		r.ID = cw.newID()
		cw.cfg.Rooms = append(cw.cfg.Rooms, r)
	}
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.toast("保存失败: "+err.Error(), mcore.SeverityError)
		return
	}
	// The open conversation carries a copy of the directory, so an edit that
	// repoints the room has to update it too, or the stale path would be
	// written back on the next save.
	if cw.room.conv != nil && cw.room.conv.RoomID == r.ID {
		cw.room.conv.Workspace = r.Workspace
	}
	cw.room.editorOpen = false
	cw.invalidate()
}

func (cw *ChatWindow) deleteRoom() {
	f := &cw.room.form
	if f.editing < 0 || f.editing >= len(cw.cfg.Rooms) {
		cw.room.editorOpen = false
		return
	}
	r := cw.cfg.Rooms[f.editing]
	cw.cfg.Rooms = append(cw.cfg.Rooms[:f.editing], cw.cfg.Rooms[f.editing+1:]...)
	if err := config.SaveConfig(cw.cfg); err != nil {
		cw.toast("删除失败: "+err.Error(), mcore.SeverityError)
		return
	}
	if all, err := cw.convManager.ListConversations(); err == nil {
		for _, conv := range all {
			if conv.RoomID == r.ID {
				_ = cw.convManager.DeleteConversation(conv.ID)
			}
		}
	}
	if r.ConvID != "" {
		_ = cw.convManager.DeleteConversation(r.ConvID)
	}
	if cw.room.active == r.ID {
		cw.leaveRoom()
	}
	cw.room.editorOpen = false
	cw.invalidate()
}

// openRoom makes a room current, entering its newest session and creating
// the first one for a room never talked in.
func (cw *ChatWindow) openRoom(id string) {
	cw.reloadRoomSessions(id)
	var conv *models.Conversation
	var err error
	if len(cw.room.sessions) > 0 {
		conv, err = cw.loadRoomSession(id, cw.room.sessions[0].ID)
	} else {
		conv, err = cw.newRoomConv(id)
	}
	if err != nil {
		cw.toast("打开群聊失败: "+err.Error(), mcore.SeverityError)
		return
	}
	cw.room.active = id
	cw.room.conv = conv
	cw.reloadRoomSessions(id)
	cw.invalidate()
}

// newRoomSession starts a fresh conversation in the active room and switches
// to it, so every member begins the new session with a clean context.
func (cw *ChatWindow) newRoomSession() {
	if cw.room.active == "" {
		return
	}
	conv, err := cw.newRoomConv(cw.room.active)
	if err != nil {
		cw.toast("新建会话失败: "+err.Error(), mcore.SeverityError)
		return
	}
	cw.room.conv = conv
	cw.room.detailAgent = ""
	cw.room.detailTurnID = ""
	cw.reloadRoomSessions(cw.room.active)
	cw.invalidate()
}

// switchRoomSession makes another of the room's conversations current. The
// conversation a running turn writes into is handed back as the same object,
// so its streamed chunks stay visible.
func (cw *ChatWindow) switchRoomSession(convID string) {
	if cw.room.conv != nil && cw.room.conv.ID == convID {
		return
	}
	if tc := cw.room.turnConv; tc != nil && tc.ID == convID {
		cw.room.conv = tc
	} else {
		conv, err := cw.loadRoomSession(cw.room.active, convID)
		if err != nil {
			cw.toast("切换会话失败: "+err.Error(), mcore.SeverityError)
			return
		}
		cw.room.conv = conv
	}
	cw.room.detailAgent = ""
	cw.room.detailTurnID = ""
	cw.invalidate()
}

// reloadRoomSessions refreshes the session column: the room's conversations,
// newest first. The legacy single-conversation pointer is folded in by its
// RoomID, which ensureRoomConv has stamped since the rooms existed.
func (cw *ChatWindow) reloadRoomSessions(roomID string) {
	all, err := cw.convManager.ListConversations()
	if err != nil {
		cw.room.sessions = nil
		return
	}
	sessions := make([]roomSession, 0, 4)
	for _, conv := range all {
		if conv.RoomID != roomID {
			continue
		}
		sessions = append(sessions, roomSession{
			ID:      conv.ID,
			Label:   roomSessionLabel(conv),
			Updated: conv.UpdatedAt,
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].Updated.After(sessions[j].Updated) })
	cw.room.sessions = sessions
}

// roomSessionLabel names a session by its first question, or its birth time
// while it has none.
func roomSessionLabel(conv models.Conversation) string {
	for _, msg := range conv.Messages {
		if !msg.IsHuman() {
			continue
		}
		for _, line := range strings.Split(msg.Content, "\n") {
			if s := strings.TrimSpace(line); s != "" {
				return s
			}
		}
	}
	return conv.CreatedAt.Format("01-02 15:04")
}

// loadRoomSession reads one of the room's conversations, refusing another
// room's id so a stale click cannot graft a foreign history into this room.
func (cw *ChatWindow) loadRoomSession(roomID, convID string) (*models.Conversation, error) {
	conv, err := cw.convManager.LoadConversation(convID)
	if err != nil {
		return nil, err
	}
	if conv.RoomID != roomID {
		return nil, errRoomMissing
	}
	r, ok := cw.cfg.FindRoom(roomID)
	if ok {
		conv.Workspace = r.Workspace
	}
	return conv, nil
}

// newRoomConv creates a fresh conversation bound to the room.
func (cw *ChatWindow) newRoomConv(roomID string) (*models.Conversation, error) {
	r, ok := cw.cfg.FindRoom(roomID)
	if !ok {
		return nil, errRoomMissing
	}
	conv, err := cw.convManager.CreateConversation(r.Name, "", "")
	if err != nil {
		return nil, err
	}
	conv.RoomID = roomID
	conv.Workspace = r.Workspace
	if err := cw.convManager.SaveConversation(conv); err != nil {
		return nil, err
	}
	return conv, nil
}

// leaveRoom returns to the room list, releasing the conversation. A turn
// still running keeps its own reference, so leaving never truncates a reply.
func (cw *ChatWindow) leaveRoom() {
	cw.room.active = ""
	cw.room.conv = nil
	cw.room.sessions = nil
}

// roomConv is the active room's conversation, opened once and then returned
// as the same object. Rendering calls this every frame, so it must neither
// re-read the file nor hand back a fresh copy.
func (cw *ChatWindow) roomConv() *models.Conversation {
	if cw.room.active == "" {
		return nil
	}
	if cw.room.conv != nil && cw.room.conv.RoomID == cw.room.active {
		return cw.room.conv
	}
	conv, err := cw.newRoomConv(cw.room.active)
	if err != nil {
		return nil
	}
	cw.room.conv = conv
	cw.reloadRoomSessions(cw.room.active)
	return conv
}
