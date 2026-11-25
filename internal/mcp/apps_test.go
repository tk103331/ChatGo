package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestToolUIReadsBothMetaForms(t *testing.T) {
	meta := &mcp.Meta{AdditionalFields: map[string]any{
		"ui": map[string]any{
			"resourceUri": "ui://weather/dashboard",
			"visibility":  []any{"app"},
		},
	}}
	uri, visibility := ToolUI(meta)
	if uri != "ui://weather/dashboard" {
		t.Fatalf("uri = %q", uri)
	}
	if len(visibility) != 1 || visibility[0] != "app" {
		t.Fatalf("visibility = %#v", visibility)
	}

	legacy := &mcp.Meta{AdditionalFields: map[string]any{
		"ui/resourceUri": "ui://legacy",
	}}
	if uri, _ := ToolUI(legacy); uri != "ui://legacy" {
		t.Fatalf("legacy uri = %q", uri)
	}
	if uri, _ := ToolUI(nil); uri != "" {
		t.Fatalf("nil meta uri = %q", uri)
	}
}

func TestToolVisibilityDefaultsToBoth(t *testing.T) {
	plain := MCPTool{Name: "plain"}
	if !plain.ModelVisible() || !plain.AppVisible() {
		t.Fatal("a tool with no visibility must be visible to model and app")
	}
	appOnly := MCPTool{Name: "refresh", UIVisibility: []string{"app"}}
	if appOnly.ModelVisible() {
		t.Fatal("app-only tool must be hidden from the model")
	}
	if !appOnly.AppVisible() {
		t.Fatal("app-only tool must stay callable by its app")
	}
	modelOnly := MCPTool{Name: "write", UIVisibility: []string{"model"}}
	if modelOnly.AppVisible() {
		t.Fatal("model-only tool must not be callable by an app")
	}
}

func TestReadUIResource(t *testing.T) {
	html := `<!DOCTYPE html><html><body>hi</body></html>`
	srv := server.NewMCPServer("apps-test", "1.0.0")
	srv.AddResource(mcp.NewResource("ui://test/view", "view",
		mcp.WithMIMEType(UIResourceMIME)), func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		return []mcp.ResourceContents{mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: UIResourceMIME,
			Text:     html,
			Meta: map[string]any{"ui": map[string]any{
				"csp":           map[string]any{"connectDomains": []any{"https://api.example.com"}},
				"prefersBorder": false,
				"permissions":   map[string]any{"clipboardWrite": map[string]any{}},
			}},
		}}, nil
	})
	cli := client.NewClient(transport.NewInProcessTransport(srv))
	if err := cli.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if _, err := cli.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test", Version: "1"},
			Capabilities: mcp.ClientCapabilities{
				Extensions: map[string]any{UIExtension: UIExtensionCapability()},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := ReadUIResource(context.Background(), cli, "ui://test/view")
	if err != nil {
		t.Fatal(err)
	}
	if res.HTML != html || res.MIMEType != UIResourceMIME {
		t.Fatalf("resource = %#v", res)
	}
	if len(res.CSP.ConnectDomains) != 1 || res.CSP.ConnectDomains[0] != "https://api.example.com" {
		t.Fatalf("csp = %#v", res.CSP)
	}
	if !res.Permissions.ClipboardWrite {
		t.Fatal("clipboardWrite permission not read")
	}
	if res.PrefersBorder == nil || *res.PrefersBorder {
		t.Fatalf("prefersBorder = %#v", res.PrefersBorder)
	}
}

