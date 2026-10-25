package kit

import "testing"

// Two selectors with different keys must not share their state, or opening one
// would open both.
func TestBranchStateKeysAreDistinct(t *testing.T) {
	openA, queryA := branchStateKeys("gitpanel")
	openB, queryB := branchStateKeys("gitgraph")
	if openA == openB || queryA == queryB {
		t.Fatalf("selectors share state: %q/%q vs %q/%q", openA, queryA, openB, queryB)
	}
	if openA == "" || queryA == "" {
		t.Fatal("keys must not be empty")
	}
	plainOpen, plainQuery := branchStateKeys("")
	if plainOpen != "chatgo.branch.open" || plainQuery != "chatgo.branch.query" {
		t.Fatalf("the keyless form must keep its old names, got %q/%q", plainOpen, plainQuery)
	}
	if plainOpen == openA {
		t.Fatal("a keyed selector must not collide with a keyless one")
	}
}
