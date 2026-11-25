package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// AppCallbacks connect a view to the host application. Every field is
// optional; a nil function leaves that feature unavailable to the view.
type AppCallbacks struct {
	// OpenLink opens an http(s) or mailto URL in the user's browser.
	OpenLink func(url string) error
	// PostMessage hands text to the chat composer, where the user confirms it.
	PostMessage func(text string) error
	// UpdateModelContext stores context the view wants the model to see in
	// later turns. content is the view's text, structured is its JSON.
	UpdateModelContext func(content, structured json.RawMessage)
	// Log records a notifications/message entry from the view.
	Log func(level, message string)
	// Theme reports the host's current appearance: "light" or "dark".
	Theme func() string
	// Locale reports the host's BCP 47 locale.
	Locale func() string
	// Size reports the room the view has, in pixels.
	Size func() (width, height int)
}

// AppOptions describe one view to render.
type AppOptions struct {
	Server    string
	Tool      MCPTool
	ToolID    string
	Arguments string
	Result    string
	Error     string
	Resource  *AppResource
	Version   string
	Callbacks AppCallbacks
	// OnSize reports the size the view asked for with size-changed, in CSS
	// pixels. An inline host sizes its element from it.
	OnSize func(width, height int)
}

// AppHost is one sandboxed MCP Apps view: the resource HTML, the JSON-RPC
// bridge to it, and the server connection tools are proxied to.
type AppHost struct {
	token string
	opts  AppOptions
	cli   *client.Client
	tools map[string]MCPTool

	out    chan []byte
	closed chan struct{}
	once   sync.Once

	mu          sync.Mutex
	initialized bool
	displayMode string
	appModes    []string
	inputSent   bool
	resultSent  bool
}

// AppManager serves every open view. Register its ServeHTTP as a custom
// scheme before the app's window is created, so views can load.
type AppManager struct {
	mu   sync.Mutex
	apps map[string]*AppHost
}

// NewAppManager creates an empty registry of views.
func NewAppManager() *AppManager {
	return &AppManager{apps: map[string]*AppHost{}}
}

// Open registers a view from opts, using cli for tools/call and resources/read.
func (m *AppManager) Open(cli *client.Client, status *MCPServerStatus, opts AppOptions) (*AppHost, error) {
	if opts.Resource == nil || opts.Resource.HTML == "" {
		return nil, fmt.Errorf("界面资源为空")
	}
	token, err := appToken()
	if err != nil {
		return nil, err
	}
	a := &AppHost{
		token:       token,
		opts:        opts,
		cli:         cli,
		tools:       map[string]MCPTool{},
		out:         make(chan []byte, 64),
		closed:      make(chan struct{}),
		displayMode: "fullscreen",
	}
	if status != nil {
		for _, t := range status.Tools {
			if t.AppVisible() {
				a.tools[t.Name] = t
			}
		}
	}
	m.mu.Lock()
	m.apps[token] = a
	m.mu.Unlock()
	return a, nil
}

// appToken returns a random URL host safe for a custom scheme.
func appToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Close removes the view, first asking it to tear down. The registry entry
// stays a moment so the teardown request can still be delivered.
func (m *AppManager) Close(a *AppHost) {
	if a == nil {
		return
	}
	a.push(map[string]any{
		"jsonrpc": "2.0", "id": "teardown", "method": "ui/resource-teardown",
		"params": map[string]any{"reason": "view closed"},
	})
	time.AfterFunc(time.Second, func() {
		m.mu.Lock()
		delete(m.apps, a.token)
		m.mu.Unlock()
		a.once.Do(func() { close(a.closed) })
	})
}

// CloseAll tears down every open view, as when the window closes.
func (m *AppManager) CloseAll() {
	m.mu.Lock()
	apps := make([]*AppHost, 0, len(m.apps))
	for _, a := range m.apps {
		apps = append(apps, a)
	}
	m.mu.Unlock()
	for _, a := range apps {
		m.Close(a)
	}
}

// lookup finds the view a request's host names.
func (m *AppManager) lookup(token string) *AppHost {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.apps[token]
}

