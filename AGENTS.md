# Working on The Source

## Purpose

The Source is an API-first Project Gutenberg catalogue and text server. It gives
programs (and the occasional curious human) a dependable supply of books and,
eventually, random data. The UI is a small test client, not an ambitious new
religion called Frontend.

## Language and shape

1. **Go is the server language.** Prefer the standard library. A dependency must
   have a specific job that is harder or less safe to do ourselves.
2. **No JavaScript on the server.** Browser JavaScript is allowed in small,
   page-specific files. It calls the public API; it does not grow a framework.
3. **No SPA and no front-end framework.** Every human page has its own URL and
   is server-rendered with `html/template`.
4. **Plain data and functions beat elaborate object hierarchies.** A book is a
   struct, not a behavioural lifestyle.
5. **The API is first class.** The web UI must exercise public endpoints rather
   than secret server-only shortcuts.

## Data and configuration

1. `pg_catalog.csv` is the canonical Gutenberg metadata input. Parse it with
   Go's CSV reader: it contains quoted and multi-line fields.
2. The huge text corpus lives on The Lemon, not in this repository. Checked-in
   fixtures model its layout for tests and local development.
3. Runtime configuration lives in TOML, not an expanding colony of environment
   variables. Secrets do not belong in committed configuration.
4. Stream book text from disk; never casually load _War and Peace_ into memory
   to prove that RAM exists.

## Engineering habits

1. Work directly on `main`; make small, coherent commits and push them once
   tested. There are no branches to admire from a safe distance.
2. Run `go test ./...`, `go vet ./...`, and `git diff --check` before commits
   that affect Go code.
3. Add tests for API behaviour and edge cases alongside implementation.
4. Keep error responses JSON under `/api/`; ordinary browser pages may use a
   simple HTML error page.
5. Comments and commits may be witty, but must still tell the next unfortunate
   reader something useful.
