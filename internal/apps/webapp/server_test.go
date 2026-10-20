package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEntryPathGetsBridge(t *testing.T) {
	a, ok := Find("slidev")
	if !ok {
		t.Skip("slidev not discovered")
	}
	h := newHandler(a, newBridge(), map[string]http.Handler{})
	for _, p := range []string{"/", "/index.html"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", p, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `src="/__chatgo.js"`) {
			t.Fatalf("%s: bridge script not injected", p)
		}
	}
}

func TestCallScriptPassesNameAndArgs(t *testing.T) {
	js, err := callScript("c1", "slidev_add_slide", map[string]any{"markdown": "# hi \" '"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `window.chatgo.handle("slidev_add_slide",{`) {
		t.Fatalf("name must be the first argument: %s", js)
	}
	if strings.Contains(js, `handle({"id"`) {
		t.Fatalf("payload object must not be passed as the name: %s", js)
	}
	if !strings.Contains(js, `"id":"c1"`) && !strings.Contains(js, `id:"c1"`) {
		t.Fatalf("result must carry the call id: %s", js)
	}
}
