CREATE TABLE IF NOT EXISTS job_queue (
    id              BIGSERIAL PRIMARY KEY,
    idempotency_key TEXT    NOT NULL UNIQUE,
    job_type        TEXT    NOT NULL,
    payload         TEXT    NOT NULL,
    status          TEXT    NOT NULL DEFAULT 'queued',
    priority        INTEGER NOT NULL DEFAULT 0,
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    lease_token     TEXT,
    leased_at       TIMESTAMP WITH TIME ZONE,
    heartbeat_at    TIMESTAMP WITH TIME ZONE,
    scheduled_at    TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL,
    error           TEXT
);
