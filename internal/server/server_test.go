package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stevelittlefish/the-source/internal/catalog"
	"github.com/stevelittlefish/the-source/internal/lyrics"
)

// testSongs builds a tiny lyrics database in a temp file for the handler tests.
func testSongs(t *testing.T) *lyrics.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyrics.db")
	w, err := lyrics.NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	songs := []lyrics.Song{
		{ID: 10, Title: "Killa Cam", Artist: "Cam'ron", Tag: "rap", Language: "en", Year: 2004, Views: 173166, Lyrics: "[Chorus]\nKilla Cam, Killa Cam\nKilla Cam, Cam\n\n[Verse 1]\nWith the goons I spar\nStay in tune with ma\n"},
		{ID: 20, Title: "Quiet Song", Artist: "Nobody", Tag: "pop", Language: "en", Year: 1999, Views: 5, Lyrics: "One line only\n"},
		{ID: 30, Title: "Chanson", Artist: "Personne", Tag: "pop", Language: "fr", Year: 2010, Views: 42, Lyrics: "Premiere ligne\nDeuxieme ligne\n"},
	}
	for _, song := range songs {
		if err := w.Add(song); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Finish(); err != nil {
		t.Fatal(err)
	}
	store, err := lyrics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func testServer(t *testing.T) *Server {
	t.Helper()
	c, err := catalog.Load(strings.NewReader("Text#,Type,Issued,Title,Language,Authors,Subjects,LoCC,Bookshelves\n1,Text,2000,A book,en,Writer,Subject,P,Shelf\n2,Text,2000,B book,en; fr,Writer,Subject,P,Shelf\n3,Text,2000,C book,fr,Writer,Subject,P,Shelf\n"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.txt"), []byte("A small book.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(c, testSongs(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestAPI(t *testing.T) {
	s := testServer(t)
	for _, tt := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/api/v1/books", 200, `"total":2`},
		{"/api/v1/books?language=all", 200, `"total":3`},
		{"/api/v1/books?language=fr", 200, `"total":2`},
		{"/api/v1/books?available=true", 200, `"total":1`},
		{"/api/v1/books?available=false", 200, `"total":1`},
		{"/api/v1/books?q=nosuchbook", 200, `"books":[]`},
		{"/api/v1/books?limit=1", 200, `"next_cursor":"1"`},
		{"/api/v1/books?limit=1&cursor=1", 200, `"id":2`},
		{"/api/v1/books?limit=0", 400, "invalid_query"},
		{"/api/v1/books?cursor=-1", 400, "invalid_query"},
		{"/api/v1/books?language=en&language=fr", 400, "invalid_query"},
		{"/api/v1/books?typo=yes", 400, "invalid_query"},
		{"/api/v1/books/999", 404, "book_not_found"},
		{"/api/v1/books/2/text", 404, "text_unavailable"},
		{"/api/v1/books/1", 200, `"available":true`},
		{"/api/v1/nope", 404, "not_found"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.contains) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatal("invalid JSON")
			}
		})
	}
}
func TestTextRangeAndHead(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest("GET", "/api/v1/books/1/text", nil)
	r.Header.Set("Range", "bytes=0-6")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "A small" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatal("reader must be able to embed same-origin text")
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("HEAD", "/api/v1/books/1/text", nil))
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d %q", w.Code, w.Body.String())
	}
}
func TestMethodsAndPages(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/books", nil))
	if w.Code != 405 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/", "/books", "/books/browse", "/books/random", "/books/excerpts", "/books/1", "/read/1", "/lyrics", "/lyrics/browse", "/lyrics/random", "/lyrics/excerpts", "/lyrics/10", "/static/browse.js", "/static/songs.js", "/static/song.js", "/static/stanza.js", "/static/style.css"} {
		w = httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
func TestCorpusSymlinkCannotEscape(t *testing.T) {
	s := testServer(t)
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.root.Symlink(outside, "2.txt"); err != nil {
		t.Fatal(err)
	}
	s.texts[2] = "2.txt"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/2/text", nil))
	if w.Code == 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("escaped corpus: %d %s", w.Code, w.Body.String())
	}
}
