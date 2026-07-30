CREATE TABLE IF NOT EXISTS media (
    id                 TEXT PRIMARY KEY,
    type               TEXT NOT NULL,
    title              TEXT NOT NULL,
    alternative_titles TEXT NOT NULL, -- JSON array of strings
    description        TEXT NOT NULL,
    cover_url          TEXT NOT NULL,
    authors            TEXT NOT NULL, -- JSON array of strings
    artists            TEXT NOT NULL, -- JSON array of strings
    status             TEXT NOT NULL,
    genres             TEXT NOT NULL, -- JSON array of strings
    created_at         TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at         TIMESTAMP WITH TIME ZONE NOT NULL
);
