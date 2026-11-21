package ui

import (
	"testing"

	"chatgo/internal/config"
)

// An ACP agent without a configured icon file still sends its brand logo,
// which is what the desktop draws beside its name.
func TestWebRuntimeAgentIconFallsBackToBrandLogo(t *testing.T) {
	r := webRuntime{cw: &ChatWindow{cfg: &config.Config{}}}
	data, typ := r.AgentIcon("acp:OpenCode")
	if len(data) == 0 || typ == "" {
		t.Fatalf("no brand logo: %d bytes, %q", len(data), typ)
	}
	if data, _ := r.AgentIcon("agent:Unknown"); len(data) != 0 {
		t.Fatalf("unknown agent got %d bytes", len(data))
	}
}
