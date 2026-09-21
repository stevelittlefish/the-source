package server

import (
	"encoding/json"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentYearIndex(t *testing.T) {
	original := testServer(t)
	dir := t.TempDir()
	bookPath := filepath.Join(dir, "1.txt")
	cache := filepath.Join(t.TempDir(), "state", "years.json")
	if err := os.WriteFile(bookPath, []byte("Original publication: Publisher, 1927\n"), 0600); err != nil {
		t.Fatal(err)
	}
	open := func() *Server {
		t.Helper()
		s, err := New(original.catalog, nil, dir, cache)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	if s.years[1] != 1927 {
		t.Fatalf("cold index: %v", s.years)
	}
	s.Close()
	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	var saved yearIndex
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	// Mark the cache so a restart proves it reuses metadata, not just that
	// parsing the same header happens to produce the same answer.
	entry := saved.Entries[1]
	entry.Year = 1901
	saved.Entries[1] = entry
	if err := saveYearIndex(cache, saved); err != nil {
		t.Fatal(err)
	}
	s = open()
	if s.years[1] != 1901 {
		t.Fatal("restart reread an unchanged header")
	}
	s.Close()
	if err := os.WriteFile(bookPath, []byte("Original publication: Different publisher, 1950\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s = open()
	if s.years[1] != 1950 {
		t.Fatal("changed book did not refresh")
	}
	// Closing the corpus proves even a filtered random request needs no disk.
	s.Close()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/random?year_from=1950", nil))
	if w.Code != 200 {
		t.Fatalf("request used disk: %d %s", w.Code, w.Body.String())
	}
	if err := os.WriteFile(cache, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	s = open()
	s.Close()
	if s.years[1] != 1950 {
		t.Fatal("corrupt cache was not rebuilt")
	}
	if err := os.Remove(bookPath); err != nil {
		t.Fatal(err)
	}
	s = open()
	defer s.Close()
	if len(s.years) != 0 {
		t.Fatal("deleted book retained")
	}
}

func TestSelectionBounds(t *testing.T) {
	s := testServer(t)
	s.texts = map[int]string{1: "1.txt", 2: "2.txt", 3: "3.txt"}
	s.years = map[int]int{1: 1900, 2: 1950, 3: 0}
	s.buildPools()
	for _, tc := range []struct {
		language string
		bounds   yearRange
		count    int
	}{
		{"", yearRange{}, 3}, {"", yearRange{1, 9999}, 2}, {"en", yearRange{1900, 1900}, 1},
		{"fr", yearRange{1901, 0}, 1}, {"en", yearRange{1951, 0}, 0}, {"", yearRange{0, 1899}, 0},
	} {
		if got := len(s.selection(tc.language, tc.bounds)); got != tc.count {
			t.Fatalf("%+v: %d", tc, got)
		}
	}
}

func benchmarkCatalog(b *testing.B) *Server {
	b.Helper()
	c, err := catalog.LoadFile("../../pg_catalog.csv")
	if err != nil {
		b.Fatal(err)
	}
	s := &Server{catalog: c, texts: make(map[int]string), years: make(map[int]int)}
	for _, book := range c.Search("", "") {
		s.texts[book.ID] = "fixture"
		s.years[book.ID] = 1800 + book.ID%201
	}
	s.buildPools()
	return s
}
func BenchmarkYearSelectionFullCatalog(b *testing.B) {
	s := benchmarkCatalog(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(s.selection("en", yearRange{1901, 1950})) == 0 {
			b.Fatal("empty selection")
		}
	}
}
func BenchmarkRandomYearAPIFullCatalog(b *testing.B) {
	s := benchmarkCatalog(b)
	r := httptest.NewRequest("GET", "/api/v1/books/random?year_from=1901&year_to=1950", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		s.random(w, r)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}
