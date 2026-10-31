// Package webui serves ChatGo's local browser UI from the desktop process.
package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"chatgo/internal/config"
	"chatgo/internal/llm"
	"chatgo/pkg/models"
)

const defaultTitle = "新会话"

type Server struct {
	cfg       *config.Config
	cm        *models.ConversationManager
	webDir    string
	target    config.Provider
	targetErr error

	httpServer *http.Server
	listener   net.Listener

	mu   sync.Mutex
	runs map[string]*runState
}

// runState is the live copy of a conversation while a reply streams. GET
// reads it instead of the file, so a reader never sees a partial JSON write.
type runState struct {
	mu     sync.Mutex
	conv   *models.Conversation
	target config.Provider
	cancel context.CancelFunc
	done   chan struct{}
}

func NewServer(cfg *config.Config, cm *models.ConversationManager, webDir string) (*Server, error) {
	if webDir == "" {
		return nil, errors.New("web directory is empty")
	}
	target, err := pickTarget(cfg)
	s := &Server{
		cfg:       cfg,
		cm:        cm,
		webDir:    webDir,
		target:    target,
		targetErr: err,
		runs:      map[string]*runState{},
	}
	s.httpServer = &http.Server{Handler: s.Handler()}
	return s, nil
}

// Ready reports whether a model is configured for the web UI.
func (s *Server) Ready() bool { return s.targetErr == nil }

// Target returns the provider and model the web UI chats with.
func (s *Server) Target() (provider, model string, ok bool) {
	if s.targetErr != nil {
		return "", "", false
	}
	return s.target.Name, s.target.Model, true
}

// Start listens on addr in the background and returns the URL clients use.
// Pass 127.0.0.1:0 to let the system choose a free port.
func (s *Server) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s.listener = ln
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("web server: %v", err)
		}
	}()
	return "http://" + ln.Addr().String(), nil
}

// ListenAndServe listens on addr until the server is closed.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln
	return s.httpServer.Serve(ln)
}

// Close stops accepting requests and waits briefly for active handlers.
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// pickTarget chooses the model the web UI chats with: the active desktop
// provider, then the first enabled one. Provider groups are deferred to a
// later phase; this first cut runs a plain chat client.
func pickTarget(cfg *config.Config) (config.Provider, error) {
	if cfg == nil {
		return config.Provider{}, errors.New("no config")
	}
	name := cfg.CurrentProvider
	if name == "" {
		name = cfg.Chat.Provider
	}
	var picked config.Provider
	found := false
	for _, p := range cfg.Providers {
		if p.Name == name {
			picked, found = p, true
			break
		}
	}
	if !found || !picked.Enabled {
		found = false
		for _, p := range cfg.Providers {
			if p.Enabled {
				picked, found = p, true
				break
			}
		}
	}
	if !found {
		return config.Provider{}, errors.New("尚未启用任何模型，请在 ChatGo 设置 → 模型中配置")
	}
	models := picked.ModelsList()
	if len(models) == 0 {
		return config.Provider{}, fmt.Errorf("模型「%s」尚未选择具体模型", picked.Name)
	}
	model := cfg.Chat.Model
	if model == "" || !contains(models, model) {
		model = models[0]
	}
	picked.Model = model
	if err := picked.Validate(); err != nil {
		return config.Provider{}, err
	}
	return picked, nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("GET /api/sessions/{id}/context", s.handleListContext)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/messages", s.handleSendMessage)
	mux.HandleFunc("POST /api/sessions/{id}/cancel", s.handleCancel)
	mux.Handle("/", s.static())
	return mux
}

