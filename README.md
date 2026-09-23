# The Source

The Source is a small, API-first server over two corpora:

- **Books** — Project Gutenberg metadata and text, for browsing, fetching, and
  asking for a random book when your application has run out of ideas.
- **Lyrics** — a large song-lyrics collection, with the same shape of browse,
  search, individual records, random selection and random excerpts.

Both are equal features of the service, served side by side under
`/api/v1/books` and `/api/v1/lyrics` with parallel web UIs at `/books` and
`/lyrics`. Books are always present; lyrics are optional and switch on when a
corpus is configured.

The complete corpora stay on **The Lemon**. Development uses tiny, realistic
fixtures; this repository is not attempting to become a particularly inefficient
mirror of either the Library of Congress or the Billboard Hot 100.

## Principles

- Go server, standard library first.
- A JSON API is the primary product.
- A small server-rendered web UI is an API client and testing aid.
- No React, no SPA, no build step, and no front-end framework summoning circle.
- Metadata search only. Searching every word in every book is a separate
  indexing project, not a cheerful little query parameter.

## Run locally

Install Go 1.26 or newer, then:

```sh
go run .
# Open http://127.0.0.1:45068/books
```

Port **45068** is `0xB00C`: “book”, after a small hexadecimal spelling accident.
The default configuration binds to loopback. Set `server_addr = ":45068"` on
The Lemon to listen on all interfaces.

The full metadata catalogue is included, along with two **synthetic, clearly
labelled text fixtures** for IDs 1 and 1342. Select **Text installed** in the UI
to find them. These are plumbing tests, not suspiciously short editions.

## API

The home page at `/` links to the book catalogue, random tools and API docs.

For client authors and coding agents:

- [Interactive API explorer](/docs) — Swagger UI with Try it out, served at
  `/docs` on the running server. Vendored Swagger UI 5.33.0 is embedded in
  the binary, with no CDN, remote validator, Node build step or new Go dependency.
  Upstream licences and upgrade notes live in `internal/server/web/swagger/`.

- [OpenAPI 3.1 specification](docs/openapi.yaml) — the structured API contract,
  also served at `/openapi.yaml`.
- [Client guide](docs/API.md) — curl examples, defaults, errors, and limitations,
  also served at `/api-guide.md`.

```text
GET /health

GET /api/v1/books?language=en&q=frankenstein&limit=25&cursor=...
GET /api/v1/books/{id}
GET /api/v1/books/{id}/text
GET /api/v1/books/random?language=en
GET /api/v1/books/excerpts/random?paragraphs=3
GET /api/v1/books/languages

GET /api/v1/lyrics?q=concrete+jungle&tag=rap&limit=25&cursor=...
GET /api/v1/lyrics/{id}
GET /api/v1/lyrics/{id}/text
GET /api/v1/lyrics/random?tag=pop
GET /api/v1/lyrics/excerpts/random
GET /api/v1/lyrics/tags
GET /api/v1/lyrics/languages
```

## Command-line client

`cmd/thesource` is a small CLI over the same API, aimed at scripts and LLM
agents that would rather not hand-assemble query strings. It reads
`thesource.toml` (just `base_url = "http://lemon.com:45068/"`) from the
directory holding the binary, following symlinks.

```sh
go build -o ~/.local/bin/thesource ./cmd/thesource
cp cmd/thesource/thesource.toml ~/.local/bin/

thesource help                      # the complete reference, in one screen
thesource books search pride prejudice --limit 5
thesource books text 1342 --bytes 20000
thesource lyrics excerpt --tag rap
```

JSON goes to stdout; API errors go to stderr as their JSON and exit 1. Usage
mistakes exit 2, and a missing config or unreachable server exits 3.

English is the default browse language. The service will distinguish a missing
catalogue record from a catalogued book whose text is not installed locally.

Random selection also defaults to English. `GET /api/v1/books/random`
returns one book record, uniformly selected from installed text records.
Use `?language=fr` for another language or explicitly `?language=all` to
remove the filter. Empty language values are rejected. No matches returns
404 `no_matching_books`; responses are not cached. Repeats are possible:
randomness has no recollection of your previous literary disappointment.

