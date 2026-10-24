package kb

import (
	"database/sql"
	"fmt"
	"strings"
)

// Result is one retrieved passage.
type Result struct {
	ChunkID string
	DocID   string
	Doc     string
	Source  string
	Path    string
	Heading string
	Text    string
	Score   float64
}

// Search runs a full-text query over one knowledge base, or over every
// knowledge base when kbID is empty.
func (d *DB) Search(kbID, query string, limit int) ([]Result, error) {
	return d.search(kbID, "", query, limit)
}

// SearchDocument runs a full-text query restricted to one document.
func (d *DB) SearchDocument(docID, query string, limit int) ([]Result, error) {
	if strings.TrimSpace(docID) == "" {
		return nil, nil
	}
	return d.search("", docID, query, limit)
}

func (d *DB) search(kbID, docID, query string, limit int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	match := matchQuery(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 8
	}
	args := []any{match}
	where := `chunk_fts MATCH ?`
	if kbID != "" {
		where += ` AND chunk_fts.kb_id = ?`
		args = append(args, kbID)
	}
	if docID != "" {
		// chunk ids are "<doc id>#<seq>", so a prefix match scopes the query
		// to one document without joining the chunk table inside FTS5.
		where += ` AND chunk_fts.chunk_id GLOB ?`
		args = append(args, docID+"#*")
	}
	args = append(args, limit)
	rows, err := d.conn.Query(`
		SELECT chunk_fts.chunk_id, bm25(chunk_fts) AS score
		FROM chunk_fts
		WHERE `+where+`
		ORDER BY score
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []hitScore
	for rows.Next() {
		var h hitScore
		if err := rows.Scan(&h.chunkID, &h.score); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return d.loadResults(hits)
}

// loadResults joins the matching chunks with their documents.
func (d *DB) loadResults(hits []hitScore) ([]Result, error) {
	out := make([]Result, 0, len(hits))
	for _, h := range hits {
		var r Result
		var heading string
		err := d.conn.QueryRow(`
			SELECT c.id, c.doc_id, d.title, d.source, d.path, c.heading, c.text
			FROM chunk c JOIN doc d ON d.id = c.doc_id
			WHERE c.id = ?`, h.chunkID).
			Scan(&r.ChunkID, &r.DocID, &r.Doc, &r.Source, &r.Path, &heading, &r.Text)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		r.Heading = heading
		// bm25() returns lower values for better matches; negate it so callers
		// can sort descending like every other score.
		r.Score = -h.score
		out = append(out, r)
	}
	return out, nil
}

// Format renders results as the text a tool returns to the model.
func Format(results []Result, maxChars int) string {
	if len(results) == 0 {
		return "(没有找到相关内容)"
	}
	var b strings.Builder
	for i, r := range results {
		loc := r.Doc
		if r.Heading != "" {
			loc += " › " + r.Heading
		}
		if r.Path != "" {
			loc += " (" + r.Path + ")"
		}
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, loc))
		b.WriteString(strings.TrimSpace(r.Text))
		b.WriteString("\n\n")
		if maxChars > 0 && b.Len() >= maxChars {
			break
		}
	}
	return strings.TrimSpace(b.String())
}
