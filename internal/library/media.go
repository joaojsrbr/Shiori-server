package library

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"
)

// MediaType specifies the kind of media.
type MediaType string

const (
	MediaTypeManga   MediaType = "manga"
	MediaTypeManhwa  MediaType = "manhwa"
	MediaTypeComic   MediaType = "comic"
	MediaTypeNovel   MediaType = "novel"
	MediaTypeAnime   MediaType = "anime"
	MediaTypeUnknown MediaType = "unknown"
)

// MediaStatus specifies the publication/release status.
type MediaStatus string

const (
	StatusOngoing   MediaStatus = "ongoing"
	StatusCompleted MediaStatus = "completed"
	StatusHiatus    MediaStatus = "hiatus"
	StatusCancelled MediaStatus = "cancelled"
	StatusUnknown   MediaStatus = "unknown"
)

// Media represents a canonical media item in the library.
type Media struct {
	ID                string      `json:"id"`
	SourceURL         string      `json:"source_url"`
	Type              MediaType   `json:"type"`
	Title             string      `json:"title"`
	AlternativeTitles []string    `json:"alternative_titles"`
	Description       string      `json:"description"`
	CoverURL          string      `json:"cover_url"`
	Authors           []string    `json:"authors"`
	Artists           []string    `json:"artists"`
	Status            MediaStatus `json:"status"`
	Genres            []string    `json:"genres"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// MediaCreateRequest holds data to create a new Media.
type MediaCreateRequest struct {
	SourceURL         string      `json:"source_url"`
	Type              MediaType   `json:"type"`
	Title             string      `json:"title"`
	AlternativeTitles []string    `json:"alternative_titles"`
	Description       string      `json:"description"`
	CoverURL          string      `json:"cover_url"`
	Authors           []string    `json:"authors"`
	Artists           []string    `json:"artists"`
	Status            MediaStatus `json:"status"`
	Genres            []string    `json:"genres"`
}

// MediaRepository abstracts the data access for Media.
type MediaRepository interface {
	Create(ctx context.Context, req MediaCreateRequest) (*Media, error)
	GetByID(ctx context.Context, id string) (*Media, error)
	List(ctx context.Context) ([]*Media, error)
}

// NewULID generates a new random ULID as string.
func NewULID() string {
	return ulid.Make().String()
}
