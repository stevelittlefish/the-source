package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/stevelittlefish/the-source/internal/catalog"
)

type Server struct {
	catalog   *catalog.Catalog
	root      *os.Root
	mux       *http.ServeMux
	available map[int]bool
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
	s := &Server{catalog: c, root: root, mux: http.NewServeMux(), available: make(map[int]bool)}
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
		id, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".txt"))
		if err == nil && id > 0 && entry.Name() == strconv.Itoa(id)+".txt" {
			info, err := root.Stat(entry.Name())
			if err == nil && info.Mode().IsRegular() {
				s.available[id] = true
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
		respond(w, 200, bookResponse{b, s.available[id]})
		return
	}
	if !s.available[id] {
		fail(w, 404, "text_unavailable", "This book's text is not installed.")
		return
	}
	f, err := s.root.Open(strconv.Itoa(id) + ".txt")
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
	language := "en"
	if q.Has("language") {
		language = strings.TrimSpace(q.Get("language"))
		if language == "all" {
			language = ""
		} else if language == "" {
			fail(w, 400, "invalid_query", "Use language=all for all languages.")
			return
		}
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
		available := s.available[b.ID]
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
