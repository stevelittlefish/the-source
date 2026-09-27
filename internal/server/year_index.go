package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/stevelittlefish/the-source/internal/catalog"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// bookIndexVersion 2 added Complete. Version 1 files were checkpointed
// mid-build under the same name, so none of them can prove they finished.
const bookIndexVersion = 2

// yearEntry is one installed book: where its text lives and its original
// publication year (0 when the header doesn't say). Size and Modified are
// only read from version 1 files; nothing checks them any more.
type yearEntry struct {
	Path     string
	Size     int64 `json:",omitempty"`
	Modified int64 `json:",omitempty"`
	Year     int
}
type yearIndex struct {
	Version  int
	Corpus   string
	Complete bool
	Entries  map[int]yearEntry
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

// indexBooks fills s.texts and s.years. The Gutenberg mirror never changes,
// so a finished index at path is trusted outright: no directory listing, no
// stat, no header. Only a missing, unreadable, unfinished or foreign index
// starts the one-time build, which scans every book and says so. Delete the
// index file to force a rebuild. With no path (tests), it always scans.
func (s *Server) indexBooks(dir, path string) error {
	start := time.Now()
	corpus, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	previous, reason := readBookIndex(path, corpus)
	if reason == "" {
		s.texts = make(map[int]string, len(previous.Entries))
		s.years = make(map[int]int, len(previous.Entries))
		for id, entry := range previous.Entries {
			s.texts[id] = entry.Path
			s.years[id] = entry.Year
		}
		log.Printf("book index: loaded %d installed books from %s in %s; trusting it, no scan (delete the file to force a rebuild)",
			len(s.texts), path, time.Since(start).Round(time.Millisecond))
		return nil
	}
	log.Printf("book index: %s", reason)
	log.Printf("book index: BUILDING IT ONCE. Every book in %s is checked and its header read; on a cold disk this takes minutes. Later starts load the finished index and skip all of this.", dir)
	if err := s.scanBooks(); err != nil {
		return err
	}
	next := yearIndex{Version: bookIndexVersion, Corpus: corpus, Entries: make(map[int]yearEntry, len(s.texts))}
	s.years = make(map[int]int, len(s.texts))
	reused, read, known := 0, 0, 0
	lastReport := time.Now()
	yearStart := time.Now()
	log.Printf("book index: reading publication years for %d books (%d already known from the old index)", len(s.texts), len(previous.Entries))
	for id, textPath := range s.texts {
		entry := yearEntry{Path: textPath}
		if old, ok := previous.Entries[id]; ok && old.Path == textPath {
			entry.Year = old.Year // An interrupted or older build already read this one.
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
		if len(next.Entries)%2000 == 0 || time.Since(lastReport) > 10*time.Second {
			lastReport = time.Now()
			log.Printf("book index: %d/%d years, %d reused, %d headers read; %s", len(next.Entries), len(s.texts), reused, read, progress(len(next.Entries), len(s.texts), yearStart))
			if read > 0 {
				// Checkpoint unfinished: a restart resumes without rereading headers.
				if err := saveYearIndex(path, next); err != nil {
					return fmt.Errorf("checkpoint book index: %w", err)
				}
			}
		}
	}
	next.Complete = true
	if err := saveYearIndex(path, next); err != nil {
		return fmt.Errorf("save book index: %w", err)
	}
	saved := "not saved: no year_index_path"
	if path != "" {
		saved = "saved to " + path
	}
	log.Printf("book index: BUILT in %s; %d books, %d with a known year, %d without; %d years reused, %d headers read; %s",
		time.Since(start).Round(time.Millisecond), len(s.texts), known, len(s.years)-known, reused, read, saved)
	return nil
}

// readBookIndex returns the saved index, and an empty reason if it can be
// trusted as it stands. Otherwise the reason says why a build is needed, and
// whatever entries it has may still save rereading headers.
func readBookIndex(path, corpus string) (yearIndex, string) {
	if path == "" {
		return yearIndex{}, "no year_index_path configured, so the index lives in memory only"
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return yearIndex{}, "no index at " + path + " yet"
	}
	if err != nil {
		return yearIndex{}, fmt.Sprintf("cannot open %s (%v)", path, err)
	}
	defer f.Close()
	var index yearIndex
	if err := json.NewDecoder(f).Decode(&index); err != nil {
		return yearIndex{}, fmt.Sprintf("index at %s is unreadable (%v)", path, err)
	}
	switch {
	case index.Corpus != corpus:
		return yearIndex{}, fmt.Sprintf("index at %s is for %q, not %q", path, index.Corpus, corpus)
	case index.Version != bookIndexVersion:
		return index, fmt.Sprintf("index at %s is format version %d, and this server writes %d", path, index.Version, bookIndexVersion)
	case !index.Complete:
		return index, fmt.Sprintf("index at %s is from a build that never finished", path)
	}
	return index, ""
}

// rate describes how fast a loop is going: "1234/s after 45s".
func rate(done int, since time.Time) string {
	elapsed := time.Since(since)
	perSecond := 0.0
	if elapsed > 0 {
		perSecond = float64(done) / elapsed.Seconds()
	}
	return fmt.Sprintf("%.0f/s after %s", perSecond, elapsed.Round(time.Second))
}

// progress adds a percentage and an estimate of the time left to rate.
func progress(done, total int, since time.Time) string {
	if total == 0 || done == 0 {
		return rate(done, since)
	}
	left := time.Duration(float64(time.Since(since)) / float64(done) * float64(total-done))
	return fmt.Sprintf("%.0f%%, %s, about %s left", 100*float64(done)/float64(total), rate(done, since), left.Round(time.Second))
}

// scanBooks lists the corpus directory and finds each book's text file. It
// only runs while building the index.
func (s *Server) scanBooks() error {
	start := time.Now()
	s.texts = make(map[int]string)
	f, err := s.root.Open(".")
	if err != nil {
		return err
	}
	// List in batches so a slow directory read still reports as it goes. Every
	// progress line is logged by the loop doing the work, so a line in the log
	// means that much really got done.
	log.Printf("book scan: listing the book directory")
	var entries []os.DirEntry
	lastReport := time.Now()
	for {
		batch, err := f.ReadDir(1000)
		entries = append(entries, batch...)
		if errors.Is(err, io.EOF) || (err == nil && len(batch) == 0) {
			break
		}
		if err != nil {
			f.Close()
			return err
		}
		if len(entries)%10000 == 0 || time.Since(lastReport) > 10*time.Second {
			lastReport = time.Now()
			log.Printf("book scan: %d entries listed so far, %s", len(entries), rate(len(entries), start))
		}
	}
	f.Close()
	log.Printf("book scan: %d entries listed in %s; checking each for a text file", len(entries), time.Since(start).Round(time.Millisecond))
	checkStart := time.Now()
	lastReport = time.Now()
	for i, entry := range entries {
		if i > 0 && (i%5000 == 0 || time.Since(lastReport) > 10*time.Second) {
			lastReport = time.Now()
			log.Printf("book scan: %d/%d entries checked, %d texts found; %s", i, len(entries), len(s.texts), progress(i, len(entries), checkStart))
		}
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
		info, err := s.root.Stat(path)
		if err == nil && info.Mode().IsRegular() {
			// Prefer the mirror copy when both layouts contain the same ID.
			if nested || s.texts[id] == "" {
				s.texts[id] = path
			}
		}
	}
	log.Printf("book scan: done, %d installed texts in %s", len(s.texts), time.Since(start).Round(time.Millisecond))
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
