package ui

import (
	"testing"

	"chatgo/pkg/models"

	mgo "github.com/egoist/mygo/ui"

	mtheme "chatgo/internal/ui/theme"
	mcore "chatgo/internal/ui/uikit"
)

// The picker groups model refs by provider in first-appearance order; a group
// reference is its own pseudo-provider and provider-less refs land in the
// anonymous group.
func TestModelPickGroups(t *testing.T) {
	refs := []string{"OpenAI::gpt-4", "OpenAI::gpt-4o", "DeepSeek::v3", "Router::"}
	groups := modelPickGroups(refs, func(name string) string {
		if name == "OpenAI" {
			return "openai"
		}
		return ""
	})
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[0].name != "OpenAI" || groups[0].typ != "openai" || len(groups[0].models) != 2 {
		t.Fatalf("group 0 = %+v", groups[0])
	}
	if groups[1].name != "DeepSeek" || len(groups[1].models) != 1 {
		t.Fatalf("group 1 = %+v", groups[1])
	}
	if groups[2].name != "Router" || len(groups[2].models) != 1 {
		t.Fatalf("group 2 = %+v", groups[2])
	}
	// Every group keeps a mark, generic when the provider type is unknown.
	for _, g := range groups {
		if g.logo() == nil {
			t.Fatalf("group %q has no mark", g.name)
		}
	}
}

// Row labels strip the provider prefix; a group reference keeps its display
// name.
func TestModelPickRowLabel(t *testing.T) {
	cw := testWindow()
	if got := cw.modelPickRowLabel("OpenAI::gpt-4"); got != "gpt-4" {
		t.Fatalf("row label = %q", got)
	}
	if got := cw.modelPickRowLabel("Router::"); got != "Router" {
		t.Fatalf("group row label = %q", got)
	}
}

// The picker body renders its searchable model list, and a query that matches
// nothing still renders with the empty hint.
func TestModelPickerBodyRenders(t *testing.T) {
	cw := testWindow()
	cw.cur = &models.Conversation{ID: "c", Title: "t"}
	refs := cw.agentModelRefs() // OpenAI::gpt-4 and OpenAI::gpt-4o
	query := ""
	renderBody := func() bool {
		img := mgo.Render(func(c *mgo.Context) {
			mcore.Use(c, mcore.Settings{Density: mtheme.Compact})
			cw.modelPickerBody(c, refs, &query, func() {})
		}, 720, 420, 1)
		return img != nil
	}
	if !renderBody() {
		t.Fatal("the picker did not render")
	}
	query = "不存在的"
	if !renderBody() {
		t.Fatal("the empty picker did not render")
	}
}
