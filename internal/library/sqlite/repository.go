package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joaojsr/shiori-server/internal/library"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, req library.MediaCreateRequest) (*library.Media, error) {
	return r.CreateMedia(ctx, req)
}

func (r *Repository) CreateMedia(ctx context.Context, req library.MediaCreateRequest) (*library.Media, error) {
	id := library.NewULID()
	now := time.Now().UTC()

	altTitles, _ := json.Marshal(req.AlternativeTitles)
	authors, _ := json.Marshal(req.Authors)
	artists, _ := json.Marshal(req.Artists)
	genres, _ := json.Marshal(req.Genres)

	query := `
		INSERT INTO media (
			id, source_url, type, title, alternative_titles, description, cover_url,
			authors, artists, status, genres, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON CONFLICT(source_url) DO UPDATE SET
			type = excluded.type,
			title = excluded.title,
			alternative_titles = excluded.alternative_titles,
			description = excluded.description,
			cover_url = excluded.cover_url,
			authors = excluded.authors,
			artists = excluded.artists,
			status = excluded.status,
			genres = excluded.genres,
			updated_at = excluded.updated_at
		RETURNING id, created_at
	`

	var returnedID, returnedCreatedAtStr string
	err := r.db.QueryRowContext(ctx, query,
		id, req.SourceURL, req.Type, req.Title, string(altTitles), req.Description, req.CoverURL,
		string(authors), string(artists), req.Status, string(genres), now, now,
	).Scan(&returnedID, &returnedCreatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("inserting media: %w", err)
	}

	returnedCreatedAt, _ := time.Parse(time.RFC3339, returnedCreatedAtStr)

	return &library.Media{
		ID:                returnedID,
		SourceURL:         req.SourceURL,
		Type:              req.Type,
		Title:             req.Title,
		AlternativeTitles: req.AlternativeTitles,
		Description:       req.Description,
		CoverURL:          req.CoverURL,
		Authors:           req.Authors,
		Artists:           req.Artists,
		Status:            req.Status,
		Genres:            req.Genres,
		CreatedAt:         returnedCreatedAt,
		UpdatedAt:         now,
	}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*library.Media, error) {
	query := `
		SELECT 
			id, source_url, type, title, alternative_titles, description, cover_url,
			authors, artists, status, genres, created_at, updated_at
		FROM media
		WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)

	var m library.Media
	var altTitles, authors, artists, genres string
	var creStr, updStr string

	err := row.Scan(
		&m.ID, &m.SourceURL, &m.Type, &m.Title, &altTitles, &m.Description, &m.CoverURL,
		&authors, &artists, &m.Status, &genres, &creStr, &updStr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Typically you'd return a custom ErrNotFound
		}
		return nil, fmt.Errorf("getting media: %w", err)
	}

	json.Unmarshal([]byte(altTitles), &m.AlternativeTitles)
	json.Unmarshal([]byte(authors), &m.Authors)
	json.Unmarshal([]byte(artists), &m.Artists)
	json.Unmarshal([]byte(genres), &m.Genres)

	// Ensure empty slices instead of nil for JSON consistency
	if m.AlternativeTitles == nil {
		m.AlternativeTitles = []string{}
	}
	if m.Authors == nil {
		m.Authors = []string{}
	}
	if m.Artists == nil {
		m.Artists = []string{}
	}
	if m.Genres == nil {
		m.Genres = []string{}
	}

	m.CreatedAt, _ = time.Parse(time.RFC3339, creStr)
	m.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)

	return &m, nil
}

func (r *Repository) List(ctx context.Context) ([]*library.Media, error) {
	query := `
		SELECT 
			id, source_url, type, title, alternative_titles, description, cover_url,
			authors, artists, status, genres, created_at, updated_at
		FROM media
		ORDER BY created_at DESC
		LIMIT 50
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing media: %w", err)
	}
	defer rows.Close()

	var results []*library.Media
	for rows.Next() {
		var m library.Media
		var altTitles, authors, artists, genres string
		var creStr, updStr string

		if err := rows.Scan(
			&m.ID, &m.SourceURL, &m.Type, &m.Title, &altTitles, &m.Description, &m.CoverURL,
			&authors, &artists, &m.Status, &genres, &creStr, &updStr,
		); err != nil {
			return nil, fmt.Errorf("scanning media row: %w", err)
		}

		json.Unmarshal([]byte(altTitles), &m.AlternativeTitles)
		json.Unmarshal([]byte(authors), &m.Authors)
		json.Unmarshal([]byte(artists), &m.Artists)
		json.Unmarshal([]byte(genres), &m.Genres)

		if m.AlternativeTitles == nil {
			m.AlternativeTitles = []string{}
		}
		if m.Authors == nil {
			m.Authors = []string{}
		}
		if m.Artists == nil {
			m.Artists = []string{}
		}
		if m.Genres == nil {
			m.Genres = []string{}
		}

		m.CreatedAt, _ = time.Parse(time.RFC3339, creStr)
		m.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)

		results = append(results, &m)
	}

	return results, rows.Err()
}

