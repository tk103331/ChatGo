package webui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// TerminalBacklog bounds how much output a session keeps for a phone that
// reconnects. Past it, the oldest bytes go and the client is told to reset.
const TerminalBacklog = 512 << 10

// TerminalInfo is one persistent desktop shell the phone may attach to.
type TerminalInfo struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Dir      string    `json:"dir"`
	Cols     int       `json:"cols"`
	Rows     int       `json:"rows"`
	Exited   bool      `json:"exited,omitempty"`
	ExitCode int       `json:"exit_code,omitempty"`
	Created  time.Time `json:"created"`
}

// TerminalOutput is one long-poll answer: output since the client's cursor.
type TerminalOutput struct {
	Seq      int64  `json:"seq"`
	Data     []byte `json:"data,omitempty"`
	Reset    bool   `json:"reset,omitempty"`
	Exited   bool   `json:"exited,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// TerminalManager owns the desktop shells a phone can attach to. Its sessions
// outlive a dropped relay connection, so a phone reconnects to the same shell.
type TerminalManager struct {
	mu       sync.Mutex
	sessions map[string]*TerminalSession
	order    []string
	resolve  func(string) (string, error)
}

// NewTerminalManager builds a manager that resolves every requested directory
// through resolve before starting a shell.
func NewTerminalManager(resolve func(string) (string, error)) *TerminalManager {
	return &TerminalManager{sessions: map[string]*TerminalSession{}, resolve: resolve}
}

// TerminalSession is one shell and the output kept for reconnecting phones.
type TerminalSession struct {
	info TerminalInfo
	pty  *os.File
	cmd  *exec.Cmd

	mu       sync.Mutex
	buf      []byte
	startSeq int64
	seq      int64
	exited   bool
	exitCode int
	notify   chan struct{}
}

// Start starts a shell in dir.
func (m *TerminalManager) Start(dir string, cols, rows int, title string) (TerminalInfo, error) {
	if m.resolve == nil {
		return TerminalInfo{}, errors.New("终端需要桌面版运行时")
	}
	root, err := m.resolve(dir)
	if err != nil {
		return TerminalInfo{}, err
	}
	cols, rows = clampTerminalSize(cols, rows)
	shell := os.Getenv("SHELL")
	args := []string(nil)
	if runtime.GOOS == "windows" {
		shell = "powershell.exe"
	} else if strings.TrimSpace(shell) == "" {
		shell = "/bin/sh"
	} else {
		args = []string{"-l"}
	}
	cmd := exec.Command(shell, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return TerminalInfo{}, fmt.Errorf("无法启动终端：%w", err)
	}
	id := newID()
	if strings.TrimSpace(title) == "" {
		title = filepath.Base(root)
	}
	s := &TerminalSession{
		info: TerminalInfo{
			ID: id, Title: title, Dir: root, Cols: cols, Rows: rows, Created: time.Now(),
		},
		pty: f, cmd: cmd, notify: make(chan struct{}, 1),
	}
	m.mu.Lock()
	// Keep a bounded number of shells; the oldest goes first.
	if len(m.order) >= 8 {
		oldest := m.order[0]
		m.order = m.order[1:]
		if old := m.sessions[oldest]; old != nil {
			delete(m.sessions, oldest)
			go old.Close()
		}
	}
	m.sessions[id] = s
	m.order = append(m.order, id)
	m.mu.Unlock()
	go s.readLoop()
	go s.waitLoop()
	return s.info, nil
}

// List returns the live sessions, newest first.
func (m *TerminalManager) List() []TerminalInfo {
	m.mu.Lock()
	out := make([]TerminalInfo, 0, len(m.order))
	for _, id := range m.order {
		if s := m.sessions[id]; s != nil {
			out = append(out, s.infoLocked())
		}
	}
	m.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Get finds one session.
func (m *TerminalManager) Get(id string) (*TerminalSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Close kills every session, as the server ends.
func (m *TerminalManager) Close() {
	m.mu.Lock()
	list := make([]*TerminalSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.sessions = map[string]*TerminalSession{}
	m.order = nil
	m.mu.Unlock()
	for _, s := range list {
		s.Close()
	}
}

// Remove kills one session.
func (m *TerminalManager) Remove(id string) bool {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
	if ok {
		s.Close()
	}
	return ok
}

// Info returns the session's metadata.
func (s *TerminalSession) Info() TerminalInfo { return s.infoLocked() }

func (s *TerminalSession) infoLocked() TerminalInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := s.info
	info.Exited, info.ExitCode = s.exited, s.exitCode
	return info
}

// Write sends the phone's bytes to the shell.
func (s *TerminalSession) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := s.pty.Write(data)
	return err
}

// Resize changes the shell's window size.
func (s *TerminalSession) Resize(cols, rows int) error {
	cols, rows = clampTerminalSize(cols, rows)
	if err := pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}); err != nil {
		return err
	}
	s.mu.Lock()
	s.info.Cols, s.info.Rows = cols, rows
	s.mu.Unlock()
	return nil
}

// Output returns output after since, waiting up to wait for more. A reset says
// the client's cursor is older than what the session kept.
func (s *TerminalSession) Output(since int64, wait time.Duration) TerminalOutput {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		if since < s.startSeq || since > s.seq {
			data := append([]byte(nil), s.buf...)
			out := TerminalOutput{Seq: s.seq, Data: data, Reset: true, Exited: s.exited, ExitCode: s.exitCode}
			s.mu.Unlock()
			return out
		}
		if since < s.seq || s.exited {
			at := since - s.startSeq
			if at < 0 {
				at = 0
			}
			data := append([]byte(nil), s.buf[at:]...)
			out := TerminalOutput{Seq: s.seq, Data: data, Exited: s.exited, ExitCode: s.exitCode}
			s.mu.Unlock()
			return out
		}
		s.mu.Unlock()
		select {
		case <-s.notify:
		case <-deadline.C:
			return TerminalOutput{Seq: since}
		}
	}
}

// Close ends the shell.
func (s *TerminalSession) Close() {
	_ = s.pty.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *TerminalSession) readLoop() {
	buf := make([]byte, 32<<10)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.append(buf[:n])
			select {
			case s.notify <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *TerminalSession) append(data []byte) {
	s.mu.Lock()
	s.buf = append(s.buf, data...)
	s.seq += int64(len(data))
	if len(s.buf) > TerminalBacklog {
		cut := len(s.buf) - TerminalBacklog
		s.buf = append([]byte(nil), s.buf[cut:]...)
		s.startSeq += int64(cut)
	}
	s.mu.Unlock()
}

func (s *TerminalSession) waitLoop() {
	err := s.cmd.Wait()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	}
	s.mu.Lock()
	s.exited, s.exitCode = true, code
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func clampTerminalSize(cols, rows int) (int, int) {
	if cols < 20 {
		cols = 48
	}
	if rows < 8 {
		rows = 28
	}
	if cols > 240 {
		cols = 240
	}
	if rows > 100 {
		rows = 100
	}
	return cols, rows
}
