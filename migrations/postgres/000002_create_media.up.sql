CREATE TABLE IF NOT EXISTS media (
    id                 TEXT PRIMARY KEY,
    type               TEXT NOT NULL,
    title              TEXT NOT NULL,
    alternative_titles JSONB NOT NULL DEFAULT '[]'::jsonb,
    description        TEXT NOT NULL,
    cover_url          TEXT NOT NULL,
    authors            JSONB NOT NULL DEFAULT '[]'::jsonb,
    artists            JSONB NOT NULL DEFAULT '[]'::jsonb,
    status             TEXT NOT NULL,
    genres             JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL
);
