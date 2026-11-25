package ui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	mcore "chatgo/internal/ui/uikit"
	"chatgo/pkg/a2ui"
	"chatgo/pkg/a2ui/hybrid"
	"chatgo/pkg/a2ui/native"
	"chatgo/pkg/models"
)

func TestDecodeA2UIHandlesSingleAndArray(t *testing.T) {
	single := decodeA2UI(json.RawMessage(`{"version":"1.0","createSurface":{"surfaceId":"a"}}`))
	if len(single) != 1 || single[0].CreateSurface == nil {
		t.Fatalf("single = %#v", single)
	}
	many := decodeA2UI(json.RawMessage(`[
		{"version":"1.0","createSurface":{"surfaceId":"a"}},
		{"version":"1.0","updateDataModel":{"surfaceId":"a","path":"/x","value":1}}
	]`))
	if len(many) != 2 || many[1].UpdateDataModel == nil {
		t.Fatalf("many = %#v", many)
	}
}

func TestSurfacePrefixRoundTrip(t *testing.T) {
	msg := a2ui.Message{CreateSurface: &a2ui.CreateSurface{SurfaceID: "form"}}
	prefixed := prefixSurface(msg, "reply-1")
	if prefixed.CreateSurface.SurfaceID != "reply-1"+a2uiSurfaceSep+"form" {
		t.Fatalf("prefixed = %q", prefixed.CreateSurface.SurfaceID)
	}
	msgID, original := splitA2UISurface(prefixed.CreateSurface.SurfaceID)
	if msgID != "reply-1" || original != "form" {
		t.Fatalf("split = %q %q", msgID, original)
	}
}

func TestA2UIMessageRendersNatively(t *testing.T) {
	cw := testWindow()
	cw.a2ui = hybrid.New(native.New(native.Options{}), nil)
	cw.a2uiFed = map[string]int{}
	cw.a2uiFwd = map[string]map[string]any{}
	conv := &models.Conversation{ID: "c1"}
	conv.Messages = []models.Message{{
		ID: "a1", Role: "assistant", Content: "下面是表单",
		A2UI: []json.RawMessage{json.RawMessage(`{"version":"1.0","createSurface":{"surfaceId":"form","catalogId":"` + a2ui.BasicCatalogID + `","dataModel":{"name":"Ada"},"components":[{"id":"root","component":"Column","children":["t","f"]},{"id":"t","component":"Text","text":"# Hello"},{"id":"f","component":"TextField","label":"Name","value":{"@path":"/name"}}]}}`)},
	}}
	cw.cur = conv
	cw.mode = modeChat
	cw.center = newCenterState()
	cw.center.ensureRoot()
	cw.openCenterTab(0, &centerTab{ID: "chat:c1", ConvID: "c1", Kind: centerChat})
	img := mgo.Render(cw.view, 800, 600, 1)
	if img == nil {
		t.Fatal("render failed")
	}
	if cw.a2uiFed["a1"] != 1 {
		t.Fatalf("fed = %d", cw.a2uiFed["a1"])
	}
}

func TestRefreshA2UITheme(t *testing.T) {
	cw := testWindow()
	cw.a2uiNative = native.New(native.Options{})
	cw.themeMode = mcore.Dark
	cw.refreshA2UITheme()
	if cw.a2uiNative.Theme().Text != native.DarkTheme().Text {
		t.Fatal("dark theme not applied")
	}
	cw.themeMode = mcore.Light
	cw.refreshA2UITheme()
	if cw.a2uiNative.Theme().Text != native.DefaultTheme().Text {
		t.Fatal("light theme not applied")
	}
}

func TestBuiltinA2UIToolRendersSurface(t *testing.T) {
	cw := testWindow()
	var got []json.RawMessage
	ctx := withA2UISink(context.Background(), func(raws []json.RawMessage) { got = append(got, raws...) })
	out, err := cw.a2uiTool(ctx, `{
		"surface_id": "form",
		"components": [
			{"id": "root", "component": "Column", "children": ["t"]},
			{"id": "t", "component": "Text", "text": "Hello"}
		],
		"data_model": {"name": "Ada"}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("envelopes = %d", len(got))
	}
	var msg a2ui.Message
	if err := json.Unmarshal(got[0], &msg); err != nil {
		t.Fatal(err)
	}
	if msg.CreateSurface == nil || msg.CreateSurface.SurfaceID != "form" {
		t.Fatalf("message = %#v", msg)
	}
	if !strings.Contains(out, "已渲染") {
		t.Fatalf("result = %q", out)
	}
}

func TestBuiltinA2UIToolNeedsRootAndSink(t *testing.T) {
	cw := testWindow()
	if _, err := cw.a2uiTool(context.Background(), `{"surface_id":"s","components":[]}`); err == nil {
		t.Fatal("tool worked without a sink")
	}
	ctx := withA2UISink(context.Background(), func([]json.RawMessage) {})
	if _, err := cw.a2uiTool(ctx, `{"surface_id":"s","components":[{"id":"x","component":"Text","text":"x"}]}`); err == nil {
		t.Fatal("missing root accepted")
	}
}
