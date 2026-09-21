// Package config loads the service's TOML settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	YearIndexPath string `toml:"year_index_path"`
	CatalogPath   string `toml:"catalog_path"`
	BooksDir      string `toml:"books_dir"`
	LyricsDBPath  string `toml:"lyrics_db_path"`
	LyricsCSVPath string `toml:"lyrics_csv_path"`
	ServerAddr    string `toml:"server_addr"`
}

// Relative paths follow the configuration file. Changing directory should not
// relocate the library.
func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	if err := toml.NewDecoder(f).DisallowUnknownFields().Decode(&c); err != nil {
		return c, fmt.Errorf("decode config: %w", err)
	}
	if strings.TrimSpace(c.CatalogPath) == "" || strings.TrimSpace(c.BooksDir) == "" || strings.TrimSpace(c.ServerAddr) == "" {
		return c, fmt.Errorf("config requires catalog_path, books_dir, and server_addr")
	}
	if !filepath.IsAbs(c.CatalogPath) {
		c.CatalogPath = filepath.Join(filepath.Dir(path), c.CatalogPath)
	}
	if !filepath.IsAbs(c.BooksDir) {
		c.BooksDir = filepath.Join(filepath.Dir(path), c.BooksDir)
	}
	if c.YearIndexPath == "" {
		c.YearIndexPath = "data/year-index.json"
	}
	if !filepath.IsAbs(c.YearIndexPath) {
		c.YearIndexPath = filepath.Join(filepath.Dir(path), c.YearIndexPath)
	}
	// Lyrics are optional: an empty path simply runs a books-only server.
	if c.LyricsDBPath != "" && !filepath.IsAbs(c.LyricsDBPath) {
		c.LyricsDBPath = filepath.Join(filepath.Dir(path), c.LyricsDBPath)
	}
	// lyrics_csv_path is the raw corpus the server builds the database from when
	// lyrics_db_path is missing. Optional; resolved like the other paths.
	if c.LyricsCSVPath != "" && !filepath.IsAbs(c.LyricsCSVPath) {
		c.LyricsCSVPath = filepath.Join(filepath.Dir(path), c.LyricsCSVPath)
	}
	return c, nil
}
