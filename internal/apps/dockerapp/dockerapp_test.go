package dockerapp

import "testing"

func TestParseLevel(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"plain info line", 1},
		{"ERROR connection refused", 3},
		{"[ERR] timeout", 3},
		{"FATAL: out of memory", 3},
		{"panic: runtime error", 3},
		{"WARN disk almost full", 2},
		{"warning: deprecated", 2},
		{"DEBUG verbose detail", 0},
		{"TRACE entry", 0},
	}
	for _, tc := range cases {
		if got := int(parseLevel(tc.line)); got != tc.want {
			t.Errorf("parseLevel(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.0 GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.n); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestShortID(t *testing.T) {
	long := "abcdef1234567890abcdef1234567890abcdef12"
	if got := shortID(long); got != "abcdef123456" {
		t.Errorf("shortID = %q, want abcdef123456", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID = %q, want abc", got)
	}
}

func TestContainerName(t *testing.T) {
	c := Container{ID: "abcdef1234567890", Names: []string{"/my-app"}}
	if got := c.Name(); got != "my-app" {
		t.Errorf("Name = %q, want my-app", got)
	}
	if got := c.ShortID(); got != "abcdef123456" {
		t.Errorf("ShortID = %q, want abcdef123456", got)
	}
	empty := Container{ID: "abcdef1234567890"}
	if got := empty.Name(); got != "abcdef123456" {
		t.Errorf("Name = %q, want abcdef123456", got)
	}
}

func TestImageTag(t *testing.T) {
	im := Image{ID: "sha256:abcdef1234567890abcdef1234567890abcdef12", RepoTags: []string{"nginx:latest"}}
	if got := im.Tag(); got != "nginx:latest" {
		t.Errorf("Tag = %q, want nginx:latest", got)
	}
	if got := im.ShortID(); got != "abcdef123456" {
		t.Errorf("ShortID = %q, want abcdef123456", got)
	}
	none := Image{ID: "sha256:abcdef1234567890"}
	if got := none.Tag(); got != "<none>" {
		t.Errorf("Tag = %q, want <none>", got)
	}
}
