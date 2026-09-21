package server

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/stevelittlefish/the-source/internal/lyrics"
)

// parseViews reads the optional views_from and views_to bounds, mirroring the
// year filters on books. Zero (or absent) means unbounded on that side.
func parseViews(q url.Values) (from, to int, err error) {
	for key, target := range map[string]*int{"views_from": &from, "views_to": &to} {
		if !q.Has(key) {
			continue
		}
		n, convErr := strconv.Atoi(q.Get(key))
		if len(q[key]) != 1 || convErr != nil || n < 0 {
			return 0, 0, fmt.Errorf("%s must be a non-negative integer, specified once", key)
		}
		*target = n
	}
	if from != 0 && to != 0 && from > to {
		return 0, 0, fmt.Errorf("views_from must not exceed views_to")
	}
	return from, to, nil
}

// randomStanza returns the lines of one randomly chosen stanza — a run of
// non-blank lines — from a lyrics body, reservoir-sampled so the whole song is
// never held as a list of stanzas at once. Section markers like "[Chorus]" are
// dropped as lines, and a lone marker or single line is not a stanza worth
// serving. Returns nil when nothing suitable is found.
func randomStanza(body string) []string {
	var current, chosen []string
	seen := 0
	consider := func() {
		if len(current) >= 2 {
			seen++
			if rand.IntN(seen) == 0 {
				chosen = append([]string(nil), current...)
			}
		}
		current = nil
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			consider()
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		current = append(current, line)
	}
	consider()
	return chosen
}

// lyricsReady fails the request when no lyrics database is configured, so the
// endpoints degrade cleanly on a books-only deployment.
func (s *Server) lyricsReady(w http.ResponseWriter) bool {
	if s.songs == nil {
		fail(w, 404, "lyrics_unavailable", "This server has no lyrics corpus installed.")
		return false
	}
	return true
}

// lyricsQuery reads the optional shared filters (language, tag) common to
// listing and random selection. Unlike books, lyrics do not default to English:
// the corpus is single-language per file, so "no filter" is the honest default.
func lyricsQuery(q map[string][]string) (language, tag string) {
	if v := q["language"]; len(v) == 1 && strings.ToLower(strings.TrimSpace(v[0])) != "all" {
		language = strings.ToLower(strings.TrimSpace(v[0]))
	}
	if v := q["tag"]; len(v) == 1 {
		tag = strings.ToLower(strings.TrimSpace(v[0]))
	}
	return language, tag
}

