# The Source: client guide

Machine-readable contract: [openapi.yaml](openapi.yaml).
Interactive documentation with live requests is available at `/docs`.
The running server serves it at `/openapi.yaml`, and this guide at
`/api-guide.md`. The spec is OpenAPI 3.1 written in JSON syntax (valid YAML
1.2), so agents can also parse it with a standard JSON parser.

Default base URL: `http://lemon:45068` (or your server hostname).
All operations use GET; HEAD is also supported with no response body.
No authentication, request bodies, SDK, or special headers are required.
There is no cross-origin browser CORS setup; the bundled UI uses the same origin.

## Essential rules

- Selection defaults to **English**. Use `language=fr` for French or explicitly
  `language=all` to remove the filter. An empty value is invalid. Multilingual
  records containing the chosen language qualify.
- Fetching a specific book ID is language-independent. Language discovery
  lists all catalog languages, including ones without installed text.
- Random book and excerpt endpoints select installed Text records only.
  Random-book responses contain metadata; fetch `/books/{id}/text` for content.
- Random responses have `Cache-Control: no-store`. Repeats are normal.
- Browse and random endpoints reject unknown or repeated query parameters.
  Omit optional parameters instead of sending empty values.
- Use error `code`, not human-readable `message`, for client branching.

## Requests

Run these against your chosen host:

```sh
curl --fail-with-body 'http://lemon:45068/health'
curl --fail-with-body 'http://lemon:45068/api/v1/books/languages'
curl --fail-with-body 'http://lemon:45068/api/v1/books?language=en&q=austen&limit=25'
curl --fail-with-body 'http://lemon:45068/api/v1/books?available=true'
curl --fail-with-body 'http://lemon:45068/api/v1/books/1342'
curl --fail-with-body -o pg1342.txt 'http://lemon:45068/api/v1/books/1342/text'
curl --fail-with-body -H 'Range: bytes=0-65535' 'http://lemon:45068/api/v1/books/1342/text'
curl --fail-with-body 'http://lemon:45068/api/v1/books/random?year_from=1901&year_to=1950'
curl --fail-with-body 'http://lemon:45068/api/v1/books/excerpts/random?paragraphs=3&year_from=1901'
curl --fail-with-body 'http://lemon:45068/api/v1/books/excerpts/random?paragraphs=1&language=all'
```

The local corpus contains synthetic fixtures. Results depend on the installed
corpus; these commands do not promise a particular randomly chosen book.

## Response shapes

Metadata fields: `id` (integer), `type`, `issued`, `title`, `authors`
(display strings), and `languages`, `subjects`, `locc`, `bookshelves`
(arrays of strings).

Browse returns `{"books":[...],"total":123,"next_cursor":"25"}`.
Book detail and random-book endpoints return a single book object.
These objects include `available` (boolean).

Excerpts return `{"book":{...},"paragraphs":["First paragraph.","Second paragraph."]}`.
The nested source book has the metadata fields but **no available field**.
The number of paragraphs equals the requested count (default 3; range 1–10).
The example above illustrates shape, not actual eligible prose.

`original_publication_year` is an optional integer in individual-book,
random-book and excerpt-source records. It is omitted when unknown and
is not populated by browsing. Do not treat an omitted year as zero or infer
it from `issued`: that is the Gutenberg release date.

## Browsing and pagination

`q` matches every whitespace-separated word, case-insensitively, as a
substring in title, authors, subjects or bookshelves. Maximum 256 UTF-8 bytes.
There is no content search, stemming or relevance ranking.

Results sort by case-insensitive title, then numeric ID. `limit` defaults
to 25 (1–100). Pass `next_cursor` back as `cursor` with the **same filters
and limit** until it is empty. Cursors are nonnegative offsets, not snapshots:
catalog refreshes may move entries. Empty searches return 200 with an empty
books array, not 404.

`available=true` selects installed text, `available=false` missing text,
and omission both. The index is built at startup; restart after corpus changes.

## Publication years

Both random endpoints accept inclusive `year_from` and `year_to`, each
1–9999. Either can be omitted; from must not exceed to.
“After 1900” means `year_from=1901`.

Only the explicit `Original publication:` header is used. The parser expects
one unambiguous four-digit year; uncertain/bracketed and multiple years are
unknown. Unknown years are excluded when either bound is active. No copyright
or Gutenberg release-date fallback exists.

Header extraction reads at most 64 KiB per book during startup, before the
HTTP listener opens. Known and unknown years persist across restarts in the
configured year index. Unchanged files reuse cached metadata; new/changed files
are parsed once. Language/year selection uses sorted in-memory indexes and
binary search with no request-time header reads. The first installation may
take time to build its index; progress is logged. Docker preserves it in the
`/srv/the-source` host directory (the dev override uses a named volume).
Excerpt extraction still reads selected books.

## Excerpt quality and bounded retries

