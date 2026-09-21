package lyrics

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// expected is the header of the Genius export. We pin it so a differently
// shaped file fails loudly instead of importing nonsense into the wrong columns.
var expected = []string{"title", "tag", "artist", "year", "views", "features", "lyrics", "id", "language_cld3", "language_ft", "language"}

// Import turns the raw Genius lyrics CSV at in into a read-only SQLite database
// at out, which must not already exist. It streams the source so a multi-GB file
// never lands in memory at once, logging progress as it goes. Bad rows
// (unparseable, empty title/lyrics, non-numeric id) are skipped and counted;
// duplicate ids are dropped, keeping the first. tag=misc rows (poems, scripts,
// essays) are dropped unless includeMisc is set.
func Import(in, out string, includeMisc bool) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()

	// Make sure the destination directory exists, and steer SQLite's temporary
	// files (the full-text index build spills gigabytes of them) into it. The
	// production container runs on a read-only root filesystem with no /tmp, so
	// the default temp locations are unwritable; the output directory is the one
	// place we know is writable, since we are about to write the database there.
	dir := filepath.Dir(out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if os.Getenv("SQLITE_TMPDIR") == "" {
		os.Setenv("SQLITE_TMPDIR", dir)
	}

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

	writer, err := NewWriter(out)
	if err != nil {
		return err
	}

	start := time.Now()
	row, kept, skipped, dupes, misc := 1, 0, 0, 0, 0
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
		// Genius files everything non-musical under "misc": poems, scripts,
		// essays, book chapters, open letters. This is a song server, so drop
		// them by default (mirrors the book catalogue keeping only Type=Text).
		tag := strings.ToLower(strings.TrimSpace(record[index["tag"]]))
		if !includeMisc && tag == "misc" {
			misc++
			continue
		}
		year, _ := strconv.Atoi(strings.TrimSpace(record[index["year"]]))
		views, _ := strconv.Atoi(strings.TrimSpace(record[index["views"]]))
		song := Song{
			ID:       id,
			Title:    title,
			Artist:   strings.TrimSpace(record[index["artist"]]),
			Tag:      tag,
			Language: strings.ToLower(strings.TrimSpace(record[index["language"]])),
			Year:     CleanYear(year),
			Views:    views,
			Features: ParsePGArray(record[index["features"]]),
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
	log.Printf("done: %d songs imported, %d skipped, %d duplicate ids, %d misc dropped; %s total",
		kept, skipped, dupes, misc, time.Since(start).Round(time.Second))
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
