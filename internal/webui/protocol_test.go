package webui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	a2aclient "chatgo/internal/a2a"
	"chatgo/internal/config"
	"chatgo/pkg/models"
)

type protocolRunner struct {
	chunks []string
	err    error
	req    RunRequest
}

func (r *protocolRunner) Capabilities() map[string]bool { return nil }
func (r *protocolRunner) Run(ctx context.Context, conv *models.Conversation, replyIdx int, req RunRequest, live Live) error {
	r.req = req
	for _, chunk := range r.chunks {
		live.Update(func(c *models.Conversation) {
			c.Messages[replyIdx].Content += chunk
		})
	}
	return r.err
}
func (r *protocolRunner) Agents() []AgentInfo { return nil }
func (r *protocolRunner) Tools(string) []ToolInfo {
	return nil
}
func (r *protocolRunner) Workspaces() []string { return nil }
func (r *protocolRunner) Projects() []ProjectInfo {
	return nil
}
func (r *protocolRunner) Models(string) []ModelInfo { return nil }
func (r *protocolRunner) ACPOptions(string) []ACPConfigOptionInfo {
	return nil
}
func (r *protocolRunner) SessionACPOptions(string) []ACPConfigOptionInfo { return nil }
func (r *protocolRunner) SetSessionConfig(string, string, string) error  { return nil }
func (r *protocolRunner) CreateWorktree(string, string, bool) (string, error) {
	return "", nil
}
func (r *protocolRunner) Skills(string) []SkillInfo { return nil }
func (r *protocolRunner) Revert(context.Context, *models.Conversation, string) error {
	return nil
}

func protocolServer(t *testing.T, runner Runner) *Server {
	return protocolServerWithToken(t, runner, testWebToken)
}

func protocolServerWithToken(t *testing.T, runner Runner, token string) *Server {
	t.Helper()
	dir := t.TempDir()
	cm, err := models.NewConversationManagerAt(dir)
	if err != nil {
		t.Fatalf("conversation manager: %v", err)
	}
	cfg := &config.Config{
		Providers: []config.Provider{{
			Name: "Local", Type: config.ProviderTypeOllama,
			Model: "llama3", Models: []string{"llama3"}, Enabled: true,
			BaseURL: "http://127.0.0.1:1",
		}},
		CurrentProvider: "Local",
		WebToken:        token,
	}
	s, err := NewServerWithRunner(cfg, cm, dir, runner)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return s
}

func postJSON(t *testing.T, ts *httptest.Server, path, body string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("Authorization", "Bearer "+testWebToken)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp, data
}

func TestMCPChatTool(t *testing.T) {
	s := protocolServer(t, &protocolRunner{chunks: []string{"hello ", "world"}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	initBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test","version":"1.0"},"capabilities":{}}}`
	resp, data := postJSON(t, ts, "/mcp", initBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initialize status %d: %s", resp.StatusCode, data)
	}
	if !strings.Contains(string(data), `"serverInfo"`) {
		t.Fatalf("initialize response: %s", data)
	}

	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"chat","arguments":{"message":"你好"}}}`
	resp, data = postJSON(t, ts, "/mcp", callBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/call status %d: %s", resp.StatusCode, data)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("tools/call response: %s", data)
	}
	if !strings.Contains(string(data), "conversation_id") {
		t.Fatalf("tools/call did not return a conversation id: %s", data)
	}

	callBody = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}`
	resp, data = postJSON(t, ts, "/mcp", callBody)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `Chat`) {
		t.Fatalf("list_agents status %d: %s", resp.StatusCode, data)
	}
}

