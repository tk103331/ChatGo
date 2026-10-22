package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/internal/ui/icons"
	mcore "chatgo/internal/ui/uikit"
)

// agentAvatar builds the current conversation agent's avatar for bubbles.
func (cw *ChatWindow) agentAvatar(c *mgo.Context) func() {
	if cw.cur == nil {
		return nil
	}
	switch {
	case strings.HasPrefix(cw.cur.Agent, "acp:"):
		name := strings.TrimPrefix(cw.cur.Agent, "acp:")
		return func() { cw.acpLeading(c, name, 28) }
	case strings.HasPrefix(cw.cur.Agent, "agent:"):
		name := strings.TrimPrefix(cw.cur.Agent, "agent:")
		return func() { cw.agentMarkByName(c, name, 24) }
	default:
		return func() { mgo.Icon(c, icons.Must(chatAgentIcon)).Size(24, 24).TextColor(mcore.Tokens(c).Accent) }
	}
	return nil
}
