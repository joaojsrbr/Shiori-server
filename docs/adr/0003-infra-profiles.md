# ADR-003: Perfis de Infraestrutura

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O Shiori deve funcionar em dois modos distintos:

1. **Portátil (portable):** executável único Windows, sem dependências externas
   além de um navegador Chrome/Edge opcional.
2. **Docker:** containers com PostgreSQL, Valkey, MinIO e Playwright.

Cada capacidade de infraestrutura (banco, fila, storage, navegador) precisa
de uma implementação diferente por perfil, mas o domínio e os handlers não
devem saber qual perfil está ativo.

## Decisão

Implementar **dois perfis explícitos** atrás de **interfaces comuns**, sem
detecção automática e sem fallback silencioso.

### Mapeamento de perfis

| Capacidade | Perfil `portable` | Perfil `docker` |
|---|---|---|
| Banco | SQLite via `modernc.org/sqlite` v1.54+ | PostgreSQL via `pgx/v5` |
| Fila | SQLite durável (tabela `job_queue`) | Valkey via `go-redis/v9` |
| Storage | Filesystem local relativo | MinIO via `minio-go/v7` |
| Navegador | chromedp + Chrome/Edge local | Worker Playwright (container) |
| IA | LM Studio externo (HTTP) | LM Studio externo (HTTP) |

### Seleção do perfil

```
--profile portable   (padrão no build Windows)
--profile docker
SHIORI_PROFILE=portable|docker
```

### Interfaces de porta

```go
// Exemplos conceituais — nomes finais definidos na implementação

type MediaRepository interface { ... }
type QueueProvider interface { ... }
type StorageProvider interface { ... }
type BrowserProvider interface { ... }
```

Cada interface terá um adapter por perfil em pacotes separados:

```
internal/platform/database/sqlite/    → MediaRepository SQLite
internal/platform/database/postgres/  → MediaRepository PostgreSQL
internal/platform/queue/sqlitequeue/   → QueueProvider SQLite
internal/platform/queue/valkeyqueue/   → QueueProvider Valkey
internal/platform/storage/localfs/     → StorageProvider filesystem
internal/platform/storage/s3/          → StorageProvider MinIO/S3
```

### Regras

- O perfil é escolhido **uma vez** na inicialização.
- Nunca detectar infraestrutura automaticamente.
- Nunca fazer fallback de PostgreSQL para SQLite após falha.
- Não espalhar condicionais de perfil pelo domínio ou handlers.
- A "fábrica" de adapters fica em `internal/platform/` e é chamada pelo
  `cmd/api/main.go`.

### Dependências por perfil

| Biblioteca | Versão | Perfil |
|---|---|---|
| `modernc.org/sqlite` | v1.54.0 | portable |
| `github.com/jackc/pgx/v5` | v5.10.x | docker |
| `github.com/redis/go-redis/v9` | v9.x | docker |
| `github.com/minio/minio-go/v7` | v7.x | docker |
| `github.com/chromedp/chromedp` | v0.16+ | portable |

O binário portátil **compila com todas as dependências** mas só inicializa os
adapters do perfil selecionado. Build tags poderão ser avaliadas no futuro para
reduzir o tamanho do binário, mas não são necessárias na primeira iteração.

## Consequências

- O domínio é testável com mocks/fakes das interfaces.
- Testes de repositório rodam com SQLite E PostgreSQL real, validando
  equivalência de comportamento.
- O executável portátil funciona sem Docker, PostgreSQL, Valkey ou MinIO.
- O Docker Compose não precisa do adapter SQLite.
- Adicionar um novo perfil (ex: cloud-managed) requer apenas novos adapters.
