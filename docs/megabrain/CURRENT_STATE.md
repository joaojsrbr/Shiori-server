# Estado atual

Atualizado em: 2026-08-02

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
- O provider chromedp mantém uma sessão ativa entre `Navigate` e `Snapshot`,
  captura o DOM já renderizado por JavaScript, URL final e assets, e sinaliza
  desafios Cloudflare visíveis para interação humana. Scripts Cloudflare em
  páginas normais não geram falso positivo, e desafios gerenciados recebem até
  10 segundos para concluir automaticamente antes do estado de ação humana.
- Desafios interativos usam handoff temporário por screencast/WebSocket com
  TTL, controlador único, validação de origem e de input, cancelamento e
  verificação do DOM pelo backend antes da retomada. A página sincroniza toda a
  área útil com o viewport remoto para manter frame e coordenadas alinhados
  (ADR 0008). No portátil single-user, perfis Chromium persistentes e
  serializados por hostname mantêm cookies legítimos sem expô-los ao backend.
  Formulários/redirecionamentos de login usam o mesmo handoff e a URL original
  é reaberta depois da autenticação antes de qualquer extração. Pedidos de
  extração também aceitam `requires_login` + `login_url` para fontes com login
  não convencional; a autenticação continua no perfil da fonte mesmo quando o
  provedor de identidade usa outro host.
- ADRs iniciais estão em `docs/adr/`.
- A entrada do NuExtract3 usa metadados + Markdown, orçamento conservador da
  janela e chunks semânticos com limite rígido (ADR 0009).
- O template NuExtract padrão é embutido no executável; o arquivo em `config/`
  funciona como override opcional.
- `/api/v1/debug/extract` reutiliza o mesmo pipeline do worker e só é registrado
  quando `--log-level debug` está ativo.
- `DELETE /api/v1/media/{mediaId}` remove a obra e, por cascata, capítulos,
  histórico e associações com coleções; as chaves das imagens são coletadas
  antes da transação e removidas do storage após o commit.

## Artefatos locais

- `dist/shiori-server.exe` é o único artefato Windows, usa o subsistema console
  e permanece ignorado.
- Dados portáteis, SQLite, storage, backups e logs não podem entrar no Git.

