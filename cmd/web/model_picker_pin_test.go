package main

import (
	"fmt"
	"testing"

	ui "github.com/egoist/mygo/ui"
)

// The web model picker's search field is pinned above the list: scrolling the
// models leaves it where it was.
func TestWebModelPickerSearchStaysWhileTheListScrolls(t *testing.T) {
	a := testApp()
	a.models = []modelInfo{{Ref: "Local::llama3", Label: "llama3", Provider: "Local", ProviderType: "ollama"}}
	for i := range 60 {
		a.models = append(a.models, modelInfo{
			Ref:          fmt.Sprintf("Local::model-%c%c", 'a'+i%26, 'a'+i/26),
			Label:        fmt.Sprintf("model-%c%c", 'a'+i%26, 'a'+i/26),
			Provider:     "Local",
			ProviderType: "ollama",
		})
	}
	tr := ui.NewTester(a.view, 1180, 820)
	for range 4 {
		tr.Frame()
	}
	if err := tr.Click("llama3"); err != nil {
		t.Fatalf("open the model picker: %v", err)
	}
	for range 4 {
		tr.Frame()
	}
	before, ok := tr.AccessByRole(ui.RoleTextField, "搜索模型")
	if !ok {
		t.Fatalf("the picker has no search field; texts: %v", tr.Texts())
	}
	// Scroll the list down; the pinned field must not move.
	row, ok := tr.Find("model-aa")
	if !ok {
		t.Fatalf("the model list did not render; texts: %v", tr.Texts())
	}
	tr.Scroll(row.X+row.W/2, row.Y+row.H/2, 0, 300)
	for range 4 {
		tr.Frame()
	}
	after, ok := tr.AccessByRole(ui.RoleTextField, "搜索模型")
	if !ok {
		t.Fatalf("the search field is gone after scrolling")
	}
	if after.Bounds.Y != before.Bounds.Y {
		t.Fatalf("the search field moved with the list: before %v, after %v",
			before.Bounds, after.Bounds)
	}
}
