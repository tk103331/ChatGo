package codeeditorapp

import "testing"

func TestFindNext(t *testing.T) {
	if got := findNext("aXbxc", "x", 0, false); got != 3 {
		t.Fatalf("findNext: %d", got)
	}
	if got := findNext("aXbxc", "x", 4, false); got != -1 {
		t.Fatalf("findNext from end: %d", got)
	}
	if got := findNext("aXbxc", "x", 0, true); got != 1 {
		t.Fatalf("findNext ci: %d", got)
	}
	if got := findNext("abc", "", 0, false); got != -1 {
		t.Fatalf("empty needle: %d", got)
	}
}

func TestFindPrev(t *testing.T) {
	if got := findPrev("axbxc", "x", 4, false); got != 3 {
		t.Fatalf("findPrev: %d", got)
	}
	if got := findPrev("axbxc", "x", 2, false); got != 1 {
		t.Fatalf("findPrev earlier: %d", got)
	}
	if got := findPrev("axbxc", "x", 0, false); got != -1 {
		t.Fatalf("findPrev at 0: %d", got)
	}
}

func TestReplaceAll(t *testing.T) {
	if got := replaceAll("aXbxc", "x", "y", false); got != "aXbyc" {
		t.Fatalf("replace: %q", got)
	}
	if got := replaceAll("aXbxc", "x", "y", true); got != "aybyc" {
		t.Fatalf("replace ci: %q", got)
	}
	if got := replaceAll("abc", "", "y", false); got != "abc" {
		t.Fatalf("empty find: %q", got)
	}
}
