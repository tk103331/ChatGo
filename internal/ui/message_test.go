package ui

import "testing"

func TestReasoningFoldFollowsStreaming(t *testing.T) {
	var f reasoningFold

	f.follow(false)
	if f.open {
		t.Fatal("completed thinking should start folded")
	}

	f.follow(true)
	if !f.open {
		t.Fatal("streaming thinking should open")
	}

	f.open = false
	f.follow(true)
	if f.open {
		t.Fatal("a manual collapse should hold while the stream continues")
	}

	f.follow(false)
	f.open = true
	f.follow(false)
	if !f.open {
		t.Fatal("opening a completed fold should stick")
	}
}
