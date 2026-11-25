package ui

import (
	"encoding/json"
	"testing"

	mgo "github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
	"chatgo/pkg/a2ui/hybrid"
	"chatgo/pkg/a2ui/native"
	"chatgo/pkg/models"
)

func TestA2UIInputInMessageList(t *testing.T) {
	cw := testWindow()
	cw.mode = modeChat
	cw.section = sectionChat
	cw.drafts = map[string]string{}
	cw.a2ui = hybrid.New(native.New(native.Options{}), nil)
	cw.a2uiFed = map[string]int{}
	cw.a2uiOwned = map[string][]string{}
	cw.a2uiData = map[string]map[string]any{}
	conv := &models.Conversation{ID: "c1", Agent: "agent:"}
	conv.Messages = []models.Message{{
		ID: "a1", Role: "assistant", Content: "表单：",
		A2UI: []json.RawMessage{json.RawMessage(`{"version":"1.0","createSurface":{"surfaceId":"form","catalogId":"` + a2ui.BasicCatalogID + `","dataModel":{"name":""},"components":[{"id":"root","component":"Card","child":"col"},{"id":"col","component":"Column","children":["f"]},{"id":"f","component":"TextField","label":"姓名","value":{"@path":"/name"}}]}}`)},
	}}
	cw.cur = conv
	cw.center = newCenterState()
	cw.center.ensureRoot()
	cw.openCenterTab(0, &centerTab{ID: "chat:c1", ConvID: "c1", Kind: centerChat})
	tt := mgo.NewTester(cw.view, 1000, 800)
	tt.SetFocused(true)
	rect, ok := tt.Find("姓名")
	if !ok {
		t.Fatal("label not found")
	}
	tt.ClickAt(rect.X+40, rect.Y+float32(rect.H)+12)
	tt.Type("A")
	tt.Frame()
	tt.Type("d")
	tt.Frame()
	tt.Type("a")
	tt.Frame()
	data, _ := cw.a2ui.DataModel("a1\x00form")
	if a2ui.ResolveString(map[string]any{"@path": "/name"}, data, nil) != "Ada" {
		t.Fatalf("data = %#v", data)
	}
}
