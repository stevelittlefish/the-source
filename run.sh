#!/bin/sh
# Run the server from source. Uses source.local.toml when you have one (it is
# gitignored, for your own paths), otherwise the checked-in dev fixtures.
# Extra arguments go to the server, so `./run.sh -config other.toml` wins.
set -eu
cd "$(dirname "$0")"

config=source.dev.toml
[ -f source.local.toml ] && config=source.local.toml

echo "Using $config" >&2
exec go run . -config "$config" "$@"
