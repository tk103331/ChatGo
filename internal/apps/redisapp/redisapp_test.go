package redisapp

import "testing"

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"GET foo", []string{"GET", "foo"}},
		{"SET foo \"hello world\"", []string{"SET", "foo", "hello world"}},
		{"SET foo 'single quoted'", []string{"SET", "foo", "single quoted"}},
		{"  LRANGE list 0 10  ", []string{"LRANGE", "list", "0", "10"}},
	}
	for _, tc := range cases {
		got, err := splitArgs(tc.line)
		if err != nil {
			t.Errorf("splitArgs(%q) error: %v", tc.line, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("splitArgs(%q) = %v, want %v", tc.line, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitArgs(%q) = %v, want %v", tc.line, got, tc.want)
				break
			}
		}
	}
}

func TestSplitArgsUnclosed(t *testing.T) {
	if _, err := splitArgs(`SET foo "unclosed`); err == nil {
		t.Error("expected error for unclosed quote")
	}
}

func TestRenderResult(t *testing.T) {
	cases := []struct {
		res  any
		want string
	}{
		{nil, "(nil)"},
		{"OK", "OK"},
		{int64(3), "(integer) 3"},
		{[]any{"a", int64(2)}, "1) a\n2) (integer) 2"},
		{float64(1.5), "1.5"},
	}
	for _, tc := range cases {
		if got := renderResult(tc.res); got != tc.want {
			t.Errorf("renderResult(%v) = %q, want %q", tc.res, got, tc.want)
		}
	}
}

func TestFmtTTL(t *testing.T) {
	if got := fmtTTL(-1); got != "∞" {
		t.Errorf("fmtTTL(-1) = %q, want ∞", got)
	}
	if got := fmtTTL(0); got != "0s" {
		t.Errorf("fmtTTL(0) = %q, want 0s", got)
	}
}

func TestConnAddr(t *testing.T) {
	c := Conn{Host: "localhost", Port: 6379}
	if got := c.Addr(); got != "localhost:6379" {
		t.Errorf("Addr = %q, want localhost:6379", got)
	}
}

func TestSplitLines(t *testing.T) {
	if got := splitLines("a\nb\nc", 2); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitLines = %v, want [a b]", got)
	}
	if got := splitLines("", 5); len(got) != 1 || got[0] != "" {
		t.Errorf("splitLines empty = %v, want [\"\"]", got)
	}
}
