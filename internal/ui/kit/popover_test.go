package kit

import (
	"testing"

	mgo "github.com/egoist/mygo/ui"
)

// TestPopoverUpIsStartAligned pins the anchors an upward popover uses. The
// panel must hang from the anchor's top start with its own bottom start; the
// earlier AnchorTopEnd put the panel's leading edge on the anchor's trailing
// edge, so the panel appeared shifted away from its button.
func TestPopoverUpIsStartAligned(t *testing.T) {
	at, self := popoverUpAnchors()
	if at != mgo.AnchorTopStart {
		t.Fatalf("an upward popover must attach to the anchor's top start, got %v", at)
	}
	if self != mgo.AnchorBottomStart {
		t.Fatalf("the panel must offer its bottom start, got %v", self)
	}
	// Anchors on the start side share a column; an End anchor would offset the
	// panel by the button's width.
	for _, a := range []mgo.Anchor{at, self} {
		if a == mgo.AnchorTopEnd || a == mgo.AnchorEnd || a == mgo.AnchorBottomEnd {
			t.Fatalf("anchor %v is on the end side, which offsets the panel", a)
		}
	}
}
