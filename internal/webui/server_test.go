package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"chatgo/internal/config"
	"chatgo/pkg/models"
)

func testServer(t *testing.T) (*Server, *models.ConversationManager) {
	t.Helper()
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatalf("conversation manager: %v", err)
	}
	cfg := &config.Config{
		Providers: []config.Provider{{
			Name: "Local", Type: config.ProviderTypeOllama,
			Model: "llama3", Models: []string{"llama3", "llama3.1"}, Enabled: true,
			// A closed port makes a reply fail immediately, so a test never
			// reaches a real model or waits on the network.
			BaseURL: "http://127.0.0.1:1",
		}},
		CurrentProvider: "Local",
	}
	s, err := NewServer(cfg, cm, dir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s, cm
}

func serve(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func newConversation(t *testing.T, cm *models.ConversationManager, title string) *models.Conversation {
	t.Helper()
	conv, err := cm.CreateConversation(title, "Local", "llama3")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return conv
}

func TestRenameSession(t *testing.T) {
	s, cm := testServer(t)
	conv := newConversation(t, cm, "旧标题")

	w := serve(t, s, http.MethodPatch, "/api/sessions/"+conv.ID, `{"title":"  新标题  "}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rename status %d: %s", w.Code, w.Body)
	}
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Title != "新标题" {
		t.Fatalf("title = %q, want 新标题", loaded.Title)
	}
}

func TestRenameSessionUpdatesLiveRun(t *testing.T) {
	s, cm := testServer(t)
	conv := newConversation(t, cm, "旧标题")
	live := &runState{conv: conv}
	s.runs[conv.ID] = live

	w := serve(t, s, http.MethodPatch, "/api/sessions/"+conv.ID, `{"title":"运行中改名"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("rename status %d: %s", w.Code, w.Body)
	}
	if live.conv.Title != "运行中改名" {
		t.Fatalf("live title = %q, want 运行中改名", live.conv.Title)
	}
	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Title != "运行中改名" {
		t.Fatalf("stored title = %q, want 运行中改名", loaded.Title)
	}
}

func TestRenameSessionRejectsEmptyTitle(t *testing.T) {
	s, cm := testServer(t)
	conv := newConversation(t, cm, "标题")
	if w := serve(t, s, http.MethodPatch, "/api/sessions/"+conv.ID, `{"title":"  "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
}

func TestRenameSessionNotFound(t *testing.T) {
	s, _ := testServer(t)
	if w := serve(t, s, http.MethodPatch, "/api/sessions/missing", `{"title":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", w.Code)
	}
}

// conversationWithPairs builds u1, r1, u2, r2 with a turn per exchange.
func conversationWithPairs(t *testing.T, cm *models.ConversationManager) *models.Conversation {
	t.Helper()
	conv := newConversation(t, cm, "会话")
	now := time.Now()
	u1 := conv.NewHumanMessage("u1", "第一问", now)
	r1 := conv.NewAgentMessage("r1", "agent:Test", "Test", now)
	u2 := conv.NewHumanMessage("u2", "第二问", now)
	r2 := conv.NewAgentMessage("r2", "agent:Test", "Test", now)
	conv.Messages = []models.Message{u1, r1, u2, r2}
	conv.Turns = []models.Turn{
		{ID: "t1", UserMsgID: "u1", ReplyMsgID: "r1", User: "第一问", Assistant: "答一", Provider: "Local", Model: "llama3", StartedAt: now, EndedAt: now},
		{ID: "t2", UserMsgID: "u2", ReplyMsgID: "r2", User: "第二问", Assistant: "答二", Provider: "Local", Model: "llama3", StartedAt: now, EndedAt: now},
	}
	return conv
}

func TestPrepareEditBranchesConversation(t *testing.T) {
	_, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	target := config.Provider{Name: "Local", Model: "llama3.1"}
	at := time.Now()

	replyIdx, turnIdx := prepareEdit(conv, 0, "改过的第一问", target, at)

	if replyIdx != 1 || len(conv.Messages) != 2 {
		t.Fatalf("messages = %d (reply %d), want 2 (reply 1)", len(conv.Messages), replyIdx)
	}
	if conv.Messages[0].Content != "改过的第一问" || conv.Messages[0].ID != "u1" {
		t.Fatalf("edited message = %#v", conv.Messages[0])
	}
	if conv.Messages[1].IsHuman() || conv.Messages[1].Content != "" {
		t.Fatalf("reply = %#v, want an empty assistant message", conv.Messages[1])
	}
	if len(conv.Turns) != 1 || turnIdx != 0 {
		t.Fatalf("turns = %#v, want one new turn", conv.Turns)
	}
	turn := conv.Turns[0]
	if turn.UserMsgID != "u1" || turn.ReplyMsgID != conv.Messages[1].ID || turn.User != "改过的第一问" {
		t.Fatalf("turn = %#v", turn)
	}
	if turn.Provider != "Local" || turn.Model != "llama3.1" {
		t.Fatalf("turn model = %s/%s", turn.Provider, turn.Model)
	}
	if conv.Provider != "Local" || conv.Model != "llama3.1" {
		t.Fatalf("session model = %s/%s", conv.Provider, conv.Model)
	}
	if conv.Messages[1].Seq <= conv.Messages[0].Seq {
		t.Fatalf("reply seq %d did not advance past %d", conv.Messages[1].Seq, conv.Messages[0].Seq)
	}
}

func TestPrepareRegenerateKeepsPreviousTurns(t *testing.T) {
	_, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	target := config.Provider{Name: "Local", Model: "llama3"}
	at := time.Now()

	replyIdx, turnIdx := prepareRegenerate(conv, 3, target, at)

	if replyIdx != 3 || len(conv.Messages) != 4 {
		t.Fatalf("messages = %d (reply %d), want 4 (reply 3)", len(conv.Messages), replyIdx)
	}
	if got := conv.Messages[3]; got.IsHuman() || got.ID == "r2" || got.AuthorName != "Test" || got.AuthorID != "agent:Test" {
		t.Fatalf("replacement reply = %#v", got)
	}
	if len(conv.Turns) != 2 || turnIdx != 1 {
		t.Fatalf("turns = %#v, want the first turn kept plus the new one", conv.Turns)
	}
	if conv.Turns[0].ID != "t1" {
		t.Fatalf("first turn = %#v, want it untouched", conv.Turns[0])
	}
	turn := conv.Turns[1]
	if turn.UserMsgID != "u2" || turn.User != "第二问" || turn.ReplyMsgID != conv.Messages[3].ID {
		t.Fatalf("new turn = %#v", turn)
	}
	if turn.Input != 0 || turn.Output != 0 || turn.Total != 0 || !turn.EndedAt.IsZero() {
		t.Fatalf("new turn kept stale usage or end time: %#v", turn)
	}
}

func TestPrepareRegenerateDropsLaterMessages(t *testing.T) {
	_, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	at := time.Now()

	prepareRegenerate(conv, 1, config.Provider{Name: "Local", Model: "llama3"}, at)

	if len(conv.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(conv.Messages))
	}
	if len(conv.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(conv.Turns))
	}
	if conv.Turns[0].UserMsgID != "u1" || conv.Turns[0].ReplyMsgID != conv.Messages[1].ID {
		t.Fatalf("turn = %#v", conv.Turns[0])
	}
}

func TestEditRejectsAssistantMessage(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/r2/edit", `{"text":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body)
	}
}

