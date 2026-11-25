package a2ui

import "testing"

func TestPointerGetSet(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{"b": []any{"x", "y"}},
		"n": 1.0,
	}
	if got, ok := PointerGet(data, "/a/b/1"); !ok || got != "y" {
		t.Fatalf("get = %#v %v", got, ok)
	}
	if _, ok := PointerGet(data, "/a/missing"); ok {
		t.Fatal("missing path found")
	}
	out, err := PointerSet(data, "/a/b/0", "z")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PointerGet(out, "/a/b/0"); got != "z" {
		t.Fatalf("set = %#v", got)
	}
	out, err = PointerSet(out, "/a/new/deep", true)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PointerGet(out, "/a/new/deep"); got != true {
		t.Fatalf("created path = %#v", got)
	}
	out, err = PointerSet(out, "/n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := PointerGet(out, "/n"); ok {
		t.Fatal("nil delete left the key")
	}
	out, err = PointerSet(out, "/", map[string]any{"fresh": 1.0})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := PointerGet(out, "/fresh"); got != 1.0 {
		t.Fatalf("root replace = %#v", got)
	}
}

func TestPointerEscapes(t *testing.T) {
	data := map[string]any{"a/b": map[string]any{"~x": 1.0}}
	if got, ok := PointerGet(data, "/a~1b/~0x"); !ok || got != 1.0 {
		t.Fatalf("escaped = %#v %v", got, ok)
	}
}
