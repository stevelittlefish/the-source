package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.toml")
	if err := os.WriteFile(path, []byte("catalog_path = \"catalog.csv\"\nbooks_dir = \"books\"\nserver_addr = \":9999\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CatalogPath != filepath.Join(dir, "catalog.csv") || got.BooksDir != filepath.Join(dir, "books") || got.ServerAddr != ":9999" {
		t.Fatalf("config = %#v", got)
	}
}

func TestRejectInvalidConfig(t *testing.T) {
	for _, body := range []string{
		"catalog_path = 'x'\nbooks_dir = 'b'\nserver_addr = ':1234'\ntypo = 'oops'",
		"catalog_path = 'x'\ncatalog_path = 'y'",
		"catalog_path = 'x'",
	} {
		path := filepath.Join(t.TempDir(), "source.toml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}
