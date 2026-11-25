package a2ui

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePointer splits a JSON Pointer into its unescaped tokens. "" and "/"
// name the root and return no tokens.
func ParsePointer(path string) ([]string, error) {
	if path == "" || path == "/" {
		return nil, nil
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("a2ui: json pointer %q does not start with /", path)
	}
	parts := strings.Split(path[1:], "/")
	for i, part := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

// PointerGet reads the value at path from root.
func PointerGet(root any, path string) (any, bool) {
	parts, err := ParsePointer(path)
	if err != nil {
		return nil, false
	}
	cur := root
	for _, part := range parts {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(v) {
				return nil, false
			}
			cur = v[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// PointerSet writes value at path, creating maps for missing segments. A nil
// value deletes a map key, as updateDataModel specifies.
func PointerSet(root any, path string, value any) (any, error) {
	parts, err := ParsePointer(path)
	if err != nil {
		return root, err
	}
	if len(parts) == 0 {
		return value, nil
	}
	if root == nil {
		root = map[string]any{}
	}
	switch root.(type) {
	case map[string]any, []any:
	default:
		return root, fmt.Errorf("a2ui: data model root is not an object or array")
	}
	if err := setAt(root, parts, value); err != nil {
		return root, err
	}
	return root, nil
}

// setAt mutates the container at parts[0].
func setAt(node any, parts []string, value any) error {
	part := parts[0]
	last := len(parts) == 1
	switch v := node.(type) {
	case map[string]any:
		if last {
			if value == nil {
				delete(v, part)
			} else {
				v[part] = value
			}
			return nil
		}
		next, ok := v[part]
		if !ok || next == nil {
			next = map[string]any{}
			v[part] = next
		}
		switch next.(type) {
		case map[string]any, []any:
		default:
			next = map[string]any{}
			v[part] = next
		}
		return setAt(next, parts[1:], value)
	case []any:
		i, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("a2ui: array index %q: %w", part, err)
		}
		if last {
			if i < 0 || i > len(v) {
				return fmt.Errorf("a2ui: array index %d out of range", i)
			}
			if i == len(v) {
				if value == nil {
					return nil
				}
				// Caller holds the outer container; append in place is not
				// possible through any, so arrays must be addressed inside a
				// map. The root array case is handled by PointerSet's caller.
				return fmt.Errorf("a2ui: cannot append to an array through interface value")
			}
			v[i] = value
			return nil
		}
		if i < 0 || i >= len(v) {
			return fmt.Errorf("a2ui: array index %d out of range", i)
		}
		next := v[i]
		switch next.(type) {
		case map[string]any, []any:
		default:
			next = map[string]any{}
			v[i] = next
		}
		return setAt(next, parts[1:], value)
	default:
		return fmt.Errorf("a2ui: cannot descend into %T", node)
	}
}

// PointerEquals compares two JSON values structurally.
func PointerEquals(a, b any) bool {
	return deepEqual(a, b)
}

// deepEqual is a small structural comparison for decoded JSON.
func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bv[k]) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
