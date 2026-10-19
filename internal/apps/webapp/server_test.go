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
