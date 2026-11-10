#!/usr/bin/env bash
# Starts a local wrangler dev server with a throwaway state dir, runs the Go e2e test, cleans up.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${UMBRELLA_E2E_PORT:-8791}"
TMP="$(mktemp -d)"
LOG="$TMP/wrangler.log"
PID=""
cleanup() {
  if [ -n "$PID" ]; then
    kill "$PID" 2>/dev/null || true
    pkill -P "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  pkill -f "wrangler.*--port $PORT" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

cd "$ROOT/server"
[ -d node_modules ] || npm ci --silent
export CLOUDFLARE_API_TOKEN="${CLOUDFLARE_API_TOKEN:-proxy-injected}"
npx wrangler dev --local --port "$PORT" --persist-to "$TMP/state" --var ADMIN_TOKEN:dev >"$LOG" 2>&1 &
PID=$!

for _ in $(seq 1 120); do
  if curl -s -o /dev/null "http://localhost:$PORT/"; then break; fi
  if ! kill -0 "$PID" 2>/dev/null; then echo "wrangler exited early:"; cat "$LOG"; exit 1; fi
  sleep 1
done
curl -s -o /dev/null "http://localhost:$PORT/" || { echo "server did not start:"; cat "$LOG"; exit 1; }

cd "$ROOT/client"
export GOTOOLCHAIN=local
UMBRELLA_E2E_SERVER="http://localhost:$PORT" UMBRELLA_E2E_ADMIN_TOKEN=dev \
  go test -count=1 -tags e2e -v ./e2e_test/
