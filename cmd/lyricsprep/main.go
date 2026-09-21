// Command lyricsprep turns the raw Genius lyrics CSV into a read-only SQLite
// database the server can query. It runs offline, where the data lives (The
// Lemon), and streams the source so a 5.8 GB file never lands in memory at once.
//
//	lyricsprep -in song_lyrics_en.csv -out lyrics.db
//
// The server builds the same database itself on first start when it is missing,
// so this command is only needed to prepare one ahead of time.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/stevelittlefish/the-source/internal/lyrics"
)

func main() {
	in := flag.String("in", "", "source lyrics CSV")
	out := flag.String("out", "", "destination SQLite database (must not exist)")
	includeMisc := flag.Bool("include-misc", false, "keep tag=misc rows (poems, scripts, essays, book chapters); excluded by default as non-songs")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("usage: lyricsprep -in song_lyrics_en.csv -out lyrics.db")
	}
	if _, err := os.Stat(*out); err == nil {
		log.Fatalf("%s already exists; remove it to rebuild", *out)
	}
	if err := lyrics.Import(*in, *out, *includeMisc); err != nil {
		os.Remove(*out)
		log.Fatal(err)
	}
}
