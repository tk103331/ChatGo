// Package toolview names a tool call for display. The desktop conversation and
// the browser client render tool traces the same way, so the wording lives
// here rather than in either UI.
package toolview

import (
	"encoding/json"
	"strings"
)

// Kinds mirror the ACP tool categories. A built-in call infers one from its
// name and arguments so both surfaces draw the same mark.
const (
	KindRead    = "read"
	KindEdit    = "edit"
	KindDelete  = "delete"
	KindMove    = "move"
	KindSearch  = "search"
	KindExecute = "execute"
	KindThink   = "think"
	KindFetch   = "fetch"
	KindSwitch  = "switch_mode"
	KindOther   = "other"
)

// Label names a tool category for display.
func Label(kind string) string {
	switch kind {
	case KindRead:
		return "读取"
	case KindEdit:
		return "编辑"
	case KindDelete:
		return "删除"
	case KindMove:
		return "移动"
	case KindSearch:
		return "搜索"
	case KindExecute:
		return "执行"
	case KindThink:
		return "思考"
	case KindFetch:
		return "获取"
	case KindSwitch:
		return "切换模式"
	default:
		return ""
	}
}

// Icon names the icon a category reads as.
func Icon(kind string) string {
	switch kind {
	case KindRead:
		return "file-text"
	case KindEdit:
		return "pencil"
	case KindDelete:
		return "circle-x"
	case KindMove:
		return "move"
	case KindSearch:
		return "search"
	case KindExecute:
		return "terminal"
	case KindThink:
		return "brain"
	case KindFetch:
		return "globe"
	case KindSwitch:
		return "sliders-horizontal"
	default:
		return "wrench"
	}
}

// Kind infers a built-in tool's category from its name and arguments, the way
// an ACP adapter reports one explicitly. The argument action wins when it
// names a concrete verb, so a single "file" tool splits into read and write
// rows.
func Kind(name, args string) string {
	lower := strings.ToLower(name)
	action := strings.ToLower(ArgField(args, "action"))
	switch {
	case action == "read" || action == "list" || action == "cat":
		return KindRead
	case action == "write" || action == "create" || action == "update" || action == "apply":
		return KindEdit
	case action == "delete" || action == "remove":
		return KindDelete
	case action == "search" || action == "find" || action == "query":
		return KindSearch
	}
	switch {
	case strings.Contains(lower, "delete") || strings.Contains(lower, "remove"):
		return KindDelete
	case strings.Contains(lower, "move") || strings.Contains(lower, "rename"):
		return KindMove
	case strings.Contains(lower, "search") || strings.Contains(lower, "grep") ||
		strings.Contains(lower, "find") || strings.Contains(lower, "glob") ||
		strings.Contains(lower, "knowledge"):
		return KindSearch
	case strings.Contains(lower, "exec") || strings.Contains(lower, "command") ||
		strings.Contains(lower, "shell") || strings.Contains(lower, "bash") ||
		strings.Contains(lower, "run") || strings.Contains(lower, "sandbox"):
		return KindExecute
	case strings.Contains(lower, "http") || strings.Contains(lower, "fetch") ||
		strings.Contains(lower, "web") || strings.Contains(lower, "url"):
		return KindFetch
	case strings.Contains(lower, "write") || strings.Contains(lower, "edit") ||
		strings.Contains(lower, "create") || strings.Contains(lower, "patch"):
		return KindEdit
	case strings.Contains(lower, "read") || strings.Contains(lower, "file") ||
		strings.Contains(lower, "cat") || strings.Contains(lower, "list"):
		return KindRead
	}
	return KindOther
}

// ArgField reads one string field out of a tool call's JSON arguments.
func ArgField(args, field string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

// ArgPath is the file a call works on, empty when its arguments name none.
func ArgPath(args string) string {
	var raw any
	if err := json.Unmarshal([]byte(args), &raw); err != nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"path", "file", "file_path", "filePath", "filepath", "target_file", "targetFile", "filename", "fileName"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ArgSummary is the one-line summary of a call: its most telling argument, or
// nothing when the arguments say nothing useful.
func ArgSummary(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return ""
	}
	for _, key := range []string{"command", "cmd", "path", "file", "query", "pattern", "url", "name"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