An excerpt is consecutive qualifying prose paragraphs, not a word-count slice.
Gutenberg START/END markers are required. Wrapper text is excluded, and
heuristics reject headings, contents lists, credits and copyright matter.
Wrapped lines are joined with spaces; paragraphs remain separate.

Each paragraph must have at least 12 whitespace-separated words, sentence
punctuation and predominantly lowercase letters. This may omit short dialogue,
poetry and uncased scripts. Unusual front matter can still resemble prose.
Rejected sections break runs; we do not stitch across them.

The server tries at most eight shuffled matching books, with an 8 MiB scan
limit per book and a 64 KiB limit on lines/paragraphs. It keeps a small sliding
window rather than loading complete books. Selection is uniform among eligible
windows inside the successful book, not all paragraphs across the library.
Raw text downloads remain unmodified and include the original wrapper.

If extraction returns 422, retry a bounded number of times or request fewer
paragraphs. Do not create an infinite loop in honour of infinite literature.

## Lyrics

The lyrics corpus is a separate collection under `/api/v1/lyrics`, backed by a
prepared read-only SQLite database rather than the book catalogue. It is
optional: a server started without a lyrics database answers every lyrics
endpoint with 404 `lyrics_unavailable`.

```sh
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/tags'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/languages'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics?tag=rap&limit=25'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics?q=concrete+jungle'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/10'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/10/text'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/random?tag=pop'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/excerpts/random'
```

Differences from books worth noting:

- **No English default.** `language` and `tag` are optional filters; omitting
  them (or `language=all`) searches everything. Both are case-insensitive.
- **Full-text search.** `q` runs SQLite FTS5 over title, artist and the lyrics
  body — so you can search by a line as well as by name (title OR artist OR
  text, in one box). Every whitespace-separated word is required; maximum 256
  UTF-8 bytes. Results are ranked by relevance — a title match outweighs an
  artist match, which outweighs a body match, and the more-viewed song wins
  ties — so the obvious hit surfaces first. Empty `q` falls back to plain
  browsing in `id` order. Page with `next_cursor` as `cursor`.
- **Lyrics bodies are omitted** from listings, item metadata, random and excerpt
  responses to keep them light. Fetch the body from `/lyrics/{id}/text`, which
  returns plain text (including `[Chorus]`-style markers) with no byte-range or
  conditional-request support.
- **Random** ranges over the whole table independent of storage order (a
  precomputed indexed bucket, not a full scan); responses carry
  `Cache-Control: no-store`. Both `/lyrics/random` and `/lyrics/excerpts/random`
  accept `views_from` and `views_to` to bound popularity (each a non-negative
  integer, `views_from ≤ views_to`; omit either side for open-ended), alongside
  `language` and `tag`.
- **Excerpts return a stanza:** `{"song":{...},"lines":[...]}`. A stanza is a
  run of consecutive non-blank lines between blank lines; bracketed section
  markers are dropped and a stanza has at least two lines. On 422
  `no_suitable_excerpt`, retry a bounded number of times.

Song objects carry `id`, `title`, `artist`, `views`, and the optional
`tag`, `language`, `year` (implausible values are discarded) and `features`
(featured artists). The corpus is built offline by `cmd/lyricsprep` from the
source CSV; see the working notes for how to rebuild it.

## Errors

Application errors use:

```json
{"error":{"code":"no_matching_books","message":"No installed texts match these filters."}}
```

| HTTP | Code | Client action |
| --- | --- | --- |
| 400 | invalid_query | Fix parameters; do not retry unchanged. |
| 404 | book_not_found | Check the Gutenberg ID. |
| 404 | text_unavailable | Metadata exists, but text is absent/unavailable. |
| 404 | no_matching_books | Broaden language/year filters or install texts. |
| 404 | not_found | Fix the API path. |
| 405 | method_not_allowed | Use GET or HEAD. |
| 422 | no_suitable_excerpt | Retry a few times or reduce paragraph count. |
| 500 | text_error | Server could not open text; inspect server logs. |
| 404 | song_not_found | Check the song ID. |
| 404 | no_matching_songs | Broaden the language/tag filters. |
| 404 | lyrics_unavailable | No lyrics corpus is installed on this server. |
| 500 | lyrics_error | Server could not query the lyrics database; inspect logs. |

The text endpoint additionally uses ordinary HTTP semantics: 206 for ranges,
304 for unchanged content, 412 for failed preconditions, and **plain-text
416** errors for bad/unsatisfiable ranges. Check status and Content-Type before
decoding JSON. HEAD never has a body. Multi-range responses may be multipart.

## Keeping clients and docs aligned

Treat `docs/openapi.yaml` as the API contract. This guide explains operational
details. When changing endpoints, update both documents and API tests in the
same commit. HTML routes (`/books`, `/books/browse`, `/books/random`,
`/books/excerpts`, `/read/{id}`, `/lyrics`) are human interfaces; clients
should use `/api/v1/...`.
