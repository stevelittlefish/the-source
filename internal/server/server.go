package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/stevelittlefish/the-source/internal/catalog"
)

type Server struct {
	catalog *catalog.Catalog
	root    *os.Root
	mux     *http.ServeMux
	texts   map[int]string
}

type bookResponse struct {
	catalog.Book
	Available bool `json:"available"`
}

func New(c *catalog.Catalog, dir string) (*Server, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open corpus: %w", err)
	}
	s := &Server{catalog: c, root: root, mux: http.NewServeMux(), texts: make(map[int]string)}
	// Inspect names only once. No book contents are read during startup.
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		root.Close()
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		number := strings.TrimSuffix(name, ".txt")
		id, err := strconv.Atoi(number)
		if err != nil || id < 1 || number != strconv.Itoa(id) {
			continue
		}
		path := name
		nested := name == number
		if nested {
			path = number + "/pg" + number + ".txt"
		}
		info, err := root.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			// Prefer the mirror copy when both layouts contain the same ID.
			if nested || s.texts[id] == "" {
				s.texts[id] = path
			}
		}
	}
	s.mux.HandleFunc("/api/", s.api)
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			fail(w, 405, "method_not_allowed", "Use GET or HEAD.")
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	s.routes()
	return s, nil
}

func (s *Server) Close() error { return s.root.Close() }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
	s.mux.ServeHTTP(w, r)
}

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, 405, "method_not_allowed", "Use GET or HEAD.")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	switch path {
	case "excerpts/random":
		s.excerpt(w, r)
		return
	case "books/random":
		s.random(w, r)
		return
	case "books":
		s.list(w, r)
		return
	case "languages":
		respond(w, 200, map[string]any{"languages": s.catalog.Languages()})
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "books" || (len(parts) == 3 && parts[2] != "text") {
		fail(w, 404, "not_found", "Unknown API endpoint.")
		return
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id < 1 {
		fail(w, 404, "book_not_found", "Unknown book ID.")
		return
	}
	b, ok := s.catalog.Get(id)
	if !ok {
		fail(w, 404, "book_not_found", "Unknown book ID.")
		return
	}
	if len(parts) == 2 {
		respond(w, 200, bookResponse{b, s.texts[id] != ""})
		return
	}
	if s.texts[id] == "" {
		fail(w, 404, "text_unavailable", "This book's text is not installed.")
		return
	}
	f, err := s.root.Open(s.texts[id])
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, "text_unavailable", "This book's text is not installed.")
			return
		}
		log.Printf("open text %d: %v", id, err)
		fail(w, 500, "text_error", "Cannot open book text.")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fail(w, 404, "text_unavailable", "This book's text is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, strconv.Itoa(id)+".txt", info.ModTime(), f)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for key, values := range q {
		if (key != "language" && key != "q" && key != "limit" && key != "cursor" && key != "available") || len(values) != 1 {
			fail(w, 400, "invalid_query", "Unknown or repeated query parameter.")
			return
		}
	}
	language, languageErr := queryLanguage(q)
	if languageErr != nil {
		fail(w, 400, "invalid_query", languageErr.Error())
		return
	}
	limit := 25
	var err error
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			fail(w, 400, "invalid_query", "limit must be between 1 and 100.")
			return
		}
	}
	offset := 0
	if q.Has("cursor") {
		offset, err = strconv.Atoi(q.Get("cursor"))
		if err != nil || offset < 0 {
			fail(w, 400, "invalid_query", "Invalid cursor.")
			return
		}
	}
	if len(q.Get("q")) > 256 {
		fail(w, 400, "invalid_query", "Search is limited to 256 bytes.")
		return
	}
	if q.Has("available") && q.Get("available") != "true" && q.Get("available") != "false" {
		fail(w, 400, "invalid_query", "available must be true or false.")
		return
	}
	matched := s.catalog.Search(language, q.Get("q"))
	items := make([]bookResponse, 0, limit)
	total := 0
	for _, b := range matched {
		available := s.texts[b.ID] != ""
		if q.Has("available") && available != (q.Get("available") == "true") {
			continue
		}
		if total >= offset && len(items) < limit {
			items = append(items, bookResponse{b, available})
		}
		total++
	}
	next := ""
	if offset < total && len(items) < total-offset {
		next = strconv.Itoa(offset + len(items))
	}
	respond(w, 200, struct {
		Books []bookResponse `json:"books"`
		Total int            `json:"total"`
		Next  string         `json:"next_cursor"`
	}{items, total, next})
}

// Removing the language filter must be deliberate, even before coffee.
func queryLanguage(q url.Values) (string, error) {
	if !q.Has("language") {
		return "en", nil
	}
	if len(q["language"]) != 1 {
		return "", fmt.Errorf("Specify language once.")
	}
	language := strings.ToLower(strings.TrimSpace(q.Get("language")))
	if language == "" {
		return "", fmt.Errorf("Use language=all for all languages.")
	}
	if language == "all" {
		return "", nil
	}
	return language, nil
}

func (s *Server) random(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	for key := range q {
		if key != "language" {
			fail(w, 400, "invalid_query", "Only language is supported for random books.")
			return
		}
	}
	language, err := queryLanguage(q)
	if err != nil {
		fail(w, 400, "invalid_query", err.Error())
		return
	}
	var chosen catalog.Book
	count := 0
	// Reservoir sampling gives each installed matching work an equal chance.
	for _, book := range s.catalog.Search(language, "") {
		if s.texts[book.ID] == "" {
			continue
		}
		count++
		if rand.IntN(count) == 0 {
			chosen = book
		}
	}
	if count == 0 {
		fail(w, 404, "no_matching_books", "No installed texts match this language.")
		return
	}
	respond(w, 200, bookResponse{chosen, true})
}
