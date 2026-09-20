// Package catalog loads and queries Project Gutenberg's metadata export.
package catalog

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Book is a Project Gutenberg work. Text is stored elsewhere because 90,000
// copies of text in memory would be a rather literal interpretation of library.
type Book struct {
	ID          int      `json:"id"`
	Type        string   `json:"type"`
	Issued      string   `json:"issued"`
	Title       string   `json:"title"`
	Languages   []string `json:"languages"`
	Authors     string   `json:"authors"`
	Subjects    []string `json:"subjects"`
	LoCC        []string `json:"locc"`
	Bookshelves []string `json:"bookshelves"`
}

// Catalog keeps records in predictable title order and makes ID lookup cheap.
type Catalog struct {
	books  []Book
	byID   map[int]Book
	search map[int]string
}

// LoadFile reads Gutenberg's CSV export. encoding/csv, unlike an optimistic
// strings.Split, understands that book titles may contain commas and newlines.
func LoadFile(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer f.Close()
	return Load(f)
}

// Load reads a catalog CSV from r.
func Load(r io.Reader) (*Catalog, error) {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read catalog header: %w", err)
	}
	expected := []string{"Text#", "Type", "Issued", "Title", "Language", "Authors", "Subjects", "LoCC", "Bookshelves"}
	if !sameFields(header, expected) {
		return nil, fmt.Errorf("unexpected catalog header")
	}

	result := &Catalog{byID: make(map[int]Book), search: make(map[int]string)}
	row := 1
	for {
		row++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read catalog row %d: %w", row, err)
		}
		if len(record) != len(expected) {
			return nil, fmt.Errorf("catalog row %d: got %d fields, want %d", row, len(record), len(expected))
		}
		id, err := strconv.Atoi(record[0])
		if err != nil || id < 1 {
			return nil, fmt.Errorf("catalog row %d: invalid text number %q", row, record[0])
		}
		if _, exists := result.byID[id]; exists {
			return nil, fmt.Errorf("catalog row %d: duplicate text number %d", row, id)
		}
		book := Book{
			ID: id, Type: record[1], Issued: record[2], Title: squashSpace(record[3]),
			Languages: splitList(strings.ToLower(record[4])), Authors: squashSpace(record[5]),
			Subjects: splitList(record[6]), LoCC: splitList(record[7]), Bookshelves: splitList(record[8]),
		}
		result.books = append(result.books, book)
		result.byID[id] = book
		result.search[id] = strings.ToLower(strings.Join([]string{book.Title, book.Authors, strings.Join(book.Subjects, " "), strings.Join(book.Bookshelves, " ")}, " "))
	}
	sort.Slice(result.books, func(i, j int) bool {
		left, right := strings.ToLower(result.books[i].Title), strings.ToLower(result.books[j].Title)
		if left == right {
			return result.books[i].ID < result.books[j].ID
		}
		return left < right
	})
	return result, nil
}

// Get returns a record by its Gutenberg text number.
func (c *Catalog) Get(id int) (Book, bool) { book, ok := c.byID[id]; return book, ok }

// Search returns title-sorted books in language whose searchable metadata
// contains every query word. An empty query is ordinary browsing.
func (c *Catalog) Search(language, query string) []Book {
	language = strings.ToLower(strings.TrimSpace(language))
	words := strings.Fields(strings.ToLower(query))
	matched := make([]Book, 0)
	for _, book := range c.books {
		if book.Type != "Text" || (language != "" && !slices.Contains(book.Languages, language)) {
			continue
		}
		haystack := c.search[book.ID]
		allWords := true
		for _, word := range words {
			if !strings.Contains(haystack, word) {
				allWords = false
				break
			}
		}
		if allWords {
			matched = append(matched, book)
		}
	}
	return matched
}

// Languages returns the catalog's distinct language codes in lexical order.
func (c *Catalog) Languages() []string {
	seen := make(map[string]struct{})
	for _, book := range c.books {
		if book.Type == "Text" {
			for _, language := range book.Languages {
				seen[language] = struct{}{}
			}
		}
	}
	languages := make([]string, 0, len(seen))
	for language := range seen {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

func sameFields(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func splitList(value string) []string {
	parts := strings.Split(value, ";")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func squashSpace(value string) string { return strings.Join(strings.Fields(value), " ") }
