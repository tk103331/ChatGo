package a2a

import (
	"testing"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
)

func TestA2UIDataPartIsNotFlattenedIntoText(t *testing.T) {
	part := sdk.NewDataPart([]any{
		map[string]any{"version": "1.0", "createSurface": map[string]any{"surfaceId": "main"}},
	})
	part.MediaType = A2UIMediaType
	part.Metadata = map[string]any{"mimeType": A2UIMediaType}
	parts := []*sdk.Part{sdk.NewTextPart("hello"), part}

	if got := partsText(parts); got != "hello" {
		t.Fatalf("text = %q", got)
	}
	got := collectA2UI(parts)
	if len(got) != 1 {
		t.Fatalf("a2ui = %d", len(got))
	}
	var env map[string]any
	_ = env
	if string(got[0]) == "" {
		t.Fatal("empty envelope")
	}
}

func TestA2UIDataPartDetectedByMetadata(t *testing.T) {
	part := sdk.NewDataPart(map[string]any{"version": "1.0", "createSurface": map[string]any{"surfaceId": "s"}})
	part.Metadata = map[string]any{"mimeType": A2UIMediaType}
	if !isA2UIPart(part) {
		t.Fatal("metadata mimeType not recognized")
	}
	if isA2UIPart(sdk.NewTextPart("x")) {
		t.Fatal("text part recognized as A2UI")
	}
}
