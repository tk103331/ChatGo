// Package web renders A2UI surfaces with the web renderer in an embedded
// MyGo web view. It serves the renderer and bridges A2UI messages, actions
// and size changes over a custom scheme.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"chatgo/pkg/a2ui"
)

// Scheme is the custom URL scheme the renderer loads from. Register a
// Manager with mygo.Protocol.Handle before creating the window that shows it.
const Scheme = "a2ui-web"

// OpenOptions describe one surface's page.
type OpenOptions struct {
	SurfaceID     string
	CatalogID     string
	SendDataModel bool
	// BundleID selects one registered web renderer; empty uses the manager's
	// default.
	BundleID string
	// OnAction receives user actions with the surface's current data model.
	OnAction func(a2ui.Action, map[string]any)
	// OnResize receives the page's content size in CSS pixels.
	OnResize func(width, height int)
	// OnError receives protocol errors from the page's bundle.
	OnError func(error)
	// OnRendererMessage receives renderer-to-agent messages from the page,
	// such as a renderer function response or an agent function call.
	OnRendererMessage func(a2ui.RendererMessage)
}

// Bundle is one web renderer implementation a surface may select.
type Bundle struct {
	ID  string
	JS  []byte
	CSS []byte
}

// Manager serves renderer pages and their bridge endpoints. One manager per
// app is enough; it is safe for concurrent surfaces.
type Manager struct {
	mu        sync.Mutex
	views     map[string]*View
	bundles   map[string]*Bundle
	defaultID string
}

// NewManager returns a manager serving the embedded bundles: "basic",
// "react" and "copilot". A non-empty bundle replaces the built-in "basic" one,
// so callers can test a custom vanilla renderer.
func NewManager(bundle []byte) *Manager {
	if len(bundle) == 0 {
		return NewManagerWithBundles(BuiltinBundles(), BasicBundleID)
	}
	return NewManagerWithBundles(map[string]*Bundle{
		BasicBundleID: {ID: BasicBundleID, JS: bundle},
	}, BasicBundleID)
}

// NewManagerWithBundles serves several selectable renderer bundles. defaultID
// names the bundle a surface gets when it does not ask for one.
func NewManagerWithBundles(bundles map[string]*Bundle, defaultID string) *Manager {
	if len(bundles) == 0 {
		bundles = map[string]*Bundle{BasicBundleID: {ID: BasicBundleID, JS: basicBundle}}
	}
	if defaultID == "" {
		for id := range bundles {
			defaultID = id
			break
		}
	}
	return &Manager{views: map[string]*View{}, bundles: bundles, defaultID: defaultID}
}

// HasBundle reports whether a bundle id is registered.
func (m *Manager) HasBundle(id string) bool {
	_, ok := m.bundles[id]
	return ok
}

