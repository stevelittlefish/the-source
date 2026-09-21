# Ingesting the lyrics corpus on The Lemon

The lyrics API is served from a prepared, read-only SQLite database. The raw
`song_lyrics_en.csv` (~5.8 GB) is **never** read at request time and never
loaded into the running server — `cmd/lyricsprep` turns it into `lyrics.db`
once, offline, and the server only queries the result.

The production container is `FROM scratch`: no shell, no Go toolchain, read-only
root filesystem. **You cannot build the database inside it.** Build it on the
host and mount it in.

## 1. Build the `lyricsprep` binary

It is pure Go (`modernc.org/sqlite`, no cgo), so it cross-compiles to a static
Linux binary from anywhere:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o lyricsprep ./cmd/lyricsprep
```

Copy `lyricsprep` to The Lemon, or run it directly there if the repo and Go are
present (`go run ./cmd/lyricsprep ...`).

## 2. Check disk

The database is larger than the CSV — the lyrics are stored as text *and*
indexed for full-text search. Budget roughly **1.4× the CSV size** (a 5.8 GB CSV
produced a ~7.5 GB database in testing). Ensure the target filesystem has room:

```sh
df -h /mnt/data/lyrics
```

## 3. Build the database (to a temp path, then swap)

`lyricsprep` refuses to overwrite an existing output, and the server opens the
database `immutable`. Build to a temporary name beside the final file and rename
it into place so a running server never sees a half-written file:

```sh
cd /mnt/data/lyrics
lyricsprep -in song_lyrics_en.csv -out lyrics.db.new
mv -f lyrics.db.new lyrics.db          # atomic within the same filesystem
```

Import is two phases: a fast streaming insert (a few million rows in under a
minute) and a slower one-time full-text index build. Progress is logged. Bad
rows (unparseable, empty title/lyrics, non-numeric id) are skipped and counted;
duplicate ids are dropped, keeping the first.

## 4. Wire it into the server

The container reads `source.docker.toml`, which expects the database at
`/data/lyrics/lyrics.db`. `compose.yaml` bind-mounts the host directory
read-only:

```yaml
- type: bind
  source: /mnt/data/lyrics      # host: where lyrics.db was built
  target: /data/lyrics          # container: matches lyrics_db_path
  read_only: true
```

The server opens the database at startup, so **restart to pick up a new build**:

```sh
docker compose up -d            # or: docker compose restart source
```

Startup logs `lyrics corpus: /data/lyrics/lyrics.db`. A missing or unreadable
file is fatal by design — fail loud rather than silently serve no lyrics. To run
a **books-only** server, leave `lyrics_db_path` empty in the config instead; the
lyrics endpoints then return `404 lyrics_unavailable`.

## 5. Verify

```sh
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/tags'
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics?limit=1'   # check "total"
curl --fail-with-body 'http://lemon:45068/api/v1/lyrics/excerpts/random'
```

## Rebuilding later

Re-run steps 3–4 with the new CSV. The rename swap means the old database keeps
serving until the instant you `mv`; the restart then switches over. Keep the
previous `lyrics.db` until the new one is verified if you want a quick rollback.
