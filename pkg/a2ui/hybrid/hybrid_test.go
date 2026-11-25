package hybrid

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

type fakeRenderer struct {
	name     string
	catalog  string
	messages []a2ui.Message
	surfaces map[string]bool
}

func (f *fakeRenderer) Apply(msgs ...a2ui.Message) []error {
	f.messages = append(f.messages, msgs...)
	if f.surfaces == nil {
		f.surfaces = map[string]bool{}
	}
	for _, m := range msgs {
		if m.CreateSurface != nil {
			f.surfaces[m.CreateSurface.SurfaceID] = true
		}
		if m.DeleteSurface != nil {
			delete(f.surfaces, m.DeleteSurface.SurfaceID)
		}
	}
	return nil
}
func (f *fakeRenderer) View(c *ui.Context, id string) { ui.Text(c, f.name+":"+id) }
func (f *fakeRenderer) Close()                        {}
func (f *fakeRenderer) SupportsCatalog(id string) bool {
	return id == "" || id == f.catalog
}

func msg(t *testing.T, raw string) a2ui.Message {
	t.Helper()
	var m a2ui.Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoutesByCatalog(t *testing.T) {
	native := &fakeRenderer{name: "native", catalog: a2ui.BasicCatalogID}
	web := &fakeRenderer{name: "web", catalog: "https://example.com/custom"}
	r := New(native, web)
	r.Apply(msg(t, `{"version":"1.0","createSurface":{"surfaceId":"basic","catalogId":"`+a2ui.BasicCatalogID+`"}}`))
	r.Apply(msg(t, `{"version":"1.0","createSurface":{"surfaceId":"rich","catalogId":"https://example.com/custom"}}`))
	if got := r.RouteOf("basic"); got != "native" {
		t.Fatalf("basic route = %q", got)
	}
	if got := r.RouteOf("rich"); got != "web" {
		t.Fatalf("rich route = %q", got)
	}
	if len(native.messages) != 1 || len(web.messages) != 1 {
		t.Fatalf("native=%d web=%d", len(native.messages), len(web.messages))
	}

	r.Apply(msg(t, `{"version":"1.0","createSurface":{"surfaceId":"mixed","catalogId":"`+a2ui.BasicCatalogID+`","components":[{"id":"root","component":"Text","text":"x","@catalogId":"https://example.com/custom"}]}}`))
	if got := r.RouteOf("mixed"); got != "web" {
		t.Fatalf("mixed route = %q", got)
	}
}

func TestMessagesFollowTheirSurface(t *testing.T) {
	native := &fakeRenderer{name: "native", catalog: a2ui.BasicCatalogID}
	web := &fakeRenderer{name: "web"}
	r := New(native, web)
	r.Apply(
		msg(t, `{"version":"1.0","createSurface":{"surfaceId":"s","catalogId":"`+a2ui.BasicCatalogID+`"}}`),
		msg(t, `{"version":"1.0","updateDataModel":{"surfaceId":"s","path":"/a","value":1}}`),
	)
	if len(native.messages) != 2 || len(web.messages) != 0 {
		t.Fatalf("native=%d web=%d", len(native.messages), len(web.messages))
	}
	r.Apply(msg(t, `{"version":"1.0","deleteSurface":{"surfaceId":"s"}}`))
	if r.RouteOf("s") != "" {
		t.Fatal("route survived delete")
	}
}

func TestMissingRendererIsAnError(t *testing.T) {
	r := New(nil, nil)
	errs := r.Apply(msg(t, `{"version":"1.0","createSurface":{"surfaceId":"s"}}`))
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
}
