package postgres

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
	rows, err := r.db.QueryContext(ctx, `SELECT id,source_url,type,title,alternative_titles,description,cover_url,authors,artists,status,genres,created_at,updated_at FROM media WHERE ($1='' OR id<$1) ORDER BY id DESC LIMIT $2`, cursor, limit+1)
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
func (r *Repository) ListCollections(ctx context.Context) ([]*library.Collection, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,description,created_at,updated_at FROM collections ORDER BY updated_at DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []*library.Collection{}
	for rows.Next() {
		c, e := scanPGCollection(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (r *Repository) CreateCollection(ctx context.Context, name, description string) (*library.Collection, error) {
	now := time.Now().UTC()
	c := &library.Collection{ID: library.NewULID(), Name: name, Description: description, CreatedAt: now, UpdatedAt: now}
	_, err := r.db.ExecContext(ctx, `INSERT INTO collections(id,name,description,created_at,updated_at) VALUES($1,$2,$3,$4,$5)`, c.ID, c.Name, c.Description, c.CreatedAt, c.UpdatedAt)
	return c, err
}
func (r *Repository) GetCollection(ctx context.Context, id string) (*library.Collection, error) {
	c, e := scanPGCollection(r.db.QueryRowContext(ctx, `SELECT id,name,description,created_at,updated_at FROM collections WHERE id=$1`, id))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return c, e
}
func (r *Repository) UpdateCollection(ctx context.Context, id, name, description string) (*library.Collection, error) {
	res, e := r.db.ExecContext(ctx, `UPDATE collections SET name=$1,description=$2,updated_at=$3 WHERE id=$4`, name, description, time.Now().UTC(), id)
	if e != nil {
		return nil, e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	return r.GetCollection(ctx, id)
}
func (r *Repository) DeleteCollection(ctx context.Context, id string) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM collections WHERE id=$1`, id)
	return e
}
func (r *Repository) AddCollectionMedia(ctx context.Context, cid, mid string) error {
	_, e := r.db.ExecContext(ctx, `INSERT INTO collection_media(collection_id,media_id,added_at) VALUES($1,$2,$3) ON CONFLICT(collection_id,media_id) DO NOTHING`, cid, mid, time.Now().UTC())
	return e
}
func (r *Repository) RemoveCollectionMedia(ctx context.Context, cid, mid string) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM collection_media WHERE collection_id=$1 AND media_id=$2`, cid, mid)
	return e
}
func (r *Repository) ListCollectionMedia(ctx context.Context, cid string, limit int, cursor string) ([]*library.Media, string, error) {
	limit = pageLimit(limit)
	rows, e := r.db.QueryContext(ctx, `SELECT m.id,m.source_url,m.type,m.title,m.alternative_titles,m.description,m.cover_url,m.authors,m.artists,m.status,m.genres,m.created_at,m.updated_at FROM collection_media cm JOIN media m ON m.id=cm.media_id WHERE cm.collection_id=$1 AND ($2='' OR m.id<$2) ORDER BY m.id DESC LIMIT $3`, cid, cursor, limit+1)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	items := []*library.Media{}
	for rows.Next() {
		m, x := scanFeatureMedia(rows)
		if x != nil {
			return nil, "", x
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
	rows, e := r.db.QueryContext(ctx, `SELECT h.chapter_id,c.media_id,m.title,c.number,h.position,h.progress,h.completed,h.updated_at FROM reading_history h JOIN chapters c ON c.id=h.chapter_id JOIN media m ON m.id=c.media_id WHERE ($1='' OR h.chapter_id<$1) ORDER BY h.updated_at DESC,h.chapter_id DESC LIMIT $2`, cursor, limit+1)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	items := []*library.HistoryEntry{}
	for rows.Next() {
		var x library.HistoryEntry
		if e := rows.Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.Position, &x.Progress, &x.Completed, &x.UpdatedAt); e != nil {
			return nil, "", e
		}
		items = append(items, &x)
	}
	next := ""
	if len(items) > limit {
		next = items[limit-1].ChapterID
		items = items[:limit]
	}
	return items, next, rows.Err()
}
func (r *Repository) UpsertHistory(ctx context.Context, id string, position int, progress float64, completed bool) (*library.HistoryEntry, error) {
	_, e := r.db.ExecContext(ctx, `INSERT INTO reading_history(chapter_id,position,progress,completed,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(chapter_id) DO UPDATE SET position=EXCLUDED.position,progress=EXCLUDED.progress,completed=EXCLUDED.completed,updated_at=EXCLUDED.updated_at`, id, position, progress, completed, time.Now().UTC())
	if e != nil {
		return nil, e
	}
	var x library.HistoryEntry
	e = r.db.QueryRowContext(ctx, `SELECT h.chapter_id,c.media_id,m.title,c.number,h.position,h.progress,h.completed,h.updated_at FROM reading_history h JOIN chapters c ON c.id=h.chapter_id JOIN media m ON m.id=c.media_id WHERE h.chapter_id=$1`, id).Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.Position, &x.Progress, &x.Completed, &x.UpdatedAt)
	return &x, e
}
func (r *Repository) DeleteHistory(ctx context.Context, id string) error {
	_, e := r.db.ExecContext(ctx, `DELETE FROM reading_history WHERE chapter_id=$1`, id)
	return e
}
func (r *Repository) ListDownloads(ctx context.Context, limit int, cursor string) ([]*library.Download, string, error) {
	limit = pageLimit(limit)
	rows, e := r.db.QueryContext(ctx, `SELECT c.id,c.media_id,m.title,c.number,COUNT(i.position),c.created_at FROM chapters c JOIN media m ON m.id=c.media_id JOIN chapter_images i ON i.chapter_id=c.id WHERE ($1='' OR c.id<$1) GROUP BY c.id,c.media_id,m.title,c.number,c.created_at ORDER BY c.id DESC LIMIT $2`, cursor, limit+1)
	if e != nil {
		return nil, "", e
	}
	defer rows.Close()
	items := []*library.Download{}
	for rows.Next() {
		var x library.Download
		if e := rows.Scan(&x.ChapterID, &x.MediaID, &x.Title, &x.Number, &x.ImageCount, &x.CreatedAt); e != nil {
			return nil, "", e
		}
		x.Status = "completed"
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
	rows, e := r.db.QueryContext(ctx, `SELECT storage_key FROM chapter_images WHERE chapter_id=$1`, id)
	if e != nil {
		return nil, e
	}
	keys := []string{}
	for rows.Next() {
		var key string
		if x := rows.Scan(&key); x != nil {
			rows.Close()
			return nil, x
		}
		keys = append(keys, key)
	}
	rows.Close()
	if _, e = r.db.ExecContext(ctx, `DELETE FROM chapter_images WHERE chapter_id=$1`, id); e != nil {
		return nil, e
	}
	return keys, nil
}
func scanPGCollection(s rowScanner) (*library.Collection, error) {
	var c library.Collection
	e := s.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	return &c, e
}
func scanFeatureMedia(s rowScanner) (*library.Media, error) {
	var m library.Media
	var alt, authors, artists, genres []byte
	if e := s.Scan(&m.ID, &m.SourceURL, &m.Type, &m.Title, &alt, &m.Description, &m.CoverURL, &authors, &artists, &m.Status, &genres, &m.CreatedAt, &m.UpdatedAt); e != nil {
		return nil, e
	}
	for raw, dst := range map[string]*[]string{string(alt): &m.AlternativeTitles, string(authors): &m.Authors, string(artists): &m.Artists, string(genres): &m.Genres} {
		_ = json.Unmarshal([]byte(raw), dst)
		if *dst == nil {
			*dst = []string{}
		}
	}
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
		INSERT INTO settings (key, value) VALUES ('app_settings', $1)
		ON CONFLICT(key) DO UPDATE SET value = EXCLUDED.value
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
		WHERE ($1 = '' OR id < $1)
		ORDER BY visited_at DESC, id DESC
		LIMIT $2
	`, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	items := []*library.BrowserHistoryEntry{}
	for rows.Next() {
		var item library.BrowserHistoryEntry
		if err := rows.Scan(&item.ID, &item.URL, &item.Title, &item.VisitedAt); err != nil {
			return nil, "", err
		}
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
	now := time.Now().UTC()

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO filter_presets (id, name, filters, created_at)
		VALUES ($1, $2, $3, $4)
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
