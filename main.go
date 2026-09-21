package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/stevelittlefish/the-source/internal/catalog"
	"github.com/stevelittlefish/the-source/internal/config"
	"github.com/stevelittlefish/the-source/internal/lyrics"
	"github.com/stevelittlefish/the-source/internal/server"
)

// ensureLyricsDB builds the lyrics database from the CSV when it does not yet
// exist, so a fresh deployment needs only the raw corpus. An existing database
// is left untouched (rebuild by deleting it). If neither the database nor a
// readable CSV is present, it fails loudly rather than serve no lyrics. The
// build goes to a temporary file renamed into place, so a crash mid-import
// never leaves a half-written database that would look complete next start.
func ensureLyricsDB(dbPath, csvPath string) error {
	if _, err := os.Stat(dbPath); err == nil {
		return nil // already built
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if csvPath == "" {
		return fmt.Errorf("no lyrics database at %s and no lyrics_csv_path to build one from", dbPath)
	}
	if _, err := os.Stat(csvPath); err != nil {
		return fmt.Errorf("no lyrics database at %s and its lyrics_csv_path is unreadable: %w", dbPath, err)
	}
	// Probe the target directory for writability before handing off to SQLite,
	// whose "unable to open database file (14)" hides the underlying reason. A
	// bare os error names it: read-only filesystem, permission denied (often a
	// bind mount owned by another uid, or NFS root_squash), out of space.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("lyrics directory %s is not usable: %w", dir, err)
	}
	probe := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("lyrics directory %s is not writable, so the database cannot be built there: %w; mount it read-write and make it writable by the container user", dir, err)
	}
	os.Remove(probe)
	log.Printf("no lyrics database at %s; building it from %s (one-time, several minutes)", dbPath, csvPath)
	tmp := dbPath + ".building"
	os.Remove(tmp) // clear any leftover from a previous aborted build
	if err := lyrics.Import(csvPath, tmp, false); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("build lyrics database: %w", err)
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("install lyrics database: %w", err)
	}
	return nil
}

func main() {
	path := flag.String("config", "source.dev.toml", "TOML configuration file")
	flag.Parse()
	c, err := config.Load(*path)
	if err != nil {
		log.Fatal(err)
	}
	books, err := catalog.LoadFile(c.CatalogPath)
	if err != nil {
		log.Fatal(err)
	}
	var songs *lyrics.Store
	if c.LyricsDBPath != "" {
		if err := ensureLyricsDB(c.LyricsDBPath, c.LyricsCSVPath); err != nil {
			log.Fatal(err)
		}
		songs, err = lyrics.Open(c.LyricsDBPath)
		if err != nil {
			log.Fatal(err)
		}
		defer songs.Close()
		log.Printf("lyrics corpus: %s", c.LyricsDBPath)
	}
	app, err := server.New(books, songs, c.BooksDir, c.YearIndexPath)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	srv := &http.Server{Addr: c.ServerAddr, Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
			srv.Close()
		}
		close(done)
	}()
	log.Printf("THE SOURCE — books on tap, plumbing included. Listening on %s", c.ServerAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
}
