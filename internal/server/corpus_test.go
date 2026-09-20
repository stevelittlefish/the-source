package server

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirrorLayout(t *testing.T) {
	original := testServer(t)
	dir := t.TempDir()
	for path, body := range map[string]string{
		"1/pg1.txt":   "Mirror text",
		"1.txt":       "Flat fallback",
		"2/other.txt": "Not the expected book",
		"03/pg03.txt": "Not a canonical ID",
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(original.catalog, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.texts[1] != "1/pg1.txt" || len(s.texts) != 1 {
		t.Fatalf("index: %#v", s.texts)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/1/text", nil))
	if w.Code != 200 || w.Body.String() != "Mirror text" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books?available=true", nil))
	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatal(w.Body.String())
	}
	if err := os.Remove(filepath.Join(dir, "1/pg1.txt")); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/1/text", nil))
	if w.Code != 404 {
		t.Fatalf("removed text: %d", w.Code)
	}
}
