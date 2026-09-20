# The Source

The Source is a small, API-first server that makes Project Gutenberg metadata
and text available for browsing, fetching, and eventually asking for a random
book when your application has run out of ideas.

The complete text corpus stays on **The Lemon**. Development uses a tiny,
realistic fixture corpus; this repository is not attempting to become a
particularly inefficient mirror of the Library of Congress.

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

```text
GET /health
GET /api/v1/books?language=en&q=frankenstein&limit=25&cursor=...
GET /api/v1/books/{id}
GET /api/v1/books/{id}/text
GET /api/v1/languages
```

English is the default browse language. The service will distinguish a missing
catalogue record from a catalogued book whose text is not installed locally.

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
- `GET /api/v1/languages` returns `{"languages":["en",...]}`.
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
button work normally. The reader embeds the streaming text endpoint.

## Configuration and deployment

`source.dev.toml` is the default; use `-config` for another file:

```toml
catalog_path = "pg_catalog.csv"
books_dir = "testdata/books"
server_addr = "127.0.0.1:45068"
```

Paths are relative to the config file. Unknown settings and duplicate TOML keys
are rejected. The only Go dependency is `github.com/pelletier/go-toml/v2`:
parsing TOML is already a solved problem, and we would like it to remain one.

```sh
go build -o the-source .
./the-source -config source.local.toml
```

Templates, JavaScript and CSS are embedded in the binary. Supply the catalog,
configuration and corpus separately. This release expects **flat**
`<books_dir>/<id>.txt` paths; it does not unpack or understand the unknown
archive format on The Lemon yet. Confirm that layout before production use.
The corpus index is built from filenames at startup; restart after changing it
or replacing the metadata. No book contents are read during indexing.

The service is read-only and has no authentication. Configure TLS/access
controls at your existing reverse proxy if exposing it outside your server.

## Development

```sh
go test ./...
go vet ./...
git diff --check
# With the service running and Chromium installed:
node scripts/browser-smoke.mjs
```

See [AGENTS.md](AGENTS.md) for working conventions. Small tested commits go
straight to main. Wit is encouraged; quietly broken parsers are less welcome.

## Data

`pg_catalog.csv` is Project Gutenberg's metadata catalogue. It is deliberately
kept as CSV rather than imported into a database: it is only about 21 MB and
79,381 records, including 61,856 English text records in this snapshot.
The record count is lower than the line count because titles can span lines.
At startup, The Source parses it and prepares lookup and metadata-search data.

Before deploying, inspect The Lemon's corpus layout and configure its
Gutenberg-ID-to-text-file resolver. Local tests use small text fixtures instead.

## Status

Browse, metadata search, individual records, fixture text streaming, and the
browser UI are implemented. Next: confirm The Lemon's archive format and add
random data fetching. The books remain patiently unaware.
