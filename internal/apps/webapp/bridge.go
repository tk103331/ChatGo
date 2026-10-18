package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BridgeJS is injected into every web app page. It exposes the host API as
// window.chatgo and the tool dispatcher the host calls.
const BridgeJS = `
(function () {
  if (window.chatgo) return;
  var handlers = {};
  var pending = {};
  var seq = 0;
  window.chatgo = {
    version: 1,
    registerTool: function (name, fn) { handlers[name] = fn; },
    registerTools: function (map) {
      Object.keys(map).forEach(function (k) { handlers[k] = map[k]; });
    },
    tools: function () { return Object.keys(handlers); },
    handle: function (name, args) {
      var fn = handlers[name];
      if (!fn) return Promise.reject(new Error("tool not registered: " + name));
      return Promise.resolve(fn(args || {}));
    },
    notify: function (payload) {
      return fetch("/__chatgo/notify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload || {})
      }).then(function (r) { return r.ok; });
    },
    state: function () {
      try { return JSON.parse(sessionStorage.getItem("chatgo:state") || "{}"); } catch (e) { return {}; }
    },
    saveState: function (value) {
      sessionStorage.setItem("chatgo:state", JSON.stringify(value || {}));
      return true;
    }
  };
})();
`

// callRequest is one tool invocation the page handles.
type callRequest struct {
	name string
	args map[string]any
	done chan callResult
}

type callResult struct {
	out string
	err error
}

// bridge routes host calls to the page and page notifications to handlers.
type bridge struct {
	mu      sync.Mutex
	pending map[string]callRequest
	seq     int64
	onNote  func(payload map[string]any)
}

func newBridge() *bridge {
	return &bridge{pending: map[string]callRequest{}}
}

// serve handles the bridge endpoints under /__chatgo/.
func (b *bridge) serve(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimPrefix(r.URL.Path, "/__chatgo") {
	case "/notify":
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		cb := b.onNote
		b.mu.Unlock()
		if cb != nil {
			cb(payload)
		}
		w.WriteHeader(http.StatusNoContent)
	case "/result":
		var body struct {
			ID     string `json:"id"`
			Result any    `json:"result"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		req, ok := b.pending[body.ID]
		delete(b.pending, body.ID)
		b.mu.Unlock()
		if ok {
			if body.Error != "" {
				req.done <- callResult{err: fmt.Errorf("%s", body.Error)}
			} else {
				req.done <- callResult{out: stringify(body.Result)}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// begin registers a pending call and returns its id.
func (b *bridge) begin(name string, args map[string]any) (string, callRequest) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := fmt.Sprintf("c%d", b.seq)
	req := callRequest{name: name, args: args, done: make(chan callResult, 1)}
	b.pending[id] = req
	return id, req
}

// finish drops a pending call.
func (b *bridge) finish(id string) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

// notification is one message the page sent through chatgo.notify.
type Notification struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// parseNote reads the useful fields of a page notification.
func parseNote(payload map[string]any) Notification {
	n := Notification{}
	if v, ok := payload["kind"].(string); ok {
		n.Kind = v
	}
	if v, ok := payload["message"].(string); ok {
		n.Message = v
	} else if v, ok := payload["title"].(string); ok {
		n.Message = v
	}
	return n
}

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

// callTimeout bounds how long a tool call waits for the page.
const callTimeout = 20 * time.Second
