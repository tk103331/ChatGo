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
}

// A custom agent has no brand: the phone gets the generated avatar the
// desktop draws for it.
func TestWebRuntimeAgentIconFallsBackToGeneratedAvatar(t *testing.T) {
	r := webRuntime{cw: &ChatWindow{cfg: &config.Config{}}}
	data, typ := r.AgentIcon("agent:Researcher")
	if len(data) == 0 || typ != "image/png" {
		t.Fatalf("no generated avatar: %d bytes, %q", len(data), typ)
	}
	// An unknown agent still has a name, so it still gets a picture.
	data, typ = r.AgentIcon("agent:Unknown")
	if len(data) == 0 || typ != "image/png" {
		t.Fatalf("unknown agent got %d bytes, %q", len(data), typ)
	}
}

// A seeded ACP connection prefers its generated avatar over the brand logo,
// as the desktop's picker does.
func TestWebRuntimeAgentIconUsesTheAvatarSeed(t *testing.T) {
	r := webRuntime{cw: &ChatWindow{cfg: &config.Config{
		ACPAgents: []config.ACPConnection{{Name: "OpenCode", AvatarSeed: "seed-1"}},
	}}}
	data, typ := r.AgentIcon("acp:OpenCode")
	if len(data) == 0 || typ != "image/png" {
		t.Fatalf("seeded connection did not use its avatar: %d bytes, %q", len(data), typ)
	}
}
