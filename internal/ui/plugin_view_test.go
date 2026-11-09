package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseViewAllTypes(t *testing.T) {
	for _, typ := range []string{"table", "chart", "keyvalue", "list", "code", "diff", "links", "markdown", "image"} {
		raw := fmt.Sprintf(`{"$chatgo":{"v":1,"view":{"type":%q,"title":"t"}}}`, typ)
		v, ok := parseView(raw)
		if !ok || v.Type != typ || v.Title != "t" {
			t.Fatalf("parse %s = %+v, %v", typ, v, ok)
		}
	}
}

func TestParseViewRejects(t *testing.T) {
	cases := []string{
		`not json`,
		`{"$chatgo":{"v":2,"view":{"type":"table"}}}`,
		`{"$chatgo":{"v":1,"view":{"type":"html"}}}`,
		`{"$chatgo":{"v":1,"view":{}}}`,
		`{"other":{"type":"table"}}`,
		`{"$chatgo":{"v":1,"view":` + strings.Repeat(" ", maxViewBytes) + `}}`,
	}
	for _, raw := range cases {
		if _, ok := parseView(raw); ok {
			t.Fatalf("accepted invalid view: %s", raw)
		}
	}
}

func TestSafeWebURL(t *testing.T) {
	for _, ok := range []string{"https://example.com/a.png", "https://x.test"} {
		if !safeWebURL(ok) {
			t.Fatalf("rejected %q", ok)
		}
	}
	for _, bad := range []string{"http://example.com", "file:///etc/passwd", "javascript:alert(1)", ""} {
		if safeWebURL(bad) {
			t.Fatalf("accepted %q", bad)
		}
	}
}
