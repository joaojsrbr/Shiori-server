# Arquitetura vigente

## Perfis

Portable:

- um único `shiori-server.exe`;
- SQLite em `data/shiori.db`;
- fila durável SQLite;
- filesystem local;
- chromedp com Chrome/Edge instalado.

Docker:

- PostgreSQL;
- Valkey;
- MinIO;
- worker Playwright separado.

## Dependências

```text
transport -> application -> domain
infrastructure -----------^
```

O domínio não importa adapters. Banco, fila, storage, browser e IA são portas
substituíveis.

## Contratos e decisões

- OpenAPI: `api/openapi/shiori.yaml`.
- ADRs: `docs/adr/`.
- Migrations SQLite e PostgreSQL ficam separadas por dialeto.
- Comportamento observável dos repositórios deve ser equivalente entre perfis.
- NuExtract é acessado por adapter LM Studio e saída validada por schema.

