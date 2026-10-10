package ui

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "github.com/ZacharyZhang-NY/MujicaUI/core"
	"github.com/ZacharyZhang-NY/MujicaUI/icons"
)

// agentAvatar builds the current conversation agent's avatar for bubbles.
func (cw *ChatWindow) agentAvatar(c *mgo.Context) func() {
	if cw.cur == nil {
		return nil
	}
	switch {
	case strings.HasPrefix(cw.cur.Agent, "acp:"):
		if logo := acpLogo(strings.TrimPrefix(cw.cur.Agent, "acp:")); logo != nil {
			return func() { mgo.Image(c, logo).Size(28, 28) }
		}
	case strings.HasPrefix(cw.cur.Agent, "agent:"):
		name := strings.TrimPrefix(cw.cur.Agent, "agent:")
		for _, a := range cw.cfg.Agents {
			if a.Name == name {
				ic := agentIcon(a.Icon)
				return func() { mgo.Icon(c, icons.Must(ic)).Size(24, 24).TextColor(mcore.Tokens(c).AccentText) }
			}
		}
	default:
		return func() { mgo.Icon(c, icons.Must(chatAgentIcon)).Size(24, 24).TextColor(mcore.Tokens(c).AccentText) }
	}
	return nil
}
