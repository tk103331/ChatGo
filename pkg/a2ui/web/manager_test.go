package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chatgo/pkg/a2ui"
)

func TestManagerServesPageBundleAndBridge(t *testing.T) {
	m := NewManager(nil)
	var gotAction a2ui.Action
	var gotW, gotH int
	v, err := m.Open(OpenOptions{
		SurfaceID: "main",
		OnAction:  func(a a2ui.Action, data map[string]any) { gotAction = a },
		OnResize:  func(w, h int) { gotW, gotH = w, h },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.URL(), Scheme+"://") {
		t.Fatalf("url = %q", v.URL())
	}

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Host = v.token
	m.ServeHTTP(page, req)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "/bundle.js") {
		t.Fatalf("page = %d %s", page.Code, page.Body.String())
	}
	if csp := page.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("csp = %q", csp)
	}

	bundle := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	req.URL.Host = v.token
	m.ServeHTTP(bundle, req)
	if !strings.Contains(bundle.Body.String(), "createSurface") {
		t.Fatal("bundle does not handle createSurface")
	}

	v.Send([]byte(`{"version":"1.0","createSurface":{"surfaceId":"main"}}`))
	pending := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/pending", nil)
	req.URL.Host = v.token
	m.ServeHTTP(pending, req)
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), "createSurface") {
		t.Fatalf("pending = %d %s", pending.Code, pending.Body.String())
	}

	body, _ := json.Marshal(map[string]any{
		"type":      "action",
		"action":    a2ui.NewAction("main", "button", "submit"),
		"dataModel": map[string]any{"name": "Ada"},
	})
	rpc := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(string(body)))
	req.URL.Host = v.token
	m.ServeHTTP(rpc, req)
	if rpc.Code != http.StatusNoContent {
		t.Fatalf("rpc = %d", rpc.Code)
	}
	if gotAction.Name != "submit" {
		t.Fatalf("action = %#v", gotAction)
	}

	body, _ = json.Marshal(map[string]any{"type": "resize", "width": 640, "height": 420})
	req = httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(string(body)))
	req.URL.Host = v.token
	m.ServeHTTP(httptest.NewRecorder(), req)
	if gotW != 640 || gotH != 420 {
		t.Fatalf("resize = %dx%d", gotW, gotH)
	}

	v.Close()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Host = v.token
	gone := httptest.NewRecorder()
	m.ServeHTTP(gone, req)
	if gone.Code != http.StatusNotFound {
		t.Fatalf("closed view = %d", gone.Code)
	}
}

func TestManagerCustomBundle(t *testing.T) {
	m := NewManager([]byte("window.__custom = true;"))
	v, _ := m.Open(OpenOptions{SurfaceID: "s"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	req.URL.Host = v.token
	m.ServeHTTP(rec, req)
	if rec.Body.String() != "window.__custom = true;" {
		t.Fatalf("bundle = %q", rec.Body.String())
	}
}

func TestManagerForwardsRendererMessages(t *testing.T) {
	m := NewManager(nil)
	var got a2ui.RendererMessage
	v, err := m.Open(OpenOptions{
		SurfaceID:         "s",
		OnRendererMessage: func(msg a2ui.RendererMessage) { got = msg },
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"type": "rendererMessage",
		"rendererMessage": map[string]any{
			"version":                  "1.0",
			"rendererFunctionResponse": map[string]any{"functionCallId": "f1", "value": true},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(string(body)))
	req.URL.Host = v.token
	m.ServeHTTP(httptest.NewRecorder(), req)
	if got.FunctionResponse == nil || got.FunctionResponse.FunctionCallID != "f1" {
		t.Fatalf("message = %#v", got)
	}
}

func TestMultipleBundlesPerView(t *testing.T) {
	m := NewManagerWithBundles(map[string]*Bundle{
		"one": {ID: "one", JS: []byte("window.one = 1;"), CSS: []byte(".one{}")},
		"two": {ID: "two", JS: []byte("window.two = 1;")},
	}, "one")
	if !m.HasBundle("two") || len(m.BundleIDs()) != 2 {
		t.Fatalf("bundles = %v", m.BundleIDs())
	}
	v, err := m.Open(OpenOptions{SurfaceID: "s", BundleID: "two"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/bundle.js", nil)
	req.URL.Host = v.token
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Body.String() != "window.two = 1;" {
		t.Fatalf("bundle = %q", rec.Body.String())
	}
	if _, err := m.Open(OpenOptions{SurfaceID: "x", BundleID: "missing"}); err == nil {
		t.Fatal("unknown bundle accepted")
	}
}
