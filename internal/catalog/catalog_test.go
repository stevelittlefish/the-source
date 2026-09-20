package catalog

import (
	"strings"
	"testing"
)

const fixtureCSV = `Text#,Type,Issued,Title,Language,Authors,Subjects,LoCC,Bookshelves
3,Text,1973-11-01,"A Tale, Indeed",en,"Doe, Jane","Fiction; Adventure",PR,Adventure
9,Text,1980-01-01,"Le livre
français",fr,"Dupont, Jean",Histoire,DC,Histoire
12,Text,1975-12-01,Another Tale,en,"Smith, John",Fiction,PR,Classics
`

func TestLoadHandlesQuotedAndMultilineFields(t *testing.T) {
	catalog, err := Load(strings.NewReader(fixtureCSV))
	if err != nil {
		t.Fatal(err)
	}
	book, ok := catalog.Get(9)
	if !ok {
		t.Fatal("book 9 missing")
	}
	if book.Title != "Le livre français" {
		t.Fatalf("title = %q", book.Title)
	}
	if got := catalog.Search("en", "tale fiction"); len(got) != 2 {
		t.Fatalf("English fiction tale results = %d, want 2", len(got))
	}
	if got := catalog.Search("fr", "livre"); len(got) != 1 || got[0].ID != 9 {
		t.Fatalf("French search = %#v", got)
	}
}

func TestLoadRejectsUnexpectedHeader(t *testing.T) {
	_, err := Load(strings.NewReader("id,title\n1,Nope\n"))
	if err == nil {
		t.Fatal("Load accepted an unexpected header")
	}
}

func TestFullCatalog(t *testing.T) {
	c, err := LoadFile("../../pg_catalog.csv")
	if err != nil {
		t.Fatal(err)
	}
	english := len(c.Search("en", ""))
	t.Logf("catalog: %d records; %d English text records", len(c.books), english)
	if english == 0 {
		t.Fatal("no English books found")
	}
	if b, ok := c.Get(1342); !ok || b.Title != "Pride and Prejudice" {
		t.Fatalf("unexpected Austen: %+v", b)
	}
}