func TestRegenerateRejectsUserMessage(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/u2/regenerate", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body)
	}
}

func TestRegenerateRejectsLeadingReply(t *testing.T) {
	s, cm := testServer(t)
	conv := newConversation(t, cm, "问候")
	conv.Messages = []models.Message{conv.NewAgentMessage("greeting", "agent:Test", "Test", time.Now())}
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/greeting/regenerate", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body)
	}
}

func TestEditConflictWhileRunning(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	s.runs[conv.ID] = &runState{conv: conv}

	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/u1/edit", `{"text":"x"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", w.Code, w.Body)
	}
}

func TestDropOrphanTurns(t *testing.T) {
	conv := &models.Conversation{
		Messages: []models.Message{{ID: "u1"}, {ID: "r1"}, {ID: "u2"}},
		Turns: []models.Turn{
			{ID: "keep", UserMsgID: "u1", ReplyMsgID: "r1"},
			{ID: "orphan-reply", UserMsgID: "u2", ReplyMsgID: "r2"},
			{ID: "orphan-user", UserMsgID: "u9", ReplyMsgID: "u2"},
		},
	}
	dropOrphanTurns(conv)
	if len(conv.Turns) != 1 || conv.Turns[0].ID != "keep" {
		t.Fatalf("turns = %#v", conv.Turns)
	}
}

