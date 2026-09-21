package server

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestOriginalYear(t *testing.T) {
	for _, tc := range []struct {
		text string
		year int
	}{
		{"Release date: 2026\nOriginal publication: New York: Somerset Books, Inc., 1927\nLanguage: English", 1927},
		{"Original publication: New York:\n Somerset Books, Inc., 1927\n\nCredits: somebody", 1927},
		{"Release date: 2026\nCopyright 1927", 0},
		{"Original publication: 1927–1928", 0},
		{"Original publication: [1927?]", 0},
		{"*** START OF THE PROJECT GUTENBERG EBOOK TEST ***\nOriginal publication: 1927", 0},
	} {
		if got := originalYear(strings.NewReader(tc.text)); got != tc.year {
			t.Fatalf("%q: %d", tc.text, got)
		}
	}
}
func TestRandomYearFilters(t *testing.T) {
	s := testServer(t)
	f, err := s.root.OpenFile("1.txt", os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("Original publication: Test publisher, 1927\n\n" + excerptFixture())
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.prepareYears(s.root.Name(), ""); err != nil {
		t.Fatal(err)
	}
	s.buildPools()
	for _, endpoint := range []string{"books/random", "books/excerpts/random"} {
		for _, tc := range []struct {
			q    string
			code int
		}{
			{"", 200}, {"?year_from=1901", 200}, {"?year_from=1927&year_to=1927", 200},
			{"?year_to=1926", 404}, {"?year_from=1928", 404},
			{"?year_from=1950&year_to=1900", 400}, {"?year_from=", 400}, {"?year_to=abc", 400},
			{"?year_from=1900&year_from=1920", 400}, {"?year_to=10000", 400},
		} {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/"+endpoint+tc.q, nil))
			if w.Code != tc.code {
				t.Fatalf("%s%s: %d %s", endpoint, tc.q, w.Code, w.Body.String())
			}
			if tc.code == 200 && !strings.Contains(w.Body.String(), `"original_publication_year":1927`) {
				t.Fatal(w.Body.String())
			}
		}
	}
	s.years[1] = 0
	s.buildPools()
	for _, endpoint := range []string{"books/random", "books/excerpts/random"} {
		for _, tc := range []struct {
			q    string
			code int
		}{{"", 200}, {"?year_from=1901", 404}} {
			w := httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/"+endpoint+tc.q, nil))
			if w.Code != tc.code {
				t.Fatalf("unknown year %s%s: %d", endpoint, tc.q, w.Code)
			}
		}
	}
}
