package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"chatgo/pkg/models"
)

func (s *Server) newMCPHandler() http.Handler {
	srv := server.NewMCPServer(
		"ChatGo", "1.0.0",
		server.WithToolCapabilities(false),
		server.WithInstructions("ChatGo exposes its local agent as an MCP tool."),
	)
	srv.AddTool(mcp.NewTool("chat",
		mcp.WithDescription("运行一轮 ChatGo Agent，返回回复文本和用于继续对话的 conversation_id。"),
		mcp.WithString("message", mcp.Required(), mcp.Description("用户消息")),
		mcp.WithString("agent", mcp.Description("Agent 引用，例如 agent:Name、acp:Name、a2a:Name；留空使用普通 Chat")),
		mcp.WithString("workspace", mcp.Description("运行会话的工作区目录")),
		mcp.WithString("provider", mcp.Description("模型提供商名称")),
		mcp.WithString("model", mcp.Description("模型名称")),
		mcp.WithString("conversation_id", mcp.Description("继续已有会话时传入上一次返回的 conversation_id")),
	), s.mcpChat)
	srv.AddTool(mcp.NewTool("list_agents",
		mcp.WithDescription("列出 ChatGo 当前可选的 Agent 及其引用。"),
	), s.mcpListAgents)
	return server.NewStreamableHTTPServer(srv, server.WithStateLess(true))
}

func (s *Server) mcpChat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := req.RequireString("message")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	conv, err := s.runExternal(ctx, externalRequest{
		Text:           text,
		ConversationID: strings.TrimSpace(req.GetString("conversation_id", "")),
		Agent:          strings.TrimSpace(req.GetString("agent", "")),
		Workspace:      strings.TrimSpace(req.GetString("workspace", "")),
		Provider:       strings.TrimSpace(req.GetString("provider", "")),
		Model:          strings.TrimSpace(req.GetString("model", "")),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	result := map[string]any{
		"conversation_id": conv.ID,
		"content":         lastAssistantContent(conv),
	}
	body, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func (s *Server) mcpListAgents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agents := []AgentInfo{{Ref: "", Name: "Chat", Kind: "chat"}}
	if s.runner != nil {
		agents = append(agents, s.runner.Agents()...)
	}
	body, err := json.Marshal(map[string]any{"agents": agents})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(body)), nil
}

func lastAssistantContent(conv *models.Conversation) string {
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if !conv.Messages[i].IsHuman() {
			return conv.Messages[i].Content
		}
	}
	return ""
}