// ServeHTTP serves the host shell at chatgo-mcp://<token>/ and the view at
// chatgo-mcp://<token>.view/view. The two hosts give the shell and the
// sandboxed view different origins, so the view cannot reach the shell's
// page or the app's files.
func (m *AppManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	if strings.HasSuffix(host, ".view") {
		a := m.lookup(strings.TrimSuffix(host, ".view"))
		if a == nil {
			http.NotFound(w, r)
			return
		}
		a.serveView(w, r)
		return
	}
	a := m.lookup(host)
	if a == nil {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		a.serveShell(w, r)
	case "/rpc":
		a.serveRPC(w, r)
	case "/pending":
		a.servePending(w, r)
	case "/favicon.ico":
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// URL is the page the host's web view loads.
func (a *AppHost) URL() string { return "chatgo-mcp://" + a.token + "/" }

// ViewURL is the sandboxed view's origin; it differs from URL()'s host so the
// view cannot reach the shell's DOM.
func (a *AppHost) ViewURL() string { return "chatgo-mcp://" + a.token + ".view/view" }

// Token identifies the view's registry entry.
func (a *AppHost) Token() string { return a.token }

// serveShell returns the outer page that hosts the sandboxed view.
func (a *AppHost) serveShell(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, shellHTML(a.permissionsAllow()))
}

// permissionsAllow is the iframe allow attribute built from the view's
// declared permissions. Features not declared stay denied.
func (a *AppHost) permissionsAllow() string {
	var out []string
	p := a.opts.Resource.Permissions
	if p.Camera {
		out = append(out, "camera")
	}
	if p.Microphone {
		out = append(out, "microphone")
	}
	if p.Geolocation {
		out = append(out, "geolocation")
	}
	if p.ClipboardWrite {
		out = append(out, "clipboard-write")
	}
	return strings.Join(out, "; ")
}

// serveView returns the untrusted HTML with the CSP its metadata asked for.
func (a *AppHost) serveView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || (r.URL.Path != "/" && r.URL.Path != "/view" && r.URL.Path != "/view.html") {
		http.NotFound(w, r)
		return
	}
	res := a.opts.Resource
	policy := appCSP(res.CSP)
	w.Header().Set("Content-Type", UIResourceMIME+"; charset=utf-8")
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A custom scheme may not honour response headers everywhere; the meta
	// tag repeats the same policy inside the document.
	_, _ = io.WriteString(w, injectCSP(res.HTML, policy))
}

// injectCSP adds the policy as a meta element after the document's head tag,
// so the view is restricted even where response headers are not applied.
func injectCSP(html, policy string) string {
	meta := `<meta http-equiv="Content-Security-Policy" content="` + policy + `">`
	lower := strings.ToLower(html)
	if i := strings.Index(lower, "<head"); i >= 0 {
		if j := strings.IndexByte(lower[i:], '>'); j >= 0 {
			at := i + j + 1
			return html[:at] + meta + html[at:]
		}
	}
	return meta + html
}

// appCSP builds the policy the extension specifies: the declared origins,
// and a restrictive default when the view declares none.
func appCSP(c AppCSP) string {
	join := func(list []string, fallback string) string {
		if len(list) == 0 {
			return fallback
		}
		return strings.Join(list, " ")
	}
	parts := []string{
		"default-src 'none'",
		"script-src 'self' 'unsafe-inline' " + join(c.ResourceDomains, ""),
		"style-src 'self' 'unsafe-inline' " + join(c.ResourceDomains, ""),
		"img-src 'self' data: blob: " + join(c.ResourceDomains, ""),
		"font-src 'self' data: " + join(c.ResourceDomains, ""),
		"media-src 'self' data: blob: " + join(c.ResourceDomains, ""),
		"connect-src " + join(c.ConnectDomains, "'none'"),
		"frame-src " + join(c.FrameDomains, "'none'"),
		"base-uri " + join(c.BaseURIDomains, "'self'"),
		"object-src 'none'",
		"form-action 'none'",
	}
	return strings.Join(parts, "; ")
}

