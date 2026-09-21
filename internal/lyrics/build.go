package lyrics

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
)

// schema is the whole shape of the corpus. songs holds the data; songs_fts is
// an external-content FTS5 index that points back at songs (content='songs'),
// so the searchable text is not duplicated. It is populated in one 'rebuild'
// once all rows are in, which is far faster than maintaining it per-insert.
const schema = `
CREATE TABLE songs (
    id       INTEGER PRIMARY KEY,
    title    TEXT NOT NULL,
    artist   TEXT NOT NULL,
    tag      TEXT,
    language TEXT,
    year     INTEGER,
    views    INTEGER NOT NULL DEFAULT 0,
    features TEXT,
    lyrics   TEXT NOT NULL,
    bucket   REAL NOT NULL          -- random value in [0,1) for O(log n) sampling
);
CREATE INDEX songs_language_tag ON songs(language, tag);
-- (bucket, views) lets a views-range random skip non-matching rows in-index,
-- so filtering by popularity stays fast instead of degrading to a scan.
CREATE INDEX songs_bucket_views ON songs(bucket, views);
CREATE INDEX songs_tag_bucket ON songs(tag, bucket);
CREATE INDEX songs_lang_bucket ON songs(language, bucket);
-- (language|tag, views) cover the browse COUNT: counting a views range within
-- one language or tag is answered from the index alone. Without them the count
-- of a filtered, views-bounded browse falls back to a full table scan, which
-- on the full corpus drags every lyrics body off disk (seconds per request).
CREATE INDEX songs_language_views ON songs(language, views);
CREATE INDEX songs_tag_views ON songs(tag, views);
CREATE VIRTUAL TABLE songs_fts USING fts5(
    title, artist, lyrics,
    content='songs', content_rowid='id',
    tokenize='unicode61 remove_diacritics 2'
);
`

// Writer builds a fresh lyrics database. It is not safe for concurrent use;
// preparation is a single-threaded stream from the source CSV.
type Writer struct {
	db   *sql.DB
	tx   *sql.Tx
	stmt *sql.Stmt
	n    int
}

// NewWriter creates path (which must not already exist as a populated corpus),
// installs the schema, and opens a transaction ready for rows. journal_mode and
// synchronous are relaxed because a failed one-off import is simply rerun.
func NewWriter(path string) (*Writer, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(off)&_pragma=synchronous(off)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	w := &Writer{db: db}
	if err := w.begin(); err != nil {
		db.Close()
		return nil, err
	}
	return w, nil
}

func (w *Writer) begin() error {
	tx, err := w.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO songs (id, title, artist, tag, language, year, views, features, lyrics, bucket) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	w.tx, w.stmt = tx, stmt
	return nil
}

// Add inserts one song. Empty optional fields are stored as SQL NULL so that
// DISTINCT and filtering behave. Rows are committed in batches to bound memory.
func (w *Writer) Add(s Song) error {
	_, err := w.stmt.Exec(s.ID, s.Title, s.Artist,
		nullString(s.Tag), nullString(s.Language), nullInt(s.Year),
		s.Views, nullString(EncodeFeatures(s.Features)), s.Lyrics, rand.Float64())
	if err != nil {
		return err
	}
	w.n++
	if w.n%50000 == 0 {
		if err := w.stmt.Close(); err != nil {
			return err
		}
		if err := w.tx.Commit(); err != nil {
			return err
		}
		return w.begin()
	}
	return nil
}

// Count reports how many rows have been added so far.
func (w *Writer) Count() int { return w.n }

// Finish commits the final batch, builds the full-text index, gathers query
// statistics and closes the database.
func (w *Writer) Finish() error {
	if err := w.stmt.Close(); err != nil {
		return err
	}
	if err := w.tx.Commit(); err != nil {
		return err
	}
	if _, err := w.db.Exec("INSERT INTO songs_fts(songs_fts) VALUES('rebuild')"); err != nil {
		return fmt.Errorf("build search index: %w", err)
	}
	if _, err := w.db.Exec("ANALYZE"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	return w.db.Close()
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

// CleanYear keeps only plausible song years and discards obvious junk (the
// corpus contains values like 2). Anything outside the range becomes 0, which
// Add stores as NULL.
func CleanYear(year int) int {
	if year < 1500 || year > 2100 {
		return 0
	}
	return year
}