func (s *Server) lyricsList(w http.ResponseWriter, r *http.Request) {
	if !s.lyricsReady(w) {
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if (key != "language" && key != "tag" && key != "q" && key != "limit" && key != "cursor") || len(values) != 1 {
			fail(w, 400, "invalid_query", "Unknown or repeated query parameter.")
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
	language, tag := lyricsQuery(q)
	songs, total, err := s.songs.Search(lyrics.Filter{
		Language: language, Tag: tag, Query: q.Get("q"), Limit: limit, Offset: offset,
	})
	if err != nil {
		log.Printf("lyrics search: %v", err)
		fail(w, 500, "lyrics_error", "Cannot search lyrics.")
		return
	}
	next := ""
	if offset+len(songs) < total {
		next = strconv.Itoa(offset + len(songs))
	}
	respond(w, 200, struct {
		Songs []lyrics.Song `json:"songs"`
		Total int           `json:"total"`
		Next  string        `json:"next_cursor"`
	}{songs, total, next})
}

func (s *Server) lyricsItem(w http.ResponseWriter, r *http.Request, idText string, text bool) {
	if !s.lyricsReady(w) {
		return
	}
	id, err := strconv.Atoi(idText)
	if err != nil || id < 1 {
		fail(w, 404, "song_not_found", "Unknown song ID.")
		return
	}
	if text {
		body, ok, err := s.songs.Text(id)
		if err != nil {
			log.Printf("lyrics text %d: %v", id, err)
			fail(w, 500, "lyrics_error", "Cannot read lyrics.")
			return
		}
		if !ok {
			fail(w, 404, "song_not_found", "Unknown song ID.")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		w.Write([]byte(body))
		return
	}
	song, ok, err := s.songs.Get(id)
	if err != nil {
		log.Printf("lyrics get %d: %v", id, err)
		fail(w, 500, "lyrics_error", "Cannot read song.")
		return
	}
	if !ok {
		fail(w, 404, "song_not_found", "Unknown song ID.")
		return
	}
	respond(w, 200, song)
}

func (s *Server) lyricsRandom(w http.ResponseWriter, r *http.Request) {
	if !s.lyricsReady(w) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	for key := range q {
		if key != "language" && key != "tag" && key != "views_from" && key != "views_to" {
			fail(w, 400, "invalid_query", "Use language, tag, views_from and views_to for random lyrics.")
			return
		}
	}
	language, tag := lyricsQuery(q)
	viewsFrom, viewsTo, err := parseViews(q)
	if err != nil {
		fail(w, 400, "invalid_query", err.Error())
		return
	}
	song, ok, err := s.songs.Random(lyrics.RandomFilter{Language: language, Tag: tag, ViewsFrom: viewsFrom, ViewsTo: viewsTo})
	if err != nil {
		log.Printf("lyrics random: %v", err)
		fail(w, 500, "lyrics_error", "Cannot select a song.")
		return
	}
	if !ok {
		fail(w, 404, "no_matching_songs", "No songs match these filters.")
		return
	}
	respond(w, 200, song)
}

// lyricsExcerpt returns a random block of lines from a random song. Lyrics have
// honest boundaries — blank lines separate verses and choruses — so a stanza is
// simply the text between blanks, no prose-detection heuristics required.
func (s *Server) lyricsExcerpt(w http.ResponseWriter, r *http.Request) {
	if !s.lyricsReady(w) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	for key := range q {
		if key != "language" && key != "tag" && key != "views_from" && key != "views_to" {
			fail(w, 400, "invalid_query", "Use language, tag, views_from and views_to for random excerpts.")
			return
		}
	}
	language, tag := lyricsQuery(q)
	viewsFrom, viewsTo, err := parseViews(q)
	if err != nil {
		fail(w, 400, "invalid_query", err.Error())
		return
	}
	filter := lyrics.RandomFilter{Language: language, Tag: tag, ViewsFrom: viewsFrom, ViewsTo: viewsTo}
	for attempt := 0; attempt < 8; attempt++ {
		if r.Context().Err() != nil {
			return
		}
		song, ok, err := s.songs.Random(filter)
		if err != nil {
			log.Printf("lyrics excerpt: %v", err)
			fail(w, 500, "lyrics_error", "Cannot select a song.")
			return
		}
		if !ok {
			fail(w, 404, "no_matching_songs", "No songs match these filters.")
			return
		}
		body, _, err := s.songs.Text(song.ID)
		if err != nil {
			continue
		}
		if stanza := randomStanza(body); len(stanza) > 0 {
			song.Lyrics = ""
			respond(w, 200, struct {
				Song  lyrics.Song `json:"song"`
				Lines []string    `json:"lines"`
			}{song, stanza})
			return
		}
	}
	fail(w, 422, "no_suitable_excerpt", "No suitable stanza found in the sampled songs.")
}

func (s *Server) lyricsTags(w http.ResponseWriter, r *http.Request) {
	if !s.lyricsReady(w) {
		return
	}
	tags, err := s.songs.Tags()
	if err != nil {
		log.Printf("lyrics tags: %v", err)
		fail(w, 500, "lyrics_error", "Cannot list tags.")
		return
	}
	respond(w, 200, map[string]any{"tags": tags})
}

func (s *Server) lyricsLanguages(w http.ResponseWriter, r *http.Request) {
	if !s.lyricsReady(w) {
		return
	}
	languages, err := s.songs.Languages()
	if err != nil {
		log.Printf("lyrics languages: %v", err)
		fail(w, 500, "lyrics_error", "Cannot list languages.")
		return
	}
	respond(w, 200, map[string]any{"languages": languages})
}
