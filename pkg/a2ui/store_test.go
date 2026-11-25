package a2ui

import (
	"encoding/json"
	"testing"
)

const createJSON = `{
  "version": "1.0",
  "createSurface": {
    "surfaceId": "main",
    "catalogId": "https://a2ui.org/specification/v1_0/catalogs/basic/catalog.json",
    "dataModel": {"contact": {"firstName": "Ada"}},
    "components": [
      {"id": "root", "component": "Column", "children": ["name", "field"]},
      {"id": "name", "component": "Text", "text": {"@path": "/contact/firstName"}},
      {"id": "field", "component": "TextField", "label": "First name", "value": {"@path": "/contact/firstName"}}
    ]
  }
}`

func mustMessage(t *testing.T, raw string) Message {
	t.Helper()
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestStoreAppliesSurfaceAndData(t *testing.T) {
	st := NewStore()
	if errs := st.Apply(mustMessage(t, createJSON)); len(errs) != 0 {
		t.Fatalf("apply: %v", errs)
	}
	snap, ok := st.Snapshot("main")
	if !ok {
		t.Fatal("surface missing")
	}
	if snap.Surface.CatalogID != BasicCatalogID {
		t.Fatalf("catalog = %q", snap.Surface.CatalogID)
	}
	field, ok := snap.Surface.Component("field")
	if !ok || field.Type != "TextField" || field.PropString("label") != "First name" {
		t.Fatalf("field = %#v", field)
	}
	if got, _ := PointerGet(snap.Surface.Data, "/contact/firstName"); got != "Ada" {
		t.Fatalf("data = %#v", got)
	}

	if errs := st.Apply(mustMessage(t, `{"version":"1.0","updateDataModel":{"surfaceId":"main","path":"/contact/firstName","value":"Grace"}}`)); len(errs) != 0 {
		t.Fatal(errs)
	}
	snap, _ = st.Snapshot("main")
	if got, _ := PointerGet(snap.Surface.Data, "/contact/firstName"); got != "Grace" {
		t.Fatalf("updated data = %#v", got)
	}

	if errs := st.Apply(mustMessage(t, `{"version":"1.0","updateComponents":{"surfaceId":"main","components":[{"id":"name","component":"Text","text":"Hello"}]}}`)); len(errs) != 0 {
		t.Fatal(errs)
	}
	snap, _ = st.Snapshot("main")
	name, _ := snap.Surface.Component("name")
	if name.PropString("text") != "Hello" {
		t.Fatalf("component not replaced: %#v", name)
	}

	if errs := st.Apply(mustMessage(t, `{"version":"1.0","deleteSurface":{"surfaceId":"main"}}`)); len(errs) != 0 {
		t.Fatal(errs)
	}
	if _, ok := st.Snapshot("main"); ok {
		t.Fatal("surface survived delete")
	}
}

func TestStoreReportsEachBadMessage(t *testing.T) {
	st := NewStore()
	errs := st.Apply(
		mustMessage(t, `{"version":"1.0","updateComponents":{"surfaceId":"missing","components":[]}}`),
		mustMessage(t, `{"version":"1.0","createSurface":{"surfaceId":"ok"}}`),
		mustMessage(t, `{"version":"1.0","createSurface":{"surfaceId":"ok"}}`),
	)
	if len(errs) != 2 {
		t.Fatalf("errors = %v", errs)
	}
	if _, ok := st.Snapshot("ok"); !ok {
		t.Fatal("valid message after a bad one was dropped")
	}
}

func TestSnapshotIsIsolated(t *testing.T) {
	st := NewStore()
	if errs := st.Apply(mustMessage(t, createJSON)); len(errs) != 0 {
		t.Fatal(errs)
	}
	snap, _ := st.Snapshot("main")
	snap.Surface.Components["root"] = Component{ID: "root", Type: "Text"}
	snap.Surface.Data = map[string]any{}
	again, _ := st.Snapshot("main")
	if again.Surface.Components["root"].Type != "Column" {
		t.Fatal("mutating a snapshot changed the store")
	}
	if got, _ := PointerGet(again.Surface.Data, "/contact/firstName"); got != "Ada" {
		t.Fatal("mutating snapshot data changed the store")
	}
}

func TestSetDataWritesAndBumpsRevision(t *testing.T) {
	st := NewStore()
	st.Apply(mustMessage(t, createJSON))
	first, _ := st.Snapshot("main")
	rev, ok := st.SetData("main", "/contact/firstName", "Grace")
	if !ok || rev <= first.Revision {
		t.Fatalf("revision = %d, want > %d", rev, first.Revision)
	}
	snap, _ := st.Snapshot("main")
	if got, _ := PointerGet(snap.Surface.Data, "/contact/firstName"); got != "Grace" {
		t.Fatalf("data = %#v", got)
	}
}