func TestAppHostHandshakeSendsToolData(t *testing.T) {
	host := testAppHost(t)
	init, err := host.initialize(json.RawMessage(`{"appCapabilities":{"availableDisplayModes":["inline","fullscreen"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if init["protocolVersion"] != UISpecVersion {
		t.Fatalf("protocolVersion = %v", init["protocolVersion"])
	}
	ctx, _ := init["hostContext"].(map[string]any)
	if ctx == nil || ctx["toolInfo"] == nil {
		t.Fatalf("hostContext = %#v", init["hostContext"])
	}

	host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
		JSONRPC: "2.0", Method: "ui/notifications/initialized",
	})
	msg := nextMessage(t, host)
	if msg["method"] != "ui/notifications/tool-input" {
		t.Fatalf("first message = %#v", msg)
	}
	params := msg["params"].(map[string]any)
	args := params["arguments"].(map[string]any)
	if args["location"] != "Paris" {
		t.Fatalf("arguments = %#v", args)
	}
	msg = nextMessage(t, host)
	if msg["method"] != "ui/notifications/tool-result" {
		t.Fatalf("second message = %#v", msg)
	}
	result := msg["params"].(map[string]any)
	if result["structuredContent"] == nil {
		t.Fatalf("result = %#v", result)
	}
}

func TestAppHostBlocksUnsuitableLinks(t *testing.T) {
	host := testAppHost(t)
	opened := ""
	host.opts.Callbacks.OpenLink = func(url string) error {
		opened = url
		return nil
	}
	call := func(raw string) *rpcMessage {
		return host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
			JSONRPC: "2.0", ID: json.RawMessage("1"), Method: "ui/open-link",
			Params: json.RawMessage(raw),
		})
	}
	for _, bad := range []string{
		`{"url":"file:///etc/passwd"}`,
		`{"url":"javascript:alert(1)"}`,
		`{"url":"chatgo://evil"}`,
		`{"url":""}`,
		`{"url":"not a url"}`,
	} {
		if reply := call(bad); reply == nil || reply.Error == nil {
			t.Fatalf("%s should have been refused, got %#v", bad, reply)
		}
	}
	if reply := call(`{"url":"https://example.com/a"}`); reply == nil || reply.Error != nil {
		t.Fatalf("web link refused: %#v", reply)
	}
	if opened != "https://example.com/a" {
		t.Fatalf("opened = %q", opened)
	}
}

func TestAppHostKeepsAppOnlyToolsOutOfModel(t *testing.T) {
	host := testAppHost(t)
	if _, ok := host.tools["refresh"]; !ok {
		t.Fatal("app-only tool should be callable from the view")
	}
	if _, ok := host.tools["public"]; !ok {
		t.Fatal("model+app tool should be callable from the view")
	}
	if _, ok := host.tools["secret"]; ok {
		t.Fatal("model-only tool must not be exposed to the view")
	}
}

func TestAppCSPUsesDeclaredOriginsAndDefaults(t *testing.T) {
	strict := appCSP(AppCSP{})
	for _, want := range []string{"default-src 'none'", "connect-src 'none'", "frame-src 'none'", "base-uri 'self'", "object-src 'none'"} {
		if !strings.Contains(strict, want) {
			t.Fatalf("default CSP %q lacks %q", strict, want)
		}
	}
	open := appCSP(AppCSP{
		ConnectDomains:  []string{"https://api.example.com"},
		ResourceDomains: []string{"https://cdn.example.com"},
		FrameDomains:    []string{"https://player.example.com"},
		BaseURIDomains:  []string{"https://example.com"},
	})
	for _, want := range []string{
		"connect-src https://api.example.com",
		"script-src 'self' 'unsafe-inline' https://cdn.example.com",
		"frame-src https://player.example.com",
		"base-uri https://example.com",
	} {
		if !strings.Contains(open, want) {
			t.Fatalf("CSP %q lacks %q", open, want)
		}
	}
}

func TestAppManagerServesShellAndSandboxedView(t *testing.T) {
	m := NewAppManager()
	host, err := m.Open(nil, nil, AppOptions{
		Server: "weather", Tool: MCPTool{Name: "get_weather", Title: "Weather"},
		Resource: &AppResource{
			URI: "ui://weather/view", MIMEType: UIResourceMIME,
			HTML: "<!DOCTYPE html><html><body>forecast</body></html>",
			CSP:  AppCSP{ConnectDomains: []string{"https://api.example.com"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	token := host.Token()

	shell := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Host = token
	m.ServeHTTP(shell, req)
	if shell.Code != http.StatusOK || !strings.Contains(shell.Body.String(), "<iframe") {
		t.Fatalf("shell = %d %s", shell.Code, shell.Body.String())
	}
	if !strings.Contains(shell.Body.String(), "sandbox=") {
		t.Fatal("shell must sandbox the view")
	}

	view := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/view", nil)
	req.URL.Host = token + ".view"
	m.ServeHTTP(view, req)
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), "forecast") {
		t.Fatalf("view = %d %s", view.Code, view.Body.String())
	}
	if csp := view.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "https://api.example.com") {
		t.Fatalf("view CSP = %q", csp)
	}

	missing := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Host = "unknown"
	m.ServeHTTP(missing, req)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown token = %d", missing.Code)
	}
}

// testAppHost builds an initialized-looking host with one input and result.
func testAppHost(t *testing.T) *AppHost {
	t.Helper()
	m := NewAppManager()
	host, err := m.Open(nil, &MCPServerStatus{Tools: []MCPTool{
		{Name: "public"},
		{Name: "refresh", UIVisibility: []string{"app"}},
		{Name: "secret", UIVisibility: []string{"model"}},
	}}, AppOptions{
		Server:    "weather",
		Tool:      MCPTool{Name: "get_weather", Title: "Weather", UIResourceURI: "ui://weather/view"},
		ToolID:    "call-1",
		Arguments: `{"location":"Paris"}`,
		Result:    `{"content":[{"type":"text","text":"Sunny"}],"structuredContent":{"temperature":72}}`,
		Resource: &AppResource{
			URI: "ui://weather/view", MIMEType: UIResourceMIME,
			HTML: "<!DOCTYPE html><html></html>",
		},
		Callbacks: AppCallbacks{Size: func() (int, int) { return 640, 480 }},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(host) })
	return host
}

func nextMessage(t *testing.T, host *AppHost) map[string]any {
	t.Helper()
	select {
	case raw := <-host.out:
		var msg map[string]any
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatal(err)
		}
		return msg
	case <-time.After(time.Second):
		t.Fatal("no message queued")
		return nil
	}
}

func TestAppHostProxiesToolsCallWithVisibility(t *testing.T) {
	srv := server.NewMCPServer("apps-test", "1.0.0")
	srv.AddTool(mcp.NewTool("shared"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		return mcp.NewToolResultStructured(map[string]any{"echo": args["q"]}, "echo"), nil
	})
	srv.AddTool(mcp.NewTool("hidden"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("should not run"), nil
	})
	cli := client.NewClient(transport.NewInProcessTransport(srv))
	if err := cli.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if _, err := cli.Initialize(context.Background(), mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_LEGACY_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test", Version: "1"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	m := NewAppManager()
	host, err := m.Open(cli, &MCPServerStatus{Tools: []MCPTool{
		{Name: "shared"},
		{Name: "hidden", UIVisibility: []string{"model"}},
	}}, AppOptions{
		Server:   "apps-test",
		Tool:     MCPTool{Name: "shared", UIResourceURI: "ui://test/view"},
		Resource: &AppResource{URI: "ui://test/view", MIMEType: UIResourceMIME, HTML: "<html></html>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close(host) })

	reply := host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("7"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"hidden","arguments":{}}`),
	})
	if reply == nil || reply.Error == nil {
		t.Fatalf("model-only tool must be refused, got %#v", reply)
	}

	reply = host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
		JSONRPC: "2.0", ID: json.RawMessage("8"), Method: "tools/call",
		Params: json.RawMessage(`{"name":"shared","arguments":{"q":"hi"}}`),
	})
	if reply == nil || reply.Error != nil {
		t.Fatalf("shared tool failed: %#v", reply)
	}
	var result struct {
		StructuredContent map[string]any `json:"structuredContent"`
	}
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.StructuredContent["echo"] != "hi" {
		t.Fatalf("result = %#v", result)
	}
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-input" {
		t.Fatalf("expected tool-input, got %#v", msg)
	}
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-result" {
		t.Fatalf("expected tool-result, got %#v", msg)
	}
}

