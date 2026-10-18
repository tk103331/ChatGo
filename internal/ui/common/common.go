// Package common holds small shared widgets and string helpers used across
// the view packages (agents, skills, mcp, settings) and the main ui package.
package common

import (
	"strings"

	mgo "github.com/egoist/mygo/ui"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
	minput "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/input"
	mtheme "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/theme"
)

// Tag renders a small bordered label chip.
func Tag(c *mgo.Context, label string) {
	k := mcore.Tokens(c)
	u := mcore.Density(c).Unit()
	mgo.Text(c, label).FontSize(mcore.FontSize(c, mtheme.CaptionSize)).TextColor(k.AccentText).
		Padding(0, u*1.5).Radius(mtheme.ControlRadius).Border(mtheme.BorderWidth, k.Border).SingleLine()
}

// SearchField renders a compact search input and returns its element.
func SearchField(c *mgo.Context, query *string, placeholder string) mgo.Element {
	in := minput.SearchInput(c, query, minput.SearchInputOptions{Placeholder: placeholder, Label: placeholder})
	return in.Element.FillWidth()
}

// ContainsStr reports whether s contains v.
func ContainsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// SetTool adds or removes id from s.
func SetTool(s []string, id string, on bool) []string {
	if on {
		if ContainsStr(s, id) {
			return s
		}
		return append(s, id)
	}
	out := s[:0]
	for _, x := range s {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// RemoveStr returns s without v.
func RemoveStr(s []string, v string) []string {
	out := s[:0:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// FilterStrings returns the items containing query, case-insensitively.
func FilterStrings(list []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return list
	}
	var out []string
	for _, s := range list {
		if strings.Contains(strings.ToLower(s), q) {
			out = append(out, s)
		}
	}
	return out
}

// ToggleAll returns every item in want, or none.
func ToggleAll(want []string, all bool) []string {
	if all {
		return append([]string(nil), want...)
	}
	return nil
}

// SplitLines splits s into trimmed non-empty lines.
func SplitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ParseKV parses "key=value" lines into a map.
func ParseKV(s string) map[string]string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}

// JoinKV joins a map as "key=value" lines.
func JoinKV(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	lines := make([]string, 0, len(m))
	for k, v := range m {
		lines = append(lines, k+"="+v)
	}
	return strings.Join(lines, "\n")
}
