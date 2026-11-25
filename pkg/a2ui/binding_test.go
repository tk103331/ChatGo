package a2ui

import "testing"

func TestResolvePathAndCall(t *testing.T) {
	data := map[string]any{"contact": map[string]any{"email": "a@b.co", "empty": ""}}
	if got := ResolveString(map[string]any{"@path": "/contact/email"}, data, nil); got != "a@b.co" {
		t.Fatalf("path = %q", got)
	}
	funcs := DefaultFunctions()
	if got, _ := Resolve(map[string]any{"@call": "required", "args": map[string]any{"value": map[string]any{"@path": "/contact/email"}}}, data, funcs).(bool); !got {
		t.Fatal("required should pass")
	}
	if got, _ := Resolve(map[string]any{"@call": "required", "args": map[string]any{"value": map[string]any{"@path": "/contact/empty"}}}, data, funcs).(bool); got {
		t.Fatal("required should fail on empty")
	}
}

func TestCheckErrors(t *testing.T) {
	data := map[string]any{"email": "nope"}
	comp := Component{Checks: []Check{{Condition: map[string]any{
		"@call": "email", "args": map[string]any{"value": map[string]any{"@path": "/email"}},
	}, Message: "请输入有效邮箱"}}}
	errs := CheckErrors(comp, data, DefaultFunctions())
	if len(errs) != 1 || errs[0] != "请输入有效邮箱" {
		t.Fatalf("errors = %#v", errs)
	}
}

func TestMessageValidation(t *testing.T) {
	if err := (Message{Version: "1.0"}).Validate(); err == nil {
		t.Fatal("empty message accepted")
	}
	if err := (Message{Version: "1.0", CreateSurface: &CreateSurface{SurfaceID: "s"}, UpdateComponents: &UpdateComponents{SurfaceID: "s"}}).Validate(); err == nil {
		t.Fatal("two payloads accepted")
	}
	if err := (Message{Version: "9.9", CreateSurface: &CreateSurface{SurfaceID: "s"}}).Validate(); err == nil {
		t.Fatal("unknown version accepted")
	}
}

func TestResolveLegacyLiteralsAndPaths(t *testing.T) {
	data := map[string]any{"user": map[string]any{"name": "Ada"}}
	for _, tc := range []struct {
		value any
		want  any
	}{
		{map[string]any{"literalString": "Hi"}, "Hi"},
		{map[string]any{"literalNumber": 4.0}, 4.0},
		{map[string]any{"literalBoolean": true}, true},
		{map[string]any{"literalArray": []any{"a"}}, []any{"a"}},
		{map[string]any{"path": "/user/name"}, "Ada"},
		{map[string]any{"@path": "/user/name"}, "Ada"},
	} {
		if got := Resolve(tc.value, data, nil); !PointerEquals(got, tc.want) {
			t.Fatalf("%#v = %#v, want %#v", tc.value, got, tc.want)
		}
	}
}
