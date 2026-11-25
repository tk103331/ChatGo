package native

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

const formSurface = `{
  "version": "1.0",
  "createSurface": {
    "surfaceId": "form",
    "catalogId": "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json",
    "dataModel": {"contact": {"firstName": "", "subscribe": false, "volume": 20}},
    "components": [
      {"id": "root", "component": "Card", "child": "col"},
      {"id": "col", "component": "Column", "children": ["title", "row", "field", "check", "slider", "button", "unknown"]},
      {"id": "title", "component": "Text", "text": "# Contact Us", "variant": "h2"},
      {"id": "row", "component": "Row", "children": ["a", "b"]},
      {"id": "a", "component": "Text", "text": "Left"},
      {"id": "b", "component": "Text", "text": "Right"},
      {"id": "field", "component": "TextField", "label": "First Name", "value": {"@path": "/contact/firstName"}},
      {"id": "check", "component": "CheckBox", "label": "Subscribe", "value": {"@path": "/contact/subscribe"}},
      {"id": "slider", "component": "Slider", "label": "Volume", "min": 0, "max": 100, "value": {"@path": "/contact/volume"}},
      {"id": "label", "component": "Text", "text": "Send"},
      {"id": "button", "component": "Button", "child": "label", "variant": "primary", "action": {"event": {"name": "submit"}}},
      {"id": "unknown", "component": "RadarChart", "data": []}
    ]
  }
}`

func rendererFor(t *testing.T) (*Renderer, a2ui.Message) {
	t.Helper()
	var msg a2ui.Message
	if err := json.Unmarshal([]byte(formSurface), &msg); err != nil {
		t.Fatal(err)
	}
	r := New(Options{})
	if errs := r.Apply(msg); len(errs) != 0 {
		t.Fatalf("apply: %v", errs)
	}
	return r, msg
}

func TestRenderSurface(t *testing.T) {
	r, _ := rendererFor(t)
	img := ui.Render(func(c *ui.Context) { r.View(c, "form") }, 720, 900, 1)
	if img == nil || img.Bounds().Dx() != 720 {
		t.Fatalf("render = %v", img)
	}
}

func TestUnknownComponentDoesNotPanic(t *testing.T) {
	r, _ := rendererFor(t)
	img := ui.Render(func(c *ui.Context) { r.View(c, "form") }, 400, 400, 1)
	if img == nil {
		t.Fatal("render failed")
	}
}

func TestMissingSurfaceShowsPlaceholder(t *testing.T) {
	r := New(Options{})
	img := ui.Render(func(c *ui.Context) { r.View(c, "missing") }, 320, 200, 1)
	if img == nil {
		t.Fatal("render failed")
	}
}

func TestSetDataUpdatesBinding(t *testing.T) {
	r, _ := rendererFor(t)
	if _, ok := r.Store().SetData("form", "/contact/firstName", "Grace"); !ok {
		t.Fatal("SetData failed")
	}
	snap, ok := r.Store().Snapshot("form")
	if !ok {
		t.Fatal("surface missing")
	}
	if got, _ := a2ui.PointerGet(snap.Surface.Data, "/contact/firstName"); got != "Grace" {
		t.Fatalf("data = %#v", got)
	}
	img := ui.Render(func(c *ui.Context) { r.View(c, "form") }, 720, 900, 1)
	if img == nil {
		t.Fatal("render after update failed")
	}
}
