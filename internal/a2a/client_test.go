package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"chatgo/internal/config"
)

type fakeSender struct {
	stream    []sdk.Event
	streamErr error
	msg       sdk.SendMessageResult
	msgErr    error
	last      *sdk.SendMessageRequest
	cancelled sdk.TaskID
}

type roleRetrySender struct {
	fakeSender
	roles []sdk.MessageRole
}

func (s *roleRetrySender) SendMessage(_ context.Context, req *sdk.SendMessageRequest) (sdk.SendMessageResult, error) {
	s.roles = append(s.roles, req.Message.Role)
	if req.Message.Role != sdk.MessageRole("user") {
		return nil, fmt.Errorf("message role must be user")
	}
	return s.msg, nil
}

func (f *fakeSender) SendMessage(_ context.Context, req *sdk.SendMessageRequest) (sdk.SendMessageResult, error) {
	f.last = req
	if f.msgErr != nil {
		return nil, f.msgErr
	}
	return f.msg, nil
}

func (f *fakeSender) SendStreamingMessage(_ context.Context, req *sdk.SendMessageRequest) iter.Seq2[sdk.Event, error] {
	f.last = req
	return func(yield func(sdk.Event, error) bool) {
		for _, ev := range f.stream {
			if !yield(ev, nil) {
				return
			}
		}
		if f.streamErr != nil {
			yield(nil, f.streamErr)
		}
	}
}

func (f *fakeSender) CancelTask(_ context.Context, req *sdk.CancelTaskRequest) (*sdk.Task, error) {
	f.cancelled = req.ID
	return &sdk.Task{ID: req.ID}, nil
}

func testClient(s sender, streaming bool) *Client {
	return &Client{
		card:   &sdk.AgentCard{Capabilities: sdk.AgentCapabilities{Streaming: streaming}},
		sender: s,
	}
}

func artifactEvent(id sdk.ArtifactID, appendPart bool, text string) *sdk.TaskArtifactUpdateEvent {
	return &sdk.TaskArtifactUpdateEvent{
		TaskID:    "t1",
		ContextID: "c1",
		Append:    appendPart,
		Artifact:  &sdk.Artifact{ID: id, Parts: sdk.ContentParts{sdk.NewTextPart(text)}},
	}
}

func TestChatStreamMergesArtifactDeltas(t *testing.T) {
	s := &fakeSender{stream: []sdk.Event{
		&sdk.TaskStatusUpdateEvent{TaskID: "t1", ContextID: "c1", Status: sdk.TaskStatus{State: sdk.TaskStateWorking}},
		artifactEvent("a", false, "Hello"),
		artifactEvent("a", true, " world"),
		artifactEvent("a", false, "Hello world!"),
		&sdk.TaskStatusUpdateEvent{TaskID: "t1", ContextID: "c1", Status: sdk.TaskStatus{State: sdk.TaskStateCompleted}},
	}}
	var chunks []string
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{Text: "hi"}, func(s string) {
		chunks = append(chunks, s)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "Hello world!" {
		t.Fatalf("text = %q, want %q", res.Text, "Hello world!")
	}
	if strings.Join(chunks, "") != res.Text {
		t.Fatalf("chunks = %q, want %q", strings.Join(chunks, ""), res.Text)
	}
	if res.TaskID != "t1" || res.ContextID != "c1" {
		t.Fatalf("ids = %q/%q", res.TaskID, res.ContextID)
	}
	if res.State != sdk.TaskStateCompleted {
		t.Fatalf("state = %q", res.State)
	}
}

func TestChatStreamContinuesTask(t *testing.T) {
	s := &fakeSender{stream: []sdk.Event{
		&sdk.TaskStatusUpdateEvent{TaskID: "t2", ContextID: "c2", Status: sdk.TaskStatus{State: sdk.TaskStateWorking}},
		&sdk.TaskArtifactUpdateEvent{TaskID: "t2", ContextID: "c2", Artifact: &sdk.Artifact{ID: "a", Parts: sdk.ContentParts{sdk.NewTextPart("ok")}}},
	}}
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{
		Text:      "again",
		TaskID:    "t1",
		ContextID: "c1",
	}, func(string) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := s.last.Message.TaskID; got != "t1" {
		t.Fatalf("request task id = %q", got)
	}
	if got := s.last.Message.ContextID; got != "c1" {
		t.Fatalf("request context id = %q", got)
	}
	if res.TaskID != "t2" || res.ContextID != "c2" {
		t.Fatalf("result ids = %q/%q", res.TaskID, res.ContextID)
	}
}

func TestChatStreamSkipsWorkingStatusText(t *testing.T) {
	s := &fakeSender{stream: []sdk.Event{
		&sdk.TaskStatusUpdateEvent{
			TaskID: "t1", ContextID: "c1",
			Status: sdk.TaskStatus{
				State:   sdk.TaskStateWorking,
				Message: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("working...")),
			},
		},
		&sdk.TaskStatusUpdateEvent{
			TaskID: "t1", ContextID: "c1",
			Status: sdk.TaskStatus{
				State:   sdk.TaskStateInputRequired,
				Message: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("which file?")),
			},
		},
	}}
	var chunks []string
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{Text: "hi"}, func(s string) {
		chunks = append(chunks, s)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := strings.Join(chunks, ""); got != "which file?" {
		t.Fatalf("text = %q", got)
	}
	if !res.InputRequired {
		t.Fatal("input-required flag missing")
	}
}

