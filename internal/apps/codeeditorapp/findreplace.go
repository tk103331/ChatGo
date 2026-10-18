package codeeditorapp

import "strings"

// findNext returns the index of the next occurrence of needle at or after
// from, or -1; ci lowers both sides for case-insensitive matching.
func findNext(content, needle string, from int, ci bool) int {
	if needle == "" || from < 0 || from > len(content) {
		return -1
	}
	hay := content
	if ci {
		hay, needle = strings.ToLower(content), strings.ToLower(needle)
	}
	i := strings.Index(hay[from:], needle)
	if i < 0 {
		return -1
	}
	return from + i
}

// findPrev returns the index of the last occurrence of needle before from.
func findPrev(content, needle string, from int, ci bool) int {
	if needle == "" || from <= 0 {
		return -1
	}
	hay := content
	if ci {
		hay, needle = strings.ToLower(content), strings.ToLower(needle)
	}
	end := from
	if end > len(hay) {
		end = len(hay)
	}
	return strings.LastIndex(hay[:end], needle)
}

// replaceAll replaces every occurrence of find in content, unchanged when
// find is empty; ci makes the matching case-insensitive.
func replaceAll(content, find, repl string, ci bool) string {
	if find == "" {
		return content
	}
	if !ci {
		return strings.ReplaceAll(content, find, repl)
	}
	lower, needle := strings.ToLower(content), strings.ToLower(find)
	var b strings.Builder
	from := 0
	for {
		i := strings.Index(lower[from:], needle)
		if i < 0 {
			b.WriteString(content[from:])
			break
		}
		i += from
		b.WriteString(content[from:i])
		b.WriteString(repl)
		from = i + len(find)
	}
	return b.String()
}
