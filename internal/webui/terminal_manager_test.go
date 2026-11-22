package webui

import (
	"strings"
	"testing"
	"time"
)

// A terminal session keeps running with nobody attached, and a new client
// reads what it printed since its cursor.
func TestTerminalSessionSurvivesAndReplays(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	m := NewTerminalManager(func(string) (string, error) { return dir, nil })
	defer m.Close()
	info, err := m.Start(dir, 40, 10, "test")
	if err != nil {
		t.Fatal(err)
	}
	s, ok := m.Get(info.ID)
	if !ok {
		t.Fatal("session missing")
	}
	if err := s.Write([]byte("echo $((6*7))\n")); err != nil {
		t.Fatal(err)
	}
	seq := int64(0)
	var seen strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(seen.String(), "42") {
		if time.Now().After(deadline) {
			t.Fatal("terminal output did not arrive")
		}
		out := s.Output(seq, 200*time.Millisecond)
		seen.Write(out.Data)
		seq = out.Seq
	}
	// A client that lost everything since 0 gets a reset and the full buffer.
	out := s.Output(0, time.Millisecond)
	if !strings.Contains(string(out.Data), "42") {
		t.Fatalf("replay = %+v", out)
	}
	if !m.Remove(info.ID) {
		t.Fatal("session not removed")
	}
	if _, ok := m.Get(info.ID); ok {
		t.Fatal("session survived removal")
	}
}
