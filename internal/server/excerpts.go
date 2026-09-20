package server

import (
	"bufio"
	"context"
	"fmt"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// Keep a sliding paragraph window and one reservoir sample, not a whole book.
// Missing boundaries fail closed: the licence is not a plot twist.
func extractParagraphs(ctx context.Context, input io.Reader, count int) ([]string, error) {
	scanner := bufio.NewScanner(io.LimitReader(input, 8<<20))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	started, ended := false, false
	var lines, run, chosen []string
	size, windows := 0, 0
	flush := func() {
		p := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
		lines = nil
		size = 0
		if !prose(p) {
			run = nil
			return
		}
		run = append(run, p)
		if len(run) > count {
			run = run[1:]
		}
		if len(run) == count {
			windows++
			if rand.IntN(windows) == 0 {
				chosen = append([]string(nil), run...)
			}
		}
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		upper := strings.ToUpper(line)
		marker := strings.HasPrefix(upper, "***") && strings.Contains(upper, "PROJECT GUTENBERG") && (strings.Contains(upper, "EBOOK") || strings.Contains(upper, "ETEXT"))
		if marker && strings.Contains(upper, "START OF") {
			started = true
			lines = nil
			run = nil
			size = 0
			continue
		}
		if marker && strings.Contains(upper, "END OF") {
			if started {
				flush()
				ended = true
			}
			break
		}
		if !started {
			continue
		}
		if line == "" {
			if len(lines) > 0 {
				flush()
			}
			continue
		}
		size += len(line)
		if size > 64<<10 {
			return nil, fmt.Errorf("paragraph too large")
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !started || !ended {
		return nil, fmt.Errorf("missing Gutenberg boundaries or scan limit exceeded")
	}
	if len(chosen) != count {
		return nil, fmt.Errorf("no suitable paragraph run")
	}
	return chosen, nil
}

func prose(p string) bool {
	lower := strings.ToLower(p)
	for _, term := range []string{"project gutenberg", "www.", "http://", "https://", "copyright", "all rights reserved", "printed in", "contents", "credits:", "transcrib", "release date:", "illustration"} {
		if strings.Contains(lower, term) {
			return false
		}
	}
	for _, prefix := range []string{"chapter ", "book ", "part ", "prologue", "epilogue", "preface", "introduction", "dedication"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	if len(strings.Fields(p)) < 12 || !strings.ContainsAny(p, ".!?。！？") {
		return false
	}
	letters, small := 0, 0
	for _, r := range p {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsLower(r) {
				small++
			}
		}
	}
	return letters > 0 && small*2 > letters
}

func (s *Server) excerpt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	for key, v := range q {
		if (key != "language" && key != "paragraphs" && key != "year_from" && key != "year_to") || len(v) != 1 {
			fail(w, 400, "invalid_query", "Use language, paragraphs, year_from and year_to once each.")
			return
		}
	}
	language, err := queryLanguage(q)
	if err != nil {
		fail(w, 400, "invalid_query", err.Error())
		return
	}
	count := 3
	if q.Has("paragraphs") {
		count, err = strconv.Atoi(q.Get("paragraphs"))
		if err != nil || count < 1 || count > 10 {
			fail(w, 400, "invalid_query", "paragraphs must be between 1 and 10.")
			return
		}
	}
	candidates := make([]catalog.Book, 0)
	years, err := parseYears(q)
	if err != nil {
		fail(w, 400, "invalid_query", err.Error())
		return
	}
	for _, b := range s.catalog.Search(language, "") {
		if r.Context().Err() != nil {
			return
		}
		if s.texts[b.ID] != "" && s.matchesYears(b.ID, years) {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		fail(w, 404, "no_matching_books", "No installed texts match these filters.")
		return
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	for _, b := range candidates[:min(8, len(candidates))] {
		if r.Context().Err() != nil {
			return
		}
		f, err := s.root.Open(s.texts[b.ID])
		if err != nil {
			continue
		}
		paragraphs, err := extractParagraphs(r.Context(), f, count)
		f.Close()
		if err != nil {
			continue
		}
		respond(w, 200, struct {
			Book       catalog.Book `json:"book"`
			Paragraphs []string     `json:"paragraphs"`
		}{s.publicationBook(b), paragraphs})
		return
	}
	fail(w, 422, "no_suitable_excerpt", "No suitable prose found in the sampled books. Try again or request fewer paragraphs.")
}
