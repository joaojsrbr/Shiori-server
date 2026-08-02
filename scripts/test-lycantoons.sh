#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${SHIORI_BASE_URL:-http://127.0.0.1:9180}"
SOURCE_URL="https://lycantoons.com/series/defensor-da-dungeon"
MAX_SECONDS="${SHIORI_TEST_TIMEOUT:-1800}"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

pretty_json() {
  if command -v jq >/dev/null 2>&1; then
    jq .
  else
    cat
  fi
}

command -v curl >/dev/null 2>&1 || fail 'curl is required.'

echo "Checking Shiori at $BASE_URL..."
health="$(curl -fsS "$BASE_URL/health/ready")" || fail 'Shiori is not ready. Start shiori-server first.'
echo "$health" | pretty_json

echo
echo "Enqueuing extraction: $SOURCE_URL"
payload="$(printf '{"url":"%s","target":"manga"}' "$SOURCE_URL")"
response="$(curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  --data "$payload" \
  "$BASE_URL/api/v1/jobs/extract")" || fail 'Could not enqueue extraction.'
echo "$response" | pretty_json

if command -v jq >/dev/null 2>&1; then
  job_id="$(printf '%s' "$response" | jq -r '.job_id // empty')"
else
  job_id="$(printf '%s' "$response" | sed -n 's/.*"job_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
fi
[[ -n "$job_id" ]] || fail 'The API response did not contain job_id.'

echo
echo "Following job $job_id (timeout: ${MAX_SECONDS}s)..."
event_log="$(mktemp "${TMPDIR:-/tmp}/shiori-lycantoons-events.XXXXXX")"
trap 'rm -f -- "$event_log"' EXIT INT TERM

# curl returns 28 when max-time is reached. Keep the captured SSE log so the
# script can distinguish an API error from a client-side timeout.
curl_status=0
curl -fsS -N --max-time "$MAX_SECONDS" \
  "$BASE_URL/api/v1/jobs/$job_id/events" | tee "$event_log" || curl_status=$?

if grep -q '^event: error' "$event_log"; then
  fail 'Extraction emitted an error event.'
fi
if ! grep -q '^event: done' "$event_log"; then
  if [[ "$curl_status" -eq 28 ]]; then
    fail "Extraction did not finish within ${MAX_SECONDS}s."
  fi
  fail "Extraction stream ended without a done event (curl status $curl_status)."
fi

media_id="$(sed -n 's/.*"media_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$event_log" | tail -n 1)"
echo
echo 'Extraction completed.'

if [[ -n "$media_id" ]]; then
  echo
  echo "Saved media ($media_id):"
  curl -fsS "$BASE_URL/api/v1/media/$media_id" | pretty_json

  echo
  echo 'Saved chapters and reader URLs:'
  curl -fsS "$BASE_URL/api/v1/media/$media_id/chapters" | pretty_json
else
  echo 'The done event did not contain media_id; listing the library instead.'
  curl -fsS "$BASE_URL/api/v1/media" | pretty_json
fi