// BundleIDs lists the registered bundles, sorted for stable output.
func (m *Manager) BundleIDs() []string {
	out := make([]string, 0, len(m.bundles))
	for id := range m.bundles {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// DefaultBundleID names the fallback bundle.
func (m *Manager) DefaultBundleID() string { return m.defaultID }

// View is one open renderer page.
type View struct {
	token    string
	manager  *Manager
	bundleID string
	opts     OpenOptions

	mu     sync.Mutex
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

// Open registers one surface and returns its view.
func (m *Manager) Open(opts OpenOptions) (*View, error) {
	if strings.TrimSpace(opts.SurfaceID) == "" {
		return nil, fmt.Errorf("a2ui web: empty surface id")
	}
	bundleID := strings.TrimSpace(opts.BundleID)
	if bundleID == "" {
		bundleID = m.defaultID
	}
	if _, ok := m.bundles[bundleID]; !ok {
		return nil, fmt.Errorf("a2ui web: unknown bundle %q", bundleID)
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	v := &View{
		token: token, manager: m, opts: opts, bundleID: bundleID,
		out: make(chan []byte, 128), closed: make(chan struct{}),
	}
	m.mu.Lock()
	m.views[token] = v
	m.mu.Unlock()
	return v, nil
}

// URL is the page a WebView loads.
func (v *View) URL() string { return Scheme + "://" + v.token + "/" }

// SurfaceID names the surface this page renders.
func (v *View) SurfaceID() string { return v.opts.SurfaceID }

// Send queues one A2UI message envelope for the page.
func (v *View) Send(raw []byte) {
	if len(raw) == 0 {
		return
	}
	select {
	case v.out <- raw:
	case <-v.closed:
	default:
		// A page that stopped polling must not block the agent.
	}
}

// Close removes the page and tells it to stop.
func (v *View) Close() {
	v.manager.mu.Lock()
	delete(v.manager.views, v.token)
	v.manager.mu.Unlock()
	v.once.Do(func() { close(v.closed) })
}

// ServeHTTP serves the shell, bundle, bridge and long poll for every view.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	m.mu.Lock()
	v := m.views[strings.ToLower(host)]
	m.mu.Unlock()
	if v == nil {
		http.NotFound(w, r)
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		m.servePage(w, r)
	case "/bundle.js":
		v.serveBundle(w, r)
	case "/bundle.css":
		v.serveCSS(w, r)
	case "/rpc":
		v.serveRPC(w, r)
	case "/pending":
		v.servePending(w, r)
	default:
		http.NotFound(w, r)
	}
}

// servePage returns the document that loads the renderer bundle.
func (m *Manager) servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data: blob:; media-src 'self' https: data: blob:; connect-src 'self'; frame-src 'none'; base-uri 'none'; object-src 'none'")
	_, _ = io.WriteString(w, pageHTML)
}

func (v *View) serveBundle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bundle := v.manager.bundles[v.bundleID]
	if bundle == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(bundle.JS)
}

func (v *View) serveCSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bundle := v.manager.bundles[v.bundleID]
	if bundle == nil || len(bundle.CSS) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(bundle.CSS)
}

// rpcMessage is what the page sends to the host.
type rpcMessage struct {
	Type      string          `json:"type"`
	Action    *a2ui.Action    `json:"action,omitempty"`
	DataModel map[string]any  `json:"dataModel,omitempty"`
	Width     int             `json:"width,omitempty"`
	Height    int             `json:"height,omitempty"`
	Message   string          `json:"message,omitempty"`
	Renderer  json.RawMessage `json:"rendererMessage,omitempty"`
}

