package mobileapi

import (
	"context"
	"net/url"

	"chatgo/pkg/models"
)

func (c *Client) Meta(ctx context.Context) (Meta, error) {
	var out Meta
	err := c.do(ctx, "GET", "/api/meta", nil, &out)
	return out, err
}

func (c *Client) Sessions(ctx context.Context) ([]SessionSummary, error) {
	var out struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	err := c.do(ctx, "GET", "/api/sessions", nil, &out)
	return out.Sessions, err
}

func (c *Client) Session(ctx context.Context, id string) (SessionResponse, error) {
	var out SessionResponse
	err := c.do(ctx, "GET", "/api/sessions/"+url.PathEscape(id), nil, &out)
	return out, err
}

// RenameSession changes a conversation's title without touching its messages.
func (c *Client) RenameSession(ctx context.Context, id, title string) error {
	return c.do(ctx, "PATCH", "/api/sessions/"+url.PathEscape(id), map[string]string{"title": title}, nil)
}

// DeleteSession removes a conversation and its messages.
func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/sessions/"+url.PathEscape(id), nil, nil)
}

func (c *Client) CreateSession(ctx context.Context, title, agent, workspace, provider, model string) (SessionResponse, error) {
	return c.CreateSessionWithACP(ctx, title, agent, workspace, provider, model, nil)
}

// CreateSessionWithACP creates a conversation with the ACP config choices its
// first turn should start with, for an agent whose model is one of them.
func (c *Client) CreateSessionWithACP(ctx context.Context, title, agent, workspace, provider, model string, acpConfig map[string]string) (SessionResponse, error) {
	body := map[string]any{"title": title}
	if agent != "" {
		body["agent"] = agent
	}
	if workspace != "" {
		body["workspace"] = workspace
	}
	if provider != "" {
		body["provider"] = provider
	}
	if model != "" {
		body["model"] = model
	}
	if len(acpConfig) > 0 {
		body["acp_config"] = acpConfig
	}
	var out struct {
		Session models.Conversation `json:"session"`
	}
	err := c.do(ctx, "POST", "/api/sessions", body, &out)
	return SessionResponse{Session: out.Session}, err
}

func (c *Client) Send(ctx context.Context, id, text string, attachments []Attachment, provider, model string) (SendResult, error) {
	var out SendResult
	body := map[string]any{"text": text}
	if len(attachments) > 0 {
		body["attachments"] = attachments
	}
	if provider != "" {
		body["provider"] = provider
	}
	if model != "" {
		body["model"] = model
	}
	err := c.do(ctx, "POST", "/api/sessions/"+url.PathEscape(id)+"/messages", body, &out)
	return out, err
}

func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.do(ctx, "POST", "/api/sessions/"+url.PathEscape(id)+"/cancel", nil, nil)
}

func (c *Client) Answer(ctx context.Context, id, interactionID, answer string) error {
	return c.do(ctx, "POST", "/api/sessions/"+url.PathEscape(id)+"/interaction",
		map[string]any{"id": interactionID, "answer": answer}, nil)
}

func (c *Client) Decide(ctx context.Context, id, interactionID, optionID string, canceled bool) error {
	return c.do(ctx, "POST", "/api/sessions/"+url.PathEscape(id)+"/interaction",
		map[string]any{"id": interactionID, "option_id": optionID, "canceled": canceled}, nil)
}

func (c *Client) UploadAttachment(ctx context.Context, id, name, base64Data string) (Attachment, error) {
	var out struct {
		Attachment Attachment `json:"attachment"`
	}
	err := c.do(ctx, "POST", "/api/sessions/"+url.PathEscape(id)+"/attachments",
		map[string]any{"name": name, "data": base64Data}, &out)
	return out.Attachment, err
}

func (c *Client) Agents(ctx context.Context) ([]Agent, error) {
	var out struct {
		Agents []Agent `json:"agents"`
	}
	err := c.do(ctx, "GET", "/api/agents", nil, &out)
	return out.Agents, err
}

func (c *Client) Models(ctx context.Context, agent string) ([]Model, error) {
	var out struct {
		Models []Model `json:"models"`
	}
	path := "/api/models"
	if agent != "" {
		path += "?agent=" + url.QueryEscape(agent)
	}
	err := c.do(ctx, "GET", path, nil, &out)
	return out.Models, err
}

// ACPOption is one config choice an ACP agent's runtime offers: the model it
// runs, a mode, or a toggle.
type ACPOption struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Current string     `json:"current"`
	Kind    string     `json:"kind,omitempty"` // select, model or boolean
	Options []ACPValue `json:"options,omitempty"`
}

// ACPValue is one value an ACP config choice takes.
type ACPValue struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

