CREATE TABLE collections (
    id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE collection_media (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    added_at TIMESTAMPTZ NOT NULL, PRIMARY KEY (collection_id, media_id)
);
CREATE TABLE reading_history (
    chapter_id TEXT PRIMARY KEY REFERENCES chapters(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0, progress DOUBLE PRECISION NOT NULL DEFAULT 0,
    completed BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_collection_media_added ON collection_media(collection_id, added_at DESC, media_id DESC);
CREATE INDEX idx_history_updated ON reading_history(updated_at DESC, chapter_id DESC);
