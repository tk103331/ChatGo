package kb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS kb (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS doc (
	id         TEXT PRIMARY KEY,
	kb_id      TEXT NOT NULL REFERENCES kb(id) ON DELETE CASCADE,
	title      TEXT NOT NULL,
	source     TEXT NOT NULL,
	path       TEXT NOT NULL DEFAULT '',
	hash       TEXT NOT NULL DEFAULT '',
	size       INTEGER NOT NULL DEFAULT 0,
	mtime      INTEGER NOT NULL DEFAULT 0,
	chunks     INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS doc_kb ON doc(kb_id);
CREATE UNIQUE INDEX IF NOT EXISTS doc_path ON doc(kb_id, path) WHERE path <> '';

CREATE VIRTUAL TABLE IF NOT EXISTS chunk_fts USING fts5(
	chunk_id UNINDEXED,
	kb_id    UNINDEXED,
	heading,
	body,
	tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TABLE IF NOT EXISTS chunk (
	id      TEXT PRIMARY KEY,
	kb_id   TEXT NOT NULL,
	doc_id  TEXT NOT NULL REFERENCES doc(id) ON DELETE CASCADE,
	seq     INTEGER NOT NULL,
	heading TEXT NOT NULL DEFAULT '',
	text    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS chunk_kb ON chunk(kb_id);
CREATE INDEX IF NOT EXISTS chunk_doc ON chunk(doc_id, seq);
`

// DB is the knowledge base store, backed by a pure-Go SQLite file.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens (and migrates) the store at path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("初始化知识库表结构失败: %w", err)
	}
	return &DB{conn: conn, path: path}, nil
}

// OpenDefault opens ~/.chatgo/knowledge/kb.db.
func OpenDefault() (*DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return Open(filepath.Join(home, ".chatgo", "knowledge", "kb.db"))
}

// Close closes the underlying connection.
func (d *DB) Close() error { return d.conn.Close() }

// Path returns the database file path.
func (d *DB) Path() string { return d.path }

// KnowledgeBase is one named collection of documents.
type KnowledgeBase struct {
	ID          string
	Name        string
	Description string
	Docs        int
	Chunks      int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Document is one indexed text file or pasted text.
type Document struct {
	ID        string
	KBID      string
	Title     string
	Source    string
	Path      string
	Hash      string
	Size      int64
	Chunks    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

const maxTime = 1 << 62

func toUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func fromUnix(v int64) time.Time {
	if v <= 0 || v >= maxTime {
		return time.Time{}
	}
	return time.Unix(v, 0)
}
