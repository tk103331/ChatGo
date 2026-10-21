package dockerapp

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
)

// maxLogLines caps the in-memory log lines for one container.
const maxLogLines = 20000

// loadPage loads the active page's data in a background goroutine.
func (cl *Client) loadPage(p int) {
	cl.mu.Lock()
	if cl.st.loading {
		cl.mu.Unlock()
		return
	}
	cl.st.loading = true
	cl.st.err = ""
	cl.mu.Unlock()
	go func() {
		var err error
		switch p {
		case pageContainers:
			var rows []Container
			rows, err = listContainers()
			if err == nil {
				cl.mu.Lock()
				cl.st.containers = rows
				cl.mu.Unlock()
			}
		case pageImages:
			var rows []Image
			rows, err = listImages()
			if err == nil {
				cl.mu.Lock()
				cl.st.images = rows
				cl.mu.Unlock()
			}
		case pageVolumes:
			var rows []Volume
			rows, err = listVolumes()
			if err == nil {
				cl.mu.Lock()
				cl.st.volumes = rows
				cl.mu.Unlock()
			}
		case pageNetworks:
			var rows []Network
			rows, err = listNetworks()
			if err == nil {
				cl.mu.Lock()
				cl.st.networks = rows
				cl.mu.Unlock()
			}
		case pageSystem:
			info, ierr := systemInfoText()
			df, derr := diskUsageText()
			cl.mu.Lock()
			if ierr == nil {
				cl.st.sysInfo = info
			}
			if derr == nil {
				cl.st.diskUsage = df
			}
			cl.mu.Unlock()
			if ierr != nil {
				err = ierr
			} else if derr != nil {
				err = derr
			}
		}
		cl.mu.Lock()
		cl.st.loading = false
		cl.st.lastLoad = time.Now()
		if err != nil {
			cl.st.err = shortErr(err)
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// shortErr trims a Docker API error to one short line.
func shortErr(err error) string {
	msg := strings.TrimSpace(err.Error())
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return msg
}

// doAction runs a container action in the background and refreshes.
func (cl *Client) doAction(action, id string) {
	go func() {
		msg, err := containerAction(action, id)
		cl.host.Post(func() {
			if err != nil {
				cl.host.Toast(fmt.Sprintf("%s 失败: %s", actionName(action), shortErr(err)), mcore.SeverityError)
			} else {
				cl.host.Toast(msg, mcore.SeveritySuccess)
			}
		})
		cl.loadPage(pageContainers)
	}()
}

// actionName maps an action id to its Chinese label.
func actionName(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	case "pause":
		return "暂停"
	case "unpause":
		return "继续"
	case "remove", "rm":
		return "删除"
	}
	return action
}

// confirmAction asks the host to confirm before a destructive action.
func (cl *Client) confirmAction(title, desc, action, id string) {
	cl.host.Confirm(title, desc, func() { cl.runConfirmed(action, id) })
}

// runConfirmed executes a confirmed destructive action.
func (cl *Client) runConfirmed(action, id string) {
	go func() {
		var msg string
		var err error
		switch action {
		case "prune-system":
			msg, err = prune("system")
		case "rmi":
			msg, err = removeImage(id)
		case "volume-rm":
			msg, err = removeVolume(id)
		case "network-rm":
			msg, err = removeNetwork(id)
		case "remove":
			msg, err = containerAction("remove", id)
		default:
			err = fmt.Errorf("未知操作 %q", action)
		}
		cl.host.Post(func() {
			if err != nil {
				cl.host.Toast("操作失败: "+shortErr(err), mcore.SeverityError)
			} else {
				cl.host.Toast(msg, mcore.SeveritySuccess)
			}
		})
		cl.Refresh()
	}()
}

// tailN parses the tail line count from the toolbar input.
func (cl *Client) tailN() int {
	cl.mu.Lock()
	text := cl.st.logTailText
	cl.mu.Unlock()
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n > 0 {
		return n
	}
	return logTailDefault
}

// openLogs loads a container's recent logs into the logs tab.
func (cl *Client) openLogs(id string) {
	cl.mu.Lock()
	cl.st.detail = id
	cl.st.logLines = nil
	cl.st.logLoading = true
	cl.st.logState = mcode.LogViewerState{}
	cl.stopFollowLocked()
	cl.mu.Unlock()
	cl.host.Invalidate()
	n := cl.tailN()
	go func() {
		lines, err := fetchLogs(id, n)
		cl.mu.Lock()
		cl.st.logLoading = false
		if err != nil {
			cl.st.err = shortErr(err)
		} else {
			cl.st.logLines = toLogLines(lines)
			cl.st.err = ""
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	}()
}

// reloadLogs re-reads the current container's logs.
func (cl *Client) reloadLogs() {
	cl.mu.Lock()
	id := cl.st.detail
	cl.mu.Unlock()
	if id == "" {
		return
	}
	cl.openLogs(id)
}

// toggleLogFollow starts or stops following the current container's logs.
func (cl *Client) toggleLogFollow() {
	cl.mu.Lock()
	if cl.st.logFollow {
		cl.stopFollowLocked()
	} else {
		cl.startFollowLocked()
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// startFollowLocked begins streaming new log lines; caller holds cl.mu.
func (cl *Client) startFollowLocked() {
	id := cl.st.detail
	if id == "" {
		return
	}
	n := cl.tailN()
	stop, err := streamLogs(id, n, func(line string) {
		cl.mu.Lock()
		cl.st.logLines = append(cl.st.logLines, toLogLine(line))
		if len(cl.st.logLines) > maxLogLines {
			cl.st.logLines = cl.st.logLines[len(cl.st.logLines)-maxLogLines:]
		}
		cl.mu.Unlock()
		cl.host.Post(func() { cl.host.Invalidate() })
	})
	if err != nil {
		cl.st.err = shortErr(err)
		return
	}
	cl.st.logFollow = true
	cl.st.logStop = stop
}

// stopFollowLocked stops a running log stream; caller holds cl.mu.
func (cl *Client) stopFollowLocked() {
	if cl.st.logStop != nil {
		cl.st.logStop()
		cl.st.logStop = nil
	}
	cl.st.logFollow = false
}

// closeLogs stops any log stream and hides the detail pane.
func (cl *Client) closeLogs() {
	cl.mu.Lock()
	cl.stopFollowLocked()
	if cl.st.term != nil {
		cl.st.term.Close()
		cl.st.term = nil
	}
	cl.st.detail = ""
	cl.st.logLines = nil
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// toLogLines converts raw log lines to LogViewer entries.
func toLogLines(lines []string) []mcode.LogLine {
	out := make([]mcode.LogLine, 0, len(lines))
	for _, s := range lines {
		out = append(out, toLogLine(s))
	}
	return out
}

// toLogLine classifies one raw log line.
func toLogLine(s string) mcode.LogLine {
	return mcode.LogLine{Time: time.Now(), Level: parseLevel(s), Text: s}
}

// parseLevel guesses a log level from the line's text.
func parseLevel(s string) mcode.LogLevel {
	up := strings.ToUpper(s)
	switch {
	case strings.Contains(up, "FATAL"), strings.Contains(up, "PANIC"),
		strings.Contains(up, "ERROR"), strings.Contains(up, "ERR]"):
		return mcode.LogError
	case strings.Contains(up, "WARN"):
		return mcode.LogWarn
	case strings.Contains(up, "DEBUG"), strings.Contains(up, "TRACE"):
		return mcode.LogDebug
	}
	return mcode.LogInfo
}
