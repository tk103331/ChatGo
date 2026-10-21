package logapp

import (
	"io"
	"os"
	"strings"
	"time"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// loadTailFile reads the last n lines of the log into the tab.
func (cl *Client) loadTailFile(lf *logFile, n int) {
	abs := cl.abs(lf.path)
	rows, size, err := tailLines(abs, n)
	cl.mu.Lock()
	if err != nil {
		lf.err = err.Error()
		lf.loaded = false
	} else {
		lf.lines = toLogLines(rows)
		lf.lastSize = size
		lf.loaded = true
		lf.err = ""
		lf.tailN = n
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// loadFullFile reads the whole log into the tab, capped at maxLines.
func (cl *Client) loadFullFile(lf *logFile) {
	abs := cl.abs(lf.path)
	rows, size, err := loadAll(abs)
	cl.mu.Lock()
	if err != nil {
		lf.err = err.Error()
	} else {
		lf.lines = toLogLines(rows)
		lf.lastSize = size
		lf.loaded = true
		lf.err = ""
	}
	cl.mu.Unlock()
	cl.host.Invalidate()
}

// reloadTail re-tails the log with its current tail count.
func (cl *Client) reloadTail(lf *logFile) {
	cl.loadTailFile(lf, lf.tailN)
}

// toLogLines converts raw text rows into LogLines with parsed levels.
func toLogLines(rows []string) []mcode.LogLine {
	out := make([]mcode.LogLine, 0, len(rows))
	for _, r := range rows {
		if r == "" {
			continue
		}
		out = append(out, mcode.LogLine{Level: parseLevel(r), Text: r})
	}
	return out
}

// tailLines reads the last n complete lines by seeking backwards in chunks.
func tailLines(abs string, n int) ([]string, int64, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := st.Size()
	if size == 0 {
		return nil, 0, nil
	}
	const chunk = 8192
	var data []byte
	offset := size
	for offset > 0 {
		read := int64(chunk)
		if offset < read {
			read = offset
		}
		offset -= read
		buf := make([]byte, read)
		if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
			return nil, 0, err
		}
		data = append(buf, data...)
		if strings.Count(string(data), "\n") > n {
			break
		}
	}
	text := string(data)
	if offset > 0 {
		if idx := strings.IndexByte(text, '\n'); idx >= 0 {
			text = text[idx+1:]
		}
	}
	all := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	start := 0
	if len(all) > n {
		start = len(all) - n
	}
	return all[start:], size, nil
}

// loadAll reads every line of the file, capped at maxLines from the end.
func loadAll(abs string) ([]string, int64, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, 0, err
	}
	st, _ := os.Stat(abs)
	all := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(all) > maxLines {
		all = all[len(all)-maxLines:]
	}
	return all, st.Size(), nil
}

// startFollowLocked starts the polling goroutine; caller holds mu.
func (cl *Client) startFollowLocked(lf *logFile) {
	if lf.stopCh != nil || lf.path == "" {
		return
	}
	stop := make(chan struct{})
	lf.stopCh = stop
	lf.follow = true
	lf.state.Follow = true
	go cl.followLoop(lf, stop)
}

// stopFollowLocked stops the polling goroutine; caller holds mu.
func (cl *Client) stopFollowLocked(lf *logFile) {
	lf.follow = false
	if lf.stopCh != nil {
		close(lf.stopCh)
		lf.stopCh = nil
	}
}

// followLoop polls the file for appended content until stopped.
func (cl *Client) followLoop(lf *logFile, stop chan struct{}) {
	ticker := time.NewTicker(followPoll)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cl.pollFile(lf)
		}
	}
}

// pollFile reads bytes appended since lastSize and appends complete lines.
func (cl *Client) pollFile(lf *logFile) {
	abs := cl.abs(lf.path)
	st, err := os.Stat(abs)
	if err != nil {
		return
	}
	size := st.Size()
	cl.mu.Lock()
	last := lf.lastSize
	cl.mu.Unlock()
	if size == last {
		return
	}
	if size < last {
		cl.host.Post(func() { cl.reloadTail(lf) })
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(last, io.SeekStart); err != nil {
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	end := strings.LastIndexByte(string(data), '\n')
	if end < 0 {
		return
	}
	consumed := string(data[:end+1])
	newSize := last + int64(end+1)
	rows := strings.Split(strings.TrimSuffix(consumed, "\n"), "\n")
	cl.host.Post(func() {
		cl.mu.Lock()
		lf.lastSize = newSize
		for _, r := range rows {
			if r == "" {
				continue
			}
			lf.lines = append(lf.lines, mcode.LogLine{Level: parseLevel(r), Text: r})
		}
		if len(lf.lines) > maxLines {
			lf.lines = lf.lines[len(lf.lines)-maxLines:]
		}
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
}

// parseLevel guesses a line's severity from common level markers in its
// first 80 characters.
func parseLevel(line string) mcode.LogLevel {
	head := line
	if len(head) > 80 {
		head = head[:80]
	}
	up := strings.ToUpper(head)
	if strings.Contains(up, "ERR") || strings.Contains(up, "FATAL") || strings.Contains(up, "PANIC") {
		return mcode.LogError
	}
	if strings.Contains(up, "WARN") {
		return mcode.LogWarn
	}
	if strings.Contains(up, "DEBUG") || strings.Contains(up, "TRACE") {
		return mcode.LogDebug
	}
	return mcode.LogInfo
}

// toastMsg reports a follow event through the host.
func (cl *Client) toastMsg(msg string, sev mcore.Severity) {
	cl.host.Post(func() { cl.host.Toast(msg, sev) })
}
