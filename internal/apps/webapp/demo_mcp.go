package webapp

import (
	"encoding/json"
	"net/http"
)

// demoMCP serves a small streamable-HTTP MCP endpoint for the bundled draw
// example. It answers tools/list from the manifest and tools/call by
// forwarding into the live page, so the example demonstrates a web app whose
// backend and frontend share one canvas.
func (cl *Client) demoMCP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]any{"name": cl.app.ID, "version": "1.0.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}
		case "tools/list":
			tools := make([]map[string]any, 0, len(cl.app.Tools))
			for _, t := range cl.app.Tools {
				props := map[string]any{}
				required := []string{}
				for _, p := range t.Params {
					props[p.Name] = map[string]any{"type": "string", "description": p.Description}
					if p.Required {
						required = append(required, p.Name)
					}
				}
				tools = append(tools, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": map[string]any{"type": "object", "properties": props, "required": required},
				})
			}
			result = map[string]any{"tools": tools}
		case "tools/call":
			out, err := cl.call(req.Params.Name, req.Params.Arguments)
			if err != nil {
				result = map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}
			} else {
				result = map[string]any{"content": []any{map[string]any{"type": "text", "text": out}}}
			}
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	})
}
