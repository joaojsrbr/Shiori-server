CREATE TABLE collections (
    id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE collection_media (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    added_at TEXT NOT NULL, PRIMARY KEY (collection_id, media_id)
);
CREATE TABLE reading_history (
    chapter_id TEXT PRIMARY KEY REFERENCES chapters(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0, progress REAL NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL
);
CREATE INDEX idx_collection_media_added ON collection_media(collection_id, added_at DESC, media_id DESC);
CREATE INDEX idx_history_updated ON reading_history(updated_at DESC, chapter_id DESC);
