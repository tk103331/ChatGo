package ui

import "chatgo/internal/config"

// toolMeta describes one selectable tool: a builtin tool or an MCP tool.
type toolMeta struct {
	ID    string
	Label string
	Desc  string
}

// toolMetas lists every currently available tool: enabled builtins plus the
// tools of initialized MCP servers.
func (cw *ChatWindow) toolMetas() []toolMeta {
	var out []toolMeta
	for _, t := range cw.cfg.BuiltinTools {
		if !t.Enabled {
			continue
		}
		out = append(out, toolMeta{
			ID:    "builtin:" + t.Name,
			Label: t.Name,
			Desc:  config.GetBuiltinToolDescription(t.Type),
		})
	}
	for _, s := range cw.cfg.MCPServers {
		status, ok := cw.mcpManager.GetServerStatus(s.Name)
		if !ok || status.Status != "initialized" {
			continue
		}
		for _, tl := range status.Tools {
			out = append(out, toolMeta{
				ID:    "mcp:" + s.Name + ":" + tl.Name,
				Label: tl.Name,
				Desc:  tl.Description,
			})
		}
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