func TestChatFallsBackWhenStreamFails(t *testing.T) {
	s := &fakeSender{
		streamErr: fmt.Errorf("streaming unavailable"),
		msg:       sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("fallback")),
	}
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "fallback" {
		t.Fatalf("text = %q", res.Text)
	}
}

func TestChatRetriesWithLowercaseRole(t *testing.T) {
	s := &roleRetrySender{fakeSender: fakeSender{
		msg: sdk.NewMessage(sdk.MessageRole("agent"), sdk.NewTextPart("lowercase ok")),
	}}
	c := testClient(s, false)

	res, err := c.Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "lowercase ok" {
		t.Fatalf("text = %q", res.Text)
	}
	if len(s.roles) != 2 || s.roles[0] != sdk.MessageRoleUser || s.roles[1] != sdk.MessageRole("user") {
		t.Fatalf("roles = %v", s.roles)
	}
}

func TestChatFallsBackToLegacyTransport(t *testing.T) {
	primary := &fakeSender{msgErr: fmt.Errorf("message role must be user")}
	legacy := &fakeSender{msg: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("legacy ok"))}
	c := testClient(primary, false)
	c.legacy = legacy

	res, err := c.Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "legacy ok" {
		t.Fatalf("text = %q", res.Text)
	}
	if legacy.last == nil || legacy.last.Message.Role != sdk.MessageRoleUser {
		t.Fatalf("legacy request = %#v", legacy.last)
	}
}

func TestChatDoesNotHideUnrelatedErrors(t *testing.T) {
	primary := &fakeSender{msgErr: fmt.Errorf("connection reset")}
	legacy := &fakeSender{msg: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("should not run"))}
	c := testClient(primary, false)
	c.legacy = legacy

	if _, err := c.Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {}); err == nil {
		t.Fatal("expected the connection error")
	}
	if legacy.last != nil {
		t.Fatal("an unrelated error must not retry over the legacy transport")
	}
}

func TestChatStreamKeepsPartialTextOnError(t *testing.T) {
	s := &fakeSender{
		stream:    []sdk.Event{artifactEvent("a", false, "partial")},
		streamErr: fmt.Errorf("connection reset"),
	}
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {})
	if err == nil {
		t.Fatal("expected stream error")
	}
	if res.Text != "partial" {
		t.Fatalf("text = %q", res.Text)
	}
}

func TestChatOnceTaskFailureReportsState(t *testing.T) {
	s := &fakeSender{msg: &sdk.Task{
		ID:        "t1",
		ContextID: "c1",
		Status:    sdk.TaskStatus{State: sdk.TaskStateFailed, Message: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("boom"))},
	}}
	res, err := testClient(s, false).Chat(context.Background(), ChatRequest{Text: "hi"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "TASK_STATE_FAILED") {
		t.Fatalf("err = %v", err)
	}
	if res.Text != "boom" {
		t.Fatalf("text = %q", res.Text)
	}
}

