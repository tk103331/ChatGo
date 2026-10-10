package ui

import (
	"strings"

	"chatgo/internal/config"
)

// toolMeta describes one selectable tool: a builtin tool or a whole MCP server.
type toolMeta struct {
	ID    string
	Label string
	Desc  string
}

// toolMetas lists every tool the active agent may use: the enabled builtin
// tools and MCP servers, narrowed to the agent's whitelist when it has one.
func (cw *ChatWindow) toolMetas() []toolMeta {
	var out []toolMeta
	for _, name := range cw.visibleBuiltinToolNames() {
		desc := ""
		for _, t := range cw.cfg.BuiltinTools {
			if t.Name == name {
				desc = config.GetBuiltinToolDescription(t.Type)
				break
			}
		}
		out = append(out, toolMeta{ID: "builtin:" + name, Label: name, Desc: desc})
	}
	for _, name := range cw.visibleMCPServerNames() {
		out = append(out, toolMeta{ID: "mcp:" + name, Label: name, Desc: "MCP 服务器"})
	}
	return out
}

// toolEnabled reports whether a tool is selected; unknown tools default on.
func (cw *ChatWindow) toolEnabled(id string) bool {
	if v, ok := cw.toolOn[id]; ok {
		return v
	}
	return true
}

// filterMetas returns the tools whose label or description contains query.
func filterMetas(list []toolMeta, query string) []toolMeta {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	var out []toolMeta
	for _, m := range list {
		if strings.Contains(strings.ToLower(m.Label), q) || strings.Contains(strings.ToLower(m.Desc), q) {
			out = append(out, m)
		}
	}
	return out
}

// selectedToolIDs returns the IDs of the tools the user has left on.
func (cw *ChatWindow) selectedToolIDs() []string {
	var out []string
	for _, m := range cw.toolMetas() {
		if cw.toolEnabled(m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}
