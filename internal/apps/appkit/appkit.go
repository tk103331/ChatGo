// Package appkit holds what every Apps-menu application shares: the Host
// interface for the embedding shell, tab bookkeeping and a standalone window
// runtime, so an app can run either inside ChatGo or on its own.
package appkit

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	mygo "github.com/egoist/mygo"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/internal/ui/chatpanel"
	"chatgo/pkg/models"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// Host is the capability set an application needs from its shell.
type Host interface {
	// Root is the directory the app operates on.
	Root() string
	// Workspace is the selected project path, or "" for the process default.
	Workspace() string
	// Post runs fn on the UI thread.
	Post(fn func())
	// Invalidate schedules a redraw.
	Invalidate()
	// Toast shows a transient message.
	Toast(msg string, sev mcore.Severity)
	// Confirm asks the user before a destructive action.
	Confirm(title, desc string, action func())
	// Prompt asks for one line of free text.
	Prompt(title, label, initial string, action func(string))
	// OpenFile shows a file in the shell's preview area.
	OpenFile(path string)
	// OpenFileDialog asks for a file to open and returns its absolute path,
	// or "" when the user cancels.
	OpenFileDialog(dir string) string
	// SaveFileDialog asks where to save a file and returns the chosen
	// absolute path, or "" when the user cancels.
	SaveFileDialog(name string) string
	// Language maps a path extension, as ".go", to a syntax name.
	Language(ext string) string
	// NewID returns an id unique within the shell.
	NewID() string
	// NativeWindow is the window the app draws into, for embedded WebViews.
	NativeWindow() *mygo.Window
}

// Tabs is a string-keyed tab list with stable selection.
type Tabs struct {
	Items  []string
	Active string
}

// Add appends a tab and selects it.
func (t *Tabs) Add(key string) {
	t.Items = append(t.Items, key)
	t.Active = key
}

// Close removes the tab at i and selects a neighbour.
func (t *Tabs) Close(i int) {
	if i < 0 || i >= len(t.Items) {
		return
	}
	closed := t.Items[i]
	t.Items = append(t.Items[:i], t.Items[i+1:]...)
	if t.Active != closed {
		return
	}
	switch {
	case len(t.Items) == 0:
		t.Active = ""
	case i >= len(t.Items):
		t.Active = t.Items[len(t.Items)-1]
	default:
		t.Active = t.Items[i]
	}
}

// Standalone is the runtime for an app opened in its own window.
type Standalone struct {
	// Dir is the directory the app starts in.
	Dir string
	// Lang maps a path extension to a syntax name; nil means the default.
	Lang func(ext string) string

	mu      sync.Mutex
	win     *mygo.Window
	post    func(func())
	invalid func(func())
	toasts  []Toast
	seq     atomic.Int64
	started time.Time

	panel        *chatpanel.Panel
	messages     []models.Message
	streaming    bool
	streamCancel context.CancelFunc
	react        *llm.ReactClient
	cfg          *config.Config
	system       string
	tools        []Tool
	bodyW        float32
	chatHidden   bool
	status       string
	agentTried   bool
}

// Toast is a message queued for the standalone window's status line.
type Toast struct {
	Msg string
	Sev mcore.Severity
}

// Attach binds the runtime to a window's update pumps.
func (s *Standalone) Attach(w *mygo.Window, post, invalidate func(func())) {
	s.mu.Lock()
	s.win, s.post, s.invalid = w, post, invalidate
	s.mu.Unlock()
}

// NativeWindow is the standalone window, or nil before it is attached.
func (s *Standalone) NativeWindow() *mygo.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.win
}

// Root returns the working directory of the standalone app.
func (s *Standalone) Root() string { return s.Dir }

// Workspace mirrors Root for apps that read the project path.
func (s *Standalone) Workspace() string { return s.Dir }

// Post runs fn through the window pump, or inline before one is attached.
func (s *Standalone) Post(fn func()) {
	s.mu.Lock()
	post := s.post
	s.mu.Unlock()
	if post == nil {
		fn()
		return
	}
	post(fn)
}

// Invalidate redraws the standalone window.
func (s *Standalone) Invalidate() {
	s.mu.Lock()
	inv := s.invalid
	s.mu.Unlock()
	if inv != nil {
		inv(func() {})
	}
}

// Toast records a message shown by the app's own status line.
func (s *Standalone) Toast(msg string, sev mcore.Severity) {
	s.mu.Lock()
	s.toasts = append(s.toasts, Toast{Msg: msg, Sev: sev})
	if len(s.toasts) > 4 {
		s.toasts = s.toasts[len(s.toasts)-4:]
	}
	s.mu.Unlock()
	s.Invalidate()
}

// Toasts returns the queued messages, newest last.
func (s *Standalone) Toasts() []Toast {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Toast, len(s.toasts))
	copy(out, s.toasts)
	return out
}

// ClearToasts drops the queued messages.
func (s *Standalone) ClearToasts() {
	s.mu.Lock()
	s.toasts = nil
	s.mu.Unlock()
}

// Confirm runs the action: a standalone window has no modal, so it proceeds
// and reports through Toast instead of blocking.
func (s *Standalone) Confirm(_, _ string, action func()) { action() }

// Prompt cannot collect input without a modal, so it runs the action empty.
func (s *Standalone) Prompt(_, _, _ string, action func(string)) { action("") }

// OpenFile is a no-op: a standalone window has no preview pane.
func (s *Standalone) OpenFile(string) {}

// OpenFileDialog asks for a file with the native dialog.
func (s *Standalone) OpenFileDialog(dir string) string {
	picks, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: s.NativeWindow(), Title: "打开文件", DefaultPath: dir})
	if err != nil || len(picks) == 0 {
		return ""
	}
	return picks[0]
}

// SaveFileDialog asks where to save with the native dialog.
func (s *Standalone) SaveFileDialog(name string) string {
	p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: s.NativeWindow(), Title: "保存文件", DefaultPath: name})
	if err != nil || p == "" {
		return ""
	}
	return p
}

// Language names a path extension's syntax.
func (s *Standalone) Language(ext string) string {
	if s.Lang != nil {
		return s.Lang(ext)
	}
	return DefaultLanguage(ext)
}

// NewID returns an id unique to this runtime.
func (s *Standalone) NewID() string {
	if s.started.IsZero() {
		s.started = time.Now()
	}
	return s.started.Format("150405") + "-" + itoa(int(s.seq.Add(1)))
}

// DefaultLanguage maps common extensions to MujicaUI syntax names.
func DefaultLanguage(ext string) string {
	return map[string]string{
		".go": "go", ".js": "javascript", ".ts": "typescript", ".tsx": "tsx",
		".json": "json", ".md": "markdown", ".py": "python", ".rs": "rust",
		".java": "java", ".c": "c", ".h": "c", ".cpp": "cpp", ".sh": "bash",
		".yaml": "yaml", ".yml": "yaml", ".toml": "toml", ".xml": "xml",
		".css": "css", ".html": "html",
	}[ext]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