The **Random book** page at `/books/random` calls this API and shows metadata,
JSON, read/download links, a permanent book link, and a language selector.
Language defaults apply to selection endpoints; fetching a specific ID still
retrieves that book regardless of language.

- `language=en` includes multilingual works containing English; use
  `language=all` to remove the language filter.
- `q` matches every whitespace-separated search word, case-insensitively,
  as a substring of title, authors, subjects or bookshelves. No content search.
- `available=true` selects installed texts; `available=false` selects missing
  ones. Omit it to browse both.
- Results are sorted by title, then ID. `limit` defaults to 25, maximum 100.
  Pass the returned `next_cursor` with the same filters for the next page.
  Cursors are numeric offsets into the filtered catalogue; a catalogue refresh
  can move entries between pages.
- Browse returns `{"books":[...],"total":123,"next_cursor":"25"}`.
  Book records include `languages` and `available`; detail returns one record.
- `GET /api/v1/books/languages` returns `{"languages":["en",...]}`.
- Errors use `{"error":{"code":"...","message":"..."}}`. Unknown IDs return
  404 `book_not_found`; absent text returns 404 `text_unavailable`.
- Text is served as UTF-8 plain text with streaming, HEAD and byte ranges.
  Corpus files must already be UTF-8; there is no automatic transcoding.
- Browsing includes records whose Gutenberg type is `Text`.
  Other record types remain accessible by ID.

```sh
curl 'http://127.0.0.1:45068/api/v1/books?q=austen&language=en'
curl 'http://127.0.0.1:45068/api/v1/books?available=true'
curl http://127.0.0.1:45068/api/v1/books/1342/text
```

Normal browser pages live at `/books`, `/books/{id}` and `/read/{id}`.
Search forms and pagination navigate between URLs, so bookmarks and the back
button work normally. The dark interface shows subjects and text availability,
and book details include the full API JSON. The reader fetches text in 64 KB
chunks, with a Load more button for longer works.

## Random excerpts

Both random endpoints accept inclusive original-publication year bounds:

```text
/api/v1/books/random?year_from=1901&year_to=1950
/api/v1/books/excerpts/random?paragraphs=3&year_from=1901
```

Use 1901 for “after 1900”, or 1900 for “1900 onwards”. Either bound may be
omitted; supplied years must be 1–9999 and from must not exceed to. English
remains the default, independently of year filters.

Years come only from the explicit `Original publication:` header field.
Missing, ambiguous or bracketed/uncertain years are unknown, and excluded
when either bound is supplied. No Gutenberg release-date or copyright-date
fallback is used. A known year appears as `original_publication_year` in
random-book, excerpt-source and individual-book responses.

Header reads are bounded to 64 KiB and indexed **before serving requests**.
The first startup builds a persistent index, logging progress and checkpointing
every 2,000 books. Restarts reuse known and unknown years for unchanged files;
only new or changed files (path, size or modification time) need header reads.
Year/language selection uses sorted in-memory pools and binary search, with no
request-time header reads or global lock. Excerpts still read their selected
books to find prose.

`year_index_path` defaults to `data/year-index.json` relative to the config.
Docker stores the index at `/srv/the-source/year-index.json` on The Lemon,
bind-mounted at `/var/lib/source` in the container. This survives container
rebuilds and removal. The development override uses a `source-state` named
volume instead. The book mount remains read-only.
Restart after corpus changes; remove only the index file if a forced rebuild
is needed. Both random UI pages provide optional year controls.

`GET /api/v1/books/excerpts/random?paragraphs=3` returns
`{"book":{...},"paragraphs":["...","...","..."]}`. Paragraph count defaults to
3 and must be 1–10. English is the default; `language=all` explicitly removes
the filter. The UI is at `/books/excerpts`.

The extractor requires Gutenberg START/END markers, removes the wrapper,
and heuristically rejects headings, contents, credits and copyright matter.
It chooses a consecutive run of complete prose paragraphs without joining
across rejected sections. Wrapped lines are joined; paragraph boundaries stay
intact. Raw book downloads are unchanged.

