#!/usr/bin/env bash
# Usage: scripts/build.sh <server-url> [version]   -> dist/umbrella-{darwin,linux}-{amd64,arm64}, dist/umbrella-windows-amd64.exe
set -euo pipefail
SERVER="${1:?usage: scripts/build.sh <server-url>}"
VERSION="${2:-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/dist"
mkdir -p "$OUT"
cd "$ROOT/client"
export CGO_ENABLED=0
LDFLAGS="-s -w -X main.DefaultServer=$SERVER -X main.Version=$VERSION"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  os="${target%/*}"; arch="${target#*/}"
  ext=""; [ "$os" = "windows" ] && ext=".exe"
  echo "building $os/$arch"
  GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" -o "$OUT/umbrella-$os-$arch$ext" ./cmd/umbrella
done
ls -l "$OUT"
