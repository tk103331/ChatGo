package logapp

import (
	"os"
	"path/filepath"
	"testing"

	mcode "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/code"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"2026-10-20 10:00:00 INFO server started", int(mcode.LogInfo)},
		{"[INFO] ok", int(mcode.LogInfo)},
		{"plain text line", int(mcode.LogInfo)},
		{"2026-10-20 10:00:01 WARN disk almost full", int(mcode.LogWarn)},
		{"[WARNING] deprecated", int(mcode.LogWarn)},
		{"ERROR connection refused", int(mcode.LogError)},
		{"[ERR] timeout", int(mcode.LogError)},
		{"FATAL: out of memory", int(mcode.LogError)},
		{"DEBUG verbose detail", int(mcode.LogDebug)},
		{"TRACE entry", int(mcode.LogDebug)},
	}
	for _, c := range cases {
		if got := int(parseLevel(c.line)); got != c.want {
			t.Errorf("parseLevel(%q) = %d, want %d", c.line, got, c.want)
		}
	}
}

func TestParseTailN(t *testing.T) {
	if got := parseTailN("100"); got != 100 {
		t.Errorf("parseTailN(\"100\") = %d, want 100", got)
	}
	if got := parseTailN(""); got != defaultTail {
		t.Errorf("parseTailN(\"\") = %d, want %d", got, defaultTail)
	}
	if got := parseTailN("abc"); got != defaultTail {
		t.Errorf("parseTailN(\"abc\") = %d, want %d", got, defaultTail)
	}
	if got := parseTailN("0"); got != defaultTail {
		t.Errorf("parseTailN(\"0\") = %d, want %d", got, defaultTail)
	}
	if got := parseTailN("999999999"); got != maxLines {
		t.Errorf("parseTailN(\"999999999\") = %d, want %d", got, maxLines)
	}
}

func TestTailLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	var content string
	for i := 1; i <= 100; i++ {
		content += "line " + itoa(i) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, size, err := tailLines(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 10 {
		t.Fatalf("tailLines returned %d rows, want 10", len(rows))
	}
	if rows[0] != "line 91" || rows[9] != "line 100" {
		t.Fatalf("tailLines returned wrong rows: %v", rows)
	}
	if size != int64(len(content)) {
		t.Fatalf("tailLines size = %d, want %d", size, len(content))
	}
	rowsAll, _, err := tailLines(path, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsAll) != 100 {
		t.Fatalf("tailLines(500) returned %d rows, want 100", len(rowsAll))
	}
}

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "all.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, _, err := loadAll(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0] != "a" || rows[2] != "c" {
		t.Fatalf("loadAll = %v", rows)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