func TestAGUIRunStreamsEvents(t *testing.T) {
	runner := &protocolRunner{chunks: []string{"hel", "lo"}}
	s := protocolServer(t, runner)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body := `{"threadId":"thread-1","runId":"run-1","messages":[{"id":"m1","role":"user","content":"你好"}],"forwardedProps":{"agent":"agent:Test"}}`
	resp, data := postJSON(t, ts, "/agui", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agui status %d: %s", resp.StatusCode, data)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("content type = %q", got)
	}
	text := string(data)
	for _, want := range []string{
		`"type":"RUN_STARTED"`,
		`"type":"TEXT_MESSAGE_START"`,
		`"delta":"hel"`,
		`"delta":"lo"`,
		`"type":"TEXT_MESSAGE_END"`,
		`"type":"RUN_FINISHED"`,
		`"threadId":"thread-1"`,
		`"runId":"run-1"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in stream:\n%s", want, text)
		}
	}
	if runner.req.Agent != "agent:Test" {
		t.Fatalf("agent = %q", runner.req.Agent)
	}

	body = `{"threadId":"thread-1","runId":"run-2","messages":[{"id":"m2","role":"user","content":"继续"}]}`
	resp, data = postJSON(t, ts, "/agui", body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(data), `"runId":"run-2"`) {
		t.Fatalf("second agui run %d: %s", resp.StatusCode, data)
	}
	all, err := s.cm.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(all) != 1 || len(all[0].Messages) != 4 {
		t.Fatalf("AG-UI thread conversations = %d, messages = %d", len(all), len(all[0].Messages))
	}
}

func TestA2AAgentCardAndMessage(t *testing.T) {
	s := protocolServer(t, &protocolRunner{chunks: []string{"a2a ", "reply"}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, cardBody := getWithHeaders(t, ts, agentCardPath, map[string]string{
		"Authorization": "Bearer " + testWebToken,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("card status %d: %s", resp.StatusCode, cardBody)
	}
	wantURL := ts.URL + "/a2a/invoke"
	if !strings.Contains(string(cardBody), wantURL) {
		t.Fatalf("card does not point at %s: %s", wantURL, cardBody)
	}

	body := `{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m1","role":"user","parts":[{"text":"你好"}]}}}`
	resp, data := postJSON(t, ts, "/a2a/invoke", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("message/send status %d: %s", resp.StatusCode, data)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode: %v\n%s", err, data)
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		t.Fatalf("a2a error: %s", data)
	}
	if !strings.Contains(string(envelope.Result), `"a2a "`) || !strings.Contains(string(envelope.Result), `"reply"`) {
		t.Fatalf("a2a result: %s", envelope.Result)
	}
}

func TestA2AClientAgainstEndpoint(t *testing.T) {
	s := protocolServer(t, &protocolRunner{chunks: []string{"client ", "roundtrip"}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	cli, err := a2aclient.New(context.Background(), config.A2AConnection{
		Name: "local", URL: ts.URL, APIKey: testWebToken,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var chunks []string
	res, err := cli.Chat(context.Background(), a2aclient.ChatRequest{Text: "你好"}, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if res.Text != "client roundtrip" || strings.Join(chunks, "") != res.Text {
		t.Fatalf("text = %q, chunks = %v", res.Text, chunks)
	}
	if res.TaskID == "" || res.ContextID == "" {
		t.Fatalf("missing task identity: %#v", res)
	}

	next, err := cli.Chat(context.Background(), a2aclient.ChatRequest{
		Text: "继续", ContextID: res.ContextID,
	}, nil)
	if err != nil {
		t.Fatalf("continue chat: %v", err)
	}
	if next.Text != "client roundtrip" || next.ContextID != res.ContextID {
		t.Fatalf("continued result = %#v", next)
	}
	all, err := s.cm.ListConversations()
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("A2A context created %d conversations, want 1", len(all))
	}
}

func TestA2ARESTMessageSend(t *testing.T) {
	s := protocolServer(t, &protocolRunner{chunks: []string{"rest ", "reply"}})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	body := `{"message":{"messageId":"m1","role":"user","parts":[{"text":"你好"}]}}`
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/a2a/message:send", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/a2a+json")
	req.Header.Set("Accept", "application/a2a+json")
	req.Header.Set("A2A-Version", "1.0")
	req.Header.Set("Authorization", "Bearer "+testWebToken)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("rest send: %v", err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rest status %d: %s", resp.StatusCode, data)
	}
	if !strings.Contains(string(data), `"rest "`) || !strings.Contains(string(data), `"reply"`) {
		t.Fatalf("rest result: %s", data)
	}
}
