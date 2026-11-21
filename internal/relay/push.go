package relay

import (
	"context"
	"log"
)

// Notification is a metadata-only push. It never carries prompts or replies.
type Notification struct {
	DesktopID string   `json:"desktop_id"`
	DeviceIDs []string `json:"device_ids,omitempty"`
	Kind      string   `json:"kind"`
	SessionID string   `json:"session_id,omitempty"`
	Title     string   `json:"title,omitempty"`
	Body      string   `json:"body,omitempty"`
}

// Pusher delivers notifications to APNs or FCM. The relay keeps the push
// credentials; a nil Pusher logs and drops, which is useful in development.
type Pusher interface {
	Push(ctx context.Context, targets []PushTarget, n Notification) error
}

// LogPusher logs notifications instead of sending them.
type LogPusher struct{ Logger *log.Logger }

func (p LogPusher) Push(_ context.Context, targets []PushTarget, n Notification) error {
	if p.Logger == nil {
		return nil
	}
	p.Logger.Printf("push: kind=%s desktop=%s devices=%d", n.Kind, n.DesktopID, len(targets))
	return nil
}
