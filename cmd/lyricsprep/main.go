// Command lyricsprep turns the raw Genius lyrics CSV into a read-only SQLite
// database the server can query. It runs offline, where the data lives (The
// Lemon), and streams the source so a 5.8 GB file never lands in memory at once.
//
//	lyricsprep -in song_lyrics_en.csv -out lyrics.db
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stevelittlefish/the-source/internal/lyrics"
)

// expected is the header of the Genius export. We pin it so a differently
// shaped file fails loudly instead of importing nonsense into the wrong columns.
var expected = []string{"title", "tag", "artist", "year", "views", "features", "lyrics", "id", "language_cld3", "language_ft", "language"}

func main() {
	in := flag.String("in", "", "source lyrics CSV")
	out := flag.String("out", "", "destination SQLite database (must not exist)")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("usage: lyricsprep -in song_lyrics_en.csv -out lyrics.db")
	}
	if _, err := os.Stat(*out); err == nil {
		log.Fatalf("%s already exists; remove it to rebuild", *out)
	}
	if err := run(*in, *out); err != nil {
		os.Remove(*out)
		log.Fatal(err)
	}
}

func run(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.ReuseRecord = true
	reader.FieldsPerRecord = len(expected)
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if !sameFields(header, expected) {
		return fmt.Errorf("unexpected CSV header: %v", header)
	}
	index := columnIndex(expected)

	writer, err := lyrics.NewWriter(out)
	if err != nil {
		return err
	}

	start := time.Now()
	row, kept, skipped, dupes := 1, 0, 0, 0
	seen := make(map[int]struct{})
	for {
		row++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Genius data is not pristine; log and skip a broken row rather
			// than abandon several million good ones over one bad quote.
			skipped++
			log.Printf("row %d: skipped (%v)", row, err)
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(record[index["id"]]))
		if err != nil || id < 1 {
			skipped++
			continue
		}
		if _, ok := seen[id]; ok {
			dupes++
			continue
		}
		seen[id] = struct{}{}
		title := strings.TrimSpace(record[index["title"]])
		lyric := record[index["lyrics"]]
		if title == "" || strings.TrimSpace(lyric) == "" {
			skipped++
			continue
		}
		year, _ := strconv.Atoi(strings.TrimSpace(record[index["year"]]))
		views, _ := strconv.Atoi(strings.TrimSpace(record[index["views"]]))
		song := lyrics.Song{
			ID:       id,
			Title:    title,
			Artist:   strings.TrimSpace(record[index["artist"]]),
			Tag:      strings.TrimSpace(record[index["tag"]]),
			Language: strings.ToLower(strings.TrimSpace(record[index["language"]])),
			Year:     lyrics.CleanYear(year),
			Views:    views,
			Features: lyrics.ParsePGArray(record[index["features"]]),
			Lyrics:   lyric,
		}
		if err := writer.Add(song); err != nil {
			return fmt.Errorf("row %d: insert: %w", row, err)
		}
		kept++
		if kept%100000 == 0 {
			log.Printf("%d songs imported (%d skipped, %d dupes); %s elapsed",
				kept, skipped, dupes, time.Since(start).Round(time.Second))
		}
	}
	log.Printf("finishing: %d songs; building full-text index...", kept)
	if err := writer.Finish(); err != nil {
		return err
	}
	log.Printf("done: %d songs imported, %d skipped, %d duplicate ids; %s total",
		kept, skipped, dupes, time.Since(start).Round(time.Second))
	return nil
}

func columnIndex(header []string) map[string]int {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[name] = i
	}
	return index
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
