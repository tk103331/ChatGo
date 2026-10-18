package webapp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	mygo "github.com/egoist/mygo"
)

func writeApp(t *testing.T, root, id string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id: " + id + "\nname: 测试应用\nentry: index.html\ntools:\n  - name: t_echo\n    description: 回显\n    params:\n      - name: text\n        description: 文本\n        required: true\n"
	if err := os.WriteFile(filepath.Join(dir, "webapp.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	page := "<html><head><title>x</title></head><body><div id=app></div></body></html>"
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadAndDefaults(t *testing.T) {
	root := t.TempDir()
	dir := writeApp(t, root, "demo")
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "demo" || a.Name != "测试应用" || a.Entry != "index.html" {
		t.Fatalf("manifest: %+v", a.Manifest)
	}
	if a.Icon != "globe" {
		t.Fatalf("icon default: %q", a.Icon)
	}
	if len(a.Tools) != 1 || !a.Tools[0].Params[0].Required {
		t.Fatalf("tool params: %+v", a.Tools)
	}
}

func TestLoadRejectsBadManifest(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "bad")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "webapp.yaml"), []byte("id: Bad_ID\nname: x\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644)
	if _, err := Load(dir); err == nil {
		t.Fatal("uppercase id should be rejected")
	}
	dir2 := filepath.Join(root, "noentry")
	os.MkdirAll(dir2, 0o755)
	os.WriteFile(filepath.Join(dir2, "webapp.yaml"), []byte("id: noentry\nname: x\nentry: missing.html\n"), 0o644)
	if _, err := Load(dir2); err == nil {
		t.Fatal("missing entry should be rejected")
	}
}

func TestDiscoverFindsBundled(t *testing.T) {
	apps := Discover()
	var draw *App
	for i := range apps {
		if apps[i].ID == "draw" {
			draw = &apps[i]
		}
	}
	if draw == nil {
		t.Fatal("bundled draw app not discovered")
	}
	if !draw.Embedded() {
		t.Fatal("bundled app should be embedded")
	}
	if len(draw.Tools) != 4 {
		t.Fatalf("draw tools: %d", len(draw.Tools))
	}
	if !strings.Contains(draw.SystemPrompt(), "draw_add_node") {
		t.Fatalf("prompt missing tools: %s", draw.SystemPrompt())
	}
}

func TestDiscoverProjectOverridesBundled(t *testing.T) {
	root := t.TempDir()
	writeApp(t, root, "draw")
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "webapps"), 0o755)
	os.Rename(filepath.Join(root, "draw"), filepath.Join(root, "webapps", "draw"))
	apps := Discover()
	n := 0
	for _, a := range apps {
		if a.ID == "draw" {
			n++
			if a.Embedded() {
				t.Fatal("project app should win over the bundled one")
			}
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one draw app, got %d", n)
	}
}

func TestHandlerServesPageAndBridge(t *testing.T) {
	root := t.TempDir()
	dir := writeApp(t, root, "demo")
	a, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(a, newBridge(), nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "/__chatgo.js") {
		t.Fatalf("entry should inject bridge: %s", body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/__chatgo.js", nil))
	body, _ = io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "registerTools") {
		t.Fatalf("bridge script: %s", body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing file: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA fallback: %d", rec.Code)
	}
}

func TestBridgeResultRouting(t *testing.T) {
	b := newBridge()
	id, req := b.begin("t_echo", map[string]any{"text": "hi"})
	go func() {
		rec := httptest.NewRecorder()
		b.serve(rec, httptest.NewRequest(http.MethodPost, "/__chatgo/result",
			strings.NewReader(`{"id":"`+id+`","result":{"echo":"hi"}}`)))
	}()
	res := <-req.done
	if res.err != nil {
		t.Fatal(res.err)
	}
	if !strings.Contains(res.out, "hi") {
		t.Fatalf("result: %s", res.out)
	}
}

func TestBridgeNotify(t *testing.T) {
	b := newBridge()
	got := make(chan map[string]any, 1)
	b.onNote = func(p map[string]any) { got <- p }
	rec := httptest.NewRecorder()
	b.serve(rec, httptest.NewRequest(http.MethodPost, "/__chatgo/notify",
		strings.NewReader(`{"kind":"info","message":"hello"}`)))
	select {
	case p := <-got:
		if parseNote(p).Message != "hello" {
			t.Fatalf("note: %+v", p)
		}
	default:
		t.Fatal("notification not delivered")
	}
}

func TestMCPURLAndServers(t *testing.T) {
	a, ok := Find("draw")
	if !ok {
		t.Fatal("draw not found")
	}
	cl := New(&nopHost{}, a)
	if len(cl.app.WebMCP) != 1 {
		t.Fatalf("want 1 webmcp, got %d", len(cl.app.WebMCP))
	}
	m := cl.app.WebMCP[0]
	got, err := cl.MCPURL(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "http://127.0.0.1:") || !strings.HasSuffix(got, "/mcp") {
		t.Fatalf("url: %q", got)
	}
	servers := cl.MCPServers()
	if len(servers) != 1 {
		t.Fatalf("servers: %d", len(servers))
	}
	srv := servers[0]
	if srv.Name != "webapp:draw/canvas" || !strings.HasPrefix(srv.URL, "http://127.0.0.1:") || !srv.Enabled {
		t.Fatalf("server: %+v", srv)
	}
	cl.Close()
}

func TestMCPURLKeepsAbsolute(t *testing.T) {
	cl := New(&nopHost{}, App{Manifest: Manifest{ID: "x"}})
	defer cl.Close()
	if got, _ := cl.MCPURL(WebMCP{URL: "https://api.example/mcp"}); got != "https://api.example/mcp" {
		t.Fatalf("absolute url rewritten: %q", got)
	}
	if got, _ := cl.MCPURL(WebMCP{URL: "mcp"}); !strings.HasSuffix(got, "/mcp") || !strings.HasPrefix(got, "http://127.0.0.1:") {
		t.Fatalf("relative url: %q", got)
	}
}

func TestMountedRoute(t *testing.T) {
	a := App{Manifest: Manifest{ID: "m", Entry: "index.html"}, Dir: t.TempDir()}
	os.WriteFile(filepath.Join(a.Dir, "index.html"), []byte("<html></html>"), 0o644)
	b := newBridge()
	routes := map[string]http.Handler{
		"/mcp": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("mcp-ok"))
		}),
	}
	h := newHandler(a, b, routes)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if rec.Body.String() != "mcp-ok" {
		t.Fatalf("mount: %q", rec.Body.String())
	}
}

