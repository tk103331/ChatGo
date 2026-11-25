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

func TestCallRendererFunctionResponds(t *testing.T) {
	r, _ := rendererFor(t)
	var got a2ui.RendererMessage
	r.opts.OnRendererMessage = func(m a2ui.RendererMessage) { got = m }
	call := &a2ui.CallRendererFunction{
		SurfaceID: "form", FunctionCallID: "f1",
		CallFunction: map[string]any{
			"@call": "required", "catalogId": a2ui.BasicCatalogID,
			"args": map[string]any{"value": map[string]any{"@path": "/contact/firstName"}},
		},
	}
	errs := r.Apply(a2ui.Message{Version: a2ui.Version, CallRendererFunction: call})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if got.FunctionResponse == nil || got.FunctionResponse.FunctionCallID != "f1" {
		t.Fatalf("response = %#v", got)
	}
	if got.FunctionResponse.Value != false {
		t.Fatalf("value = %#v", got.FunctionResponse.Value)
	}
}

func TestRendererInitiatedAgentFunction(t *testing.T) {
	r, _ := rendererFor(t)
	var sent a2ui.RendererMessage
	r.opts.OnRendererMessage = func(m a2ui.RendererMessage) { sent = m }
	var result any
	var resultErr *a2ui.RendererError
	r.opts.OnAgentFunctionResult = func(id string, v any, err *a2ui.RendererError) { result, resultErr = v, err }
	id, err := r.CallAgentFunction("form", "lookup", a2ui.BasicCatalogID, map[string]any{"q": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if sent.CallAgentFunction == nil || sent.CallAgentFunction.FunctionCallID != id {
		t.Fatalf("sent = %#v", sent)
	}
	errs := r.Apply(a2ui.Message{Version: a2ui.Version, AgentFunctionResponse: &a2ui.AgentFunctionResponse{
		FunctionCallID: id, Value: "ok",
	}})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if result != "ok" || resultErr != nil {
		t.Fatalf("result = %#v err = %#v", result, resultErr)
	}
}

func TestSetThemeChangesNextFrame(t *testing.T) {
	r, _ := rendererFor(t)
	before := r.Theme()
	r.SetTheme(DarkTheme())
	if r.Theme() == before {
		t.Fatal("theme did not change")
	}
	img := ui.Render(func(c *ui.Context) { r.View(c, "form") }, 720, 900, 1)
	if img == nil {
		t.Fatal("render after theme change failed")
	}
}