// shellHTML is the host page: a sandboxed iframe and the bridge that carries
// JSON-RPC between it and Go. The iframe is same-origin with itself (it may
// keep storage), but a different host from this page, so it still cannot
// touch the shell's DOM.
func shellHTML(allow string) string {
	allowJSON, _ := json.Marshal(allow)
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="color-scheme" content="light dark">
<style>
  html, body { margin: 0; height: 100%; background: transparent; overflow: hidden; }
  iframe { display: block; width: 100%; height: 100%; border: 0; background: #fff; color-scheme: light dark; }
</style>
</head>
<body>
<iframe id="view" sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads"></iframe>
<script>
(function () {
  var frame = document.getElementById('view');
  var base = new URL(location.href);
  var view = new URL(base.href);
  view.host = base.host + '.view';
  view.pathname = '/view';
  view.search = '';
  var allow = ` + string(allowJSON) + `;
  if (allow) frame.setAttribute('allow', allow);
  frame.src = view.href;

  function post(message) {
    if (!frame.contentWindow) return;
    frame.contentWindow.postMessage(message, '*');
  }

  function relay(message) {
    return fetch('rpc', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(message)
    }).then(function (res) {
      if (res.status === 204) return null;
      if (!res.ok) throw new Error('rpc ' + res.status);
      return res.json();
    }).then(function (reply) {
      if (reply) post(reply);
    }).catch(function (err) {
      post({
        jsonrpc: '2.0',
        id: message.id,
        error: { code: -32603, message: String(err) }
      });
    });
  }

  window.addEventListener('message', function (event) {
    if (event.source !== frame.contentWindow) return;
    var data = event.data;
    if (!data || typeof data !== 'object' || data.jsonrpc !== '2.0') return;
    relay(data);
  });

  // Poll while the view is visible. A hidden view keeps its page and stops
  // asking the host for messages until it is shown again.
  var pumping = false;
  function pump() {
    if (pumping || document.hidden) return;
    pumping = true;
    fetch('pending').then(function (res) {
      if (res.status === 204) return null;
      if (!res.ok) throw new Error('pending ' + res.status);
      return res.json();
    }).then(function (message) {
      pumping = false;
      if (message) post(message);
      if (!document.hidden) setTimeout(pump, 0);
    }).catch(function () {
      pumping = false;
      if (!document.hidden) setTimeout(pump, 700);
    });
  }

  document.addEventListener('visibilitychange', function () { if (!document.hidden) pump(); });
  frame.addEventListener('load', function () { pump(); });
})();
</script>
</body>
</html>`
}

// serveRPC handles one JSON-RPC message from the view's shell.
func (a *AppHost) serveRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	reply := a.dispatch(r, msg)
	if reply == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}

// servePending holds a request until the host has a message for the view or
// waits too long, then the shell polls again.
func (a *AppHost) servePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case message := <-a.out:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(message)
	case <-time.After(20 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	case <-a.closed:
		w.WriteHeader(http.StatusNoContent)
	}
}

// rpcMessage is one JSON-RPC request, notification or response.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// hasID reports whether the message is a request rather than a notification.
func (m rpcMessage) hasID() bool {
	id := bytes.TrimSpace(m.ID)
	return len(id) > 0 && !bytes.Equal(id, []byte("null"))
}

// dispatch handles one message and returns the response for a request, or
// nil for a notification or a response.
func (a *AppHost) dispatch(r *http.Request, msg rpcMessage) *rpcMessage {
	if msg.Method == "" {
		return nil // a response to a host request, such as teardown
	}
	ctx := r.Context()
	switch msg.Method {
	case "ui/initialize", "initialize":
		result, err := a.initialize(msg.Params)
		return replyTo(msg, result, err)
	case "ui/notifications/initialized", "notifications/initialized":
		a.onInitialized()
		return nil
	case "tools/call":
		result, err := a.callTool(ctx, msg.Params)
		return replyTo(msg, result, err)
	case "tools/list":
		result, err := a.listTools()
		return replyTo(msg, result, err)
	case "resources/read":
		result, err := a.readResource(ctx, msg.Params)
		return replyTo(msg, result, err)
	case "ui/open-link":
		result, err := a.openLink(msg.Params)
		return replyTo(msg, result, err)
	case "ui/message":
		result, err := a.postMessage(msg.Params)
		return replyTo(msg, result, err)
	case "ui/update-model-context":
		result, err := a.updateModelContext(msg.Params)
		return replyTo(msg, result, err)
	case "ui/request-display-mode":
		result, err := a.requestDisplayMode(msg.Params)
		return replyTo(msg, result, err)
	case "ping":
		return replyTo(msg, map[string]any{}, nil)
	case "notifications/message":
		a.logMessage(msg.Params)
		return nil
	case "ui/notifications/size-changed":
		a.setSize(msg.Params)
		return nil
	}
	if !msg.hasID() {
		return nil
	}
	return &rpcMessage{
		JSONRPC: "2.0", ID: msg.ID,
		Error: &rpcError{Code: -32601, Message: "method not found: " + msg.Method},
	}
}

// replyTo wraps a result in a JSON-RPC response, or an error when handler
// returns one.
func replyTo(msg rpcMessage, result any, err error) *rpcMessage {
	if !msg.hasID() {
		return nil
	}
	if err != nil {
		return &rpcMessage{
			JSONRPC: "2.0", ID: msg.ID,
			Error: &rpcError{Code: -32000, Message: err.Error()},
		}
	}
	raw, merr := json.Marshal(result)
	if merr != nil {
		return &rpcMessage{
			JSONRPC: "2.0", ID: msg.ID,
			Error: &rpcError{Code: -32603, Message: merr.Error()},
		}
	}
	return &rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: raw}
}

// initialize answers the view's handshake with the host's capabilities and
// context, as the extension specifies.
func (a *AppHost) initialize(params json.RawMessage) (map[string]any, error) {
	var p struct {
		AppCapabilities struct {
			AvailableDisplayModes []string `json:"availableDisplayModes"`
		} `json:"appCapabilities"`
	}
	_ = json.Unmarshal(params, &p)
	a.mu.Lock()
	// A view may reload; its new handshake starts a new lifetime, so the
	// tool data must be delivered again after the next initialized.
	a.initialized = false
	a.inputSent = false
	a.resultSent = false
	a.appModes = p.AppCapabilities.AvailableDisplayModes
	mode := a.displayMode
	if len(a.appModes) > 0 && !containsString(a.appModes, mode) {
		mode = a.appModes[0]
	}
	a.displayMode = mode
	a.mu.Unlock()
	res := a.opts.Resource
	return map[string]any{
		"protocolVersion": UISpecVersion,
		"hostInfo":        map[string]any{"name": "ChatGo", "version": a.version()},
		"hostCapabilities": map[string]any{
			"openLinks":       map[string]any{},
			"serverTools":     map[string]any{"listChanged": false},
			"serverResources": map[string]any{"listChanged": false},
			"logging":         map[string]any{},
			"sandbox": map[string]any{
				"permissions": sandboxPermissions(res.Permissions),
				"csp": map[string]any{
					"connectDomains":  res.CSP.ConnectDomains,
					"resourceDomains": res.CSP.ResourceDomains,
					"frameDomains":    res.CSP.FrameDomains,
					"baseUriDomains":  res.CSP.BaseURIDomains,
				},
			},
		},
		"hostContext": a.hostContext(),
	}, nil
}

// hostContext describes the host to a view: theme, size, locale and the tool
// that opened it.
func (a *AppHost) hostContext() map[string]any {
	width, height := 800, 600
	if a.opts.Callbacks.Size != nil {
		if w, h := a.opts.Callbacks.Size(); w > 0 && h > 0 {
			width, height = w, h
		}
	}
	a.mu.Lock()
	mode := a.displayMode
	a.mu.Unlock()
	ctx := map[string]any{
		"displayMode":           mode,
		"availableDisplayModes": []string{"inline", "fullscreen"},
		"containerDimensions":   map[string]any{"maxWidth": width, "maxHeight": height},
		"platform":              "desktop",
		"userAgent":             "ChatGo/" + a.version(),
		"deviceCapabilities":    map[string]any{"touch": false, "hover": true},
		"safeAreaInsets":        map[string]int{"top": 0, "right": 0, "bottom": 0, "left": 0},
		"toolInfo":              map[string]any{"id": a.opts.ToolID, "tool": a.toolInfo()},
	}
	if a.opts.Callbacks.Theme != nil {
		if t := a.opts.Callbacks.Theme(); t != "" {
			ctx["theme"] = t
		}
	}
	if a.opts.Callbacks.Locale != nil {
		if l := a.opts.Callbacks.Locale(); l != "" {
			ctx["locale"] = l
		}
	}
	return ctx
}

// sandboxPermissions lists the browser features granted to a view, in the
// shape HostCapabilities.sandbox.permissions defines.
func sandboxPermissions(p AppPermissions) map[string]any {
	out := map[string]any{}
	if p.Camera {
		out["camera"] = map[string]any{}
	}
	if p.Microphone {
		out["microphone"] = map[string]any{}
	}
	if p.Geolocation {
		out["geolocation"] = map[string]any{}
	}
	if p.ClipboardWrite {
		out["clipboardWrite"] = map[string]any{}
	}
	return out
}

// toolInfo describes the tool that opened the view.
func (a *AppHost) toolInfo() map[string]any {
	t := a.opts.Tool
	info := map[string]any{"name": t.Name, "description": t.Description}
	if t.Title != "" {
		info["title"] = t.Title
	}
	if schema, ok := t.InputSchema["inputSchema"]; ok {
		info["inputSchema"] = schema
	} else if t.InputSchema != nil {
		info["inputSchema"] = t.InputSchema
	}
	if t.UIResourceURI != "" {
		info["_meta"] = map[string]any{"ui": map[string]any{
			"resourceUri": t.UIResourceURI, "visibility": t.UIVisibility,
		}}
	}
	return info
}

// onInitialized releases the tool input and result the view is waiting for.
func (a *AppHost) onInitialized() {
	a.mu.Lock()
	a.initialized = true
	a.mu.Unlock()
	a.sendToolInput()
	a.sendToolResult()
}

// sendToolInput sends ui/notifications/tool-input once.
func (a *AppHost) sendToolInput() {
	a.mu.Lock()
	if a.inputSent {
		a.mu.Unlock()
		return
	}
	a.inputSent = true
	a.mu.Unlock()
	args := parseToolJSON(a.opts.Arguments)
	if args == nil {
		args = map[string]any{}
	}
	a.push(map[string]any{
		"jsonrpc": "2.0", "method": "ui/notifications/tool-input",
		"params": map[string]any{"arguments": args},
	})
}

// sendToolResult sends ui/notifications/tool-result once, when the view is
// initialized and the call has finished.
func (a *AppHost) sendToolResult() {
	a.mu.Lock()
	if a.resultSent || !a.initialized || a.opts.Result == "" && a.opts.Error == "" {
		a.mu.Unlock()
		return
	}
	a.resultSent = true
	a.mu.Unlock()
	a.push(map[string]any{
		"jsonrpc": "2.0", "method": "ui/notifications/tool-result",
		"params": a.resultPayload(),
	})
}

// resultPayload is the CallToolResult shape the view expects.
func (a *AppHost) resultPayload() map[string]any {
	if a.opts.Error != "" {
		return map[string]any{
			"content": []any{map[string]any{"type": "text", "text": a.opts.Error}},
			"isError": true,
		}
	}
	if v := parseToolJSON(a.opts.Result); v != nil {
		if m, ok := v.(map[string]any); ok {
			return m
		}
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": a.opts.Result}}}
	}
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": a.opts.Result}},
	}
}

// callTool proxies a view's tools/call to this server, refusing app-only
// tools the view is not allowed to call.
func (a *AppHost) callTool(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	if _, ok := a.tools[p.Name]; !ok {
		return nil, fmt.Errorf("工具不可用于界面：%s", p.Name)
	}
	if a.cli == nil {
		return nil, fmt.Errorf("MCP 连接已关闭")
	}
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := any(map[string]any{})
	if v := parseToolJSON(string(p.Arguments)); v != nil {
		args = v
	}
	result, err := a.cli.CallTool(callCtx, mcp.CallToolRequest{
		Request: mcp.Request{Method: "tools/call"},
		Params:  mcp.CallToolParams{Name: p.Name, Arguments: args},
	})
	if err != nil {
		return nil, err
	}
	a.push(map[string]any{
		"jsonrpc": "2.0", "method": "ui/notifications/tool-input",
		"params": map[string]any{"arguments": args},
	})
	a.push(map[string]any{
		"jsonrpc": "2.0", "method": "ui/notifications/tool-result",
		"params": result,
	})
	return result, nil
}

// listTools returns this server's app-visible tools.
func (a *AppHost) listTools() (any, error) {
	list := make([]map[string]any, 0, len(a.tools))
	for _, t := range a.tools {
		tool := map[string]any{"name": t.Name, "description": t.Description}
		if t.Title != "" {
			tool["title"] = t.Title
		}
		if schema, ok := t.InputSchema["inputSchema"]; ok {
			tool["inputSchema"] = schema
		}
		list = append(list, tool)
	}
	return map[string]any{"tools": list}, nil
}

// readResource proxies resources/read for this server.
func (a *AppHost) readResource(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid resources/read params: %w", err)
	}
	if p.URI == "" {
		return nil, fmt.Errorf("缺少资源地址")
	}
	if a.cli == nil {
		return nil, fmt.Errorf("MCP 连接已关闭")
	}
	readCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return a.cli.ReadResource(readCtx, mcp.ReadResourceRequest{
		Request: mcp.Request{Method: "resources/read"},
		Params:  mcp.ReadResourceParams{URI: p.URI},
	})
}

// openLink opens a URL outside the view. Only web and mail links pass: a
// view cannot launch files or custom schemes through the host.
func (a *AppHost) openLink(params json.RawMessage) (any, error) {
	var p struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid ui/open-link params: %w", err)
	}
	u, err := url.Parse(strings.TrimSpace(p.URL))
	if err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("无效链接")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
	default:
		return nil, fmt.Errorf("不允许打开该类型的链接")
	}
	if a.opts.Callbacks.OpenLink == nil {
		return nil, fmt.Errorf("宿主不支持打开链接")
	}
	if err := a.opts.Callbacks.OpenLink(u.String()); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// postMessage hands text from the view to the chat composer. The user sends
// it, so a view cannot start a model turn by itself.
func (a *AppHost) postMessage(params json.RawMessage) (any, error) {
	var p struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid ui/message params: %w", err)
	}
	text := contentText(p.Content)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("消息内容为空")
	}
	if a.opts.Callbacks.PostMessage == nil {
		return nil, fmt.Errorf("宿主不支持发送消息")
	}
	if err := a.opts.Callbacks.PostMessage(text); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
}

// contentText flattens a content block or an array of them to text.
func contentText(raw json.RawMessage) string {
	var one struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &one); err == nil && one.Text != "" {
		return one.Text
	}
	var many []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &many); err == nil {
		var b strings.Builder
		for _, c := range many {
			if c.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(c.Text)
		}
		return b.String()
	}
	return ""
}

// updateModelContext keeps the last context a view sent for future turns.
func (a *AppHost) updateModelContext(params json.RawMessage) (any, error) {
	var p struct {
		Content           json.RawMessage `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid ui/update-model-context params: %w", err)
	}
	if a.opts.Callbacks.UpdateModelContext != nil {
		a.opts.Callbacks.UpdateModelContext(p.Content, p.StructuredContent)
	}
	return map[string]any{}, nil
}