This is conservative prose detection, not perfect literary understanding:
short dialogue, poetry, uncased scripts, and some legitimate paragraphs may
be excluded, and unusual front matter can still resemble prose. Each candidate
paragraph needs at least 12 whitespace-separated words, sentence punctuation,
and predominantly lowercase letters. Selection is uniform among qualifying
windows within a sampled book, not across every paragraph in the library.

Each request tries at most eight installed books and scans at most 8 MiB per
book, retaining only a small sliding window. Missing markers, oversized input,
and books without enough suitable paragraphs are skipped. A failed sample
returns 422 `no_suitable_excerpt`; no installed language matches returns 404.
The local Austen fixture contains synthetic prose for testing this path.

## Lyrics

Lyrics are a full peer of books, not a bolt-on. The collection lives under
`/api/v1/lyrics`, backed by a prepared read-only SQLite database rather than the
book catalogue, and mirrors the book endpoints: browse with search and
pagination, individual records, plain-text bodies, random selection and random
excerpts.

```sh
curl 'http://127.0.0.1:45068/api/v1/lyrics?q=concrete+jungle'
curl 'http://127.0.0.1:45068/api/v1/lyrics?q=marley&field=artist'
curl 'http://127.0.0.1:45068/api/v1/lyrics?tag=rap&views_from=1000&limit=25'
curl http://127.0.0.1:45068/api/v1/lyrics/10/text
curl 'http://127.0.0.1:45068/api/v1/lyrics/random?tag=pop'
curl 'http://127.0.0.1:45068/api/v1/lyrics/excerpts/random'
```

Where lyrics differ from books:

- **No English default.** `language` and `tag` are optional, case-insensitive
  filters; omit them (or use `language=all`) to search everything.
- **Full-text search.** `q` runs SQLite FTS5 over title, artist and the lyrics
  body, so you can search by a line as well as by name, all in one box. Every
  whitespace-separated word is required (max 256 UTF-8 bytes); results are ranked
  by relevance, with the more-viewed song winning ties. Scope to one column with
  `field=title` or `field=artist`; omit `field` for the combined search.
- **Popularity filters.** Browse, random and excerpt endpoints accept
  `views_from`/`views_to` to bound by view count.
- **Bodies are omitted from listings.** Browse, item, random and excerpt
  responses stay light; fetch the full body (including `[Chorus]`-style markers)
  from `/lyrics/{id}/text`.
- **Excerpts return a stanza:** `{"song":{...},"lines":[...]}` — a run of
  consecutive non-blank lines. `/lyrics/tags` and `/lyrics/languages` enumerate
  the available filter values.

Song records carry `id`, `title`, `artist`, `views`, and the optional `tag`,
`language`, `year` and `features` (featured artists). The web UI lives at
`/lyrics` (landing), `/lyrics/browse`, `/lyrics/random` and `/lyrics/excerpts`,
with individual songs at `/lyrics/{id}`.

The corpus is optional. Leave `lyrics_db_path` empty to run a books-only server;
the lyrics endpoints then return 404 `lyrics_unavailable`. See
[the client guide](docs/API.md) for full details and
[docs/lyrics-ingest.md](docs/lyrics-ingest.md) for building and deploying the
database.

## Configuration and deployment

`source.dev.toml` is the default; use `-config` for another file:

```toml
catalog_path = "pg_catalog.csv"
books_dir = "testdata/books"
server_addr = "127.0.0.1:45068"
# Optional lyrics corpus; omit both for a books-only server.
lyrics_db_path = "testdata/lyrics.db"
lyrics_csv_path = "song_lyrics_en.csv"
```

Paths are relative to the config file. Unknown settings and duplicate TOML keys
are rejected. The only Go dependency is `github.com/pelletier/go-toml/v2`:
parsing TOML is already a solved problem, and we would like it to remain one.

```sh
go build -o the-source .
./the-source -config source.local.toml
```

Templates, JavaScript and CSS are embedded in the binary. Supply the catalog,
configuration and corpus separately. Set `books_dir` to The Lemon's
`cache/epub` directory, using its actual absolute path:

```toml
books_dir = "/path/to/gutenberg/cache/epub"
```

