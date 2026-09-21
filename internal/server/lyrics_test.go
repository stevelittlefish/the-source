package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLyricsAPI(t *testing.T) {
	s := testServer(t)
	for _, tt := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/api/v1/lyrics", 200, `"total":3`},
		{"/api/v1/lyrics?language=en", 200, `"total":2`},
		{"/api/v1/lyrics?language=all", 200, `"total":3`},
		{"/api/v1/lyrics?tag=rap", 200, `"total":1`},
		{"/api/v1/lyrics?q=killa", 200, `"total":1`},
		{"/api/v1/lyrics?q=nothingmatches", 200, `"songs":[]`},
		{"/api/v1/lyrics?limit=1", 200, `"next_cursor":"1"`},
		{"/api/v1/lyrics?limit=1&cursor=1", 200, `"id":20`},
		{"/api/v1/lyrics?limit=0", 400, "invalid_query"},
		{"/api/v1/lyrics?typo=1", 400, "invalid_query"},
		{"/api/v1/lyrics/10", 200, `"artist":"Cam'ron"`},
		{"/api/v1/lyrics/999", 404, "song_not_found"},
		{"/api/v1/lyrics/10/text", 200, "Killa Cam"},
		{"/api/v1/lyrics/tags", 200, `"pop"`},
		{"/api/v1/lyrics/languages", 200, `"fr"`},
	} {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", tt.path, nil))
			if w.Code != tt.status || !strings.Contains(w.Body.String(), tt.contains) {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestLyricsRandomAndExcerpt(t *testing.T) {
	s := testServer(t)
	// Random honours the tag filter and never caches.
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lyrics/random?tag=rap", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":10`) {
		t.Fatalf("random: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable random response")
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lyrics/random?tag=jazz", nil))
	if w.Code != 404 {
		t.Fatalf("random jazz: %d", w.Code)
	}
	// The only song with a multi-line stanza is the rap one; filter to it so the
	// excerpt is deterministic, then check a real stanza came back.
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lyrics/excerpts/random?tag=rap", nil))
	if w.Code != 200 {
		t.Fatalf("excerpt: %d %s", w.Code, w.Body.String())
	}
	var data struct {
		Song  struct{ ID int }
		Lines []string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data.Song.ID != 10 || len(data.Lines) < 2 {
		t.Fatalf("excerpt = %+v", data)
	}
	for _, line := range data.Lines {
		if strings.HasPrefix(line, "[") {
			t.Fatalf("section marker leaked into stanza: %q", line)
		}
	}
	// The single-line pop song yields no stanza.
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lyrics/excerpts/random?language=en&tag=pop", nil))
	if w.Code != 422 {
		t.Fatalf("expected no stanza, got %d %s", w.Code, w.Body.String())
	}
}

func TestLyricsAbsent(t *testing.T) {
	s := testServer(t)
	s.songs = nil
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/lyrics", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), "lyrics_unavailable") {
		t.Fatalf("books-only: %d %s", w.Code, w.Body.String())
	}
}
