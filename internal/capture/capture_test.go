package capture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptText(t *testing.T) {
	cases := []struct {
		text string
		min  int
		want bool
	}{
		{"", 2, false},
		{"   ", 2, false},
		{"a", 2, false},
		{"ab", 2, true},
		{"  ab  ", 2, true},
		{"中文", 2, true},
		{"中", 1, true},
		{"中", 0, true},
	}
	for _, c := range cases {
		if got := AcceptText(c.text, c.min); got != c.want {
			t.Errorf("AcceptText(%q, %d) = %v, want %v", c.text, c.min, got, c.want)
		}
	}
}

func TestClipText(t *testing.T) {
	if got := ClipText("abcdef", 3); got != "abc…" {
		t.Errorf("ClipText = %q", got)
	}
	if got := ClipText("abc", 3); got != "abc" {
		t.Errorf("ClipText short = %q", got)
	}
	if got := ClipText("abcdef", 0); got != "abcdef" {
		t.Errorf("ClipText unlimited = %q", got)
	}
}

func TestQuote(t *testing.T) {
	got := Quote("第一行\n第二行", 100)
	want := "> 第一行\n> 第二行"
	if got != want {
		t.Errorf("Quote = %q, want %q", got, want)
	}
	if got := Quote("   ", 100); got != "" {
		t.Errorf("Quote blank = %q", got)
	}
}

func TestParseFileURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"file:///Users/me/My%20Docs", "/Users/me/My Docs", true},
		{" /tmp/demo ", "/tmp/demo", true},
		{"file:///tmp/demo\x00", "/tmp/demo", true},
		{"relative/path", "", false},
		{"", "", false},
		{"https://example.com", "", false},
	}
	for _, c := range cases {
		got, ok := ParseFileURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseFileURL(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestExistingDir(t *testing.T) {
	dir := t.TempDir()
	if !ExistingDir(dir) {
		t.Errorf("ExistingDir(%q) = false", dir)
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ExistingDir(file) {
		t.Errorf("ExistingDir(file) = true")
	}
	if ExistingDir(filepath.Join(dir, "missing")) {
		t.Errorf("ExistingDir(missing) = true")
	}
}

func TestPlaceNear(t *testing.T) {
	work := Rect{X: 0, Y: 0, Width: 1440, Height: 900}
	size := Size{Width: 400, Height: 200}

	// Below the selection when there is room.
	x, y := PlaceNear(Rect{X: 100, Y: 100, Width: 200, Height: 20}, size, work)
	if x != 100 || y != 128 {
		t.Errorf("below: got %d,%d", x, y)
	}

	// Flips above near the bottom edge.
	x, y = PlaceNear(Rect{X: 100, Y: 850, Width: 200, Height: 20}, size, work)
	if x != 100 || y != 642 {
		t.Errorf("above: got %d,%d", x, y)
	}

	// Clamped inside the right edge.
	x, _ = PlaceNear(Rect{X: 1300, Y: 100, Width: 100, Height: 20}, size, work)
	if want := 1440 - 400 - 8; x != want {
		t.Errorf("clamp right: got %d, want %d", x, want)
	}

	// A zero anchor centers a third of the way down.
	x, y = PlaceNear(Rect{}, size, work)
	if x != (1440-400)/2 || y != 900/3 {
		t.Errorf("no anchor: got %d,%d", x, y)
	}

	// A box larger than the work area starts at its corner.
	x, y = PlaceNear(Rect{X: 10, Y: 10}, Size{Width: 2000, Height: 1000}, work)
	if x != 0 || y != 0 {
		t.Errorf("oversize: got %d,%d", x, y)
	}
}

func TestQuoteKeepsMarker(t *testing.T) {
	if got := Quote("a\r\nb", 10); strings.Contains(got, "\r") {
		t.Errorf("Quote kept a carriage return: %q", got)
	}
}

func TestTextOnlyClipboard(t *testing.T) {
	if !textOnlyClipboard(nil) {
		t.Error("an empty clipboard is text-only")
	}
	if !textOnlyClipboard([]string{"text/plain", "text/html"}) {
		t.Error("a text-only clipboard was not recognized")
	}
	if textOnlyClipboard([]string{"text/plain", "image/png"}) {
		t.Error("a clipboard with an image was called text-only")
	}
}
