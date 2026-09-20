package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stevelittlefish/the-source/internal/catalog"
)

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
	s, err := New(c, dir)
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
	for _, path := range []string{"/books", "/books/1", "/read/1", "/static/browse.js", "/static/style.css"} {
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
	s.available[2] = true
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/2/text", nil))
	if w.Code == 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("escaped corpus: %d %s", w.Code, w.Body.String())
	}
}
