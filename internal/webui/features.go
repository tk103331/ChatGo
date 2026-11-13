package webui

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

// handleProjects lists the projects and their worktrees a new session may
// run in, grouped the way the desktop's 新会话 page groups them.
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	var projects []ProjectInfo
	if s.runner != nil {
		projects = s.runner.Projects()
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

// handleModels lists the models an agent reference may run.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	var models []ModelInfo
	if s.runner != nil {
		models = s.runner.Models(r.URL.Query().Get("agent"))
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// handleACPOptions lists the session config options an ACP agent's runtime
// offers — the model, the modes, the toggles — with the values each one
// takes. The first request may take a while: it waits for the startup
// warm-up to land.
func (s *Server) handleACPOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireRunner(w) {
		return
	}
	agent := r.URL.Query().Get("agent")
	writeJSON(w, http.StatusOK, map[string]any{
		"options": s.runner.ACPOptions(agent),
	})
}

// handleCreateWorktree adds a working tree of a project and answers with its
// path, so the new session can open in it.
func (s *Server) handleCreateWorktree(w http.ResponseWriter, r *http.Request) {
	if !s.requireRunner(w) {
		return
	}
	var body struct {
		Project   string `json:"project"`
		Branch    string `json:"branch"`
		NewBranch bool   `json:"new_branch"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	path, err := s.runner.CreateWorktree(body.Project, body.Branch, body.NewBranch)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"path": path})
}

// FSItem is one subdirectory of a browsed host directory.
type FSItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// handleFS lists the subdirectories of a directory on this host, so a phone
// can pick a workspace without typing blind paths. An empty dir starts at the
// user's home. Only directories are listed and nothing is opened: the read is
// as much of the host as the rest of the web surface already touches.
func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	dir := strings.TrimSpace(r.URL.Query().Get("dir"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		dir = home
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("目录不存在：%s", dir))
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, fmt.Errorf("不是目录：%s", dir))
		return
	}
	files, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("无法读取目录：%s", err))
		return
	}
	entries := make([]FSItem, 0, len(files))
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		entries = append(entries, FSItem{Name: f.Name(), Path: filepath.Join(abs, f.Name())})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	parent := ""
	if abs != string(filepath.Separator) {
		parent = filepath.Dir(abs)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dir": abs, "parent": parent, "entries": entries,
	})
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
