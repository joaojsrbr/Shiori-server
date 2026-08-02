CREATE TABLE IF NOT EXISTS chapters (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    number TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL,
    video_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE(media_id, source_url)
);

CREATE TABLE IF NOT EXISTS chapter_images (
    chapter_id TEXT NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    storage_key TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(chapter_id, position)
);

CREATE INDEX IF NOT EXISTS idx_chapters_media_id ON chapters(media_id);
