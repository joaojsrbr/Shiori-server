#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BACK_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
EXE_PATH="$BACK_DIR/dist/shiori-server.exe"
TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/shiori-smoke.XXXXXX")"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf -- "$TEMP_DIR"
}
trap cleanup EXIT INT TERM

fail() {
  echo "ERROR: $*" >&2
  if [[ -f "$TEMP_DIR/server.log" ]]; then
    echo '--- server.log ---' >&2
    tail -n 100 "$TEMP_DIR/server.log" >&2 || true
  fi
  exit 1
}

http_status() {
  curl -sS -o /dev/null -w '%{http_code}' "$@"
}

wait_until_ready() {
  local port="$1"
  local attempts=60
  while (( attempts > 0 )); do
    if curl -fsS "http://127.0.0.1:${port}/health/ready" >/dev/null 2>&1; then
      return 0
    fi
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
      fail 'Server exited before becoming ready.'
    fi
    sleep 1
    ((attempts--))
  done
  fail "Server did not become ready on port ${port}."
}

start_server() {
  local port="$1"
  local level="$2"
  (
    cd "$TEMP_DIR"
    exec ./shiori-server.exe serve --profile portable --port "$port" --log-level "$level" --log-format text
  ) >"$TEMP_DIR/server.log" 2>&1 &
  SERVER_PID=$!
  wait_until_ready "$port"
}

stop_server() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
}

[[ -f "$EXE_PATH" ]] || fail "Executable not found at $EXE_PATH. Run ./scripts/build.sh first."
command -v curl >/dev/null 2>&1 || fail 'curl is required.'

cp -- "$EXE_PATH" "$TEMP_DIR/shiori-server.exe"
echo "Isolated environment: $TEMP_DIR"

version_output="$("$TEMP_DIR/shiori-server.exe" version)"
echo "$version_output"
grep -q 'shiori-server' <<<"$version_output" || fail 'Version command failed.'

start_server 8080 info
curl -fsS http://127.0.0.1:8080/health/live | grep -q '"status":"alive"' || fail 'Liveness check failed.'
curl -fsS http://127.0.0.1:8080/health/ready | grep -q '"status":"ready"' || fail 'Readiness check failed.'
curl -fsS http://127.0.0.1:8080/api/v1/capabilities | grep -q '"profile":"portable"' || fail 'Portable capability check failed.'
[[ "$(http_status -X POST -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:8080/api/v1/debug/extract)" == '404' ]] || fail 'Debug endpoint must be disabled in info mode.'
[[ -f "$TEMP_DIR/.shiori/shiori.db" ]] || fail 'SQLite database was not created.'
stop_server

start_server 18080 debug
[[ "$(http_status -X POST -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:18080/api/v1/debug/extract)" == '400' ]] || fail 'Debug endpoint must return 400 for an invalid payload.'
stop_server

echo 'Smoke test PASSED.'

