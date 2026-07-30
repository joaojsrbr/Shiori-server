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

