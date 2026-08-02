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

type MediaPageRepository interface {
	ListMediaPage(ctx context.Context, limit int, cursor string) ([]*Media, string, error)
}

type Collection struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type HistoryEntry struct {
	ChapterID string    `json:"chapter_id"`
	MediaID   string    `json:"media_id"`
	Title     string    `json:"title"`
	Number    string    `json:"number"`
	Position  int       `json:"position"`
	Progress  float64   `json:"progress"`
	Completed bool      `json:"completed"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Download struct {
	ChapterID  string    `json:"chapter_id"`
	MediaID    string    `json:"media_id"`
	Title      string    `json:"title"`
	Number     string    `json:"number"`
	ImageCount int       `json:"image_count"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// FeatureRepository backs collections, reading history and saved downloads.
type FeatureRepository interface {
	ListCollections(context.Context) ([]*Collection, error)
	CreateCollection(context.Context, string, string) (*Collection, error)
	GetCollection(context.Context, string) (*Collection, error)
	UpdateCollection(context.Context, string, string, string) (*Collection, error)
	DeleteCollection(context.Context, string) error
	ListCollectionMedia(context.Context, string, int, string) ([]*Media, string, error)
	AddCollectionMedia(context.Context, string, string) error
	RemoveCollectionMedia(context.Context, string, string) error
	ListHistory(context.Context, int, string) ([]*HistoryEntry, string, error)
	UpsertHistory(context.Context, string, int, float64, bool) (*HistoryEntry, error)
	DeleteHistory(context.Context, string) error
	ListDownloads(context.Context, int, string) ([]*Download, string, error)
	DeleteDownload(context.Context, string) ([]string, error)
}

// Chapter is a readable unit belonging to a media item. Manga-like media use
// Images; anime uses VideoURL and never stores the video payload itself.
type Chapter struct {
	ID        string         `json:"id"`
	MediaID   string         `json:"media_id"`
	Number    string         `json:"number"`
	Title     string         `json:"title"`
	SourceURL string         `json:"source_url"`
	VideoURL  string         `json:"video_url,omitempty"`
	Images    []ChapterImage `json:"images"`
	CreatedAt time.Time      `json:"created_at"`
}

type ChapterImage struct {
	Position    int    `json:"position"`
	StorageKey  string `json:"-"`
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
}

type ChapterCreateRequest struct {
	MediaID   string
	Number    string
	Title     string
	SourceURL string
	VideoURL  string
	Images    []ChapterImage
}

// ChapterRepository persists the library's reader/playback entries.
type ChapterRepository interface {
	UpsertChapter(ctx context.Context, req ChapterCreateRequest) (*Chapter, error)
	ListChapters(ctx context.Context, mediaID string) ([]*Chapter, error)
	GetChapter(ctx context.Context, id string) (*Chapter, error)
}

// NewULID generates a new random ULID as string.
func NewULID() string {
	return ulid.Make().String()
}
