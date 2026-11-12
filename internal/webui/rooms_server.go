package webui

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"chatgo/pkg/models"
)

func (s *Server) roomRunner() (RoomRunner, bool) {
	rr, ok := s.runner.(RoomRunner)
	return rr, ok
}

// anyRun reports whether any live run matches, used to mark rooms and tasks
// as running without a second registry.
func (s *Server) anyRun(match func(*runState) bool) bool {
	s.mu.Lock()
	runs := make([]*runState, 0, len(s.runs))
	for _, rs := range s.runs {
		runs = append(runs, rs)
	}
	s.mu.Unlock()
	for _, rs := range runs {
		if match(rs) {
			return true
		}
	}
	return false
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.roomRunner()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"rooms": []RoomInfo{}})
		return
	}
	rooms := rr.Rooms()
	for i := range rooms {
		id := rooms[i].ID
		rooms[i].Running = rooms[i].Running ||
			s.anyRun(func(rs *runState) bool { return rs.roomID == id })
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": rooms})
}

func (s *Server) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	var in RoomInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateRoom(in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	room, err := rr.CreateRoom(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"room": room})
}

func (s *Server) handleUpdateRoom(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid room id"))
		return
	}
	var in RoomInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateRoom(in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	room, err := rr.UpdateRoom(id, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"room": room})
}

func (s *Server) handleDeleteRoom(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid room id"))
		return
	}
	if err := rr.DeleteRoom(id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRoomSessions(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid room id"))
		return
	}
	sessions := rr.RoomSessions(id)
	if sessions == nil {
		sessions = []RoomSessionInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) handleCreateRoomSession(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid room id"))
		return
	}
	session, err := rr.CreateRoomSession(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

// handleRoomSend appends the user's message and runs one room turn. Every
// member appends its own reply through the runner, so the usual single-reply
// path does not apply.
func (s *Server) handleRoomSend(w http.ResponseWriter, r *http.Request) {
	rr, ok := s.requireRoomRunner(w)
	if !ok {
		return
	}
	roomID := r.PathValue("id")
	convID := r.PathValue("convID")
	if !validID(roomID) || !validID(convID) {
		writeError(w, http.StatusBadRequest, errors.New("invalid room or conversation id"))
		return
	}
	var body struct {
		Text        string          `json:"text"`
		QuoteID     string          `json:"quote_id,omitempty"`
		Attachments []attachmentRef `json:"attachments,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" && len(body.Attachments) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("message is empty"))
		return
	}
	conv, ok := s.loadSession(w, convID)
	if !ok {
		return
	}
	if conv.RoomID != roomID {
		writeError(w, http.StatusNotFound, errors.New("room conversation not found"))
		return
	}
	if s.getRun(convID) != nil {
		writeError(w, http.StatusConflict, errors.New("a reply is already running"))
		return
	}
	for _, room := range rr.Rooms() {
		if room.ID == roomID && room.Running {
			writeError(w, http.StatusConflict, errors.New("the room is already generating"))
			return
		}
	}
	text = embedAttachments(text, conv.ID, body.Attachments)
	now := time.Now()
	human := conv.NewHumanMessage(newID(), text, now)
	human.QuoteID = body.QuoteID
	conv.Messages = append(conv.Messages, human)
	conv.UpdatedAt = now
	s.beginRoomRun(w, rr, conv, roomID, text)
}

func (s *Server) beginRoomRun(w http.ResponseWriter, rr RoomRunner, conv *models.Conversation, roomID, text string) {
	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{
		conv: conv, cancel: cancel, done: make(chan struct{}),
		kind: "room", roomID: roomID,
	}
	s.mu.Lock()
	if s.runs[conv.ID] != nil {
		s.mu.Unlock()
		cancel()
		writeError(w, http.StatusConflict, errors.New("a reply is already running"))
		return
	}
	s.runs[conv.ID] = rs
	s.mu.Unlock()
	if err := s.cm.SaveConversation(conv); err != nil {
		s.mu.Lock()
		delete(s.runs, conv.ID)
		s.mu.Unlock()
		cancel()
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "session_id": conv.ID})
	go s.runRoom(ctx, rs, rr, roomID, text)
}

func (s *Server) runRoom(ctx context.Context, rs *runState, rr RoomRunner, roomID, text string) {
	defer func() {
		close(rs.done)
		s.mu.Lock()
		if s.runs[rs.conv.ID] == rs {
			delete(s.runs, rs.conv.ID)
		}
		s.mu.Unlock()
	}()
	err := rr.RunRoom(ctx, roomID, rs.conv, text, &liveTurn{rs: rs})
	rs.mu.Lock()
	if err != nil && !errors.Is(err, context.Canceled) {
		msg := rs.conv.NewAgentMessage(newID(), "system", "系统", time.Now())
		msg.AuthorKind = models.AuthorKindSystem
		msg.Content = "⚠️ " + err.Error()
		rs.conv.Messages = append(rs.conv.Messages, msg)
	}
	rs.conv.UpdatedAt = time.Now()
	saveErr := s.cm.SaveConversation(rs.conv)
	rs.mu.Unlock()
	if saveErr != nil {
		log.Printf("save room conversation %s: %v", rs.conv.ID, saveErr)
	}
}

func (s *Server) requireRoomRunner(w http.ResponseWriter) (RoomRunner, bool) {
	rr, ok := s.roomRunner()
	if !ok {
		writeError(w, http.StatusBadRequest,
			errors.New("群聊需要桌面版运行时，请从 ChatGo 设置 → Web 打开"))
		return nil, false
	}
	return rr, true
}

// validateRoom checks the fields a room cannot be created without.
func validateRoom(in RoomInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("群名称不能为空")
	}
	if len(in.Members) == 0 {
		return errors.New("群聊至少需要一个成员")
	}
	for _, m := range in.Members {
		if strings.TrimSpace(m.Agent) == "" {
			return errors.New("成员缺少 Agent")
		}
	}
	return nil
}
