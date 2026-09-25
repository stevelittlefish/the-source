#!/bin/sh
# Build the command-line tools into ./bin. thesource reads thesource.toml from
# its own directory, so a default one is copied alongside it the first time;
# after that it is yours to edit and this script leaves it alone.
set -eu
cd "$(dirname "$0")"

mkdir -p bin
go build -o bin/thesource ./cmd/thesource
go build -o bin/lyricsprep ./cmd/lyricsprep
[ -f bin/thesource.toml ] || cp cmd/thesource/thesource.toml bin/

echo "Built into $(pwd)/bin:"
ls -1 bin
