#!/bin/sh
# Run from any directory. Frontend failure must prevent Go compilation.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
npm ci --prefix web --no-audit --no-fund
npm run build --prefix web
test -s internal/web/dist/index.html
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -o bin/movement ./cmd/movement
