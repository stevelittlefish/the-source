package server

import (
	"embed"
	"github.com/stevelittlefish/the-source/docs"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
)

//go:embed web/*
var web embed.FS
var page = template.Must(template.ParseFS(web, "web/page.html"))

func (s *Server) routes() {
	s.mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		// Swagger creates inline style attributes. Keep this exception scoped
		// to its page; scripts and API requests still stay on this origin.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
		http.ServeFileFS(w, r, web, "web/swagger.html")
	})
	s.mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFileFS(w, r, docs.Files, "openapi.yaml")
	})
	s.mux.HandleFunc("GET /api-guide.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		http.ServeFileFS(w, r, docs.Files, "API.md")
	})
	s.mux.HandleFunc("GET /excerpts", func(w http.ResponseWriter, r *http.Request) { s.page(w, "excerpts", "Random excerpt", 0) })
	s.mux.HandleFunc("GET /random", func(w http.ResponseWriter, r *http.Request) { s.page(w, "random", "Random book", 0) })
	assets, _ := fs.Sub(web, "web")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets)))
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { s.page(w, "index", "The Source", 0) })
	s.mux.HandleFunc("GET /books", func(w http.ResponseWriter, r *http.Request) { s.page(w, "browse", "Browse the library", 0) })
	s.mux.HandleFunc("GET /books/{id}", func(w http.ResponseWriter, r *http.Request) { s.bookPage(w, r, "book") })
	s.mux.HandleFunc("GET /read/{id}", func(w http.ResponseWriter, r *http.Request) { s.bookPage(w, r, "read") })
}
func (s *Server) bookPage(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.catalog.Get(id); !ok {
		http.NotFound(w, r)
		return
	}
	s.page(w, kind, "Book "+strconv.Itoa(id), id)
}
func (s *Server) page(w http.ResponseWriter, kind, title string, id int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := page.Execute(w, struct {
		Kind, Title string
		ID          int
	}{kind, title, id})
	if err != nil {
		log.Printf("render page: %v", err)
	}
}
