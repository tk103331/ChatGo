package a2ui

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Function evaluates one @call in a binding.
type Function func(args map[string]any) (any, error)

// Functions maps registered function names to implementations.
type Functions map[string]Function

// Resolve follows dynamic values: {"@path": "/x"} reads the data model, and
// {"@call": "name", "args": {...}} runs a registered function. Other values
// pass through unchanged.
func Resolve(value any, data any, funcs Functions) any {
	switch v := value.(type) {
	case map[string]any:
		// v0.9 and v1.0 spell bindings @path/@call; v0.8 and some SDKs omit
		// the prefix.
		if path, ok := v["@path"].(string); ok {
			if got, ok := pointerGetRelative(data, path); ok {
				return got
			}
			return nil
		}
		if path, ok := v["path"].(string); ok {
			if got, ok := pointerGetRelative(data, path); ok {
				return got
			}
			return nil
		}
		// v0.8 literal wrappers: {"literalString": "Hi"}, and the other
		// primitive variants.
		for _, key := range []string{"literalString", "literalNumber", "literalBoolean", "literalArray", "literalObject", "literal"} {
			if literal, ok := v[key]; ok {
				return literal
			}
		}
		if call, ok := v["call"]; ok {
			name, _ := call.(string)
			if name == "@index" {
				if m, ok := data.(map[string]any); ok {
					return m["__a2uiIndex"]
				}
				return nil
			}
			if name == "" || funcs == nil {
				return nil
			}
			fn, ok := funcs[name]
			if !ok {
				return nil
			}
			args, _ := v["args"].(map[string]any)
			resolved := make(map[string]any, len(args))
			for k, arg := range args {
				resolved[k] = Resolve(arg, data, funcs)
			}
			out, err := fn(resolved)
			if err != nil {
				return nil
			}
			return out
		}
		if call, ok := v["@call"]; ok {
			name := ""
			switch c := call.(type) {
			case string:
				name = c
			case map[string]any:
				name, _ = c["name"].(string)
			}
			if name == "@index" {
				if m, ok := data.(map[string]any); ok {
					return m["__a2uiIndex"]
				}
				return nil
			}
			if name == "" || funcs == nil {
				return nil
			}
			fn, ok := funcs[name]
			if !ok {
				return nil
			}
			args, _ := v["args"].(map[string]any)
			resolved := make(map[string]any, len(args))
			for k, arg := range args {
				resolved[k] = Resolve(arg, data, funcs)
			}
			out, err := fn(resolved)
			if err != nil {
				return nil
			}
			return out
		}
		// Resolve nested dynamic values in ordinary objects, such as action
		// context.
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = Resolve(item, data, funcs)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = Resolve(item, data, funcs)
		}
		return out
	default:
		return value
	}
}

// ResolveString resolves value and renders the result as text.
func ResolveString(value any, data any, funcs Functions) string {
	return toText(Resolve(value, data, funcs))
}

// ResolveBool resolves value as a boolean.
func ResolveBool(value any, data any, funcs Functions) bool {
	switch v := Resolve(value, data, funcs).(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	case float64:
		return v != 0
	}
	return false
}

// ResolveFloat resolves value as a number.
func ResolveFloat(value any, data any, funcs Functions) float64 {
	switch v := Resolve(value, data, funcs).(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case jsonNumber:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return 0
}

// jsonNumber matches encoding/json's Number without importing it here for one
// type assertion.
type jsonNumber interface{ Float64() (float64, error) }

// toText renders a resolved JSON value for display.
func toText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		if data, err := json.Marshal(x); err == nil {
			return string(data)
		}
		return ""
	}
}

// pointerGetRelative reads a path, treating a path without a leading slash
// as relative to the current data model (a template item).
func pointerGetRelative(data any, path string) (any, bool) {
	if path != "" && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return PointerGet(data, path)
}

// DefaultFunctions registers the validation and formatting functions the
// basic catalog defines. Hosts can add their own under other names.
func DefaultFunctions() Functions {
	one := func(v any) string { return toText(v) }
	return Functions{
		"required": func(args map[string]any) (any, error) {
			return one(args["value"]) != "", nil
		},
		"length": func(args map[string]any) (any, error) {
			return float64(len([]rune(one(args["value"])))), nil
		},
		"numeric": func(args map[string]any) (any, error) {
			_, err := strconv.ParseFloat(one(args["value"]), 64)
			return err == nil, nil
		},
		"email": func(args map[string]any) (any, error) {
			return strings.Contains(one(args["value"]), "@") && strings.Contains(one(args["value"]), "."), nil
		},
		"regex": func(args map[string]any) (any, error) {
			pattern := one(args["pattern"])
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false, err
			}
			return re.MatchString(one(args["value"])), nil
		},
		"and": func(args map[string]any) (any, error) {
			for _, v := range args {
				if b, ok := v.(bool); ok && !b {
					return false, nil
				}
			}
			return true, nil
		},
		"or": func(args map[string]any) (any, error) {
			for _, v := range args {
				if b, ok := v.(bool); ok && b {
					return true, nil
				}
			}
			return false, nil
		},
		"not": func(args map[string]any) (any, error) {
			b, _ := args["value"].(bool)
			return !b, nil
		},
		"formatString": func(args map[string]any) (any, error) {
			return one(args["value"]), nil
		},
	}
}

// CheckErrors evaluates a component's @checks against the data model and
// returns the messages of the failing ones.
func CheckErrors(c Component, data any, funcs Functions) []string {
	var out []string
	for _, check := range c.Checks {
		ok, _ := Resolve(check.Condition, data, funcs).(bool)
		if ok {
			continue
		}
		msg := check.Message
		if msg == "" {
			msg = "校验未通过"
		}
		out = append(out, msg)
	}
	return out
}
