package kb

import (
	"strings"
	"unicode"
)

// stopwords are dropped from the search column: they add noise without meaning.
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "in": true,
	"and": true, "or": true, "is": true, "are": true, "for": true, "on": true,
	"with": true, "as": true, "at": true, "by": true, "it": true, "this": true,
	"的": true, "了": true, "和": true, "是": true, "在": true, "有": true,
	"我": true, "你": true, "他": true, "它": true, "就": true, "都": true,
	"也": true, "而": true, "与": true, "及": true, "或": true,
}

// Segment turns text into the space-separated search terms stored in FTS5.
// FTS5's unicode61 tokenizer treats a run of Han characters as one token, so
// each run is split into overlapping bigrams; latin words are lowercased and
// kept whole. Callers must use this for both indexing and queries.
func Segment(text string) string {
	var terms []string
	var han []rune
	var latin []rune
	flushHan := func() {
		if len(han) == 1 {
			terms = appendTerm(terms, string(han))
		}
		for i := 0; i+1 < len(han); i++ {
			terms = appendTerm(terms, string(han[i:i+2]))
		}
		han = han[:0]
	}
	flushLatin := func() {
		if len(latin) > 0 {
			terms = appendTerm(terms, strings.ToLower(string(latin)))
			latin = latin[:0]
		}
	}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushLatin()
			han = append(han, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushHan()
			latin = append(latin, r)
		default:
			flushHan()
			flushLatin()
		}
	}
	flushHan()
	flushLatin()
	return strings.Join(terms, " ")
}

func appendTerm(terms []string, term string) []string {
	if term == "" || stopwords[term] {
		return terms
	}
	return append(terms, term)
}

// matchQuery builds an FTS5 MATCH expression from a natural-language query.
// Terms are OR-ed so partial matches still rank, and each term is quoted so
// punctuation cannot break the expression.
func matchQuery(query string) string {
	fields := strings.Fields(Segment(query))
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, `"`+strings.ReplaceAll(f, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " OR ")
}
