package server

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRandomBook(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct {
		query  string
		status int
		id     int
	}{
		{"", 200, 1}, {"?language=en", 200, 1}, {"?language=all", 200, 1},
		{"?language=fr", 404, 0}, {"?language=", 400, 0},
		{"?language=en&language=fr", 400, 0}, {"?limit=2", 400, 0},
	} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/random"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("random responses must not be cached")
		}
		if tc.status == 200 {
			var b bookResponse
			if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
			if b.ID != tc.id || !b.Available {
				t.Fatalf("unexpected book: %+v", b)
			}
		}
	}
	// Model the startup index with only a multilingual work installed.
	s.texts = map[int]string{2: "2.txt"}
	for _, query := range []string{"", "?language=fr"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/random"+query, nil))
		var b bookResponse
		json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 200 || b.ID != 2 {
			t.Fatalf("multilingual: %d %s", w.Code, w.Body.String())
		}
	}
	s.texts = map[int]string{3: "3.txt"}
	for _, tc := range []struct {
		query  string
		status int
	}{{"", 404}, {"?language=all", 200}, {"?language=fr", 200}} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/books/random"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("French only %s: %d", tc.query, w.Code)
		}
	}
}
