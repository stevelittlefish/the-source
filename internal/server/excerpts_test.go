package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var paragraphs = []string{
	"The first visitor arrived at the little house before dawn, carrying a letter that nobody had expected.",
	"She placed the letter on the table and waited quietly while the others gathered around the fire.",
	"Outside, the old trees moved in the wind, and the road disappeared into the gathering morning mist.",
}

func excerptFixture() string {
	return "The Project Gutenberg eBook of The story of Ivy\nCredits: a volunteer\n\n*** START OF THE PROJECT GUTENBERG EBOOK THE STORY OF IVY ***\n\nTHE STORY OF IVY\nBy\nMARIE BELLOC LOWNDES\n\nSOMERSET BOOKS, INC.\nNEW YORK\n\nCOPYRIGHT, 1927, ALL RIGHTS RESERVED. PRINTED IN THE UNITED STATES.\n\nContents\nPrologue\nChapter One\nChapter Two\nEpilogue\n\nChapter One\n\n" + strings.Join(paragraphs, "\n\n") + "\n\n*** END OF THE PROJECT GUTENBERG EBOOK THE STORY OF IVY ***\nLicence and other paperwork."
}
func TestExtractParagraphs(t *testing.T) {
	f, err := os.Open("../../testdata/books/1342/pg1342.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := extractParagraphs(context.Background(), f, 3); err != nil {
		t.Fatalf("development fixture: %v", err)
	}
	for _, input := range []string{excerptFixture(), strings.ReplaceAll(excerptFixture(), "\n", "\r\n")} {
		got, err := extractParagraphs(context.Background(), strings.NewReader(input), 3)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, "|") != strings.Join(paragraphs, "|") {
			t.Fatalf("got %#v", got)
		}
	}
	for _, input := range []string{strings.Join(paragraphs, "\n\n"), strings.ReplaceAll(excerptFixture(), "*** END OF", "missing end"), strings.Replace(excerptFixture(), paragraphs[1], "Chapter Two", 1)} {
		if _, err := extractParagraphs(context.Background(), strings.NewReader(input), 3); err == nil {
			t.Fatal("accepted missing markers or stitched across a heading")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extractParagraphs(ctx, strings.NewReader(excerptFixture()), 1); err == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestExcerptAPI(t *testing.T) {
	s := testServer(t)
	f, err := s.root.OpenFile("1.txt", os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(excerptFixture())
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query         string
		status, count int
	}{
		{"", 200, 3}, {"?paragraphs=1", 200, 1}, {"?paragraphs=4", 422, 0},
		{"?paragraphs=0", 400, 0}, {"?paragraphs=11", 400, 0}, {"?paragraphs=x", 400, 0},
		{"?paragraphs=1&paragraphs=2", 400, 0}, {"?language=", 400, 0},
		{"?language=fr", 404, 0}, {"?language=all", 200, 3}, {"?words=100", 400, 0},
	} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/excerpts/random"+tc.query, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cacheable random response")
		}
		if tc.status == 200 {
			var data struct {
				Paragraphs []string
				Book       struct{ ID int }
			}
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Paragraphs) != tc.count || data.Book.ID != 1 {
				t.Fatalf("%+v", data)
			}
		}
	}
}
