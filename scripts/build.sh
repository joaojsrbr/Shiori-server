#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BACK_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$BACK_DIR"

VERSION="${SHIORI_VERSION:-1.0.0}"
COMMIT="unknown"
DIRTY="false"
BUILD_DATE="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  COMMIT="$(git rev-parse HEAD)"
  if [[ -n "$(git status --porcelain)" ]]; then
    DIRTY="true"
  fi
fi

printf 'Version: %s\nCommit: %s\nDirty: %s\nDate: %s\n' "$VERSION" "$COMMIT" "$DIRTY" "$BUILD_DATE"

PACKAGE_PATH="github.com/joaojsr/shiori-server/internal/buildinfo"
LD_FLAGS="-X ${PACKAGE_PATH}.Version=${VERSION} -X ${PACKAGE_PATH}.Commit=${COMMIT} -X ${PACKAGE_PATH}.BuildDate=${BUILD_DATE} -X ${PACKAGE_PATH}.Dirty=${DIRTY} -s -w"

mkdir -p dist
rm -f -- dist/shiori-server-debug.exe dist/shiori-server-release.exe

echo 'Building dist/shiori-server.exe (windows/amd64)...'
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -trimpath -buildvcs=false -ldflags="$LD_FLAGS" -o dist/shiori-server.exe ./cmd/api

echo 'Build finished: dist/shiori-server.exe'

