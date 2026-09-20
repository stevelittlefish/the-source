package server

import (
	"bytes"
	"github.com/stevelittlefish/the-source/docs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexLinks(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("index status: %d", w.Code)
	}
	for _, path := range []string{"/books", "/random", "/excerpts", "/docs", "/openapi.yaml", "/api-guide.md"} {
		if !strings.Contains(w.Body.String(), `href="`+path+`"`) {
			t.Errorf("missing link: %s", path)
		}
	}
	if strings.Contains(w.Body.String(), "<script") {
		t.Error("index should work without JavaScript")
	}
}

func TestServedDocsMatchSource(t *testing.T) {
	s := testServer(t)
	for _, tc := range []struct{ path, file, contentType string }{
		{"/openapi.yaml", "openapi.yaml", "application/yaml"},
		{"/api-guide.md", "API.md", "text/markdown; charset=utf-8"},
	} {
		expected, err := docs.Files.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), expected) || w.Header().Get("Content-Type") != tc.contentType {
			t.Fatalf("incorrect documentation at %s: %d", tc.path, w.Code)
		}
		w = httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("HEAD", tc.path, nil))
		if w.Code != 200 || w.Body.Len() != 0 {
			t.Fatalf("incorrect HEAD at %s", tc.path)
		}
	}
}