func (r *Repository) Delete(ctx context.Context, id string) ([]string, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("beginning media deletion: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT i.storage_key
		FROM chapter_images i
		JOIN chapters c ON c.id = i.chapter_id
		WHERE c.media_id = ?`, id)
	if err != nil {
		return nil, false, fmt.Errorf("listing media storage keys: %w", err)
	}
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, false, fmt.Errorf("scanning media storage key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Close(); err != nil {
		return nil, false, fmt.Errorf("closing media storage keys: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("listing media storage keys: %w", err)
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, id)
	if err != nil {
		return nil, false, fmt.Errorf("deleting media: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("counting deleted media: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("committing media deletion: %w", err)
	}
	return keys, count > 0, nil
}

func (r *Repository) UpsertChapter(ctx context.Context, req library.ChapterCreateRequest) (*library.Chapter, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning chapter transaction: %w", err)
	}
	defer tx.Rollback()
	id, now := library.NewULID(), time.Now().UTC()
	var chapterID, created string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO chapters (id, media_id, number, title, source_url, video_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id, source_url) DO UPDATE SET
			number=excluded.number, title=excluded.title, video_url=excluded.video_url
		RETURNING id, created_at`, id, req.MediaID, req.Number, req.Title, req.SourceURL, req.VideoURL, now,
	).Scan(&chapterID, &created)
	if err != nil {
		return nil, fmt.Errorf("upserting chapter: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM chapter_images WHERE chapter_id = ?`, chapterID); err != nil {
		return nil, fmt.Errorf("replacing chapter images: %w", err)
	}
	for _, image := range req.Images {
		if _, err = tx.ExecContext(ctx, `INSERT INTO chapter_images (chapter_id, position, storage_key, content_type) VALUES (?, ?, ?, ?)`, chapterID, image.Position, image.StorageKey, image.ContentType); err != nil {
			return nil, fmt.Errorf("inserting chapter image: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing chapter: %w", err)
	}
	createdAt, _ := time.Parse(time.RFC3339, created)
	return &library.Chapter{ID: chapterID, MediaID: req.MediaID, Number: req.Number, Title: req.Title, SourceURL: req.SourceURL, VideoURL: req.VideoURL, Images: req.Images, CreatedAt: createdAt}, nil
}

func (r *Repository) ListChapters(ctx context.Context, mediaID string) ([]*library.Chapter, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, media_id, number, title, source_url, video_url, created_at FROM chapters WHERE media_id = ? ORDER BY created_at, id`, mediaID)
	if err != nil {
		return nil, fmt.Errorf("listing chapters: %w", err)
	}
	defer rows.Close()
	items := make([]*library.Chapter, 0)
	for rows.Next() {
		chapter, err := scanChapter(rows)
		if err != nil {
			return nil, err
		}
		chapter.Images, err = r.listImages(ctx, chapter.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, chapter)
	}
	return items, rows.Err()
}

func (r *Repository) GetChapter(ctx context.Context, id string) (*library.Chapter, error) {
	chapter, err := scanChapter(r.db.QueryRowContext(ctx, `SELECT id, media_id, number, title, source_url, video_url, created_at FROM chapters WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	chapter.Images, err = r.listImages(ctx, id)
	return chapter, err
}

type rowScanner interface{ Scan(...any) error }

func scanChapter(row rowScanner) (*library.Chapter, error) {
	var chapter library.Chapter
	var created string
	if err := row.Scan(&chapter.ID, &chapter.MediaID, &chapter.Number, &chapter.Title, &chapter.SourceURL, &chapter.VideoURL, &created); err != nil {
		return nil, err
	}
	chapter.CreatedAt, _ = time.Parse(time.RFC3339, created)
	chapter.Images = []library.ChapterImage{}
	return &chapter, nil
}

func (r *Repository) listImages(ctx context.Context, chapterID string) ([]library.ChapterImage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT position, storage_key, content_type FROM chapter_images WHERE chapter_id = ? ORDER BY position`, chapterID)
	if err != nil {
		return nil, fmt.Errorf("listing chapter images: %w", err)
	}
	defer rows.Close()
	images := make([]library.ChapterImage, 0)
	for rows.Next() {
		var image library.ChapterImage
		if err := rows.Scan(&image.Position, &image.StorageKey, &image.ContentType); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}
