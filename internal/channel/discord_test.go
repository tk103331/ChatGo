package channel

import (
	"context"
	"strings"
	"testing"
)

// Discord's mention arrives as <@id>, and only the prompt should survive.
func TestStripDiscordMention(t *testing.T) {
	if got := stripDiscordMention("<@123> hello", "123"); got != " hello" {
		t.Fatalf("got %q", got)
	}
	if got := stripDiscordMention("<@!123> hello", "123"); got != " hello" {
		t.Fatalf("the nickname form was not stripped: %q", got)
	}
	if got := stripDiscordMention("<@999> hello", "123"); got != "<@999> hello" {
		t.Fatalf("another user's mention was stripped: %q", got)
	}
}

// Discord rejects a message past 2000 characters, so a long reply is split at
// a paragraph, then a line, then a space, never mid-word.
func TestSplitMessage(t *testing.T) {
	parts := splitMessage(strings.Repeat("a", 50)+"\n\n"+strings.Repeat("b", 50), 60)
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2: %q", len(parts), parts)
	}
	if parts[0] != strings.Repeat("a", 50) {
		t.Fatalf("first part is %q", parts[0])
	}
	if parts[1] != strings.Repeat("b", 50) {
		t.Fatalf("second part is %q", parts[1])
	}
	// A single long word still fits by being cut at the limit.
	parts = splitMessage(strings.Repeat("x", 130), 60)
	if len(parts) != 3 || len(parts[0]) != 60 {
		t.Fatalf("a long word was not cut at the limit: %d parts", len(parts))
	}
}

// Missing token is refused before any connection.
func TestDiscordRequiresToken(t *testing.T) {
	if err := NewDiscord().Start(context.Background(), Config{Type: "discord"}, func(Inbound) {}); err == nil {
		t.Fatal("a driver without a token must refuse to start")
	}
}