// serveRPC handles actions, size reports and errors from the page.
func (v *View) serveRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	switch msg.Type {
	case "action":
		if msg.Action != nil && v.opts.OnAction != nil {
			v.opts.OnAction(*msg.Action, msg.DataModel)
		}
	case "resize":
		if v.opts.OnResize != nil && msg.Height > 0 {
			v.opts.OnResize(msg.Width, msg.Height)
		}
	case "error":
		if v.opts.OnError != nil {
			v.opts.OnError(fmt.Errorf("%s", msg.Message))
		}
	case "rendererMessage":
		if v.opts.OnRendererMessage != nil && len(msg.Renderer) > 0 {
			var m a2ui.RendererMessage
			if err := json.Unmarshal(msg.Renderer, &m); err == nil {
				v.opts.OnRendererMessage(m)
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// servePending holds a request until a message is queued or the page gives
// up, then the bundle polls again.
func (v *View) servePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case raw := <-v.out:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	case <-time.After(20 * time.Second):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	case <-v.closed:
		w.WriteHeader(http.StatusNoContent)
	}
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// MarshalMessages encodes envelopes for the page.
func MarshalMessages(messages ...a2ui.Message) ([][]byte, error) {
	out := make([][]byte, 0, len(messages))
	for _, msg := range messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

// pageHTML is intentionally tiny: all rendering lives in bundle.js.
const pageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<link rel="stylesheet" href="/bundle.css">
<style>
  :root {
    color-scheme: light dark;
    --a2ui-bg: light-dark(#ffffff, #17191c);
    --a2ui-surface: light-dark(#f6f8fb, #212429);
    --a2ui-text: light-dark(#1b1f24, #f3f4f6);
    --a2ui-muted: light-dark(#6b7280, #9ca3af);
    --a2ui-border: light-dark(#e2e5ea, #374151);
    --a2ui-accent: light-dark(#2563eb, #38bdf8);
  }
  html, body { margin: 0; min-height: 100%; background: var(--a2ui-bg); color: var(--a2ui-text);
    font: 14px/1.5 -apple-system, BlinkMacSystemFont, "SF Pro Text", "PingFang SC", sans-serif; }
  #root { padding: 16px; display: flex; flex-direction: column; gap: 12px; }
  #root > * { min-width: 0; }
  .a2ui-col { display: flex; flex-direction: column; gap: 8px; }
  .a2ui-row { display: flex; flex-direction: row; gap: 8px; align-items: flex-start; flex-wrap: wrap; }
  .a2ui-card { background: var(--a2ui-surface); border: 1px solid var(--a2ui-border); border-radius: 10px; padding: 12px; }
  .a2ui-muted { color: var(--a2ui-muted); font-size: 12px; }
  .a2ui-h1 { font-size: 26px; font-weight: 700; margin: 0; }
  .a2ui-h2 { font-size: 22px; font-weight: 700; margin: 0; }
  .a2ui-h3 { font-size: 18px; font-weight: 650; margin: 0; }
  .a2ui-divider { height: 1px; background: var(--a2ui-border); }
  .a2ui-btn { border: 1px solid var(--a2ui-border); background: var(--a2ui-surface); color: var(--a2ui-text);
    border-radius: 7px; padding: 6px 12px; font: inherit; cursor: pointer; }
  .a2ui-btn.primary { background: var(--a2ui-accent); border-color: var(--a2ui-accent); color: #fff; }
  .a2ui-field { display: flex; flex-direction: column; gap: 4px; }
  .a2ui-field label { font-size: 12px; color: var(--a2ui-muted); }
  .a2ui-field input[type=text], .a2ui-field input[type=password], .a2ui-field textarea, .a2ui-field select {
    width: 100%; box-sizing: border-box; padding: 6px 8px; border-radius: 6px;
    border: 1px solid var(--a2ui-border); background: var(--a2ui-bg); color: var(--a2ui-text); font: inherit; }
  .a2ui-check { display: flex; gap: 8px; align-items: center; }
  .a2ui-img { max-width: 100%; max-height: 320px; border-radius: 8px; }
  .a2ui-video { width: 100%; max-height: 360px; border-radius: 8px; background: #000; }
  .a2ui-audio { display: flex; flex-direction: column; gap: 6px; }
  .a2ui-audio audio { width: 100%; }
  .a2ui-tabs { display: flex; flex-direction: column; gap: 8px; }
  .a2ui-tabbar { display: flex; gap: 4px; border-bottom: 1px solid var(--a2ui-border); flex-wrap: wrap; }
  .a2ui-tab { border: 0; background: transparent; color: var(--a2ui-muted); padding: 8px 12px;
    font: inherit; cursor: pointer; border-bottom: 2px solid transparent; margin-bottom: -1px; }
  .a2ui-tab.active { color: var(--a2ui-text); border-bottom-color: var(--a2ui-accent); font-weight: 600; }
  .a2ui-tabpanel { padding-top: 12px; }
  .a2ui-modal-wrap { display: contents; }
  .a2ui-modal-trigger { display: contents; }
  .a2ui-modal-overlay { position: fixed; inset: 0; background: rgba(0,0,0,.35);
    display: flex; align-items: center; justify-content: center; z-index: 1000; padding: 24px; }
  .a2ui-modal-content { position: relative; background: var(--a2ui-bg); color: var(--a2ui-text);
    border: 1px solid var(--a2ui-border); border-radius: 12px; padding: 20px;
    max-width: 560px; width: 100%; max-height: 80vh; overflow: auto; }
  .a2ui-modal-close { position: absolute; right: 8px; top: 8px; border: 0; background: transparent;
    color: var(--a2ui-muted); font-size: 20px; cursor: pointer; line-height: 1; }
  .a2ui-placeholder { padding: 8px 10px; border-radius: 6px; background: var(--a2ui-surface);
    color: var(--a2ui-muted); font-size: 12px; }
</style>
</head>
<body>
<div id="root"></div>
<script src="/bundle.js"></script>
</body>
</html>`
