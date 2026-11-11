#!/usr/bin/env bash
# Usage: scripts/build-desktop.sh <server-url> [version]
#   -> dist/umbrella-desktop-<version>-{x86_64.AppImage,amd64.deb}   (Linux x64)
# Builds (or reuses dist/umbrella-linux-amd64) the Go client, bundles it into the Electron app.
set -euo pipefail
SERVER="${1:?usage: scripts/build-desktop.sh <server-url> [version]}"
VERSION="${2:-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/dist"
mkdir -p "$OUT" "$ROOT/desktop/bin"

if [ -x "$OUT/umbrella-linux-amd64" ]; then
  cp "$OUT/umbrella-linux-amd64" "$ROOT/desktop/bin/umbrella"
else
  (cd "$ROOT/client" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.DefaultServer=$SERVER -X main.Version=$VERSION" \
    -o "$ROOT/desktop/bin/umbrella" ./cmd/umbrella)
fi
chmod +x "$ROOT/desktop/bin/umbrella"

cd "$ROOT/desktop"
[ -d node_modules ] || npm ci
# "dev" is not a valid semver; the app version is also what the update check compares with.
npm version --no-git-tag-version --allow-same-version "$([ "$VERSION" = dev ] && echo 0.0.0-dev || echo "${VERSION#v}")" >/dev/null
npm run dist
cp dist/umbrella-desktop-* "$OUT/"
ls -l "$OUT"/umbrella-desktop-*
