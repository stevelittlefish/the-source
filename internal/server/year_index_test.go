package server

import (
	"encoding/json"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBookIndexIsBuiltOnceThenTrusted(t *testing.T) {
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
	read := func() yearIndex {
		t.Helper()
		data, err := os.ReadFile(cache)
		if err != nil {
			t.Fatal(err)
		}
		var saved yearIndex
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}
	s := open()
	if s.years[1] != 1927 || s.texts[1] != "1.txt" {
		t.Fatalf("first build: %v %v", s.years, s.texts)
	}
	s.Close()
	if saved := read(); saved.Version != bookIndexVersion || !saved.Complete || len(saved.Entries) != 1 {
		t.Fatalf("saved index: %+v", saved)
	}
	// Mark the cache so a restart proves it loads the index rather than
	// happening to reread the same header.
	saved := read()
	entry := saved.Entries[1]
	entry.Year = 1901
	saved.Entries[1] = entry
	if err := saveYearIndex(cache, saved); err != nil {
		t.Fatal(err)
	}
	// The data never changes, so a finished index is trusted even when the
	// disk disagrees: an edited book keeps its indexed year, and a deleted one
	// stays listed. Deleting the index is how you ask for a rebuild.
	if err := os.WriteFile(bookPath, []byte("Original publication: Different publisher, 1950\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s = open()
	if s.years[1] != 1901 {
		t.Fatal("a finished index was not trusted")
	}
	// Closing the corpus proves even a filtered random request needs no disk.
	s.Close()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/random?year_from=1901", nil))
	if w.Code != 200 {
		t.Fatalf("request used disk: %d %s", w.Code, w.Body.String())
	}
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	s = open()
	s.Close()
	if s.years[1] != 1950 {
		t.Fatal("deleting the index did not rebuild it")
	}
	if err := os.WriteFile(cache, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	s = open()
	s.Close()
	if s.years[1] != 1950 || !read().Complete {
		t.Fatal("corrupt index was not rebuilt")
	}
}

func TestUnfinishedOrOldIndexResumesWithoutRereadingHeaders(t *testing.T) {
	original := testServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.txt"), []byte("Original publication: Publisher, 1927\n"), 0600); err != nil {
		t.Fatal(err)
	}
	corpus, _ := filepath.Abs(dir)
	cache := filepath.Join(t.TempDir(), "years.json")
	for _, stale := range []yearIndex{
		{Version: bookIndexVersion, Corpus: corpus, Complete: false, Entries: map[int]yearEntry{1: {Path: "1.txt", Year: 1888}}},
		{Version: 1, Corpus: corpus, Entries: map[int]yearEntry{1: {Path: "1.txt", Size: 5, Modified: 5, Year: 1888}}},
	} {
		if err := saveYearIndex(cache, stale); err != nil {
			t.Fatal(err)
		}
		s, err := New(original.catalog, nil, dir, cache)
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
		// 1888 is not in the header: seeing it means the year was carried over.
		if s.years[1] != 1888 {
			t.Fatalf("version %d, complete %v: year %d, want the carried-over 1888", stale.Version, stale.Complete, s.years[1])
		}
		data, _ := os.ReadFile(cache)
		var saved yearIndex
		if json.Unmarshal(data, &saved) != nil || !saved.Complete || saved.Version != bookIndexVersion {
			t.Fatalf("resumed index not finished: %s", data)
		}
	}
	// An index for another directory is not trusted and not reused.
	if err := saveYearIndex(cache, yearIndex{Version: bookIndexVersion, Corpus: "/elsewhere", Complete: true, Entries: map[int]yearEntry{1: {Path: "1.txt", Year: 1888}}}); err != nil {
		t.Fatal(err)
	}
	s, err := New(original.catalog, nil, dir, cache)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if s.years[1] != 1927 {
		t.Fatalf("foreign index reused: %d", s.years[1])
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
