package webui

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"chatgo/pkg/models"
)

const (
	a2aInvokePath = "/a2a/invoke"
	agentCardPath = "/.well-known/agent-card.json"
)

func (s *Server) newA2AHandlers() (jsonrpc, rest http.Handler) {
	executor := a2asrv.AgentExecutorFunc(s.a2aExecute)
	handler := a2asrv.NewHandler(executor)
	return a2asrv.NewJSONRPCHandler(handler), a2asrv.NewRESTHandler(handler)
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := &a2a.AgentCard{
		Name:        "ChatGo",
		Description: "ChatGo 本地 Agent，可使用已配置的模型、工具、MCP 服务与 ACP / A2A Agent。",
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(requestBaseURL(r)+a2aInvokePath, a2a.TransportProtocolJSONRPC),
			a2a.NewAgentInterface(requestBaseURL(r)+"/a2a", a2a.TransportProtocolHTTPJSON),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		Skills: []a2a.AgentSkill{{
			ID:          "chat",
			Name:        "Chat",
			Description: "运行一轮 ChatGo 对话，可继续已有的 A2A 上下文。",
			Tags:        []string{"chat", "agent", "coding"},
			Examples:    []string{"你好", "帮我检查当前工作区的测试"},
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(card); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func (s *Server) handleA2AOptions(w http.ResponseWriter, r *http.Request) {
	setProtocolCORS(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) a2aExecute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if ec.Message == nil {
			yield(nil, errors.New("A2A 请求缺少消息"))
			return
		}
		text := strings.TrimSpace(a2aMessageText(ec.Message))
		if text == "" {
			yield(nil, errors.New("A2A 消息中没有文本内容"))
			return
		}

		if ec.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}

		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		updates := make(chan turnUpdate, 64)
		done := make(chan error, 1)
		threadKey := "a2a:" + ec.ContextID
		options := optionsFromMap(mergedMetadata(ec))
		go func() {
			_, err := s.runExternal(runCtx, externalRequest{
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
				OnUpdate: func(u turnUpdate) {
					select {
					case updates <- u:
					case <-runCtx.Done():
					}
				},
			})
			done <- err
		}()

		var artifactID a2a.ArtifactID
		lastText := ""
		emitText := func(content string) bool {
			if content == lastText || !strings.HasPrefix(content, lastText) {
				return true
			}
			delta := content[len(lastText):]
			lastText = content
			if delta == "" {
				return true
			}
			parts := []*a2a.Part{a2a.NewTextPart(delta)}
			if artifactID == "" {
				event := a2a.NewArtifactEvent(ec, parts...)
				artifactID = event.Artifact.ID
				return yield(event, nil)
			}
			return yield(a2a.NewArtifactUpdateEvent(ec, artifactID, parts...), nil)
		}

		for {
			select {
			case u := <-updates:
				if !emitText(u.Content) {
					return
				}
			case err := <-done:
				for {
					select {
					case u := <-updates:
						if !emitText(u.Content) {
							return
						}
					default:
						if err != nil {
							msg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))
							yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed, msg), nil)
							return
						}
						yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCompleted, nil), nil)
						return
					}
				}
			}
		}
	}
}

func a2aMessageText(m *a2a.Message) string {
	var out strings.Builder
	for _, part := range m.Parts {
		out.WriteString(part.Text())
	}
	return out.String()
}

func mergedMetadata(ec *a2asrv.ExecutorContext) map[string]any {
	meta := make(map[string]any, len(ec.Metadata))
	for k, v := range ec.Metadata {
		meta[k] = v
	}
	if ec.Message != nil {
		for k, v := range ec.Message.Metadata {
			meta[k] = v
		}
	}
	return meta
}
