package webapp

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// handler serves one web app's SPA files and its bridge endpoints.
type handler struct {
	app    App
	root   fs.FS
	files  http.Handler
	bridge *bridge
	routes map[string]http.Handler
}

// newHandler serves the app at scheme://localhost/, with the bridge mounted
// at /__chatgo/ and index.html as the SPA fallback.
func newHandler(a App, b *bridge, routes map[string]http.Handler) http.Handler {
	root, err := a.data()
	if err != nil {
		root = os.DirFS(a.Dir)
	}
	return &handler{
		app:    a,
		root:   root,
		files:  http.FileServer(http.FS(root)),
		bridge: b,
		routes: routes,
	}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for prefix, mounted := range h.routes {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			mounted.ServeHTTP(w, r)
			return
		}
	}
	if strings.HasPrefix(r.URL.Path, "/__chatgo/") {
		h.bridge.serve(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/__chatgo.js") {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(BridgeJS))
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		h.serveIndex(w, r)
		return
	}
	if st, err := fs.Stat(h.root, p); err != nil || st.IsDir() {
		if strings.HasPrefix(r.Header.Get("Accept"), "text/html") {
			h.serveIndex(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	h.files.ServeHTTP(w, r)
}

// serveIndex writes the app entry with the bridge script injected.
func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	data, err := h.app.readFile(h.app.Entry)
	if err != nil {
		http.Error(w, "入口文件读取失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	page := string(data)
	tag := `<script src="/__chatgo.js"></script>`
	if i := strings.Index(strings.ToLower(page), "</head>"); i >= 0 {
		page = page[:i] + tag + page[i:]
	} else {
		page = tag + page
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(page))
}
