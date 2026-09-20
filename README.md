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

## Planned API

```text
GET /health
GET /api/v1/books?language=en&q=frankenstein&limit=25&cursor=...
GET /api/v1/books/{id}
GET /api/v1/books/{id}/text
GET /api/v1/books/random?language=en
GET /api/v1/languages
```

English is the default browse language. The service will distinguish a missing
catalogue record from a catalogued book whose text is not installed locally.

## Data

`pg_catalog.csv` is Project Gutenberg's metadata catalogue. It is deliberately
kept as CSV rather than imported into a database: it is only about 21 MB and
roughly 90,000 records, well within reach of Go and a sensible amount of RAM.
At startup, The Source will parse it and build the small indexes it needs.

Before deploying, inspect The Lemon's corpus layout and configure its
Gutenberg-ID-to-text-file resolver. Local tests use small text fixtures instead.

## Status

The foundation is being built. The books remain patiently unaware.