The mirror layout is `<books_dir>/<id>/pg<id>.txt`; for example,
`51177/pg51177.txt`. Local fixtures use the same structure. Flat
`<books_dir>/<id>.txt` files also work; the mirror copy takes precedence if
both exist. Indexing lists the corpus root and checks those exact candidate
paths without recursively walking the archive or opening book contents.
The corpus index is built from filenames at startup; restart after changing it
or replacing the metadata. Header parsing for new/changed books also happens
at startup; complete book contents are not loaded.

The service is read-only and has no authentication. Configure TLS/access
controls at your existing reverse proxy if exposing it outside your server.

## Docker

```sh
docker compose up --build -d
# Open http://localhost:45068/books
```

Compose automatically creates `/srv/the-source` for the writable year index
if it does not exist; no manual directory setup is needed.

The committed Compose file works on The Lemon without edits. It mounts
`/mnt/data/gutenberg` read-only at `/data/gutenberg` inside the container:

```text
/data/gutenberg/
├── pg_catalog.csv
└── cache/epub/<id>/pg<id>.txt
```

One mount supplies both metadata and books. The adjacent `txt-files.tar.zip`
is ignored; the service reads the extracted files. Neither books nor metadata
are baked into the image.

For development, explicitly enable the fixture override:

```sh
docker compose -f compose.yaml -f compose.dev.yaml up --build -d
```

This replaces the host library mount with two read-only fixture mounts: the
catalogue and the tiny book directory, at the same paths used in production.
It requires Docker Compose 2.24.4+ for the `!override` merge tag. The container
uses exactly the same configuration as production; no symlinks are required.
The override is named `compose.dev.yaml` so it cannot accidentally activate
on The Lemon.

The container runs as root (UID/GID 0:0), with all Linux capabilities dropped
and privilege escalation disabled. Its root filesystem and library mount stay
read-only; only the index directory is writable. Keep that directory owned by
root: dropping capabilities also removes root's normal permission bypass.
The supplied container config
listens on port 45068 and uses these fixed mount paths. To customise other
settings, bind-mount your TOML file read-only over `/etc/source.toml`.

## Development

```sh
go test ./...
go vet ./...
git diff --check
# With the service running and Chromium installed:
node scripts/browser-smoke.mjs
# Exercise Swagger UI's Try it out against the running service:
node scripts/swagger-smoke.mjs
```

See [AGENTS.md](AGENTS.md) for working conventions. Small tested commits go
straight to main. Wit is encouraged; quietly broken parsers are less welcome.

## Data

`pg_catalog.csv` is Project Gutenberg's metadata catalogue. It is deliberately
kept as CSV rather than imported into a database: it is only about 21 MB and
79,381 records, including 61,856 English text records in this snapshot.
The record count is lower than the line count because titles can span lines.
At startup, The Source parses it and prepares lookup and metadata-search data.

The Lemon's `cache/epub/<id>/pg<id>.txt` layout is supported directly.
Local tests use small text fixtures in the same layout.

### Lyrics corpus

The lyrics come from the **Genius Song Lyrics (with language information)**
dataset on Kaggle by Carlos GdCJ:

<https://www.kaggle.com/datasets/carlosgdcj/genius-song-lyrics-with-language-information>

Download `song_lyrics.csv` from there (a Kaggle account is required). Its columns
are `title, tag, artist, year, views, features, lyrics, id, language_cld3,
language_ft, language` — `cmd/lyricsprep` pins that exact header, so a
differently shaped export is rejected rather than imported into the wrong
columns. This deployment feeds it a single-language slice named
`song_lyrics_en.csv` (rows where `language` is `en`), but the full multilingual
file works unchanged.

Place the CSV at the configured `lyrics_csv_path`. On first start, when no
database exists yet, the server builds the read-only SQLite corpus from it
(see [docs/lyrics-ingest.md](docs/lyrics-ingest.md)); it is never read at request
time. Leave `lyrics_db_path` empty to run a books-only server.

## Status

Books: browse, metadata search, individual records, fixture text streaming, the
browser UI, The Lemon's mirror layout, and random installed-book selection with
excerpts. Lyrics: full-text search, browse with popularity filters, individual
records, plain-text bodies, random selection and random stanzas, over a prepared
SQLite corpus with its own web UI. The books remain patiently unaware; the songs
have opinions but keep them to themselves.