func TestAppHostResendsToolDataAfterReload(t *testing.T) {
	host := testAppHost(t)
	start := func() {
		if _, err := host.initialize(json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
		host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
			JSONRPC: "2.0", Method: "ui/notifications/initialized",
		})
	}
	start()
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-input" {
		t.Fatalf("first load input = %#v", msg)
	}
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-result" {
		t.Fatalf("first load result = %#v", msg)
	}
	// A reloaded view initializes again and must see the tool data again.
	start()
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-input" {
		t.Fatalf("reload input = %#v", msg)
	}
	if msg := nextMessage(t, host); msg["method"] != "ui/notifications/tool-result" {
		t.Fatalf("reload result = %#v", msg)
	}
}

func TestAppHostReportsSizeChange(t *testing.T) {
	host := testAppHost(t)
	var gotW, gotH int
	host.opts.OnSize = func(w, h int) { gotW, gotH = w, h }
	host.dispatch(httptest.NewRequest(http.MethodPost, "/rpc", nil), rpcMessage{
		JSONRPC: "2.0", Method: "ui/notifications/size-changed",
		Params: json.RawMessage(`{"width":640,"height":420}`),
	})
	if gotW != 640 || gotH != 420 {
		t.Fatalf("size = %dx%d, want 640x420", gotW, gotH)
	}
}
