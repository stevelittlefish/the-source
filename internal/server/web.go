package server

import (
	"embed"
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
	assets, _ := fs.Sub(web, "web")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(assets)))
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/books", http.StatusSeeOther) })
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
