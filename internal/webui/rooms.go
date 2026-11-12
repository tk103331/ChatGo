package webui

import (
	"context"
	"time"

	"chatgo/pkg/models"
)

// RoomInfo is one group chat as the browser sees it.
type RoomInfo struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Workspace          string           `json:"workspace,omitempty"`
	Activation         string           `json:"activation"`
	Master             string           `json:"master,omitempty"`
	AllowSelfResponses bool             `json:"allow_self_responses,omitempty"`
	Members            []RoomMemberInfo `json:"members"`
	Sessions           int              `json:"sessions"`
	Updated            time.Time        `json:"updated,omitempty"`
	Running            bool             `json:"running,omitempty"`
}

// RoomMemberInfo is one agent in a room.
type RoomMemberInfo struct {
	Agent         string `json:"agent"`
	Name          string `json:"name,omitempty"`
	Muted         bool   `json:"muted,omitempty"`
	Talkativeness int    `json:"talkativeness,omitempty"`
}

// RoomInput is a room's editable fields.
type RoomInput struct {
	Name               string           `json:"name"`
	Workspace          string           `json:"workspace,omitempty"`
	Activation         string           `json:"activation,omitempty"`
	Master             string           `json:"master,omitempty"`
	AllowSelfResponses bool             `json:"allow_self_responses,omitempty"`
	Members            []RoomMemberInfo `json:"members"`
}

// RoomSessionInfo is one conversation of a room or task.
type RoomSessionInfo struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Updated time.Time `json:"updated"`
}

// RoomRunner is implemented by the desktop runtime; the standalone server has
// no rooms and hides their controls.
type RoomRunner interface {
	Rooms() []RoomInfo
	CreateRoom(in RoomInput) (RoomInfo, error)
	UpdateRoom(id string, in RoomInput) (RoomInfo, error)
	DeleteRoom(id string) error
	RoomSessions(id string) []RoomSessionInfo
	CreateRoomSession(id string) (RoomSessionInfo, error)
	// RunRoom runs one turn: every selected member answers into conv in order,
	// reporting through live. The server appends the user's message first.
	RunRoom(ctx context.Context, roomID string, conv *models.Conversation, text string, live Live) error
}
