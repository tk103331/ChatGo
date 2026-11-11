package webui

import (
	"errors"
	"net/http"
	"strings"
)

// capabilities reports the features the embedded runtime offers. The standalone
// server has none, so the browser hides the controls it cannot use.
func (s *Server) capabilities() map[string]bool {
	if s.runner == nil {
		return map[string]bool{}
	}
	return s.runner.Capabilities()
}

func (s *Server) requireRunner(w http.ResponseWriter) bool {
	if s.runner == nil {
		writeError(w, http.StatusBadRequest,
			errors.New("此功能需要桌面版运行时，请从 ChatGo 设置 → Web 打开"))
		return false
	}
	return true
}

// handleAgents lists the assistants a web session may use.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	agents := []AgentInfo{{Ref: "", Name: "Chat", Kind: "chat"}}
	if s.runner != nil {
		agents = append(agents, s.runner.Agents()...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// handleTools lists built-in tools and MCP servers, with the defaults the
// requested agent starts from.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	var tools []ToolInfo
	if s.runner != nil {
		tools = s.runner.Tools(r.URL.Query().Get("agent"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

// handleWorkspaces lists the directories a new session may run in.
func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	var workspaces []string
	if s.runner != nil {
		workspaces = s.runner.Workspaces()
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

// handleSkills lists the slash-menu skills for an agent reference.
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	var skills []SkillInfo
	if s.runner != nil {
		skills = s.runner.Skills(r.URL.Query().Get("agent"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

// handleRevert rewinds the conversation workspace to the checkpoint of a
// reply, matching the desktop's "回退到此点" action.
func (s *Server) handleRevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mid := r.PathValue("messageID")
	if !validID(id) || mid == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid session or message id"))
		return
	}
	if !s.requireRunner(w) {
		return
	}
	if s.getRun(id) != nil {
		writeError(w, http.StatusConflict, errors.New("a reply is running"))
		return
	}
	conv, ok := s.loadSession(w, id)
	if !ok {
		return
	}
	if messageIndex(conv, mid) < 0 {
		writeError(w, http.StatusNotFound, errors.New("message not found"))
		return
	}
	if err := s.runner.Revert(r.Context(), conv, mid); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleInteraction answers the question or chooses the permission option a
// running turn waits on.
func (s *Server) handleInteraction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	var body struct {
		ID       string `json:"id"`
		Answer   string `json:"answer,omitempty"`
		OptionID string `json:"option_id,omitempty"`
		Canceled bool   `json:"canceled,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rs := s.getRun(id)
	if rs == nil {
		writeError(w, http.StatusNotFound, errors.New("no reply is running"))
		return
	}
	rs.mu.Lock()
	p := rs.pending
	rs.mu.Unlock()
	if p == nil {
		writeError(w, http.StatusConflict, errors.New("no pending interaction"))
		return
	}
	if body.ID != "" && body.ID != p.info.ID {
		writeError(w, http.StatusConflict, errors.New("interaction has changed"))
		return
	}
	if p.info.Kind == "ask" {
		answer := strings.TrimSpace(body.Answer)
		if body.Canceled {
			answer = "用户取消了提问"
		}
		if answer == "" {
			writeError(w, http.StatusBadRequest, errors.New("answer is empty"))
			return
		}
		if err := rs.answer(p.info.ID, answer); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
	} else {
		if err := rs.decide(p.info.ID, PermissionDecision{
			OptionID: body.OptionID, Canceled: body.Canceled,
		}); err != nil {
			writeError(w, http.StatusConflict, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
