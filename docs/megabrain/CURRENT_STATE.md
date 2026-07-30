# Estado atual

Atualizado em: 2026-07-30

## Repositório

- Branch padrão: `main`.
- Repositório Git independente de `front`.
- A fundação do backend está 100% implementada, testada e mesclada na `main`.
- Cobertura de testes com `go-sqlmock`, chamadas HTTP e simulação do Worker Pool.
- Variáveis configuráveis via `.env` suportadas nativamente.

## Fundação existente

- Module path: `github.com/joaojsr/shiori-server`.
- Go declarado em `go.mod`: `1.26.5`.
- HTTP: Chi.
- Configuração, logging, build info e servidor HTTP possuem implementação.
- OpenAPI: `api/openapi/shiori.yaml`.
- SQLite e migrations iniciais estão em `internal/platform/database/sqlite/`
  e `migrations/sqlite/`.
- Fila SQLite está em `internal/platform/queue/sqlitequeue/`.
- Filesystem local está em `internal/platform/storage/localfs/`.
- Browser provider e adapter chromedp estão em
  `internal/platform/browser/`.
- ADRs iniciais estão em `docs/adr/`.

## Artefatos locais

- `dist/shiori-server-debug.exe` e `dist/shiori-server-release.exe` gerados e ignorados.
- Dados portáteis, SQLite, storage, backups e logs não podem entrar no Git.

