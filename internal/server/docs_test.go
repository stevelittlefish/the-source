package server

import (
	"bytes"
	"github.com/stevelittlefish/the-source/docs"
	"net/http/httptest"
	"testing"
)

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
