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
