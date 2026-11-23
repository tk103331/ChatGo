package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgo/internal/config"
)

func TestChatStreamsText(t *testing.T) {
	var got struct {
		ThreadID string `json:"threadId"`
		RunID    string `json:"runId"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode: %v", err)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range []string{
			`{"type":"RUN_STARTED"}`,
			`{"type":"TEXT_MESSAGE_START","messageId":"m1"}`,
			`{"type":"TEXT_MESSAGE_CONTENT","delta":"你"}`,
			`{"type":"TEXT_MESSAGE_CONTENT","delta":"好"}`,
			`{"type":"RUN_FINISHED"}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", ev)
		}
	}))
	defer srv.Close()

	cli, err := New(config.AGUIConnection{Name: "A", URL: srv.URL, APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	var chunks []string
	res, err := cli.Chat(context.Background(), ChatRequest{Text: "hi"}, func(s string) { chunks = append(chunks, s) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "你好" || strings.Join(chunks, "") != "你好" {
		t.Fatalf("result = %+v chunks=%v", res, chunks)
	}
	if got.ThreadID == "" || got.RunID == "" || res.ThreadID != got.ThreadID {
		t.Fatalf("thread = %q run = %q result = %+v", got.ThreadID, got.RunID, res)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hi" || got.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", got.Messages)
	}
}

func TestChatContinuesThread(t *testing.T) {
	var gotThread string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ThreadID string `json:"threadId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		gotThread = in.ThreadID
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\"}\n\n")
	}))
	defer srv.Close()
	cli, _ := New(config.AGUIConnection{Name: "A", URL: srv.URL})
	if _, err := cli.Chat(context.Background(), ChatRequest{Text: "hi", ThreadID: "thread-1"}, nil); err != nil {
		t.Fatal(err)
	}
	if gotThread != "thread-1" {
		t.Fatalf("thread = %q", gotThread)
	}
}

func TestChatReportsRunError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"RUN_ERROR\",\"message\":\"boom\"}\n\n")
	}))
	defer srv.Close()
	cli, _ := New(config.AGUIConnection{Name: "A", URL: srv.URL})
	if _, err := cli.Chat(context.Background(), ChatRequest{Text: "hi"}, nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestPingAndPlainResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	cli, _ := New(config.AGUIConnection{Name: "A", URL: srv.URL})
	if note, err := cli.Ping(context.Background()); err != nil || note != "HTTP 204" {
		t.Fatalf("ping = %q %v", note, err)
	}
	if _, err := cli.Chat(context.Background(), ChatRequest{Text: "hi"}, nil); err == nil {
		t.Fatal("a JSON reply should not count as a run")
	}
}