// requestDisplayMode reports the mode the host can actually show. Views open
// in a full tab, so fullscreen is always the result.
func (a *AppHost) requestDisplayMode(params json.RawMessage) (any, error) {
	var p struct {
		Mode string `json:"mode"`
	}
	_ = json.Unmarshal(params, &p)
	a.mu.Lock()
	if p.Mode != "" && (len(a.appModes) == 0 || containsString(a.appModes, p.Mode)) {
		a.displayMode = p.Mode
	}
	mode := a.displayMode
	a.mu.Unlock()
	return map[string]any{"mode": mode}, nil
}

// setSize keeps the size the view reported and tells the host, which sizes
// the element holding the page.
func (a *AppHost) setSize(params json.RawMessage) {
	var p struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	if p.Height <= 0 {
		return
	}
	if a.opts.OnSize != nil {
		a.opts.OnSize(int(p.Width), int(p.Height))
	}
}

// logMessage records a view's log notification.
func (a *AppHost) logMessage(params json.RawMessage) {
	var p struct {
		Level string `json:"level"`
		Data  any    `json:"data"`
	}
	_ = json.Unmarshal(params, &p)
	msg := ""
	switch v := p.Data.(type) {
	case string:
		msg = v
	case map[string]any:
		if s, ok := v["message"].(string); ok {
			msg = s
		}
	}
	if a.opts.Callbacks.Log != nil {
		a.opts.Callbacks.Log(p.Level, msg)
		return
	}
	log.Printf("[MCP Apps %s] %s: %s", a.opts.Server, p.Level, msg)
}

// push queues a host-to-view message for the shell's next poll.
func (a *AppHost) push(message any) {
	raw, err := json.Marshal(message)
	if err != nil {
		return
	}
	select {
	case a.out <- raw:
	case <-a.closed:
	default:
		log.Printf("[MCP Apps] dropping message for %s: queue full", a.opts.Tool.Name)
	}
}

// version is the host version reported to views.
func (a *AppHost) version() string {
	if a.opts.Version != "" {
		return a.opts.Version
	}
	return "0.18.2"
}

// containsString reports whether list holds s.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
