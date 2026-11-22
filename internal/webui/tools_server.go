package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// toolRunner returns the desktop runtime's file, Git and terminal surface, or
// answers that this server has none.
func (s *Server) toolRunner(w http.ResponseWriter) (ToolRunner, bool) {
	if s.runner == nil {
		writeError(w, http.StatusBadRequest,
			errors.New("此功能需要桌面版运行时，请从 ChatGo 设置 → Web 打开"))
		return nil, false
	}
	tr, ok := s.runner.(ToolRunner)
	if !ok {
		writeError(w, http.StatusBadRequest, errors.New("当前桌面版本不支持文件、Git 或终端"))
		return nil, false
	}
	return tr, true
}

// handleFiles lists the files and directories of a workspace.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	files, err := tr.Files(strings.TrimSpace(r.URL.Query().Get("dir")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// handleReadFile reads one text file of a workspace.
func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	file, err := tr.ReadFile(strings.TrimSpace(r.URL.Query().Get("path")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"file": file})
}

// handleGitStatus reports a workspace's branch and changes.
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	status, err := tr.GitStatus(strings.TrimSpace(r.URL.Query().Get("dir")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status})
}

// handleGitDiff returns a path's diff, or the working tree's.
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	diff, err := tr.GitDiff(strings.TrimSpace(r.URL.Query().Get("dir")), strings.TrimSpace(r.URL.Query().Get("path")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"diff": diff})
}

// handleGitLog lists recent commits of a workspace.
func (s *Server) handleGitLog(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	commits, err := tr.GitLog(strings.TrimSpace(r.URL.Query().Get("dir")))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commits": commits})
}

// handleGitStage stages one path, or every change when path is empty.
func (s *Server) handleGitStage(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	var body struct {
		Dir  string `json:"dir"`
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := tr.GitStage(strings.TrimSpace(body.Dir), strings.TrimSpace(body.Path)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleGitCommit commits the staged changes of a workspace.
func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	var body struct {
		Dir     string `json:"dir"`
		Message string `json:"message"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := tr.GitCommit(strings.TrimSpace(body.Dir), strings.TrimSpace(body.Message)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTerminal runs one shell command in a workspace. It is a command
// console, not an interactive PTY: the phone gets the output when it ends.
func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	var body struct {
		Dir     string `json:"dir"`
		Command string `json:"command"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
	defer cancel()
	result := tr.RunCommand(ctx, strings.TrimSpace(body.Dir), strings.TrimSpace(body.Command))
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

// handleCreateProject makes a project directory, initializes Git when asked,
// and registers it as a workspace the new-session and project pages see.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.toolRunner(w)
	if !ok {
		return
	}
	var body struct {
		Parent string `json:"parent"`
		Name   string `json:"name"`
		Git    bool   `json:"git"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	path, err := tr.CreateProject(strings.TrimSpace(body.Parent), strings.TrimSpace(body.Name), body.Git)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.Publish("workspace.created", "", "", map[string]any{"path": path})
	writeJSON(w, http.StatusCreated, map[string]any{"path": path})
}
