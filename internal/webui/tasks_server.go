package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

func (s *Server) taskRunner() (TaskRunner, bool) {
	tr, ok := s.runner.(TaskRunner)
	return tr, ok
}

func (s *Server) requireTaskRunner(w http.ResponseWriter) (TaskRunner, bool) {
	tr, ok := s.taskRunner()
	if !ok {
		writeError(w, http.StatusBadRequest,
			errors.New("自动任务需要桌面版运行时，请从 ChatGo 设置 → Web 打开"))
		return nil, false
	}
	return tr, true
}

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.taskRunner()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []TaskInfo{}})
		return
	}
	tasks := tr.Tasks()
	for i := range tasks {
		id := tasks[i].ID
		tasks[i].Running = tasks[i].Running ||
			s.anyRun(func(rs *runState) bool { return rs.taskID == id })
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.requireTaskRunner(w)
	if !ok {
		return
	}
	var in TaskInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateTask(in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, err := tr.CreateTask(in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.requireTaskRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid task id"))
		return
	}
	if s.anyRun(func(rs *runState) bool { return rs.taskID == id }) {
		writeError(w, http.StatusConflict, errors.New("task is running"))
		return
	}
	var in TaskInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateTask(in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	task, err := tr.UpdateTask(id, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": task})
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.requireTaskRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid task id"))
		return
	}
	if s.anyRun(func(rs *runState) bool { return rs.taskID == id }) {
		writeError(w, http.StatusConflict, errors.New("task is running"))
		return
	}
	if err := tr.DeleteTask(id); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleTaskSessions(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.requireTaskRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid task id"))
		return
	}
	sessions := tr.TaskSessions(id)
	if sessions == nil {
		sessions = []RoomSessionInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// handleRunTask runs a task now, in the same way the scheduler would: append
// its prompt, let the ordinary runner answer, then record the outcome.
func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	tr, ok := s.requireTaskRunner(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("invalid task id"))
		return
	}
	if s.anyRun(func(rs *runState) bool { return rs.taskID == id }) {
		writeError(w, http.StatusConflict, errors.New("task is running"))
		return
	}
	var task TaskInfo
	found := false
	for _, t := range tr.Tasks() {
		if t.ID == id {
			if t.Running {
				writeError(w, http.StatusConflict, errors.New("task is running"))
				return
			}
			task, found = t, true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, errors.New("task not found"))
		return
	}
	conv, err := s.ensureTaskConv(task, tr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now()
	user := conv.NewHumanMessage(newID(), task.Prompt, now)
	reply := conv.NewAgentMessage(newID(), task.Agent, authorName(task.Agent), now)
	conv.Messages = append(conv.Messages, user, reply)
	turn := models.Turn{
		ID: newID(), UserMsgID: user.ID, ReplyMsgID: reply.ID,
		User: task.Prompt, StartedAt: now,
	}
	conv.Turns = append(conv.Turns, turn)
	conv.TaskID = task.ID
	conv.Agent = task.Agent
	if task.Workspace != "" {
		conv.Workspace = task.Workspace
	}
	conv.UpdatedAt = now
	target, err := s.turnTarget(task.Agent, "", "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req := RunRequest{Agent: task.Agent, Workspace: task.Workspace}
	if !tr.StartTask(task.ID) {
		writeError(w, http.StatusConflict, errors.New("task is running"))
		return
	}
	s.beginTaskRun(w, conv, len(conv.Messages)-1, len(conv.Turns)-1, target, req, task.ID)
}

// ensureTaskConv returns the task's conversation, reusing one when the task
// asks for it and creating a fresh run's conversation otherwise.
func (s *Server) ensureTaskConv(task TaskInfo, tr TaskRunner) (*models.Conversation, error) {
	if task.Reuse && task.ConvID != "" {
		if conv, err := s.cm.LoadConversation(task.ConvID); err == nil {
			conv.TaskID = task.ID
			conv.Agent = task.Agent
			if task.Workspace != "" {
				conv.Workspace = task.Workspace
			}
			return conv, nil
		}
	}
	conv, err := s.cm.CreateConversation(task.Name, "", "")
	if err != nil {
		return nil, err
	}
	conv.TaskID = task.ID
	conv.Agent = task.Agent
	conv.Workspace = task.Workspace
	if task.Reuse {
		tr.SetTaskConv(task.ID, conv.ID)
	}
	return conv, nil
}

func (s *Server) beginTaskRun(w http.ResponseWriter, conv *models.Conversation, replyIdx, turnIdx int, target config.Provider, req RunRequest, taskID string) {
	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{
		conv: conv, target: target, req: req, cancel: cancel,
		done: make(chan struct{}), kind: "task", taskID: taskID,
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
		if tr, ok := s.runner.(TaskRunner); ok {
			tr.MarkTask(taskID, false, "启动失败: "+err.Error())
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok": true, "session_id": conv.ID,
		"user_message_id":  conv.Turns[turnIdx].UserMsgID,
		"reply_message_id": conv.Turns[turnIdx].ReplyMsgID,
	})
	go s.run(ctx, rs, replyIdx, turnIdx)
}

func validateTask(in TaskInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("任务名称不能为空")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return errors.New("任务提示词不能为空")
	}
	switch in.Schedule.Kind {
	case "daily", "weekly", "monthly", "manual", "":
	default:
		return errors.New("不支持的调度类型：" + in.Schedule.Kind)
	}
	if in.Schedule.Hour < 0 || in.Schedule.Hour > 23 || in.Schedule.Minute < 0 || in.Schedule.Minute > 59 {
		return errors.New("调度时间超出范围")
	}
	if in.Schedule.Kind == "monthly" && (in.Schedule.Day < 1 || in.Schedule.Day > 31) {
		return errors.New("每月执行日超出范围")
	}
	return nil
}
