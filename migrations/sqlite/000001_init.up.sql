CREATE TABLE job_queue (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
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

CREATE INDEX idx_job_queue_status_scheduled ON job_queue(status, scheduled_at);
CREATE INDEX idx_job_queue_leased_at ON job_queue(leased_at) WHERE lease_token IS NOT NULL;
