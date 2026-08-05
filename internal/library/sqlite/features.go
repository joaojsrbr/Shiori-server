package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/joaojsr/shiori-server/internal/library"
)

func pageLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (r *Repository) ListMediaPage(ctx context.Context, limit int, cursor string) ([]*library.Media, string, error) {
	limit = pageLimit(limit)
	rows, err := r.db.QueryContext(ctx, `SELECT id, source_url, type, title, alternative_titles, description, cover_url, authors, artists, status, genres, created_at, updated_at FROM media WHERE (? = '' OR id < ?) ORDER BY id DESC LIMIT ?`, cursor, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]*library.Media, 0, limit)
	for rows.Next() {
		m, err := scanFeatureMedia(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, m)
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, rows.Err()
}

func (r *Repository) ListCollections(ctx context.Context) ([]*library.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,description,created_at,updated_at FROM collections ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*library.Collection{}
	for rows.Next() {
		c, err := scanSQLiteCollection(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *Repository) CreateCollection(ctx context.Context, name, description string) (*library.Collection, error) {
	c := &library.Collection{ID: library.NewULID(), Name: name, Description: description, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	_, err := r.db.ExecContext(ctx, `INSERT INTO collections(id,name,description,created_at,updated_at) VALUES(?,?,?,?,?)`, c.ID, c.Name, c.Description, c.CreatedAt, c.UpdatedAt)
	return c, err
}
func (r *Repository) GetCollection(ctx context.Context, id string) (*library.Collection, error) {
	c, err := scanSQLiteCollection(r.db.QueryRowContext(ctx, `SELECT id,name,description,created_at,updated_at FROM collections WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}
func (r *Repository) UpdateCollection(ctx context.Context, id, name, description string) (*library.Collection, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `UPDATE collections SET name=?,description=?,updated_at=? WHERE id=?`, name, description, now, id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	return r.GetCollection(ctx, id)
}
func (r *Repository) DeleteCollection(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id=?`, id)
	return err
}
func (r *Repository) AddCollectionMedia(ctx context.Context, cid, mid string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO collection_media(collection_id,media_id,added_at) VALUES(?,?,?) ON CONFLICT(collection_id,media_id) DO NOTHING`, cid, mid, time.Now().UTC())
	return err
}
func (r *Repository) RemoveCollectionMedia(ctx context.Context, cid, mid string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM collection_media WHERE collection_id=? AND media_id=?`, cid, mid)
	return err
}
func (r *Repository) ListCollectionMedia(ctx context.Context, cid string, limit int, cursor string) ([]*library.Media, string, error) {
	limit = pageLimit(limit)
	rows, err := r.db.QueryContext(ctx, `SELECT m.id,m.source_url,m.type,m.title,m.alternative_titles,m.description,m.cover_url,m.authors,m.artists,m.status,m.genres,m.created_at,m.updated_at FROM collection_media cm JOIN media m ON m.id=cm.media_id WHERE cm.collection_id=? AND (?='' OR m.id<?) ORDER BY m.id DESC LIMIT ?`, cid, cursor, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []*library.Media{}
	for rows.Next() {
		m, e := scanFeatureMedia(rows)
		if e != nil {
			return nil, "", e
		}
		items = append(items, m)
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, rows.Err()
}

func (r *Repository) ListHistory(ctx context.Context, limit int, cursor string) ([]*library.HistoryEntry, string, error) {
	limit = pageLimit(limit)
	rows, err := r.db.QueryContext(ctx, `SELECT h.chapter_id,c.media_id,m.title,c.number,h.position,h.progress,h.completed,h.updated_at FROM reading_history h JOIN chapters c ON c.id=h.chapter_id JOIN media m ON m.id=c.media_id WHERE (?='' OR h.chapter_id<?) ORDER BY h.updated_at DESC,h.chapter_id DESC LIMIT ?`, cursor, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []*library.HistoryEntry{}
	for rows.Next() {
		var x library.HistoryEntry
		var updated string
		if err := rows.Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.Position, &x.Progress, &x.Completed, &updated); err != nil {
			return nil, "", err
		}
		x.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		items = append(items, &x)
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ChapterID
		items = items[:limit]
	}
	return items, next, rows.Err()
}
func (r *Repository) UpsertHistory(ctx context.Context, chapterID string, position int, progress float64, completed bool) (*library.HistoryEntry, error) {
	_, err := r.db.ExecContext(ctx, `INSERT INTO reading_history(chapter_id,position,progress,completed,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(chapter_id) DO UPDATE SET position=excluded.position,progress=excluded.progress,completed=excluded.completed,updated_at=excluded.updated_at`, chapterID, position, progress, completed, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var x library.HistoryEntry
	var updated string
	err = r.db.QueryRowContext(ctx, `SELECT h.chapter_id,c.media_id,m.title,c.number,h.position,h.progress,h.completed,h.updated_at FROM reading_history h JOIN chapters c ON c.id=h.chapter_id JOIN media m ON m.id=c.media_id WHERE h.chapter_id=?`, chapterID).Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.Position, &x.Progress, &x.Completed, &updated)
	x.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &x, err
}
func (r *Repository) DeleteHistory(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM reading_history WHERE chapter_id=?`, id)
	return err
}

func (r *Repository) ListDownloads(ctx context.Context, limit int, cursor string) ([]*library.Download, string, error) {
	limit = pageLimit(limit)
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.media_id,m.title,c.number,COUNT(i.position),c.created_at FROM chapters c JOIN media m ON m.id=c.media_id JOIN chapter_images i ON i.chapter_id=c.id WHERE (?='' OR c.id<?) GROUP BY c.id,c.media_id,m.title,c.number,c.created_at ORDER BY c.id DESC LIMIT ?`, cursor, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := []*library.Download{}
	for rows.Next() {
		var x library.Download
		var created string
		if e := rows.Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.ImageCount, &created); e != nil {
			return nil, "", e
		}
		x.Status = "completed"
		x.CreatedAt, _ = time.Parse(time.RFC3339, created)
		items = append(items, &x)
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ChapterID
		items = items[:limit]
	}
	return items, next, rows.Err()
}
func (r *Repository) DeleteDownload(ctx context.Context, id string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT storage_key FROM chapter_images WHERE chapter_id=?`, id)
	if err != nil {
		return nil, err
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if e := rows.Scan(&key); e != nil {
			rows.Close()
			return nil, e
		}
		keys = append(keys, key)
	}
	rows.Close()
	if _, err = r.db.ExecContext(ctx, `DELETE FROM chapter_images WHERE chapter_id=?`, id); err != nil {
		return nil, err
	}
	return keys, nil
}

func scanSQLiteCollection(s rowScanner) (*library.Collection, error) {
	var c library.Collection
	var created, updated string
	if err := s.Scan(&c.ID, &c.Name, &c.Description, &created, &updated); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &c, nil
}
func scanFeatureMedia(s rowScanner) (*library.Media, error) {
	var m library.Media
	var alt, authors, artists, genres, created, updated string
	if err := s.Scan(&m.ID, &m.SourceURL, &m.Type, &m.Title, &alt, &m.Description, &m.CoverURL, &authors, &artists, &m.Status, &genres, &created, &updated); err != nil {
		return nil, err
	}
	for raw, dst := range map[string]*[]string{alt: &m.AlternativeTitles, authors: &m.Authors, artists: &m.Artists, genres: &m.Genres} {
		_ = json.Unmarshal([]byte(raw), dst)
		if *dst == nil {
			*dst = []string{}
		}
	}
	m.CreatedAt, _ = time.Parse(time.RFC3339, created)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &m, nil
}

func (r *Repository) ListProfiles(ctx context.Context) ([]*library.Profile, error) {
	return []*library.Profile{
		{
			ID:        "default",
			Name:      "Default Profile",
			AvatarURL: "",
		},
	}, nil
}

func (r *Repository) GetSettings(ctx context.Context) (*library.Settings, error) {
	row := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'app_settings'`)
	var val string
	err := row.Scan(&val)
	if err == sql.ErrNoRows {
		return &library.Settings{
			Theme:             "system",
			AdblockEnabled:    true,
			KeyboardShortcuts: map[string]any{},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	var s library.Settings
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return nil, err
	}
	if s.KeyboardShortcuts == nil {
		s.KeyboardShortcuts = map[string]any{}
	}
	return &s, nil
}

func (r *Repository) UpdateSettings(ctx context.Context, s library.Settings) (*library.Settings, error) {
	if s.Theme == "" {
		s.Theme = "system"
	}
	if s.KeyboardShortcuts == nil {
		s.KeyboardShortcuts = map[string]any{}
	}
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES ('app_settings', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, string(bytes))
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) ListBrowserHistory(ctx context.Context, limit int, cursor string) ([]*library.BrowserHistoryEntry, string, error) {
	limit = pageLimit(limit)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, url, title, visited_at
		FROM browser_history
		WHERE (? = '' OR id < ?)
		ORDER BY visited_at DESC, id DESC
		LIMIT ?
	`, cursor, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	items := []*library.BrowserHistoryEntry{}
	for rows.Next() {
		var item library.BrowserHistoryEntry
		var visited string
		if err := rows.Scan(&item.ID, &item.URL, &item.Title, &visited); err != nil {
			return nil, "", err
		}
		item.VisitedAt, _ = time.Parse(time.RFC3339, visited)
		items = append(items, &item)
	}

	next := ""
	if len(items) > limit {
		next = items[limit-1].ID
		items = items[:limit]
	}
	return items, next, rows.Err()
}

func (r *Repository) ListFilterPresets(ctx context.Context) ([]*library.FilterPreset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, filters FROM filter_presets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []*library.FilterPreset{}
	for rows.Next() {
		var item library.FilterPreset
		var filtersJSON string
		if err := rows.Scan(&item.ID, &item.Name, &filtersJSON); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(filtersJSON), &item.Filters)
		if item.Filters == nil {
			item.Filters = map[string]any{}
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateFilterPreset(ctx context.Context, name string, filters map[string]any) (*library.FilterPreset, error) {
	if filters == nil {
		filters = map[string]any{}
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	id := library.NewULID()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO filter_presets (id, name, filters, created_at)
		VALUES (?, ?, ?, ?)
	`, id, name, string(filtersJSON), now)
	if err != nil {
		return nil, err
	}

	return &library.FilterPreset{
		ID:      id,
		Name:    name,
		Filters: filters,
	}, nil
}
