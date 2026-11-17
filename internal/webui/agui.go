package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"chatgo/pkg/models"
)

// AG-UI protocol event types used by the endpoint. The wire format is one
// data-only SSE frame per event, as AG-UI clients expect.
const (
	aguiRunStarted         = "RUN_STARTED"
	aguiRunFinished        = "RUN_FINISHED"
	aguiRunError           = "RUN_ERROR"
	aguiTextMessageStart   = "TEXT_MESSAGE_START"
	aguiTextMessageContent = "TEXT_MESSAGE_CONTENT"
	aguiTextMessageEnd     = "TEXT_MESSAGE_END"
	aguiToolCallStart      = "TOOL_CALL_START"
	aguiToolCallArgs       = "TOOL_CALL_ARGS"
	aguiToolCallEnd        = "TOOL_CALL_END"
	aguiToolCallResult     = "TOOL_CALL_RESULT"
)

type aguiRunInput struct {
	ThreadID        string         `json:"threadId"`
	RunID           string         `json:"runId"`
	Messages        []aguiMessage  `json:"messages"`
	State           any            `json:"state,omitempty"`
	Tools           []any          `json:"tools,omitempty"`
	Context         []any          `json:"context,omitempty"`
	ForwardedProps  map[string]any `json:"forwardedProps,omitempty"`
	ProtocolVersion string         `json:"protocolVersion,omitempty"`
}

type aguiMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content,omitempty"`
	Name    string          `json:"name,omitempty"`
}

func (m aguiMessage) text() string {
	if len(m.Content) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var out strings.Builder
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" {
				out.WriteString(part.Text)
			}
		}
		return out.String()
	}
	return ""
}

func (s *Server) handleAGUIOptions(w http.ResponseWriter, r *http.Request) {
	setProtocolCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAGUIRun(w http.ResponseWriter, r *http.Request) {
	var input aguiRunInput
	if err := readJSONLimit(r, &input, 4<<20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(input.ThreadID) == "" || strings.TrimSpace(input.RunID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("threadId 和 runId 不能为空"))
		return
	}
	text := ""
	for i := len(input.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(input.Messages[i].Role, "user") {
			text = strings.TrimSpace(input.Messages[i].text())
			if text != "" {
				break
			}
		}
	}
	if text == "" {
		writeError(w, http.StatusBadRequest, errors.New("messages 中没有可用的 user 消息"))
		return
	}

	flusher, _ := w.(http.Flusher)
	stream := &aguiStream{
		w: w, flusher: flusher, messageID: newID(),
		threadID: input.ThreadID, runID: input.RunID,
	}
	setProtocolCORS(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	stream.send(map[string]any{
		"type":     aguiRunStarted,
		"threadId": input.ThreadID,
		"runId":    input.RunID,
	})

	options := optionsFromMap(input.ForwardedProps)
	threadKey := "agui:" + input.ThreadID
	_, err := s.runExternal(r.Context(), externalRequest{
		Text:           text,
		ConversationID: s.threadConversation(threadKey),
		Agent:          options.Agent,
		Workspace:      options.Workspace,
		Provider:       options.Provider,
		Model:          options.Model,
		Tools:          options.Tools,
		Skills:         options.Skills,
		OnStart: func(conv *models.Conversation) {
			s.bindThread(threadKey, conv.ID)
		},
		OnUpdate: stream.update,
	})

	stream.finish(err)
}

type aguiStream struct {
	w         http.ResponseWriter
	flusher   http.Flusher
	messageID string
	threadID  string
	runID     string
	text      string
	started   bool
	tools     map[string]*aguiToolState
}

type aguiToolState struct {
	name       string
	args       string
	started    bool
	ended      bool
	resultSent bool
}

func (s *aguiStream) send(event map[string]any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", data); err != nil {
		return
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *aguiStream) update(u turnUpdate) {
	if u.MessageID != "" {
		s.messageID = u.MessageID
	}
	if !s.started {
		s.started = true
		s.send(map[string]any{
			"type":      aguiTextMessageStart,
			"messageId": s.messageID,
			"role":      "assistant",
		})
	}
	s.emitText(u.Content)
	s.emitTools(u.ToolCalls)
}

func (s *aguiStream) emitText(content string) {
	if content == s.text {
		return
	}
	delta := ""
	if strings.HasPrefix(content, s.text) {
		delta = content[len(s.text):]
	} else {
		// A retried or rewritten reply replaces the streamed text. Send the
		// replacement as one chunk so the client still converges.
		delta = content
	}
	s.text = content
	if delta != "" {
		s.send(map[string]any{
			"type":      aguiTextMessageContent,
			"messageId": s.messageID,
			"delta":     delta,
		})
	}
}

func (s *aguiStream) emitTools(calls []models.ToolCall) {
	if s.tools == nil {
		s.tools = map[string]*aguiToolState{}
	}
	for i := range calls {
		call := &calls[i]
		if call.ID == "" {
			continue
		}
		state := s.tools[call.ID]
		if state == nil {
			state = &aguiToolState{}
			s.tools[call.ID] = state
		}
		if !state.started {
			state.started = true
			state.name = call.Name
			s.send(map[string]any{
				"type":            aguiToolCallStart,
				"toolCallId":      call.ID,
				"toolCallName":    call.Name,
				"parentMessageId": s.messageID,
			})
		}
		if call.Name != "" && state.name == "" {
			state.name = call.Name
		}
		if call.Arguments != "" && call.Arguments != state.args {
			delta := call.Arguments
			if strings.HasPrefix(call.Arguments, state.args) {
				delta = call.Arguments[len(state.args):]
			}
			state.args = call.Arguments
			if delta != "" {
				s.send(map[string]any{
					"type":       aguiToolCallArgs,
					"toolCallId": call.ID,
					"delta":      delta,
				})
			}
		}
		result := call.Result
		if call.Error != "" {
			result = call.Error
		}
		if result == "" {
			continue
		}
		if !state.ended {
			state.ended = true
			s.send(map[string]any{
				"type":       aguiToolCallEnd,
				"toolCallId": call.ID,
			})
		}
		if !state.resultSent {
			state.resultSent = true
			s.send(map[string]any{
				"type":       aguiToolCallResult,
				"messageId":  newID(),
				"toolCallId": call.ID,
				"content":    result,
			})
		}
	}
}

func (s *aguiStream) finish(err error) {
	if !s.started {
		s.started = true
		s.send(map[string]any{
			"type":      aguiTextMessageStart,
			"messageId": s.messageID,
			"role":      "assistant",
		})
	}
	s.send(map[string]any{
		"type":      aguiTextMessageEnd,
		"messageId": s.messageID,
	})
	if err != nil {
		s.send(map[string]any{
			"type":    aguiRunError,
			"message": err.Error(),
		})
		return
	}
	s.send(map[string]any{
		"type":     aguiRunFinished,
		"threadId": s.threadID,
		"runId":    s.runID,
	})
}

func setProtocolCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func withProtocolCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setProtocolCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
