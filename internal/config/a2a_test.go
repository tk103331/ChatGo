package config

import "testing"

func TestEnsureA2AAgents(t *testing.T) {
	got := ensureA2AAgents([]A2AConnection{
		{Name: "  remote  ", URL: " https://a.example.com "},
		{Name: "remote", URL: "https://duplicate.example.com"},
		{Name: "", URL: "https://nameless.example.com"},
		{Name: "no-url"},
		{Name: "second", URL: "https://b.example.com"},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].Name != "remote" || got[0].URL != "https://a.example.com" {
		t.Fatalf("first = %#v", got[0])
	}
	if got[1].Name != "second" {
		t.Fatalf("second = %#v", got[1])
	}
}