func TestDemoMCPToolsList(t *testing.T) {
	a, _ := Find("draw")
	cl := New(&nopHost{}, a)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	rec := httptest.NewRecorder()
	cl.demoMCP().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body)))
	if !strings.Contains(rec.Body.String(), "draw_add_node") {
		t.Fatalf("tools/list: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"required":["label"]`) {
		t.Fatalf("schema required missing: %s", rec.Body.String())
	}
}

func TestDemoMCPUnknownMethod(t *testing.T) {
	a, _ := Find("draw")
	cl := New(&nopHost{}, a)
	rec := httptest.NewRecorder()
	cl.demoMCP().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"nope"}`)))
	if !strings.Contains(rec.Body.String(), "-32601") {
		t.Fatalf("want method-not-found: %s", rec.Body.String())
	}
}

// nopHost is a minimal Host for tests that never touch a window.
type nopHost struct{}

func (nopHost) Root() string                                        { return "" }
func (nopHost) Workspace() string                                   { return "" }
func (nopHost) Post(fn func())                                      { fn() }
func (nopHost) Invalidate()                                         {}
func (nopHost) Toast(string, mcore.Severity)                        {}
func (nopHost) Confirm(_ string, _ string, a func())                { a() }
func (nopHost) Prompt(_ string, _ string, _ string, a func(string)) { a("") }
func (nopHost) OpenFile(string)                                     {}
func (nopHost) OpenFileDialog(string) string                        { return "" }
func (nopHost) SaveFileDialog(string) string                        { return "" }
func (nopHost) Language(string) string                              { return "" }
func (nopHost) NewID() string                                       { return "id" }
func (nopHost) NativeWindow() *mygo.Window                          { return nil }

func TestBundledSkills(t *testing.T) {
	a, ok := Find("draw")
	if !ok {
		t.Fatal("draw not found")
	}
	cl := New(&nopHost{}, a)
	skills := cl.Skills()
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "diagramming" || !strings.Contains(skills[0].Body, "draw_list_nodes") {
		t.Fatalf("skill: %+v", skills[0])
	}
	prompt := cl.SkillPrompt()
	if !strings.Contains(prompt, "## Skill: diagramming") {
		t.Fatalf("prompt: %s", prompt)
	}
}

func TestLocalServerServesMCP(t *testing.T) {
	a, _ := Find("draw")
	cl := New(&nopHost{}, a)
	defer cl.Close()
	base, err := cl.ensureHTTP()
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req, _ := http.NewRequest(http.MethodPost, base+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "draw_list_nodes") {
		t.Fatalf("mcp over http: %s", data)
	}
}

func TestLocalServerInitialize(t *testing.T) {
	a, _ := Find("draw")
	cl := New(&nopHost{}, a)
	defer cl.Close()
	base, err := cl.ensureHTTP()
	if err != nil {
		t.Fatal(err)
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	resp, err := http.Post(base+"/mcp", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "protocolVersion") {
		t.Fatalf("initialize: %s", data)
	}
}

func TestLocalServerUnknownPath(t *testing.T) {
	a, _ := Find("draw")
	cl := New(&nopHost{}, a)
	defer cl.Close()
	base, err := cl.ensureHTTP()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(base+"/nope", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %d", resp.StatusCode)
	}
}
