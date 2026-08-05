#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BACK_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$BACK_DIR"

LATEST_TAG=$(git tag -l "v*" | sort -V | tail -n 1)

if [[ -z "$LATEST_TAG" ]]; then
  NEW_TAG="v1.0.0"
else
  VERSION_NO_V="${LATEST_TAG#v}"
  IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION_NO_V"
  MAJOR=${MAJOR:-1}
  MINOR=${MINOR:-0}
  PATCH=${PATCH:-0}
  NEW_PATCH=$((PATCH + 1))
  NEW_TAG="v${MAJOR}.${MINOR}.${NEW_PATCH}"
fi

printf 'Última tag encontrada: %s\n' "${LATEST_TAG:-Nenhuma}"
printf 'Criando nova tag: %s\n' "$NEW_TAG"

git tag -a "$NEW_TAG" -m "Release $NEW_TAG"
printf 'Tag %s criada com sucesso localmente.\n' "$NEW_TAG"

if [[ "${1:-}" == "--push" ]] || [[ "${AUTO_PUSH:-}" == "true" ]]; then
  printf 'Enviando %s para origin...\n' "$NEW_TAG"
  git push origin "$NEW_TAG"
  printf 'Tag %s enviada para o GitHub!\n' "$NEW_TAG"
else
  printf '\nPara enviar a tag para o GitHub e disparar o autobuild de release, execute:\n'
  printf '  git push origin %s\n\n' "$NEW_TAG"
fi
