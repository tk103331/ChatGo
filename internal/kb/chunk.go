package kb

import (
	"strings"
)

const (
	maxChunkRunes = 500
	chunkOverlap  = 60
)

// Chunk is one indexed passage of a document.
type Chunk struct {
	Seq     int
	Heading string
	Text    string
}

// SplitDocument breaks a text document into overlapping chunks. A Markdown
// heading starts a new chunk and becomes its title, so a section is retrieved
// whole with the heading that explains it.
func SplitDocument(text string) []Chunk {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []Chunk
	var buf []string
	var heading string
	seq := 0
	flush := func() {
		body := strings.TrimSpace(strings.Join(buf, "\n\n"))
		buf = nil
		if body == "" {
			return
		}
		out = append(out, Chunk{Seq: seq, Heading: heading, Text: body})
		seq++
	}
	for _, p := range splitParagraphs(text) {
		if h, rest, ok := markdownHeading(p); ok {
			flush()
			heading = h
			p = rest
			if p == "" {
				continue
			}
		}
		if runeLen(p) > maxChunkRunes {
			flush()
			for _, piece := range splitLong(p) {
				buf = []string{piece}
				flush()
			}
			continue
		}
		if runeLen(strings.Join(buf, "\n\n"))+runeLen(p)+2 > maxChunkRunes && len(buf) > 0 {
			flush()
		}
		buf = append(buf, p)
	}
	flush()
	return out
}

// splitParagraphs cuts text on blank lines.
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// markdownHeading returns the title of a leading ATX Markdown heading and the
// paragraph's remaining text.
func markdownHeading(p string) (title, rest string, ok bool) {
	line, rest, hasNewline := strings.Cut(p, "\n")
	line = strings.TrimSpace(line)
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n == 0 || n > 6 {
		return "", "", false
	}
	title = strings.TrimSpace(strings.TrimLeft(line[n:], "#"))
	if title == "" {
		return "", "", false
	}
	rest = strings.TrimSpace(rest)
	if !hasNewline {
		rest = ""
	}
	return title, rest, true
}

// splitLong cuts an over-long paragraph into overlapping windows on rune
// boundaries so no chunk exceeds maxChunkRunes.
func splitLong(p string) []string {
	runes := []rune(p)
	var out []string
	step := maxChunkRunes - chunkOverlap
	for start := 0; start < len(runes); start += step {
		end := start + maxChunkRunes
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return out
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
