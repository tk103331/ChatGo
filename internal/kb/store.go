package kb

import (
	"database/sql"
	"fmt"
	"time"
)

// CreateKnowledgeBase creates a knowledge base and returns its id.
func (d *DB) CreateKnowledgeBase(name, description string) (string, error) {
	id := newID()
	now := time.Now().Unix()
	_, err := d.conn.Exec(
		`INSERT INTO kb(id, name, description, created_at, updated_at) VALUES(?,?,?,?,?)`,
		id, name, description, now, now,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// UpdateKnowledgeBase changes a knowledge base's name and description.
func (d *DB) UpdateKnowledgeBase(id, name, description string) error {
	_, err := d.conn.Exec(
		`UPDATE kb SET name=?, description=?, updated_at=? WHERE id=?`,
		name, description, time.Now().Unix(), id,
	)
	return err
}

// DeleteKnowledgeBase removes a knowledge base with its documents and chunks.
func (d *DB) DeleteKnowledgeBase(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM chunk_fts WHERE kb_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM chunk WHERE kb_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM doc WHERE kb_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM kb WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListKnowledgeBases returns every knowledge base with its document and chunk
// counts.
func (d *DB) ListKnowledgeBases() ([]KnowledgeBase, error) {
	rows, err := d.conn.Query(`
		SELECT kb.id, kb.name, kb.description, kb.created_at, kb.updated_at,
		       (SELECT COUNT(*) FROM doc WHERE doc.kb_id = kb.id),
		       (SELECT COUNT(*) FROM chunk WHERE chunk.kb_id = kb.id)
		FROM kb ORDER BY kb.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeBase
	for rows.Next() {
		var kb KnowledgeBase
		var created, updated int64
		if err := rows.Scan(&kb.ID, &kb.Name, &kb.Description, &created, &updated, &kb.Docs, &kb.Chunks); err != nil {
			return nil, err
		}
		kb.CreatedAt = fromUnix(created)
		kb.UpdatedAt = fromUnix(updated)
		out = append(out, kb)
	}
	return out, rows.Err()
}

// GetKnowledgeBase loads one knowledge base.
func (d *DB) GetKnowledgeBase(id string) (KnowledgeBase, bool, error) {
	var kb KnowledgeBase
	var created, updated int64
	err := d.conn.QueryRow(`
		SELECT kb.id, kb.name, kb.description, kb.created_at, kb.updated_at,
		       (SELECT COUNT(*) FROM doc WHERE doc.kb_id = kb.id),
		       (SELECT COUNT(*) FROM chunk WHERE chunk.kb_id = kb.id)
		FROM kb WHERE kb.id = ?`, id).
		Scan(&kb.ID, &kb.Name, &kb.Description, &created, &updated, &kb.Docs, &kb.Chunks)
	if err == sql.ErrNoRows {
		return KnowledgeBase{}, false, nil
	}
	if err != nil {
		return KnowledgeBase{}, false, err
	}
	kb.CreatedAt = fromUnix(created)
	kb.UpdatedAt = fromUnix(updated)
	return kb, true, nil
}

// ListDocuments returns the documents of a knowledge base.
func (d *DB) ListDocuments(kbID string) ([]Document, error) {
	rows, err := d.conn.Query(`
		SELECT id, kb_id, title, source, path, hash, size, chunks, created_at, updated_at
		FROM doc WHERE kb_id = ? ORDER BY title, path`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Document
	for rows.Next() {
		var doc Document
		var created, updated int64
		if err := rows.Scan(&doc.ID, &doc.KBID, &doc.Title, &doc.Source, &doc.Path, &doc.Hash,
			&doc.Size, &doc.Chunks, &created, &updated); err != nil {
			return nil, err
		}
		doc.CreatedAt = fromUnix(created)
		doc.UpdatedAt = fromUnix(updated)
		out = append(out, doc)
	}
	return out, rows.Err()
}

// DeleteDocument removes one document and its chunks.
func (d *DB) DeleteDocument(id string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM chunk_fts WHERE chunk_id IN (SELECT id FROM chunk WHERE doc_id=?)`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM chunk WHERE doc_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM doc WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// docByPath returns the document stored for a path inside a knowledge base.
func (d *DB) docByPath(kbID, path string) (Document, bool, error) {
	var doc Document
	var created, updated int64
	err := d.conn.QueryRow(`
		SELECT id, kb_id, title, source, path, hash, size, chunks, created_at, updated_at
		FROM doc WHERE kb_id = ? AND path = ?`, kbID, path).
		Scan(&doc.ID, &doc.KBID, &doc.Title, &doc.Source, &doc.Path, &doc.Hash, &doc.Size,
			&doc.Chunks, &created, &updated)
	if err == sql.ErrNoRows {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, err
	}
	doc.CreatedAt = fromUnix(created)
	doc.UpdatedAt = fromUnix(updated)
	return doc, true, nil
}

// saveDocument writes a document with its chunks, replacing any previous
// version of the same path.
func (d *DB) saveDocument(doc Document, chunks []Chunk) error {
	existing, hasExisting, err := d.docByPath(doc.KBID, doc.Path)
	if err != nil {
		return err
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if hasExisting && doc.Path != "" {
		doc.ID = existing.ID
		doc.CreatedAt = existing.CreatedAt
		if _, err := tx.Exec(`DELETE FROM chunk_fts WHERE chunk_id IN (SELECT id FROM chunk WHERE doc_id=?)`, doc.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM chunk WHERE doc_id=?`, doc.ID); err != nil {
			return err
		}
	}
	now := time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	doc.UpdatedAt = now
	doc.Chunks = len(chunks)
	if _, err := tx.Exec(`
		INSERT INTO doc(id, kb_id, title, source, path, hash, size, chunks, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, source=excluded.source, path=excluded.path,
			hash=excluded.hash, size=excluded.size, chunks=excluded.chunks,
			updated_at=excluded.updated_at`,
		doc.ID, doc.KBID, doc.Title, doc.Source, doc.Path, doc.Hash, doc.Size, doc.Chunks,
		toUnix(doc.CreatedAt), toUnix(doc.UpdatedAt)); err != nil {
		return err
	}
	for _, c := range chunks {
		chunkID := fmt.Sprintf("%s#%d", doc.ID, c.Seq)
		if _, err := tx.Exec(`INSERT INTO chunk(id, kb_id, doc_id, seq, heading, text) VALUES(?,?,?,?,?,?)`,
			chunkID, doc.KBID, doc.ID, c.Seq, c.Heading, c.Text); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO chunk_fts(chunk_id, kb_id, heading, body) VALUES(?,?,?,?)`,
			chunkID, doc.KBID, Segment(c.Heading), Segment(c.Text)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE kb SET updated_at=? WHERE id=?`, toUnix(doc.UpdatedAt), doc.KBID); err != nil {
		return err
	}
	return tx.Commit()
}

// hitScore is one FTS5 match with its raw bm25() score.
type hitScore struct {
	chunkID string
	score   float64
}

// ListChunks returns the chunks of one document in reading order.
func (d *DB) ListChunks(docID string) ([]Chunk, error) {
	rows, err := d.conn.Query(
		`SELECT seq, heading, text FROM chunk WHERE doc_id = ? ORDER BY seq`, docID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.Seq, &c.Heading, &c.Text); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
