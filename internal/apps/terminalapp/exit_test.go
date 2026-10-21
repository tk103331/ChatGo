package terminalapp

import (
	"testing"
	"time"
)

func TestShellExitRestarts(t *testing.T) {
	cl := New(&host{dir: t.TempDir()})
	id := cl.NewTab()
	tb := cl.byID[id]
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && tb.term == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if tb.term == nil {
		t.Skip("pty not available")
	}
	tb.term.Send([]byte("exit\n"))
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if tb.restarts > 0 {
			t.Logf("restarts=%d exited=%v", tb.restarts, tb.exited)
			return
		}
	}
	t.Fatalf("shell did not restart after exit; restarts=%d exited=%v", tb.restarts, tb.exited)
}
