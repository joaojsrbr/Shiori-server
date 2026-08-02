#!/usr/bin/env bash
set -u

BASE_URL="${SHIORI_BASE_URL:-http://127.0.0.1:8080}"
SOURCE_URL="${SHIORI_TEST_URL:-https://lycantoons.com/series/defensor-da-dungeon}"

command -v curl >/dev/null 2>&1 || { echo 'curl is required.' >&2; exit 1; }

section() {
  printf '\n=========================================\n%s\n' "$1"
}

pretty_json() {
  if command -v jq >/dev/null 2>&1; then jq .; else cat; fi
}

section '1. Health check (/health/ready)'
if ! curl -fsS "$BASE_URL/health/ready" | pretty_json; then
  echo 'Could not reach the server.' >&2
  exit 1
fi

section '2. Capabilities (/api/v1/capabilities)'
curl -fsS "$BASE_URL/api/v1/capabilities" | pretty_json || true

section '3. Enqueue manga extraction (/api/v1/jobs/extract)'
payload="$(printf '{"url":"%s","target":"manga"}' "$SOURCE_URL")"
curl -fsS -X POST -H 'Content-Type: application/json' -d "$payload" "$BASE_URL/api/v1/jobs/extract" | pretty_json || true

section '4. Library (/api/v1/media)'
curl -fsS "$BASE_URL/api/v1/media" | pretty_json || true

section '5. Debug extraction SSE (/api/v1/debug/extract)'
echo 'This endpoint is slow and is only registered with --log-level debug.'
curl -fsS -N --max-time 120 -X POST -H 'Content-Type: application/json' -d "$payload" "$BASE_URL/api/v1/debug/extract" || true