// ACPOptions reads the config choices an ACP connection's warm-up found. The
// first call may wait for that warm-up, so the request's deadline must allow
// it.
func (c *Client) ACPOptions(ctx context.Context, agent string) ([]ACPOption, error) {
	var out struct {
		Options []ACPOption `json:"options"`
	}
	err := c.do(ctx, "GET", "/api/acp-options?agent="+url.QueryEscape(agent), nil, &out)
	return out.Options, err
}

// SessionACPOptions reads the config choices of a live ACP conversation,
// warming its session in when it has not started yet.
func (c *Client) SessionACPOptions(ctx context.Context, id string) ([]ACPOption, error) {
	var out struct {
		Options []ACPOption `json:"options"`
	}
	err := c.do(ctx, "GET", "/api/sessions/"+url.PathEscape(id)+"/acp-options", nil, &out)
	return out.Options, err
}

// SetSessionConfig applies one ACP config choice to a live conversation.
func (c *Client) SetSessionConfig(ctx context.Context, id, configID, value string) error {
	return c.do(ctx, "PUT", "/api/sessions/"+url.PathEscape(id)+"/acp-config", map[string]string{
		"id": configID, "value": value,
	}, nil)
}

func (c *Client) Workspaces(ctx context.Context) ([]string, error) {
	var out struct {
		Workspaces []string `json:"workspaces"`
	}
	err := c.do(ctx, "GET", "/api/workspaces", nil, &out)
	return out.Workspaces, err
}

func (c *Client) Rooms(ctx context.Context) ([]Room, error) {
	var out struct {
		Rooms []Room `json:"rooms"`
	}
	err := c.do(ctx, "GET", "/api/rooms", nil, &out)
	return out.Rooms, err
}

func (c *Client) CreateRoom(ctx context.Context, in RoomInput) (Room, error) {
	var out struct {
		Room Room `json:"room"`
	}
	err := c.do(ctx, "POST", "/api/rooms", in, &out)
	return out.Room, err
}

func (c *Client) UpdateRoom(ctx context.Context, id string, in RoomInput) (Room, error) {
	var out struct {
		Room Room `json:"room"`
	}
	err := c.do(ctx, "PATCH", "/api/rooms/"+url.PathEscape(id), in, &out)
	return out.Room, err
}

func (c *Client) DeleteRoom(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/rooms/"+url.PathEscape(id), nil, nil)
}

func (c *Client) RoomSessions(ctx context.Context, id string) ([]RoomSession, error) {
	var out struct {
		Sessions []RoomSession `json:"sessions"`
	}
	err := c.do(ctx, "GET", "/api/rooms/"+url.PathEscape(id)+"/sessions", nil, &out)
	return out.Sessions, err
}

func (c *Client) CreateRoomSession(ctx context.Context, id string) (RoomSession, error) {
	var out struct {
		Session RoomSession `json:"session"`
	}
	err := c.do(ctx, "POST", "/api/rooms/"+url.PathEscape(id)+"/sessions", nil, &out)
	return out.Session, err
}

func (c *Client) RoomSend(ctx context.Context, roomID, convID, text string, attachments []Attachment) (SendResult, error) {
	var out SendResult
	body := map[string]any{"text": text}
	if len(attachments) > 0 {
		body["attachments"] = attachments
	}
	err := c.do(ctx, "POST", "/api/rooms/"+url.PathEscape(roomID)+"/sessions/"+url.PathEscape(convID)+"/messages", body, &out)
	return out, err
}

func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	var out struct {
		Tasks []Task `json:"tasks"`
	}
	err := c.do(ctx, "GET", "/api/tasks", nil, &out)
	return out.Tasks, err
}

func (c *Client) CreateTask(ctx context.Context, in TaskInput) (Task, error) {
	var out struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, "POST", "/api/tasks", in, &out)
	return out.Task, err
}

func (c *Client) UpdateTask(ctx context.Context, id string, in TaskInput) (Task, error) {
	var out struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, "PATCH", "/api/tasks/"+url.PathEscape(id), in, &out)
	return out.Task, err
}

func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/tasks/"+url.PathEscape(id), nil, nil)
}

func (c *Client) RunTask(ctx context.Context, id string) (SendResult, error) {
	var out SendResult
	err := c.do(ctx, "POST", "/api/tasks/"+url.PathEscape(id)+"/run", nil, &out)
	return out, err
}

func (c *Client) TaskSessions(ctx context.Context, id string) ([]RoomSession, error) {
	var out struct {
		Sessions []RoomSession `json:"sessions"`
	}
	err := c.do(ctx, "GET", "/api/tasks/"+url.PathEscape(id)+"/sessions", nil, &out)
	return out.Sessions, err
}