func TestPartsText(t *testing.T) {
	parts := []*sdk.Part{
		sdk.NewTextPart("text "),
		sdk.NewDataPart(map[string]string{"k": "v"}),
		{Content: sdk.URL("https://example.com/x"), Filename: "x.txt"},
		{Content: sdk.Raw("raw"), MediaType: "text/plain"},
		{Content: sdk.Raw{0x89, 0x50}, Filename: "img.png", MediaType: "image/png"},
	}
	want := `text {"k":"v"}[x.txt](https://example.com/x)raw[img.png: 2 bytes]`
	if got := partsText(parts); got != want {
		t.Fatalf("partsText = %q, want %q", got, want)
	}
}

func TestTaskTextPrefersLastAgentMessage(t *testing.T) {
	task := &sdk.Task{
		Status: sdk.TaskStatus{
			State:   sdk.TaskStateCompleted,
			Message: sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("status")),
		},
		History: []*sdk.Message{
			sdk.NewMessage(sdk.MessageRoleUser, sdk.NewTextPart("question")),
			sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("first")),
			sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("last")),
		},
		Artifacts: []*sdk.Artifact{{ID: "a", Parts: sdk.ContentParts{sdk.NewTextPart("artifact")}}},
	}
	if got := taskText(task); got != "last" {
		t.Fatalf("taskText = %q", got)
	}
	task.History = nil
	if got := taskText(task); got != "status" {
		t.Fatalf("status fallback = %q", got)
	}
	task.Status.Message = nil
	if got := taskText(task); got != "artifact" {
		t.Fatalf("artifact fallback = %q", got)
	}
}

func TestAuthHeaders(t *testing.T) {
	got := authHeaders(config.A2AConnection{APIKey: " sk-1 "})
	if got["Authorization"] != "Bearer sk-1" {
		t.Fatalf("authorization = %q", got["Authorization"])
	}
	got = authHeaders(config.A2AConnection{
		APIKey:  "sk-1",
		Headers: map[string]string{"authorization": "Custom token"},
	})
	if got["authorization"] != "Custom token" || len(got) != 1 {
		t.Fatalf("headers = %#v", got)
	}
	got = authHeaders(config.A2AConnection{APIKey: "sk-2", Headers: map[string]string{"Authorization": "  "}})
	if got["Authorization"] != "Bearer sk-2" || len(got) != 1 {
		t.Fatalf("empty authorization = %#v", got)
	}
}

