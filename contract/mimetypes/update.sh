#!/usr/bin/env bash
# Writes types_gen.go from mime-db's db.json at the version named here (MIT,
# https://github.com/jshttp/mime-db). Raise the version and run it again to take a newer list.
set -euo pipefail
version=1.54.0
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsS --max-time 60 -o "$work/db.json" "https://cdn.jsdelivr.net/npm/mime-db@$version/db.json"
(cd "$here/.." && go run ./mimetypes/gen "$work/db.json" "$version" "$here/types_gen.go")
