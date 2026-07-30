# Registro de trabalho

Adicione uma entrada por branch mesclada, da mais recente para a mais antiga.

## Modelo

```markdown
## AAAA-MM-DD — tipo/slug

- Objetivo:
- Resultado:
- Arquivos principais:
- Contrato/migrations:
- Verificações:
- Decisões/ADRs:
- Próximo passo:
```

## 2026-07-30 — fix/graceful-shutdown-and-config

- Objetivo: corrigir panics de banco fechado e habilitar config customizada.
- Resultado: Shutdown gracioso com barreira `<-workerDone`, builds de debug/release no PowerShell e suporte a `.env` com `godotenv`.
- Arquivos principais: `worker.go`, `main.go`, `config.go`, `.env.example`.
- Decisões/ADRs: O banco só é fechado após o WaitGroup da Pool de workers zerar.

## 2026-07-30 — feature/tests-coverage

- Objetivo: pagar dívida técnica e eliminar arquivos `[no test files]`.
- Resultado: Cobertura global de `http`, repositórios (via `go-sqlmock`) e simulações do NuExtract Pipeline.
- Arquivos principais: `*_test.go` em `jobs/`, `library/`, `postgres/`, `sqlite/`, `sqlitequeue/`.
- Verificações: `go test ./...` 100% limpo sem pular instâncias no-CGO.

## 2026-07-30 — feature/worker-pool

- Objetivo: viabilizar extrações assíncronas duráveis.
- Resultado: Implementação do padão Worker Pool no Go, consumindo `sqlitequeue` com Graceful Shutdown e *Ack/Nack* nativo.
- Arquivos principais: `worker/worker.go`, `jobs/extract.go`.

## 2026-07-30 — feature/nuextract-pipeline

- Objetivo: plugar a IA no fluxo de obtenção de dados.
- Resultado: Pipeline Sincrono que varre DOM (`chromedp`), aplica heurísticas de Limpeza HTML, e envia para `LMStudio` formatado em JSON via Schema.
- Arquivos principais: `nuextract/provider.go`, `ai/http.go`.

## 2026-07-30 — chore/backend-foundation-in-progress

- Objetivo: estabelecer a fundação do backend portátil e seus contratos.
- Resultado: módulos iniciais de configuração, HTTP, SQLite, fila, storage e
  navegador presentes no workspace.
- Arquivos principais: `cmd/`, `internal/platform/`, `api/openapi/`,
  `migrations/` e `docs/adr/`.
- Contrato/migrations: OpenAPI e migration SQLite inicial presentes.
- Verificações: ainda precisam ser executadas antes do primeiro commit.
- Decisões/ADRs: ADRs 0001–0007 existentes.
- Próximo passo: revisar e criar baseline intencional em `main`.

