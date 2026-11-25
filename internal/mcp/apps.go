package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCP Apps (SEP-1865) constants. A server links a tool to an HTML view with
// _meta.ui.resourceUri, and the host reads the resource with resources/read.
const (
	// UIExtension is the extension identifier advertised in initialize.
	UIExtension = "io.modelcontextprotocol/ui"
	// UIResourceMIME is the only view content type the extension defines.
	UIResourceMIME = "text/html;profile=mcp-app"
	// UISpecVersion is the MCP Apps revision this host speaks.
	UISpecVersion = "2026-01-26"
)

// UIExtensionCapability is the client capability that tells a server a host
// can render MCP Apps views.
func UIExtensionCapability() map[string]any {
	return map[string]any{"mimeTypes": []string{UIResourceMIME}}
}

// AppCSP lists the origins a view declared it needs. Empty lists mean the
// view asked for no external access.
type AppCSP struct {
	ConnectDomains  []string `json:"connectDomains,omitempty"`
	ResourceDomains []string `json:"resourceDomains,omitempty"`
	FrameDomains    []string `json:"frameDomains,omitempty"`
	BaseURIDomains  []string `json:"baseUriDomains,omitempty"`
}

// AppPermissions are the browser features a view asked for.
type AppPermissions struct {
	Camera         bool `json:"camera,omitempty"`
	Microphone     bool `json:"microphone,omitempty"`
	Geolocation    bool `json:"geolocation,omitempty"`
	ClipboardWrite bool `json:"clipboardWrite,omitempty"`
}

// AppResource is one UI resource fetched from a server.
type AppResource struct {
	URI         string
	MIMEType    string
	HTML        string
	CSP         AppCSP
	Permissions AppPermissions
	// PrefersBorder is the view's border request; nil means the host chooses.
	PrefersBorder *bool
	// Domain is the dedicated origin a view asked for, host-defined.
	Domain string
}

// ToolUI returns the UI resource a tool links to and its visibility. The
// deprecated flat "_meta.ui/resourceUri" form is read too.
func ToolUI(meta *mcp.Meta) (uri string, visibility []string) {
	if meta == nil {
		return "", nil
	}
	if raw, ok := meta.AdditionalFields["ui"]; ok {
		if m, ok := raw.(map[string]any); ok {
			if s, ok := m["resourceUri"].(string); ok {
				uri = s
			}
			visibility = stringList(m["visibility"])
		}
	}
	if uri == "" {
		if s, ok := meta.AdditionalFields["ui/resourceUri"].(string); ok {
			uri = s
		}
	}
	return uri, visibility
}

// stringList converts a decoded JSON array to strings, ignoring other values.
func stringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ReadUIResource fetches and decodes a view's HTML and its security metadata.
func ReadUIResource(ctx context.Context, cli *client.Client, uri string) (*AppResource, error) {
	if !strings.HasPrefix(uri, "ui://") {
		return nil, fmt.Errorf("不是 MCP Apps 资源：%s", uri)
	}
	res, err := cli.ReadResource(ctx, mcp.ReadResourceRequest{
		Request: mcp.Request{Method: "resources/read"},
		Params:  mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		return nil, fmt.Errorf("读取界面资源失败：%w", err)
	}
	var firstErr error
	for _, rc := range res.Contents {
		out := &AppResource{URI: uri}
		var meta map[string]any
		switch v := rc.(type) {
		case mcp.TextResourceContents:
			out.MIMEType, out.HTML, meta = v.MIMEType, v.Text, v.Meta
		case *mcp.TextResourceContents:
			out.MIMEType, out.HTML, meta = v.MIMEType, v.Text, v.Meta
		case mcp.BlobResourceContents:
			out.MIMEType, meta = v.MIMEType, v.Meta
			b, err := base64.StdEncoding.DecodeString(v.Blob)
			if err != nil {
				firstErr = fmt.Errorf("界面资源不是有效的 base64：%w", err)
				continue
			}
			out.HTML = string(b)
		case *mcp.BlobResourceContents:
			out.MIMEType, meta = v.MIMEType, v.Meta
			b, err := base64.StdEncoding.DecodeString(v.Blob)
			if err != nil {
				firstErr = fmt.Errorf("界面资源不是有效的 base64：%w", err)
				continue
			}
			out.HTML = string(b)
		default:
			continue
		}
		if out.HTML == "" {
			continue
		}
		if out.MIMEType != "" && out.MIMEType != UIResourceMIME {
			firstErr = fmt.Errorf("不支持的界面类型：%s", out.MIMEType)
			continue
		}
		out.MIMEType = UIResourceMIME
		out.readMeta(meta)
		return out, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, errors.New("界面资源没有 HTML 内容")
}

// readMeta decodes "_meta.ui" from a resource's contents.
func (a *AppResource) readMeta(meta map[string]any) {
	if meta == nil {
		return
	}
	ui, _ := meta["ui"].(map[string]any)
	if ui == nil {
		return
	}
	if csp, ok := ui["csp"].(map[string]any); ok {
		a.CSP = AppCSP{
			ConnectDomains:  stringList(csp["connectDomains"]),
			ResourceDomains: stringList(csp["resourceDomains"]),
			FrameDomains:    stringList(csp["frameDomains"]),
			BaseURIDomains:  stringList(csp["baseUriDomains"]),
		}
	}
	if perms, ok := ui["permissions"].(map[string]any); ok {
		a.Permissions = AppPermissions{
			Camera:         perms["camera"] != nil,
			Microphone:     perms["microphone"] != nil,
			Geolocation:    perms["geolocation"] != nil,
			ClipboardWrite: perms["clipboardWrite"] != nil,
		}
	}
	if v, ok := ui["prefersBorder"].(bool); ok {
		a.PrefersBorder = &v
	}
	if s, ok := ui["domain"].(string); ok {
		a.Domain = s
	}
}

// ModelVisible reports whether the agent may see and call the tool.
func (t MCPTool) ModelVisible() bool {
	return visibleTo(t.UIVisibility, "model")
}

// AppVisible reports whether a view of the same server may call the tool.
func (t MCPTool) AppVisible() bool {
	return visibleTo(t.UIVisibility, "app")
}

// visibleTo applies MCP Apps' default visibility, which is both roles.
func visibleTo(visibility []string, role string) bool {
	if len(visibility) == 0 {
		return true
	}
	for _, v := range visibility {
		if v == role {
			return true
		}
	}
	return false
}

// ModelToolNames lists the tools the agent may call, leaving app-only tools
// hidden from the model as the extension requires.
func (s *MCPServerStatus) ModelToolNames() []string {
	names := make([]string, 0, len(s.Tools))
	for _, t := range s.Tools {
		if t.ModelVisible() {
			names = append(names, t.Name)
		}
	}
	return names
}

// AppTool finds a tool a view may call by name.
func (s *MCPServerStatus) AppTool(name string) (MCPTool, bool) {
	for _, t := range s.Tools {
		if t.Name == name && t.AppVisible() {
			return t, true
		}
	}
	return MCPTool{}, false
}

// parseToolJSON decodes an argument or result string into a JSON value.
// Empty or invalid JSON becomes nil rather than failing the view.
func parseToolJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return v
}