func TestCancelTask(t *testing.T) {
	s := &fakeSender{}
	if err := testClient(s, false).CancelTask(context.Background(), "t9"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
	if s.cancelled != "t9" {
		t.Fatalf("cancelled = %q", s.cancelled)
	}
}

func TestNewChatAgainstJSONRPCServer(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	card := &sdk.AgentCard{
		Name:               "Test Agent",
		Description:        "integration",
		Version:            "1.0",
		Capabilities:       sdk.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		SupportedInterfaces: []*sdk.AgentInterface{
			sdk.NewAgentInterface(srv.URL+"/invoke", sdk.TransportProtocolJSONRPC),
		},
	}
	executor := a2asrv.AgentExecutorFunc(func(context.Context, *a2asrv.ExecutorContext) iter.Seq2[sdk.Event, error] {
		return func(yield func(sdk.Event, error) bool) {
			yield(sdk.NewMessage(sdk.MessageRoleAgent, sdk.NewTextPart("hello from a2a")), nil)
		}
	})
	mux.Handle("/invoke", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(executor)))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))

	cli, err := New(context.Background(), config.A2AConnection{Name: "test", URL: srv.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var chunks []string
	res, err := cli.Chat(context.Background(), ChatRequest{Text: "hi"}, func(s string) {
		chunks = append(chunks, s)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if res.Text != "hello from a2a" || strings.Join(chunks, "") != res.Text {
		t.Fatalf("text = %q, chunks = %v", res.Text, chunks)
	}
	if info := cli.Info(); info.Name != "Test Agent" || !info.Streaming {
		t.Fatalf("info = %#v", info)
	}
}

func TestParseAgentCardV1(t *testing.T) {
	body := []byte(`{
		"name": "v1",
		"version": "1.0",
		"capabilities": {"streaming": true},
		"supportedInterfaces": [
			{"url": "https://agent.example.com/rpc", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"}
		]
	}`)
	card, err := parseAgentCard(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(card.SupportedInterfaces) != 1 || card.SupportedInterfaces[0].ProtocolVersion != "1.0" {
		t.Fatalf("interfaces = %#v", card.SupportedInterfaces)
	}
	if !card.Capabilities.Streaming {
		t.Fatal("streaming capability lost")
	}
}

func TestParseAgentCardV03(t *testing.T) {
	body := []byte(`{
		"name": "legacy",
		"version": "0.3",
		"protocolVersion": "0.3",
		"url": "https://legacy.example.com",
		"preferredTransport": "JSONRPC",
		"capabilities": {"streaming": true},
		"defaultInputModes": ["text/plain"],
		"defaultOutputModes": ["text/plain"],
		"skills": []
	}`)
	card, err := parseAgentCard(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(card.SupportedInterfaces) != 1 {
		t.Fatalf("interfaces = %#v", card.SupportedInterfaces)
	}
	if got := card.SupportedInterfaces[0]; got.URL != "https://legacy.example.com" ||
		got.ProtocolBinding != sdk.TransportProtocolJSONRPC ||
		got.ProtocolVersion != "0.3" {
		t.Fatalf("interface = %#v", got)
	}
}

func TestParseAgentCardOpenAPISecurityScheme(t *testing.T) {
	body := []byte(`{
		"protocolVersion": "1.0",
		"name": "secure",
		"version": "dev",
		"capabilities": {"streaming": true},
		"supportedInterfaces": [
			{"url": "https://agent.example.com/a2a", "protocolBinding": "JSONRPC", "protocolVersion": "1.0"}
		],
		"securitySchemes": {
			"bearer": {
				"type": "http",
				"scheme": "bearer",
				"bearerFormat": "opaque",
				"description": "access token"
			}
		},
		"security": [{"bearer": []}],
		"securityRequirements": [{"bearer": []}],
		"defaultInputModes": ["text/plain"],
		"defaultOutputModes": ["text/plain"]
	}`)
	card, err := parseAgentCard(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(card.SupportedInterfaces) != 1 {
		t.Fatalf("interfaces = %#v", card.SupportedInterfaces)
	}
	scheme, ok := card.SecuritySchemes["bearer"].(sdk.HTTPAuthSecurityScheme)
	if !ok {
		t.Fatalf("scheme = %T, want HTTPAuthSecurityScheme", card.SecuritySchemes["bearer"])
	}
	if scheme.Scheme != "bearer" || scheme.BearerFormat != "opaque" {
		t.Fatalf("scheme = %#v", scheme)
	}
	if len(card.SecurityRequirements) != 1 {
		t.Fatalf("security requirements = %#v", card.SecurityRequirements)
	}
}

func TestChatStreamDeliversA2UILive(t *testing.T) {
	part := sdk.NewDataPart([]any{map[string]any{"version": "1.0", "createSurface": map[string]any{"surfaceId": "form"}}})
	part.MediaType = A2UIMediaType
	part.Metadata = map[string]any{"mimeType": A2UIMediaType}
	s := &fakeSender{stream: []sdk.Event{
		&sdk.TaskStatusUpdateEvent{TaskID: "t1", ContextID: "c1", Status: sdk.TaskStatus{State: sdk.TaskStateWorking}},
		&sdk.TaskArtifactUpdateEvent{TaskID: "t1", ContextID: "c1", Artifact: &sdk.Artifact{ID: "a", Parts: sdk.ContentParts{
			sdk.NewTextPart("hello"), part,
		}}},
		&sdk.TaskStatusUpdateEvent{TaskID: "t1", ContextID: "c1", Status: sdk.TaskStatus{State: sdk.TaskStateCompleted}},
	}}
	var live []string
	res, err := testClient(s, true).Chat(context.Background(), ChatRequest{
		Text: "hi", OnA2UI: func(raw json.RawMessage) { live = append(live, string(raw)) },
	}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.A2UI) != 1 || len(live) != 1 {
		t.Fatalf("a2ui = %d live = %d", len(res.A2UI), len(live))
	}
	if strings.Contains(res.Text, "createSurface") {
		t.Fatalf("A2UI leaked into text: %q", res.Text)
	}
}
