package a2ui

import (
	"encoding/json"
	"testing"
)

func TestLegacyNestedComponent(t *testing.T) {
	var c Component
	raw := `{"id":"title","component":{"Text":{"text":{"literalString":"Hello"},"usageHint":"h1"}}}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Type != "Text" || c.PropString("usageHint") != "h1" {
		t.Fatalf("component = %#v", c)
	}
	if got := ResolveString(c.Prop("text"), nil, nil); got != "Hello" {
		t.Fatalf("text = %q", got)
	}
}

func TestLegacyExplicitList(t *testing.T) {
	var c Component
	raw := `{"id":"col","component":"Column","children":{"explicitList":["a","b"]}}`
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Children) != 2 || c.Children[1] != "b" {
		t.Fatalf("children = %#v", c.Children)
	}
}

func TestLegacyNestedChildren(t *testing.T) {
	var c Component
	if err := json.Unmarshal([]byte(`{"id":"root","component":{"Column":{"children":{"explicitList":["a","b"]}}}}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Type != "Column" || len(c.Children) != 2 || c.Children[1] != "b" {
		t.Fatalf("component = %#v", c)
	}
}
