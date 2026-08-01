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

## 2026-08-01 — feature/human-challenge-handoff

- Objetivo: transformar o protótipo de challenge em um handoff humano seguro, temporário e verificável.
- Resultado: estado com TTL/cancelamento, controlador WebSocket único e same-origin, screencast com ponteiro/scroll/teclado, inputs limitados, headers de segurança, token redigido dos logs e conclusão condicionada à nova verificação do DOM.
- Arquivos principais: `internal/platform/browser/challenge.go`, `internal/jobs/challenge_handler.go`, `internal/platform/browser/chromedp/provider.go`, `api/openapi/shiori.yaml`.
- Contrato/migrations: endpoints de status, conclusão, cancelamento e WebSocket documentados no OpenAPI; sem migration.
- Verificações: testes de lifecycle/expiração/cancelamento, validação de input, rejeição cross-origin, verificação antes da retomada; `go vet ./...`; `go test ./...`; smoke test portátil.
- Decisões/ADRs: ADR 0008; nenhum bypass ou resolução automática de CAPTCHA.
- Próximo passo: implementar cookies persistentes criptografados por usuário/domínio e o adapter Playwright equivalente no Docker.

## 2026-07-30 — fix/cloudflare-challenge-detection

- Objetivo: evitar `409 User Action Required` em páginas normais que apenas carregam scripts Cloudflare.
- Resultado: a detecção passou a considerar título, texto renderizado e widgets visíveis, sem pesquisar marcadores dentro de scripts; desafios gerenciados recebem uma janela de 10 segundos para concluir antes do `409`.
- Arquivos principais: `internal/platform/browser/chromedp/provider.go`, `internal/platform/browser/chromedp/browser_test.go`.
- Contrato/migrations: sem alteração.
- Verificações: testes unitários de classificação; testes de integração com Chrome para DOM normal e desafio automático; `go vet ./...`; `go test ./...`.
- Decisões/ADRs: mantém ADR 0006; CAPTCHA ou desafio interativo persistente continua exigindo handoff humano, sem evasão.
- Próximo passo: implementar API de handoff/retomada com janela visível para desafios realmente interativos.

## 2026-07-30 — fix/browser-real-snapshot

- Objetivo: substituir o HTML provisório enviado ao NuExtract por uma captura real da página.
- Resultado: a sessão chromedp permanece ativa após a navegação e `Snapshot` retorna o DOM renderizado por JavaScript, URL final, assets e indicação de desafio Cloudflare.
- Arquivos principais: `internal/platform/browser/chromedp/provider.go`, `internal/platform/browser/chromedp/browser_test.go`.
- Contrato/migrations: sem alteração.
- Verificações: teste de integração com Chrome local e página `httptest` com mutação de DOM por JavaScript; `go vet ./...`; `go test ./...`.
- Decisões/ADRs: mantém ADR 0006; desafios Cloudflare são apenas detectados e exigem ação humana, sem evasão automática.
- Próximo passo: implementar captura do status e headers HTTP reais via eventos CDP.

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

