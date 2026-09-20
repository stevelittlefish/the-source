package server

import (
	"encoding/json"
	"fmt"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type yearEntry struct {
	Path     string
	Size     int64
	Modified int64
	Year     int
}
type yearIndex struct {
	Version int
	Corpus  string
	Entries map[int]yearEntry
}

func saveYearIndex(path string, index yearIndex) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".year-index-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(index); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func (s *Server) prepareYears(dir, path string) error {
	start := time.Now()
	corpus, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	previous := yearIndex{}
	if path != "" {
		f, err := os.Open(path)
		if err == nil {
			err = json.NewDecoder(f).Decode(&previous)
			f.Close()
			if err != nil {
				log.Printf("year index unreadable; rebuilding: %v", err)
				previous = yearIndex{}
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("read year index: %w", err)
		}
	}
	if previous.Version != 1 || previous.Corpus != corpus {
		previous = yearIndex{}
	}
	next := yearIndex{Version: 1, Corpus: corpus, Entries: make(map[int]yearEntry, len(s.texts))}
	s.years = make(map[int]int, len(s.texts))
	reused, read, known := 0, 0, 0
	log.Printf("year index: checking %d installed books (cache %q)", len(s.texts), path)
	for id, textPath := range s.texts {
		info, err := s.root.Stat(textPath)
		if err != nil {
			return fmt.Errorf("stat book %d: %w", id, err)
		}
		entry := yearEntry{Path: textPath, Size: info.Size(), Modified: info.ModTime().UnixNano()}
		old, ok := previous.Entries[id]
		if ok && old.Path == entry.Path && old.Size == entry.Size && old.Modified == entry.Modified {
			entry.Year = old.Year
			reused++
		} else {
			f, err := s.root.Open(textPath)
			if err != nil {
				return fmt.Errorf("index book %d: %w", id, err)
			}
			entry.Year = originalYear(f)
			f.Close()
			read++
		}
		next.Entries[id] = entry
		s.years[id] = entry.Year
		if entry.Year != 0 {
			known++
		}
		if len(next.Entries)%2000 == 0 {
			log.Printf("year index: %d/%d checked, %d cached, %d headers read", len(next.Entries), len(s.texts), reused, read)
			if read > 0 {
				if err := saveYearIndex(path, next); err != nil {
					return fmt.Errorf("checkpoint year index: %w", err)
				}
			}
		}
	}
	if err := saveYearIndex(path, next); err != nil {
		return fmt.Errorf("save year index: %w", err)
	}
	log.Printf("year index ready: %d known, %d unknown; %d reused, %d headers read; %s", known, len(s.years)-known, reused, read, time.Since(start).Round(time.Millisecond))
	return nil
}

func (s *Server) buildPools() {
	s.pools = make(map[string][]catalog.Book)
	for _, b := range s.catalog.Search("", "") {
		if s.texts[b.ID] == "" {
			continue
		}
		b = s.publicationBook(b)
		s.pools[""] = append(s.pools[""], b)
		seen := map[string]bool{}
		for _, language := range b.Languages {
			if language != "" && !seen[language] {
				s.pools[language] = append(s.pools[language], b)
				seen[language] = true
			}
		}
	}
	for _, pool := range s.pools {
		sort.Slice(pool, func(i, j int) bool {
			if pool[i].OriginalPublicationYear == pool[j].OriginalPublicationYear {
				return pool[i].ID < pool[j].ID
			}
			return pool[i].OriginalPublicationYear < pool[j].OriginalPublicationYear
		})
	}
}

// Two binary searches; unknown years sort before all eligible years.
func (s *Server) selection(language string, years yearRange) []catalog.Book {
	pool := s.pools[language]
	if years.from == 0 && years.to == 0 {
		return pool
	}
	from := max(1, years.from)
	lo := sort.Search(len(pool), func(i int) bool { return pool[i].OriginalPublicationYear >= from })
	hi := len(pool)
	if years.to != 0 {
		hi = sort.Search(len(pool), func(i int) bool { return pool[i].OriginalPublicationYear > years.to })
	}
	return pool[lo:hi]
}