func (s *Server) static() http.Handler {
	files := http.FileServer(http.Dir(s.webDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".wasm") {
			w.Header().Set("Content-Type", "application/wasm")
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-store")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) getRun(id string) *runState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[id]
}
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"provider": s.target.Name,
		"model":    s.target.Model,
		"ready":    s.Ready(),
		"models":   s.modelChoices(),
		"note":     "本地 Web MVP：纯对话；工具 / Agent / ACP 后续接入。",
	}
	if s.targetErr != nil {
		resp["error"] = s.targetErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

type modelChoice struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// modelChoices lists every enabled provider model the browser may pick.
func (s *Server) modelChoices() []modelChoice {
	if s.cfg == nil {
		return nil
	}
	var out []modelChoice
	for _, p := range s.cfg.Providers {
		if !p.Enabled {
			continue
		}
		for _, m := range p.ModelsList() {
			out = append(out, modelChoice{Provider: p.Name, Model: m})
		}
	}
	return out
}

// targetFor resolves a model choice from a request. An empty choice falls back
// to the server's startup target, which is what the UI uses before a pick.
func (s *Server) targetFor(provider, model string) (config.Provider, error) {
	if provider == "" && model == "" {
		return s.target, s.targetErr
	}
	if provider == "" {
		provider = s.target.Name
	}
	var picked config.Provider
	found := false
	for _, p := range s.cfg.Providers {
		if p.Name == provider {
			picked, found = p, true
			break
		}
	}
	if !found {
		return config.Provider{}, fmt.Errorf("未知模型提供商 %q", provider)
	}
	if !picked.Enabled {
		return config.Provider{}, fmt.Errorf("模型提供商 %q 未启用", provider)
	}
	models := picked.ModelsList()
	if model == "" && len(models) > 0 {
		model = models[0]
	}
	if model == "" || !contains(models, model) {
		return config.Provider{}, fmt.Errorf("模型 %q 不属于 %q", model, provider)
	}
	picked.Model = model
	if err := picked.Validate(); err != nil {
		return config.Provider{}, err
	}
	return picked, nil
}

type sessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Preview   string    `json:"preview,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Workspace string    `json:"workspace,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  int       `json:"messages"`
	Running   bool      `json:"running"`
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	all, err := s.cm.ListConversations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]sessionSummary, 0, len(all))
	seen := map[string]bool{}

	s.mu.Lock()
	ids := make([]string, 0, len(s.runs))
	for id := range s.runs {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		rs := s.getRun(id)
		if rs == nil {
			continue
		}
		if sum, ok := rs.summary(); ok {
			out = append(out, sum)
			seen[id] = true
		}
	}
	for i := range all {
		c := &all[i]
		if seen[c.ID] || c.TaskID != "" || c.RoomID != "" {
			continue
		}
		out = append(out, summarize(c, false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func summarize(c *models.Conversation, running bool) sessionSummary {
	sum := sessionSummary{
		ID:        c.ID,
		Title:     c.Title,
		Agent:     c.Agent,
		Workspace: c.Workspace,
		UpdatedAt: c.UpdatedAt,
		Messages:  len(c.Messages),
		Running:   running,
	}
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(c.Messages[i].Content); text != "" {
			sum.Preview = truncate(strings.Join(strings.Fields(text), " "), 90)
			break
		}
	}
	if sum.Title == "" {
		sum.Title = "未命名"
	}
	return sum
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title    string `json:"title"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	target, err := s.targetFor(body.Provider, body.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = defaultTitle
	}
	conv, err := s.cm.CreateConversation(truncate(title, 32), target.Name, target.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session": conv})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	running := false
	if rs := s.getRun(id); rs != nil {
		running = true
		if b, ok := rs.snapshotJSON(); ok {
			writeJSON(w, http.StatusOK, map[string]any{"session": json.RawMessage(b), "running": true})
			return
		}
	}
	conv, err := s.cm.LoadConversation(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": conv, "running": running})
}

type contextItem struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type contextResponse struct {
	Workspace string        `json:"workspace,omitempty"`
	Items     []contextItem `json:"items"`
}

func (s *Server) handleListContext(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	conv, err := s.cm.LoadConversation(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, contextResponse{
		Workspace: conv.Workspace,
		Items:     listContextFiles(conv.Workspace),
	})
}

func listContextFiles(root string) []contextItem {
	if root == "" {
		return nil
	}
	out := make([]contextItem, 0, 64)
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == ".git" || name == "node_modules" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			out = append(out, contextItem{Path: rel, Label: rel, Kind: "directory"})
			if len(out) >= 300 {
				return filepath.SkipAll
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		out = append(out, contextItem{Path: rel, Label: rel, Kind: contextKindOf(name)})
		if len(out) >= 300 {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	if rs := s.getRun(id); rs != nil {
		rs.cancel()
		<-rs.done
	}
	if err := s.cm.DeleteConversation(id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	var body struct {
		Text     string `json:"text"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, errors.New("message is empty"))
		return
	}
	target, err := s.targetFor(body.Provider, body.Model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	conv, err := s.cm.LoadConversation(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	now := time.Now()
	user := conv.NewHumanMessage(newID(), text, now)
	reply := conv.NewAgentMessage(newID(), conv.Agent, authorName(conv.Agent), now)
	conv.Messages = append(conv.Messages, user, reply)
	conv.Provider = target.Name
	conv.Model = target.Model
	turn := models.Turn{
		ID:         newID(),
		UserMsgID:  user.ID,
		ReplyMsgID: reply.ID,
		User:       text,
		Provider:   target.Name,
		Model:      target.Model,
		StartedAt:  now,
	}
	conv.Turns = append(conv.Turns, turn)
	if conv.Title == "" || conv.Title == defaultTitle {
		conv.Title = titleFromText(text)
	}
	conv.UpdatedAt = now
	replyIdx := len(conv.Messages) - 1
	turnIdx := len(conv.Turns) - 1

	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{conv: conv, target: target, cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	if s.runs[id] != nil {
		s.mu.Unlock()
		cancel()
		writeError(w, http.StatusConflict, errors.New("a reply is already running"))
		return
	}
	s.runs[id] = rs
	s.mu.Unlock()

	if err := s.cm.SaveConversation(conv); err != nil {
		s.mu.Lock()
		delete(s.runs, id)
		s.mu.Unlock()
		cancel()
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":               true,
		"user_message_id":  user.ID,
		"reply_message_id": reply.ID,
	})
	go s.run(ctx, rs, replyIdx, turnIdx)
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		writeError(w, http.StatusBadRequest, errors.New("invalid session id"))
		return
	}
	rs := s.getRun(id)
	if rs == nil {
		writeError(w, http.StatusNotFound, errors.New("no reply is running"))
		return
	}
	rs.cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// run streams one reply, keeping the live conversation in memory. It saves
// once at the end, so a GET never races a partial os.WriteFile.
func (s *Server) run(ctx context.Context, rs *runState, replyIdx, turnIdx int) {
	convID := rs.conv.ID
	defer func() {
		close(rs.done)
		s.mu.Lock()
		if s.runs[convID] == rs {
			delete(s.runs, convID)
		}
		s.mu.Unlock()
	}()

	rs.mu.Lock()
	msgs := make([]llm.ChatMessage, 0, replyIdx)
	for i := 0; i < replyIdx && i < len(rs.conv.Messages); i++ {
		m := rs.conv.Messages[i]
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		msgs = append(msgs, llm.ChatMessage{Role: m.Role, Content: m.Content})
	}
	rs.mu.Unlock()

	var (
		resp *llm.ChatResponse
		err  error
	)
	client, cerr := llm.NewClient(rs.target)
	if cerr != nil {
		err = cerr
	} else {
		onChunk := func(chunk string) {
			rs.mu.Lock()
			m := &rs.conv.Messages[replyIdx]
			m.Content += chunk
			appendSegmentText(m, "text", chunk)
			rs.mu.Unlock()
		}
		onReasoning := func(chunk string) {
			rs.mu.Lock()
			m := &rs.conv.Messages[replyIdx]
			m.Reasoning += chunk
			appendSegmentText(m, "reasoning", chunk)
			rs.mu.Unlock()
		}
		resp, err = client.Chat(ctx, msgs, onChunk, onReasoning)
	}

	rs.mu.Lock()
	m := &rs.conv.Messages[replyIdx]
	canceled := errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
	switch {
	case canceled:
		if strings.TrimSpace(m.Content) == "" {
			m.Content = "（已停止）"
		}
	case err != nil:
		if strings.TrimSpace(m.Content) == "" {
			m.Content = "⚠️ " + err.Error()
		} else {
			m.Content += "\n\n⚠️ " + err.Error()
		}
	}
	if resp != nil {
		if strings.TrimSpace(m.Content) == "" {
			m.Content = resp.Content
		}
		if strings.TrimSpace(m.Reasoning) == "" {
			m.Reasoning = resp.Reasoning
		}
		if turnIdx >= 0 && turnIdx < len(rs.conv.Turns) {
			t := &rs.conv.Turns[turnIdx]
			t.Input = resp.PromptTokens
			t.Output = resp.CompletionTokens
			t.Total = resp.TotalTokens
		}
	}
	if turnIdx >= 0 && turnIdx < len(rs.conv.Turns) {
		t := &rs.conv.Turns[turnIdx]
		t.Assistant = m.Content
		t.EndedAt = time.Now()
	}
	rs.conv.UpdatedAt = time.Now()
	saveErr := s.cm.SaveConversation(rs.conv)
	rs.mu.Unlock()
	if saveErr != nil {
		log.Printf("save conversation %s: %v", convID, saveErr)
	}
}

func (rs *runState) snapshotJSON() ([]byte, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.conv == nil {
		return nil, false
	}
	b, err := json.Marshal(rs.conv)
	return b, err == nil
}

func (rs *runState) summary() (sessionSummary, bool) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.conv == nil {
		return sessionSummary{}, false
	}
	return summarize(rs.conv, true), true
}

func appendSegmentText(m *models.Message, kind, text string) {
	if text == "" {
		return
	}
	if n := len(m.Segments); n > 0 && m.Segments[n-1].Kind == kind && m.Segments[n-1].ToolCall == nil {
		m.Segments[n-1].Text += text
		return
	}
	m.Segments = append(m.Segments, models.Segment{Kind: kind, Text: text})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func validID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func titleFromText(text string) string {
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if line == "" {
		return defaultTitle
	}
	return truncate(line, 24)
}

func authorName(agent string) string {
	switch {
	case strings.HasPrefix(agent, "acp:"):
		return strings.TrimPrefix(agent, "acp:")
	case strings.HasPrefix(agent, "agent:"):
		return strings.TrimPrefix(agent, "agent:")
	case agent != "":
		return agent
	default:
		return "ChatGo"
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func contextKindOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".md", ".markdown", ".txt", ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".json", ".yaml", ".yml", ".html", ".htm":
		return "doc"
	default:
		return "file"
	}
}
