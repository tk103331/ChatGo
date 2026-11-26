package native

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"

	"chatgo/pkg/a2ui"
)

func TestNativeTextFieldAcceptsTyping(t *testing.T) {
	var value string
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(10).Children(func() {
			ui.TextInput(c, &value).Label("Name")
		})
	}, 400, 200)
	tt.SetFocused(true)
	tt.ClickAt(100, 30)
	tt.Type("Ada")
	tt.Frame()
	if value != "Ada" {
		t.Fatalf("value = %q", value)
	}
}

func TestA2UITextFieldAcceptsTyping(t *testing.T) {
	var msg a2ui.Message
	raw := `{"version":"1.0","createSurface":{"surfaceId":"s","components":[
	{"id":"root","component":"Column","children":["f"]},
	{"id":"f","component":"TextField","label":"Name","value":{"@path":"/name"}}
	],"dataModel":{"name":""}}}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	r := New(Options{})
	r.Apply(msg)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(10).Children(func() { r.View(c, "s") })
	}, 500, 300)
	tt.SetFocused(true)
	if rect, ok := tt.Find("Name"); ok {
		t.Logf("label rect = %+v", rect)
		tt.ClickAt(rect.X+40, rect.Y+float32(rect.H)+12)
	} else {
		t.Log("label not found")
		tt.ClickAt(100, 60)
	}
	tt.Type("A")
	tt.Frame()
	tt.Type("d")
	tt.Frame()
	tt.Type("a")
	tt.Frame()
	data, _ := r.Store().DataModel("s")
	if a2ui.ResolveString(map[string]any{"@path": "/name"}, data, nil) != "Ada" {
		t.Fatalf("data = %#v", data)
	}
}

func TestLegacyFlatActionFires(t *testing.T) {
	var msg a2ui.Message
	raw := `{"version":"1.0","createSurface":{"surfaceId":"s","dataModel":{"name":"Ada","email":"a@b.co"},"components":[
	{"id":"root","component":"Column","children":["label","submit"]},
	{"id":"label","component":"Text","text":"说明"},
	{"id":"submit","component":"Button","label":{"literalString":"提交表单"},"action":{"name":"submit_contact","context":[{"key":"name","value":{"path":"/name"}},{"key":"email","value":{"path":"/email"}}]}}
	]}}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	r := New(Options{})
	var got a2ui.Action
	r.opts.OnAction = func(a a2ui.Action) { got = a }
	r.Apply(msg)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(10).Children(func() { r.View(c, "s") })
	}, 400, 300)
	tt.SetFocused(true)
	if err := tt.Click("提交表单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got.Name != "submit_contact" {
		t.Fatalf("action = %#v", got)
	}
	if got.Context["name"] != "Ada" || got.Context["email"] != "a@b.co" {
		t.Fatalf("context = %#v", got.Context)
	}
}

func TestActionWithoutContextUsesDataModel(t *testing.T) {
	var msg a2ui.Message
	raw := `{"version":"1.0","createSurface":{"surfaceId":"s","dataModel":{"name":"张伟","appointment":"2025-06-18T14:30"},"components":[
	{"id":"root","component":"Column","children":["submit"]},
	{"id":"submit","component":"Button","label":{"literalString":"提交预约"},"action":{"event":{"name":"submit_booking"}}}
	]}}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	r := New(Options{})
	var got a2ui.Action
	r.opts.OnAction = func(a a2ui.Action) { got = a }
	r.Apply(msg)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(10).Children(func() { r.View(c, "s") })
	}, 400, 300)
	tt.SetFocused(true)
	if err := tt.Click("提交预约"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got.Context["name"] != "张伟" || got.Context["appointment"] != "2025-06-18T14:30" {
		t.Fatalf("context = %#v", got.Context)
	}
}
