# ADR-005: Filas Duráveis

**Status:** Aceito  
**Data:** 2026-07-30

## Contexto

O Shiori usa jobs assíncronos para download, extração, OCR, tradução e
verificação de atualizações. A fila deve ser durável — jobs sobrevivem a
reinicializações — e funcionar nos dois perfis de infraestrutura.

## Decisão

Implementar uma interface `QueueProvider` com dois adapters:

### 1. SQLite (perfil `portable`)

Uma tabela `job_queue` no mesmo banco SQLite do aplicativo:

```sql
CREATE TABLE job_queue (
    id              INTEGER PRIMARY KEY,
    idempotency_key TEXT    NOT NULL UNIQUE,
    job_type        TEXT    NOT NULL,
    payload         TEXT    NOT NULL,  -- JSON
    status          TEXT    NOT NULL DEFAULT 'queued',
    priority        INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    lease_token     TEXT,
    leased_at       TEXT,
    heartbeat_at    TEXT,
    scheduled_at    TEXT    NOT NULL,
    created_at      TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL,
    error           TEXT
);
```

- **Polling:** goroutine periódica busca jobs `queued` ou com lease expirado.
- **Lease:** cada worker adquire um lease com token único e TTL.
- **Heartbeat:** worker atualiza `heartbeat_at` periodicamente para manter o
  lease.
- **Idempotência:** `idempotency_key` impede duplicação de jobs.
- **Backoff:** intervalo entre tentativas cresce exponencialmente com jitter.
- **Dead-letter:** após `max_attempts`, o job vai para status `failed` e fica
  disponível para inspeção.
- **Recuperação:** no startup, leases expirados são liberados.

### 2. Valkey (perfil `docker`)

`go-redis/v9` com compatibilidade de protocolo Valkey:

- **Fila:** Redis Streams (`XADD`/`XREADGROUP`) para entrega ordenada com
  consumer groups.
- **Acknowledgment:** `XACK` após processamento.
- **Pending:** `XPENDING` + `XCLAIM` para redelivery de mensagens paradas.
- **Idempotência:** chave de deduplicação com TTL.
- **Heartbeat:** extensão do tempo de pending via `XCLAIM` periódico.

### Interface comum

```go
type QueueProvider interface {
    Enqueue(ctx context.Context, job Job) error
    Dequeue(ctx context.Context, types []string) (*Job, error)
    Ack(ctx context.Context, jobID string) error
    Nack(ctx context.Context, jobID string, reason string) error
    Heartbeat(ctx context.Context, jobID string) error
    Cancel(ctx context.Context, jobID string) error
    Status(ctx context.Context, jobID string) (*JobStatus, error)
}
```

## Consequências

- O domínio e os handlers não sabem qual backend de fila está ativo.
- No portátil, a fila está limitada à concorrência do SQLite (serialização
  de escrita com WAL). Isso é aceitável para uso pessoal.
- No Docker, Valkey oferece throughput superior e pub/sub para notificações
  em tempo real.
- Testes da fila SQLite devem cobrir: crash, reinício, expiração de lease,
  idempotência e dead-letter.
- Testes da fila Valkey devem cobrir: consumer groups, redelivery e XACK.
