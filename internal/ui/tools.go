package ui

import (
	"fmt"
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
		out = append(out, toolMeta{ID: "mcp:" + name, Label: name, Desc: cw.mcpServerDesc(name)})
	}
	return out
}

// mcpServerDesc describes an MCP server by what it runs and, once connected,
// how many tools it offers, so the entries are told apart in the picker.
func (cw *ChatWindow) mcpServerDesc(name string) string {
	var s config.MCPServer
	for _, srv := range cw.cfg.MCPServers {
		if srv.Name == name {
			s = srv
			break
		}
	}
	target := strings.TrimSpace(s.Command + " " + strings.Join(s.Args, " "))
	if target == "" {
		target = strings.TrimSpace(s.URL)
	}
	typ := string(s.Type)
	if typ == "" {
		typ = "stdio"
	}
	parts := []string{typ}
	if st, ok := cw.mcpManager.GetServerStatus(name); ok && st.Status == "initialized" {
		parts = append(parts, fmt.Sprintf("%d 个工具", len(st.Tools)))
	}
	if target != "" {
		parts = append(parts, target)
	}
	return strings.Join(parts, " · ")
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
