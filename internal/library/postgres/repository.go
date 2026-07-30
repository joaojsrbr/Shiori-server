package postgres

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
	id := library.NewULID()
	now := time.Now().UTC()

	altTitles, _ := json.Marshal(req.AlternativeTitles)
	authors, _ := json.Marshal(req.Authors)
	artists, _ := json.Marshal(req.Artists)
	genres, _ := json.Marshal(req.Genres)

	query := `
		INSERT INTO media (
			id, type, title, alternative_titles, description, cover_url,
			authors, artists, status, genres, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		id, req.Type, req.Title, string(altTitles), req.Description, req.CoverURL,
		string(authors), string(artists), req.Status, string(genres), now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting media: %w", err)
	}

	return &library.Media{
		ID:                id,
		Type:              req.Type,
		Title:             req.Title,
		AlternativeTitles: req.AlternativeTitles,
		Description:       req.Description,
		CoverURL:          req.CoverURL,
		Authors:           req.Authors,
		Artists:           req.Artists,
		Status:            req.Status,
		Genres:            req.Genres,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*library.Media, error) {
	query := `
		SELECT 
			id, type, title, alternative_titles, description, cover_url,
			authors, artists, status, genres, created_at, updated_at
		FROM media
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)

	var m library.Media
	var altTitles, authors, artists, genres string

	err := row.Scan(
		&m.ID, &m.Type, &m.Title, &altTitles, &m.Description, &m.CoverURL,
		&authors, &artists, &m.Status, &genres, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Return nil, nil when not found
		}
		return nil, fmt.Errorf("getting media: %w", err)
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

	return &m, nil
}

func (r *Repository) List(ctx context.Context) ([]*library.Media, error) {
	query := `
		SELECT 
			id, type, title, alternative_titles, description, cover_url,
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

		if err := rows.Scan(
			&m.ID, &m.Type, &m.Title, &altTitles, &m.Description, &m.CoverURL,
			&authors, &artists, &m.Status, &genres, &m.CreatedAt, &m.UpdatedAt,
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

		results = append(results, &m)
	}

	return results, rows.Err()
}
