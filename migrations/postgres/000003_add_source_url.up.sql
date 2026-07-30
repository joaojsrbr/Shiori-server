ALTER TABLE media ADD COLUMN source_url TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_media_source_url ON media(source_url);
