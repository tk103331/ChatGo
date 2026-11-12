package webui

import "time"

// ScheduleInfo is a task's schedule, mirroring config.Schedule.
type ScheduleInfo struct {
	Kind    string `json:"kind"`
	Hour    int    `json:"hour"`
	Minute  int    `json:"minute"`
	Weekday int    `json:"weekday,omitempty"`
	Day     int    `json:"day,omitempty"`
}

// TaskInfo is one scheduled task as the browser sees it.
type TaskInfo struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Agent     string       `json:"agent,omitempty"`
	Prompt    string       `json:"prompt"`
	Workspace string       `json:"workspace,omitempty"`
	Schedule  ScheduleInfo `json:"schedule"`
	Reuse     bool         `json:"reuse,omitempty"`
	ConvID    string       `json:"conversation_id,omitempty"`
	Enabled   bool         `json:"enabled"`
	Running   bool         `json:"running,omitempty"`
	LastRun   time.Time    `json:"last_run,omitempty"`
	LastNote  string       `json:"last_note,omitempty"`
	Sessions  int          `json:"sessions"`
}

// TaskInput is a task's editable fields.
type TaskInput struct {
	Name      string       `json:"name"`
	Agent     string       `json:"agent,omitempty"`
	Prompt    string       `json:"prompt"`
	Workspace string       `json:"workspace,omitempty"`
	Schedule  ScheduleInfo `json:"schedule"`
	Reuse     bool         `json:"reuse,omitempty"`
	Enabled   bool         `json:"enabled"`
}

// TaskRunner is implemented by the desktop runtime. The web server runs a
// task through the ordinary Runner.Run, then reports the outcome here.
type TaskRunner interface {
	Tasks() []TaskInfo
	CreateTask(in TaskInput) (TaskInfo, error)
	UpdateTask(id string, in TaskInput) (TaskInfo, error)
	DeleteTask(id string) error
	TaskSessions(id string) []RoomSessionInfo
	SetTaskConv(id, convID string)
	// StartTask claims a task for a web run; false means another run owns it.
	StartTask(id string) bool
	MarkTask(id string, ok bool, note string)
}
