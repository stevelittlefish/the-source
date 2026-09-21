// Package lyrics serves songs from a prepared, read-only SQLite database.
//
// Unlike the book catalogue, which fits comfortably in memory, the lyrics
// corpus runs to millions of rows. Rather than reinvent an index, a search
// engine and a pager by hand, we lean on SQLite: it answers filtered and
// full-text queries without loading the corpus into RAM, and one song is one
// SELECT. The database is built offline by cmd/lyricsprep and opened read-only.
package lyrics

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Song is one row of the corpus. Lyrics is populated only by Text and Excerpt;
// list and search leave it empty so a page of results stays light.
type Song struct {
	ID       int      `json:"id"`
	Title    string   `json:"title"`
	Artist   string   `json:"artist"`
	Tag      string   `json:"tag,omitempty"`
	Language string   `json:"language,omitempty"`
	Year     int      `json:"year,omitempty"`
	Views    int      `json:"views"`
	Features []string `json:"features,omitempty"`
	Lyrics   string   `json:"lyrics,omitempty"`
}

// Store is a handle onto the read-only lyrics database.
type Store struct{ db *sql.DB }

// Filter narrows a listing. An empty field means "do not filter on this".
type Filter struct {
	Language string
	Tag      string
	Query    string
	Limit    int
	Offset   int
}

// Open opens the prepared database read-only. immutable promises SQLite the
// file will not change under it, which suits a corpus built once and served.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_pragma=query_only(true)")
	if err != nil {
		return nil, fmt.Errorf("open lyrics db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open lyrics db: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const columns = "id, title, artist, tag, language, year, views, features"

func scanSong(row interface{ Scan(...any) error }) (Song, error) {
	var song Song
	var tag, language, features sql.NullString
	var year, views sql.NullInt64
	if err := row.Scan(&song.ID, &song.Title, &song.Artist, &tag, &language, &year, &views, &features); err != nil {
		return song, err
	}
	song.Tag, song.Language = tag.String, language.String
	song.Year, song.Views = int(year.Int64), int(views.Int64)
	song.Features = splitFeatures(features.String)
	return song, nil
}

// Get returns one song's metadata (without its lyrics).
func (s *Store) Get(id int) (Song, bool, error) {
	row := s.db.QueryRow("SELECT "+columns+" FROM songs WHERE id = ?", id)
	song, err := scanSong(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Song{}, false, nil
	}
	if err != nil {
		return Song{}, false, err
	}
	return song, true, nil
}

// Text returns just a song's lyrics body.
func (s *Store) Text(id int) (string, bool, error) {
	var body string
	err := s.db.QueryRow("SELECT lyrics FROM songs WHERE id = ?", id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return body, true, nil
}

// Search returns a page of songs and the total number that match. An empty
// query browses; a non-empty query runs full-text search over title, artist
// and lyrics. Results are ordered by id so paging with Offset is stable.
func (s *Store) Search(f Filter) ([]Song, int, error) {
	match := ftsQuery(f.Query)
	var where []string
	var args []any
	if match != "" {
		where = append(where, "songs_fts MATCH ?")
		args = append(args, match)
	}
	if f.Language != "" {
		where = append(where, "s.language = ?")
		args = append(args, f.Language)
	}
	if f.Tag != "" {
		where = append(where, "s.tag = ?")
		args = append(args, f.Tag)
	}
	from := "songs s"
	if match != "" {
		from = "songs_fts JOIN songs s ON s.id = songs_fts.rowid"
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM "+from+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query("SELECT "+prefixed(columns)+" FROM "+from+clause+" ORDER BY s.id LIMIT ? OFFSET ?",
		append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	songs := make([]Song, 0, f.Limit)
	for rows.Next() {
		song, err := scanSong(rows)
		if err != nil {
			return nil, 0, err
		}
		songs = append(songs, song)
	}
	return songs, total, rows.Err()
}

// Random returns a uniformly random song matching the language and tag
// filters, ignoring row order. This matters: the corpus is stored roughly in
// id order and genre correlates with id, so anything leaning on ordering would
// skew heavily toward the rap-heavy front of the file.
func (s *Store) Random(language, tag string) (Song, bool, error) {
	var where []string
	var args []any
	if language != "" {
		where = append(where, "language = ?")
		args = append(args, language)
	}
	if tag != "" {
		where = append(where, "tag = ?")
		args = append(args, tag)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	row := s.db.QueryRow("SELECT "+columns+" FROM songs"+clause+" ORDER BY RANDOM() LIMIT 1", args...)
	song, err := scanSong(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Song{}, false, nil
	}
	if err != nil {
		return Song{}, false, err
	}
	return song, true, nil
}

// Tags returns the distinct genre tags present, in lexical order.
func (s *Store) Tags() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT tag FROM songs WHERE tag IS NOT NULL AND tag != '' ORDER BY tag")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// Languages returns the distinct language codes present, in lexical order.
func (s *Store) Languages() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT language FROM songs WHERE language IS NOT NULL AND language != '' ORDER BY language")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var languages []string
	for rows.Next() {
		var language string
		if err := rows.Scan(&language); err != nil {
			return nil, err
		}
		languages = append(languages, language)
	}
	return languages, rows.Err()
}

// prefixed rewrites the bare column list to address the songs table as s.
func prefixed(cols string) string {
	parts := strings.Split(cols, ", ")
	for i := range parts {
		parts[i] = "s." + parts[i]
	}
	return strings.Join(parts, ", ")
}

// ftsQuery turns free user text into a safe FTS5 MATCH expression: each word
// becomes a quoted term (implicitly AND-ed), so punctuation cannot smuggle in
// FTS operators or a syntax error. Empty input means "no full-text filter".
func ftsQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(fields))
	for _, field := range fields {
		quoted = append(quoted, `"`+strings.ReplaceAll(field, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " ")
}
