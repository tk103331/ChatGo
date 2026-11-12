package ui

import (
	"sort"
	"strings"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/webui"
)

// Tasks lists the scheduled tasks with their run counts.
func (r webRuntime) Tasks() []webui.TaskInfo {
	cw := r.cw
	return onMain(cw, func() []webui.TaskInfo {
		counts := taskSessionCounts(cw)
		out := make([]webui.TaskInfo, 0, len(cw.cfg.AutoTasks))
		for _, t := range cw.cfg.AutoTasks {
			info := taskInfoOf(t, counts[t.ID])
			// A desktop-scheduler run owns the same task id; showing it as
			// running keeps the browser from starting a second copy.
			info.Running = cw.autoRunning[t.ID]
			out = append(out, info)
		}
		return out
	})
}

func taskInfoOf(t config.AutoTask, sessions int) webui.TaskInfo {
	return webui.TaskInfo{
		ID: t.ID, Name: t.Name, Agent: t.Agent, Prompt: t.Prompt,
		Workspace: t.Workspace,
		Schedule: webui.ScheduleInfo{
			Kind: t.Schedule.Kind, Hour: t.Schedule.Hour, Minute: t.Schedule.Minute,
			Weekday: t.Schedule.Weekday, Day: t.Schedule.Day,
		},
		Reuse: t.Reuse, ConvID: t.ConvID, Enabled: t.Enabled,
		LastRun: t.LastRun, LastNote: t.LastNote, Sessions: sessions,
	}
}

func taskSessionCounts(cw *ChatWindow) map[string]int {
	counts := map[string]int{}
	all, err := cw.convManager.ListConversations()
	if err != nil {
		return counts
	}
	for _, conv := range all {
		if conv.TaskID != "" {
			counts[conv.TaskID]++
		}
	}
	return counts
}

func (r webRuntime) CreateTask(in webui.TaskInput) (webui.TaskInfo, error) {
	cw := r.cw
	type result struct {
		info webui.TaskInfo
		err  error
	}
	res := onMain(cw, func() result {
		task := config.AutoTask{
			ID: cw.newID(), Name: strings.TrimSpace(in.Name),
			Agent: in.Agent, Prompt: strings.TrimSpace(in.Prompt),
			Workspace: strings.TrimSpace(in.Workspace),
			Schedule:  scheduleFromInfo(in.Schedule),
			Reuse:     in.Reuse, Enabled: in.Enabled,
		}
		cw.cfg.AutoTasks = append(cw.cfg.AutoTasks, task)
		if err := config.SaveConfig(cw.cfg); err != nil {
			cw.cfg.AutoTasks = cw.cfg.AutoTasks[:len(cw.cfg.AutoTasks)-1]
			return result{err: err}
		}
		return result{info: taskInfoOf(task, 0)}
	})
	return res.info, res.err
}

func (r webRuntime) UpdateTask(id string, in webui.TaskInput) (webui.TaskInfo, error) {
	cw := r.cw
	type result struct {
		info webui.TaskInfo
		err  error
	}
	res := onMain(cw, func() result {
		for i := range cw.cfg.AutoTasks {
			if cw.cfg.AutoTasks[i].ID != id {
				continue
			}
			old := cw.cfg.AutoTasks[i]
			task := old
			task.Name = strings.TrimSpace(in.Name)
			task.Agent = in.Agent
			task.Prompt = strings.TrimSpace(in.Prompt)
			task.Workspace = strings.TrimSpace(in.Workspace)
			task.Schedule = scheduleFromInfo(in.Schedule)
			task.Reuse = in.Reuse
			task.Enabled = in.Enabled
			cw.cfg.AutoTasks[i] = task
			if err := config.SaveConfig(cw.cfg); err != nil {
				cw.cfg.AutoTasks[i] = old
				return result{err: err}
			}
			return result{info: taskInfoOf(task, taskSessionCounts(cw)[id])}
		}
		return result{err: errTaskMissing}
	})
	return res.info, res.err
}

var errTaskMissing = &taskMissingError{}

type taskMissingError struct{}

func (*taskMissingError) Error() string { return "任务不存在" }

func scheduleFromInfo(s webui.ScheduleInfo) config.Schedule {
	return config.Schedule{
		Kind: s.Kind, Hour: s.Hour, Minute: s.Minute,
		Weekday: s.Weekday, Day: s.Day,
	}
}

func (r webRuntime) DeleteTask(id string) error {
	cw := r.cw
	return onMain(cw, func() error {
		idx := -1
		for i := range cw.cfg.AutoTasks {
			if cw.cfg.AutoTasks[i].ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			return errTaskMissing
		}
		cw.cfg.AutoTasks = append(cw.cfg.AutoTasks[:idx], cw.cfg.AutoTasks[idx+1:]...)
		return config.SaveConfig(cw.cfg)
	})
}

func (r webRuntime) TaskSessions(id string) []webui.RoomSessionInfo {
	cw := r.cw
	return onMain(cw, func() []webui.RoomSessionInfo {
		all, err := cw.convManager.ListConversations()
		if err != nil {
			return nil
		}
		out := make([]webui.RoomSessionInfo, 0, 4)
		for _, conv := range all {
			if conv.TaskID != id {
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

func (r webRuntime) SetTaskConv(id, convID string) {
	cw := r.cw
	onMain(cw, func() struct{} {
		for i := range cw.cfg.AutoTasks {
			if cw.cfg.AutoTasks[i].ID == id {
				cw.cfg.AutoTasks[i].ConvID = convID
				_ = config.SaveConfig(cw.cfg)
				break
			}
		}
		return struct{}{}
	})
}

func (r webRuntime) StartTask(id string) bool {
	return onMain(r.cw, func() bool {
		if r.cw.autoRunning == nil {
			r.cw.autoRunning = map[string]bool{}
		}
		if r.cw.autoRunning[id] {
			return false
		}
		r.cw.autoRunning[id] = true
		return true
	})
}

func (r webRuntime) MarkTask(id string, ok bool, note string) {
	cw := r.cw
	onMain(cw, func() struct{} {
		delete(cw.autoRunning, id)
		for i := range cw.cfg.AutoTasks {
			if cw.cfg.AutoTasks[i].ID != id {
				continue
			}
			cw.cfg.AutoTasks[i].LastRun = time.Now()
			cw.cfg.AutoTasks[i].LastNote = note
			_ = config.SaveConfig(cw.cfg)
			break
		}
		return struct{}{}
	})
}
