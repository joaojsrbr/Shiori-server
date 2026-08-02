# Registro de trabalho

Adicione uma entrada por branch mesclada, da mais recente para a mais antiga.

## 2026-08-02 — feature/delete-media

- Objetivo: oferecer exclusão completa de mangás e animes da biblioteca.
- Resultado: endpoint destrutivo remove a obra e dependências relacionais por cascata, recolhe e apaga imagens do storage e distingue obra inexistente com 404.
- Arquivos principais: `internal/library/http.go`, repositórios SQLite/PostgreSQL e `api/openapi/shiori.yaml`.
- Contrato/migrations: `DELETE /api/v1/media/{mediaId}`; sem migration, pois as foreign keys existentes já usam `ON DELETE CASCADE`.
- Verificações: testes de handler, deleção de storage e repositórios SQLite/PostgreSQL.
- Decisões/ADRs: sem novo ADR; a operação segue a arquitetura de storage existente.
- Próximo passo: apresentar confirmação destrutiva no cliente Flutter.

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

## 2026-08-01 — feature/intelligent-login-flow

- Objetivo: combinar detecção genérica de login com uma instrução explícita fornecida pelo frontend.
- Resultado: extração aceita `requires_login` + `login_url`, valida URLs HTTP(S), abre autenticação no perfil persistente da fonte, preserva sessão entre hosts de autenticação e reabre a URL original; heurísticas reconhecem inputs, formulários, botões, rotas e termos multilíngues.
- Arquivos principais: `internal/jobs/extract.go`, `internal/platform/browser/chromedp/provider.go`, `api/openapi/shiori.yaml`.
- Contrato/migrations: dois campos opcionais no pedido de `POST /api/v1/jobs/extract`; sem migration.
- Verificações: testes de validação HTTP, fluxo de login explícito, escopo do perfil, retomada da fonte e classificação genérica; suíte Go completa.
- Decisões/ADRs: ADR 0008 atualizado; credenciais e cookies continuam exclusivamente sob controle do Chromium.
- Próximo passo: o frontend deve oferecer controle de login opcional e enviar ambos os campos de forma atômica.

## 2026-08-01 — fix/fullscreen-challenge-handoff

- Objetivo: permitir interação precisa com desafios reais usando toda a área disponível da página de handoff.
- Resultado: canvas ocupa 100% do viewport útil, `ResizeObserver` envia dimensões limitadas ao backend, o adapter chromedp ajusta as métricas do navegador, perfis persistentes/serializados por hostname conservam sessões legítimas e login/redirecionamento retomam obrigatoriamente a URL original.
- Arquivos principais: `internal/jobs/challenge_handler.go`, `internal/platform/browser/browser.go`, `internal/platform/browser/chromedp/provider.go`, `internal/platform/browser/chromedp/profile_test.go`.
- Contrato/migrations: `ChallengeStatus` ganhou `kind: challenge|login`; mensagem WebSocket interna ganhou o tipo validado `viewport`; sem migration.
- Verificações: testes de limites do viewport, cliente responsivo, classificação login/challenge/block, retomada da URL original, chave estável por hostname, exclusão concorrente do perfil e integração com Chrome real comprovando redirecionamento de login, teclado remoto e cookie persistente entre sessões; suíte Go, build e smoke portátil.
- Decisões/ADRs: ADR 0008 atualizado; headers não podem converter uma regra Cloudflare `Block` em CAPTCHA e nenhuma evasão foi adicionada.
- Próximo passo: adicionar identidade explícita ao perfil antes de suportar instalações multiusuário e oferecer revogação/limpeza de sessão por domínio.

## 2026-08-01 — fix/debug-route-mount

- Objetivo: corrigir o panic do Chi ao iniciar o executável com `--log-level debug`.
- Resultado: rotas principais e opcionais são registradas em uma única montagem de `/api/v1`; o endpoint debug continua condicionado ao modo debug; o template NuExtract padrão foi embutido para o `.exe` funcionar sozinho.
- Arquivos principais: `cmd/api/main.go`, `cmd/api/main_test.go`, `internal/extraction/nuextract/provider.go`, `internal/extraction/nuextract/default_templates.json`.
- Contrato/migrations: sem alteração.
- Verificações: teste de regressão para rotas core + debug, suíte Go, build e inicialização real do executável em debug.
- Decisões/ADRs: sem nova decisão arquitetural.
- Próximo passo: manter novas rotas opcionais dentro do mesmo subrouter versionado.

## 2026-08-01 — feature/semantic-context-pipeline

- Objetivo: reduzir o contexto enviado ao NuExtract3 sem cortar estruturas úteis e simplificar o executável portátil.
- Resultado: metadados/JSON-LD preservados, URLs normalizadas, Markdown segmentado por blocos e títulos, orçamento baseado na janela real, prompt GGUF compatível com LM Studio, endpoint debug unificado em `http.go` e build Windows reduzido a um `.exe` de console.
- Arquivos principais: `internal/jobs/extract.go`, `internal/jobs/http.go`, `internal/extraction/nuextract/chunker.go`, `internal/extraction/nuextract/provider.go`, `scripts/build.sh`.
- Contrato/migrations: porta padrão e Docker alinhados em `8080`; nenhuma migration; endpoint debug continua fora do contrato público e só existe em log-level debug.
- Verificações: testes unitários do orçamento, UTF-8, breadcrumb, deduplicação, metadados, URLs e gate da rota; suíte Go completa; build e smoke portátil.
- Decisões/ADRs: ADR 0009.
- Próximo passo: medir extrações reais com Q4/Q5 e calibrar os limites por modelo se necessário.

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

