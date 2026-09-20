package server

import (
	"bufio"
	"fmt"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var headerField = regexp.MustCompile(`^[A-Za-z][A-Za-z ]*:`)
var yearNumber = regexp.MustCompile(`\b[0-9]{4}\b`)

// Trust only the explicit original-publication field, never Issued or a
// copyright date. Gutenberg joining the internet is not the book's birthday.
func originalYear(r io.Reader) int {
	scanner := bufio.NewScanner(io.LimitReader(r, 64<<10))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	found := false
	value := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "***") {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "original publication:") {
			found = true
			value = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			continue
		}
		if found {
			if headerField.MatchString(line) || line == "" {
				break
			}
			value += " " + line
		}
	}
	if scanner.Err() != nil || strings.ContainsAny(value, "?[]") {
		return 0
	}
	years := yearNumber.FindAllString(value, -1)
	if len(years) != 1 {
		return 0
	}
	year, _ := strconv.Atoi(years[0])
	return year
}

// Request handling performs no header I/O or locking.
func (s *Server) publicationYear(id int) int {
	return s.years[id]
}
func (s *Server) publicationBook(b catalog.Book) catalog.Book {
	b.OriginalPublicationYear = s.publicationYear(b.ID)
	return b
}

type yearRange struct{ from, to int }

func parseYears(q url.Values) (yearRange, error) {
	var years yearRange
	for key, target := range map[string]*int{"year_from": &years.from, "year_to": &years.to} {
		if !q.Has(key) {
			continue
		}
		n, err := strconv.Atoi(q.Get(key))
		if len(q[key]) != 1 || err != nil || n < 1 || n > 9999 {
			return years, fmt.Errorf("%s must be a year between 1 and 9999, specified once", key)
		}
		*target = n
	}
	if years.from != 0 && years.to != 0 && years.from > years.to {
		return years, fmt.Errorf("year_from must not exceed year_to")
	}
	return years, nil
}