// The session list carries the new title without a restart.
func TestRenameShowsInSessionList(t *testing.T) {
	s, cm := testServer(t)
	conv := newConversation(t, cm, "旧标题")
	if w := serve(t, s, http.MethodPatch, "/api/sessions/"+conv.ID, `{"title":"新标题"}`); w.Code != http.StatusOK {
		t.Fatalf("rename status %d", w.Code)
	}
	w := serve(t, s, http.MethodGet, "/api/sessions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list status %d", w.Code)
	}
	var resp struct {
		Sessions []sessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, item := range resp.Sessions {
		if item.ID == conv.ID {
			if item.Title != "新标题" {
				t.Fatalf("title = %q", item.Title)
			}
			return
		}
	}
	t.Fatalf("session %s missing from the list", conv.ID)
}

// waitIdle waits until the server's live run for id finishes.
func waitIdle(t *testing.T, s *Server, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.getRun(id) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("reply did not finish")
}

func TestEditRunsAndPersists(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/u1/edit", `{"text":"改过的第一问"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)

	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "改过的第一问" {
		t.Fatalf("edited content = %q", loaded.Messages[0].Content)
	}
	if loaded.Messages[1].ID == "r1" || loaded.Messages[1].ID == "" {
		t.Fatalf("reply id = %q, want a new message", loaded.Messages[1].ID)
	}
	if len(loaded.Turns) != 1 || loaded.Turns[0].ReplyMsgID != loaded.Messages[1].ID {
		t.Fatalf("turns = %#v", loaded.Turns)
	}
	if loaded.Turns[0].EndedAt.IsZero() {
		t.Fatal("turn was not finished")
	}
	// The failure is recorded in a segment, which is what the web renders
	// once a reply has any segments at all.
	found := false
	for _, seg := range loaded.Messages[1].Segments {
		if strings.Contains(seg.Text, "⚠️") {
			found = true
		}
	}
	if !found {
		t.Fatalf("segments = %#v, want the error recorded", loaded.Messages[1].Segments)
	}
}

func TestRegenerateRunsWithTurnModel(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	w := serve(t, s, http.MethodPost, "/api/sessions/"+conv.ID+"/messages/r2/regenerate", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202: %s", w.Code, w.Body)
	}
	waitIdle(t, s, conv.ID)

	loaded, err := cm.LoadConversation(conv.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Messages) != 4 || len(loaded.Turns) != 2 {
		t.Fatalf("messages/turns = %d/%d, want 4/2", len(loaded.Messages), len(loaded.Turns))
	}
	if loaded.Messages[3].ID == "r2" {
		t.Fatal("reply was not replaced")
	}
	last := loaded.Turns[1]
	if last.Provider != "Local" || last.Model != "llama3" {
		t.Fatalf("turn model = %s/%s, want the replaced turn's", last.Provider, last.Model)
	}
	if last.UserMsgID != "u2" {
		t.Fatalf("turn user = %q", last.UserMsgID)
	}
}

func TestAgentSessionIsReadOnly(t *testing.T) {
	s, cm := testServer(t)
	conv := conversationWithPairs(t, cm)
	conv.Agent = "agent:Reviewer"
	if err := cm.SaveConversation(conv); err != nil {
		t.Fatalf("save: %v", err)
	}
	cases := []struct{ path, body string }{
		{"/api/sessions/" + conv.ID + "/messages", `{"text":"hi"}`},
		{"/api/sessions/" + conv.ID + "/messages/u1/edit", `{"text":"hi"}`},
		{"/api/sessions/" + conv.ID + "/messages/r2/regenerate", ""},
	}
	for _, tc := range cases {
		w := serve(t, s, http.MethodPost, tc.path, tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400: %s", tc.path, w.Code, w.Body)
		}
	}
}
