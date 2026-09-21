# Ingesting the lyrics corpus on The Lemon

The lyrics API is served from a prepared, read-only SQLite database. The raw
`song_lyrics_en.csv` (~5.8 GB) is **never** read at request time and never
loaded into the running server — it is turned into `lyrics.db` once and the
server only queries the result.

## The short version

Drop the CSV where the config points and start the server. If the database is
missing, the server builds it from the CSV on first boot (one-time, several
minutes; progress is logged), then serves it. If the database is present it is
used as-is. If neither the database nor a readable CSV is there, startup fails
loudly rather than serve no lyrics.

The two paths come from the config:

```toml
lyrics_db_path  = "/data/lyrics/lyrics.db"           # queried if present, else built here
lyrics_csv_path = "/data/lyrics/song_lyrics_en.csv"  # source for that one-time build
```

So a fresh deployment is: put `song_lyrics_en.csv` in `/mnt/data/lyrics/`
(bind-mounted to `/data/lyrics`), `docker compose up -d`, and wait out the first
build. `compose.yaml` mounts that directory **writable** so the server can write
the database beside the CSV. Budget disk for both files: the database is roughly
**1.4× the CSV** (a 5.8 GB CSV produced a ~9 GB database), on top of the CSV
itself. Leave `lyrics_csv_path` empty (or unset) with no database to run a
**books-only** server — the lyrics endpoints then return `404 lyrics_unavailable`.

To **rebuild** later (new CSV, or schema change), delete `lyrics.db` and restart;
the next boot rebuilds from the CSV. Keep the old file until the new one is
verified if you want a quick rollback.

## Building ahead of time (optional)

To avoid a slow first boot — or to prepare the database on a different machine —
build it with `cmd/lyricsprep` and ship the result. It is pure Go
(`modernc.org/sqlite`, no cgo), so it cross-compiles to a static Linux binary:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o lyricsprep ./cmd/lyricsprep
```

`lyricsprep` refuses to overwrite an existing output, so build to a temporary
name and rename it into place — the server opens the database `immutable` and a
rename is atomic within one filesystem:

```sh
cd /mnt/data/lyrics
lyricsprep -in song_lyrics_en.csv -out lyrics.db.new
mv -f lyrics.db.new lyrics.db
```

This produces the identical database the server would build itself (same schema,
same indexes) — the server and the tool share one importer. With the database
already in place, first boot skips straight to serving.

## Verify

```sh
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/tags'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics?limit=1'   # check "total"
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/excerpts/random'
```

Startup logs `lyrics corpus: /data/lyrics/lyrics.db` once the database is ready.

## Notes

- Import is two phases: a fast streaming insert (a few million rows in under a
  minute) and a slower one-time full-text index build. Bad rows (unparseable,
  empty title/lyrics, non-numeric id) are skipped and counted; duplicate ids are
  dropped, keeping the first; `tag=misc` rows (poems, scripts, essays) are
  dropped as non-songs.
- The server builds to `lyrics.db.building` and renames on success, so a crash
  mid-import never leaves a half-written database that would look complete on the
  next start.
